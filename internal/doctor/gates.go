package doctor

// Remote feature-gate reporting. `atrium doctor`'s version check answers "which
// build is installed"; this answers "which BRANCH of that build renders". Claude
// ships two footer implementations in one binary and picks between them on a
// server-resolved gate, so the pane can change with no version change at all —
// drift the version check structurally cannot see (#337).
//
// The values are readable because claude persists every gate it resolves to
// <config dir>/.claude.json under cachedGrowthBookFeatures, rewriting the map from
// its in-memory payload on every fetch. That makes the file a faithful,
// eventually-consistent mirror of what the CLI actually resolved.
//
// One edge is documented rather than handled: with DISABLE_GROWTHBOOK set, claude
// short-circuits gate resolution and every gate takes its LOCAL default, ignoring
// the file. For tengu_copper_thistle that default is false — the value we pin — so
// the edge can only make this read pessimistic (a spurious "flipped"), never
// falsely reassuring. That is the safe direction for a report, and special-casing
// an env var that doctor sees but a session may not would be guessing.
//
// Like VerifiedVersion, this is a RECORD, not a tripwire that acts: nothing polls
// it and nothing in the TUI consumes it. See the "Why the TUI is untouched" note
// below CheckGatesInstalled.

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/ZviBaratz/atrium/config"
	"github.com/ZviBaratz/atrium/session/agent"
)

// GateState is the outcome of comparing one pinned gate against the value the
// agent CLI last resolved. Every failure resolves to GateUnknown: a gate check is
// a report, never an alarm, and a false alarm is worse than silence.
type GateState int

const (
	// GateMatchesPin means the resolved value equals the adapter's pin — the
	// heuristics were verified against the branch this account renders.
	GateMatchesPin GateState = iota
	// GateFlipped means the resolved value differs from the pin: the heuristics
	// were verified on the other branch of this gate.
	GateFlipped
	// GateUnknown means no comparable value could be read (see fileGateReader).
	GateUnknown
)

// GateResult is the report for one (agent, config dir, gate) triple.
type GateResult struct {
	Key     agent.Key
	Name    string // adapter display name
	Account string // config-dir label: an account name, or defaultAccount
	Gate    string
	Pinned  bool
	Actual  bool // meaningful only when State == GateMatchesPin or GateFlipped
	State   GateState
}

// defaultAccount labels the ambient config dir — the one a session with no
// CLAUDE_CONFIG_DIR injected inherits.
const defaultAccount = "default"

// gateDir is one claude config dir to report on, with the label it renders under.
type gateDir struct {
	Account string
	Dir     string
}

// gateReader reads the feature gates a claude CLI resolved into one config dir.
// The method is unexported so only in-package fakes implement it, mirroring
// Runner in check.go. ok=false means "no comparable map here" — every read
// failure collapses into it, because none of them are distinguishable to a user
// and none of them justify an alarm.
type gateReader interface {
	gates(configDir string) (m map[string]any, ok bool)
}

// fileGateReader is the production gateReader: it reads <configDir>/.claude.json.
//
// Deliberately conservative, in the spirit of session/tmux/trust.go (the other
// reader of this file): a missing file just means claude is not onboarded in that
// dir, and a malformed one is claude's file and claude's problem. It is far
// simpler than trust.go because it only reads — none of that function's
// symlink-following, stat-guarding or atomic-rename machinery has anything to
// guard here, and neither does its UseNumber decoding: we compare bools and
// discard every other value rather than re-encoding the file.
type fileGateReader struct{}

func (fileGateReader) gates(configDir string) (map[string]any, bool) {
	// Refuse a relative (or empty) dir rather than let filepath.Join resolve it
	// against doctor's working directory: "" would silently read ./.claude.json,
	// reporting on a file no session has any relationship to — and a stray true in
	// it would render as a flip, the one outcome this whole file exists to avoid.
	// An unresolvable home or a relative CLAUDE_CONFIG_DIR is genuinely unknown; we
	// cannot know the cwd claude would have resolved it against.
	if !filepath.IsAbs(configDir) {
		return nil, false
	}
	data, err := os.ReadFile(filepath.Join(configDir, ".claude.json"))
	if err != nil {
		return nil, false
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, false
	}
	m, ok := root["cachedGrowthBookFeatures"].(map[string]any)
	if !ok {
		return nil, false // absent, null, or reshaped by a future claude
	}
	return m, true
}

// CheckGates compares every adapter's VerifiedGates against the value resolved in
// each dir. Pure: no config load, no home resolution, no IO of its own — that all
// lives in the caller and the reader, which is what keeps this testable without
// touching a real config dir. It never errors, and adapters pinning no gates
// contribute no rows.
func CheckGates(adapters []*agent.Adapter, dirs []gateDir, r gateReader) []GateResult {
	var out []GateResult
	for _, a := range adapters {
		if len(a.VerifiedGates) == 0 {
			continue
		}
		for _, d := range dirs {
			resolved, ok := r.gates(d.Dir)
			for _, g := range a.VerifiedGates {
				res := GateResult{
					Key: a.Key, Name: a.DisplayName, Account: d.Account,
					Gate: g.Name, Pinned: g.Value, State: GateUnknown,
				}
				if ok {
					// A gate absent from the map was never resolved here, and a
					// non-bool (the map also holds numbers and objects) is not
					// comparable to a bool pin. Both stay unknown rather than
					// guess at a value.
					if v, isBool := resolved[g.Name].(bool); isBool {
						res.Actual = v
						res.State = GateMatchesPin
						if v != g.Value {
							res.State = GateFlipped
						}
					}
				}
				out = append(out, res)
			}
		}
	}
	return out
}

// CheckGatesInstalled reports on the real environment: the ambient config dir plus
// every configured ClaudeAccount. It is the only function here that touches
// config, which is the point — config.LoadConfig seeds config.json when it is
// missing (a write), so it must stay out of the pure CheckGates that tests call.
//
// Why nothing but `atrium doctor` calls this: the startup drift hint suppresses
// via an ack keyed on the *installed version* (app/app_driftcheck.go,
// config.State.AckedDrift), and a gate flip has no version. Wiring it in would
// either be swallowed for anyone who had already acked their current version, or
// become an un-ackable permanent nag. Doctor is the "tell me the truth about my
// heuristics" command, and #337's ask was precisely that a flip is invisible to
// it.
func CheckGatesInstalled() []GateResult {
	return CheckGates(agent.Adapters(), installedGateDirs(config.LoadConfig()), fileGateReader{})
}

// installedGateDirs enumerates the config dirs whose gate state matters, ambient
// first. Dirs are deduped by path, so an account pointing at the ambient dir is
// reported once — under its account name, since that is the label a user routing
// sessions there would recognize.
func installedGateDirs(cfg *config.Config) []gateDir {
	var dirs []gateDir
	if ambient := ambientConfigDir(); ambient != "" {
		dirs = append(dirs, gateDir{Account: defaultAccount, Dir: ambient})
	}
	for _, a := range cfg.ClaudeAccounts {
		dir := a.ResolvedConfigDir()
		if dir == "" {
			continue // inherit-env account: already covered by the ambient entry
		}
		if i := indexOfDir(dirs, dir); i >= 0 {
			dirs[i].Account = a.Name
			continue
		}
		dirs = append(dirs, gateDir{Account: a.Name, Dir: dir})
	}
	return dirs
}

// ambientConfigDir resolves the config dir claude would use with no account
// routing, by claude's own rule: $CLAUDE_CONFIG_DIR if set, else the home dir,
// with the file at that dir's root (.claude.json). This has nothing to do with
// Atrium's data dir, so config.RuntimeName() is deliberately not involved.
//
// Note this reads DOCTOR's env, which is only a session's ambient env when the
// two match — a session inherits the tmux server's env, captured at server start.
// Running `atrium doctor` from inside a Claude Code session (which exports
// CLAUDE_CONFIG_DIR) therefore points this row at the caller's own account dir.
// That mislabels rather than misreports: the dedupe in installedGateDirs relabels
// the row to the account name whenever that dir is a configured account, and the
// value read is a real resolved value either way.
//
// "" means unresolvable, and the caller drops the row rather than reporting on a
// dir it had to guess at.
func ambientConfigDir() string {
	if dir := os.Getenv("CLAUDE_CONFIG_DIR"); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}

// indexOfDir returns the index of dir in dirs, or -1.
func indexOfDir(dirs []gateDir, dir string) int {
	for i, d := range dirs {
		if d.Dir == dir {
			return i
		}
	}
	return -1
}

// GatesFlipped reports whether any result is a confirmed flip — the only state
// that warrants more than an informational row.
func GatesFlipped(results []GateResult) bool {
	for _, r := range results {
		if r.State == GateFlipped {
			return true
		}
	}
	return false
}
