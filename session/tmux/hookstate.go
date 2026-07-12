package tmux

import (
	"bytes"
	"encoding/json"
	"os"
)

// Structured hook state (#290 Phase 2).
//
// Phase 1 shipped a one-word state file ("working"/"ready"). Phase 2 upgrades it to a
// small JSON record that additionally carries the SET of in-flight sub-agent ids, so
// the poller can tell a genuinely-finished turn (Stop with an empty set) from one that
// only *looks* finished because a background sub-agent is still running (Stop with a
// non-empty set — the #290 mislabel).
//
// Writers are N concurrent hook subprocesses (the parent session's injected --settings
// fires for every sub-agent lifecycle event too), so every mutation is a locked
// read-modify-write: a bare shell counter can't stay correct — macOS has no flock(1),
// and unmatched SubagentStops (nested/internal agents with no matching Start) would
// drive a ++/-- counter negative. A SET keyed by agent_id is correct where a counter is
// not: add on Start, discard on Stop, unmatched discards are no-ops, and a missing Stop
// is reaped by liveness / the watchdog rather than corrupting the count. The lock and
// the JSON parse are why the hook command is the atrium binary itself, not a printf.

// Hook event names carried on the `atrium hook --event <name>` command line. They map a
// Claude Code hook to the mutation it performs on the state record. Exported because the
// hidden `hook` subcommand (main package) forwards its --event flag straight through and
// must know which events read an agent_id from stdin.
const (
	// HookEventWorking latches state="working" (UserPromptSubmit / PreToolUse).
	HookEventWorking = "working"
	// HookEventReady latches state="ready" (Stop / StopFailure — a clean or API-error
	// end-of-turn). Gated on the in-flight set by the poller, never terminal on its own.
	HookEventReady = "ready"
	// HookEventSubagentStart adds the stdin payload's agent_id to the in-flight set.
	HookEventSubagentStart = "subagent-start"
	// HookEventSubagentStop discards the stdin payload's agent_id from the in-flight set.
	HookEventSubagentStop = "subagent-stop"
	// HookEventResetInflight empties the in-flight set. Not wired to any Claude hook; the
	// watchdog uses it (via Session.ClearInflight) to deterministically clear a stuck set
	// so a reconciled session can't re-enter pending and oscillate (#46).
	HookEventResetInflight = "reset-inflight"
)

// HookEventReadsAgentID reports whether an event's hook subprocess must parse the stdin
// payload for an agent_id. Only the sub-agent lifecycle events do; the working/ready
// latches are payload-free, so their subprocess never touches stdin (and so can never
// block on it).
func HookEventReadsAgentID(event string) bool {
	return event == HookEventSubagentStart || event == HookEventSubagentStop
}

// hookRecord is the on-disk hook state: the working/ready latch plus the set of
// currently in-flight sub-agent ids. Written atomically by UpdateHookState, read
// lock-free by the poller (rename atomicity protects it from a torn read).
type hookRecord struct {
	State    string   `json:"state,omitempty"`
	Inflight []string `json:"inflight,omitempty"`
}

// UpdateHookState applies one hook event to the record at stateFile under an exclusive
// cross-process lock, then writes it back atomically. It is the single mutation entry
// point shared by the hook subprocesses (via the `atrium hook` subcommand) and the
// watchdog's ClearInflight. agentID is used only by the sub-agent events; it is ignored
// (and may be "") for the others. Best-effort by contract: callers (hooks) must not fail
// the agent on an error here, so the error is returned for logging but never blocks.
func UpdateHookState(stateFile, event, agentID string) error {
	unlock, err := hookStateLock(stateFile)
	if err != nil {
		return err
	}
	defer unlock()

	rec, _ := readHookRecordFile(stateFile) // zero record on a missing/corrupt file
	applyHookEvent(&rec, event, agentID)
	return writeHookRecordAtomic(stateFile, rec)
}

// applyHookEvent mutates rec in place for one event. Unknown events are ignored (a
// forward-compat no-op). A sub-agent event with an empty agent_id is skipped: we can't
// key the set without an id, and adding "" would strand a phantom member the matching
// Stop could never discard — the watchdog/liveness backstop covers the untracked agent.
func applyHookEvent(rec *hookRecord, event, agentID string) {
	switch event {
	case HookEventWorking:
		rec.State = hookStateWorking
	case HookEventReady:
		rec.State = hookStateReady
	case HookEventSubagentStart:
		if agentID != "" {
			rec.Inflight = addAgent(rec.Inflight, agentID)
		}
	case HookEventSubagentStop:
		if agentID != "" {
			rec.Inflight = removeAgent(rec.Inflight, agentID)
		}
	case HookEventResetInflight:
		rec.Inflight = nil
	}
}

// addAgent returns set with id present exactly once (idempotent add).
func addAgent(set []string, id string) []string {
	for _, e := range set {
		if e == id {
			return set
		}
	}
	return append(set, id)
}

// removeAgent returns set with every occurrence of id removed. An id not in the set
// (an unmatched SubagentStop from a nested/internal agent) yields the set unchanged.
// A drained set collapses to nil so it marshals away via omitempty.
func removeAgent(set []string, id string) []string {
	var out []string
	for _, e := range set {
		if e != id {
			out = append(out, e)
		}
	}
	return out
}

// readHookRecordFile reads and parses the record at path, returning (zero, false) when
// the file is absent, empty, or unparseable. ok=false means "no usable hook signal" —
// the poller then falls back to the scrape classifier, exactly as with a missing file.
func readHookRecordFile(path string) (hookRecord, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return hookRecord{}, false
	}
	return parseHookRecord(b)
}

// parseHookRecord decodes the state file's bytes. It accepts both the Phase 2 JSON
// record and the Phase 1 bare word ("working"/"ready"): a session already running when
// atrium is upgraded keeps its old printf hooks (which write the bare word) until it is
// relaunched, so tolerating both keeps that session's status correct across the upgrade
// instead of silently dropping it to the scrape fallback.
func parseHookRecord(b []byte) (hookRecord, bool) {
	trimmed := bytes.TrimSpace(b)
	if len(trimmed) == 0 {
		return hookRecord{}, false
	}
	if trimmed[0] == '{' {
		var rec hookRecord
		if err := json.Unmarshal(trimmed, &rec); err != nil {
			return hookRecord{}, false
		}
		return rec, true
	}
	switch string(trimmed) {
	case hookStateWorking:
		return hookRecord{State: hookStateWorking}, true
	case hookStateReady:
		return hookRecord{State: hookStateReady}, true
	}
	return hookRecord{}, false
}

// writeHookRecordAtomic marshals rec to a temp file and renames it over stateFile, so a
// lock-free reader (the poller) always sees a complete record — the old one or the new
// one, never a torn write. A fixed ".tmp" suffix is safe because every writer holds the
// exclusive hookStateLock while it runs, so only one temp exists at a time.
func writeHookRecordAtomic(stateFile string, rec hookRecord) error {
	data, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	tmp := stateFile + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, stateFile)
}
