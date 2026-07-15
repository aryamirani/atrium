package tmux

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// freshHeartbeat and staleHeartbeat are unix-second timestamps relative to now: fresh sits
// well inside heartbeatTTL, stale well outside it. Seeded into a hookRecord, they let the
// poll-level tests exercise the #311 freshness hold without a clock injection (heartbeatTTL
// dwarfs a test's runtime).
func freshHeartbeat() int64 { return time.Now().Unix() }
func staleHeartbeat() int64 { return time.Now().Unix() - int64(2*heartbeatTTL/time.Second) }

// TestPollHeartbeatFreshHoldsWorking is the core #311 behavior: with the pane showing no
// busy marker and no live spinner (claude lighting "esc to interrupt" off its narrowest
// notion of busy — see agent/spinner.go — or a future spinner reword), a hook that fired
// within heartbeatTTL still proves the
// MAIN turn is live — read from Atrium's own authoritative hook file, not the pane. So the
// row holds Working. When the writer stops bumping (a crash/kill mid-tool), the heartbeat
// goes stale, the hold releases, and the bounded marker-absent grace self-heals to Idle —
// the #46 guarantee, now heartbeat-driven.
func TestPollHeartbeatFreshHoldsWorking(t *testing.T) {
	idle := "❯ \n⏵⏵ auto mode on (shift+tab to cycle) · ← for agents"
	c := idle
	s := hookPollSession(t, "claude", &c)

	seedHookRecord(t, s, hookRecord{State: hookStateWorking, LastHeartbeat: freshHeartbeat()})
	require.Equal(t, PaneWorking, s.Poll(), "a fresh hook heartbeat holds working with no marker/spinner")

	// The writer stops (crash mid-tool): heartbeat goes stale. The hold releases and the
	// bounded grace burns down to idle, exactly like a stuck bare-word file.
	seedHookRecord(t, s, hookRecord{State: hookStateWorking, LastHeartbeat: staleHeartbeat()})
	for i := 1; i < idleConfirmTicks; i++ {
		require.Equal(t, PaneWorking, s.Poll(), "stale heartbeat falls to the grace (tick %d)", i)
	}
	require.Equal(t, PaneIdle, s.Poll(), "a stale heartbeat self-heals to idle at the grace cap")
}

// TestPollHeartbeatReadyEmptyStillWins pins Igor's rule that explicit Stop is the only
// "done": a Stop fires PostToolUse right before it, so at turn end the record is
// {ready, empty, fresh-heartbeat}. The fresh heartbeat must NOT resurrect the finished
// turn — ready+empty commits idle first.
func TestPollHeartbeatReadyEmptyStillWins(t *testing.T) {
	idle := "❯ \n⏵⏵ auto mode on (shift+tab to cycle) · ← for agents"
	c := idle
	s := hookPollSession(t, "claude", &c)

	seedHookRecord(t, s, hookRecord{State: hookStateReady, LastHeartbeat: freshHeartbeat()})
	require.Equal(t, PaneIdle, s.Poll(), "ready+empty outranks a still-fresh heartbeat → idle")
}

// TestPollHeartbeatInflightStillPending guards the #290 boundary: a BACKGROUND sub-agent's
// own PreToolUse bumps the parent's heartbeat while it runs, so the record can read
// {working, inflight>0, fresh}. The session is waiting on the child, so it must read
// PanePending — the heartbeat hold is gated on an empty set and cannot mask pending as
// working.
func TestPollHeartbeatInflightStillPending(t *testing.T) {
	idle := "❯ \n⏵⏵ auto mode on (shift+tab to cycle) · ← for agents"
	c := idle
	s := hookPollSession(t, "claude", &c)

	seedHookRecord(t, s, hookRecord{State: hookStateWorking, Inflight: []string{"aa"}, LastHeartbeat: freshHeartbeat()})
	require.Equal(t, PanePending, s.Poll(), "a child's heartbeat bump must not mask pending as working")
}
