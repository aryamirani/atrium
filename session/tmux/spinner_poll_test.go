package tmux

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// spinnerRule is a box border wide enough to satisfy the chrome rule predicate.
var spinnerRule = strings.Repeat("─", 40)

// crowdedSpinnerPane reproduces the 2.1.207 bug pane: a live spinner status line above
// the box (elapsed seconds parameterized so the pane can animate across ticks), a task
// line, then the input box, then a reflowed footer that has crowded "esc to interrupt"
// out with contextual chips. footer lets a test vary the below-box hint area.
func crowdedSpinnerPane(secs int) string {
	return strings.Join([]string{
		"● Opening the PR now.",
		"",
		fmt.Sprintf("✽ Opening PR and running CI… (%ds · ↓ 34.6k tokens)", secs),
		"  ⎿  ◼ Open draft PR + CI green",
		"",
		spinnerRule,
		"❯ ",
		spinnerRule,
		"  ⏵⏵ auto mode on (shift+tab to cycle) · PR #371 · ctrl+t to hide tasks · ← for agents",
	}, "\n")
}

// TestPollSpinnerHoldsWorkingWhenMarkerCrowdedOut is the bug repro: a foreground turn
// (empty in-flight set) whose footer dropped "esc to interrupt", but whose live spinner
// keeps ticking. Pre-fix, the marker-absent grace committed idle at idleConfirmTicks; the
// spinner must now hold it working past that cap.
func TestPollSpinnerHoldsWorkingWhenMarkerCrowdedOut(t *testing.T) {
	c := crowdedSpinnerPane(1)
	require.NotContains(t, c, "esc to interrupt", "the fixture reproduces the crowded footer")
	s := hookPollSession(t, "claude", &c)
	seedHookRecord(t, s, hookRecord{State: hookStateWorking}) // empty in-flight set

	for i := 1; i <= idleConfirmTicks+3; i++ {
		c = crowdedSpinnerPane(i) // seconds tick → the pane animates → changed == true
		require.Equal(t, PaneWorking, s.Poll(), "live spinner holds working past the grace cap (tick %d)", i)
	}
}

// TestPollSpinnerStaticSelfHealsToIdle is the #46-forever backstop: a spinner signature
// that never changes (a frozen scrollback quote) with a stuck working latch must NOT pin
// working. The animation gate stops resetting the grace, so it settles to idle at the cap.
func TestPollSpinnerStaticSelfHealsToIdle(t *testing.T) {
	c := crowdedSpinnerPane(7) // static: the same content every tick
	s := hookPollSession(t, "claude", &c)
	seedHookRecord(t, s, hookRecord{State: hookStateWorking}) // stuck working, empty in-flight

	var last PaneState
	for i := 0; i < idleConfirmTicks+2; i++ {
		last = s.Poll()
	}
	require.Equal(t, PaneIdle, last, "a static spinner match self-heals to idle at the grace cap")
}

// TestPollSpinnerHookReadyWins is the authoritative-idle FP guard: a clean ready+empty hook
// outranks a spinner match, so a settled turn whose transcript tail quotes the signature (or
// whose final spinner frame hasn't repainted) stays idle even on the first tick.
func TestPollSpinnerHookReadyWins(t *testing.T) {
	c := crowdedSpinnerPane(3)
	s := hookPollSession(t, "claude", &c)
	seedHookRecord(t, s, hookRecord{State: hookStateReady}) // clean turn end, empty in-flight
	require.Equal(t, PaneIdle, s.Poll(), "a clean ready+empty hook outranks a spinner match")
}

// TestPollSpinnerOutranksPending mirrors TestPollMarkerOverridesPending for the spinner: a
// live (animating) spinner means the main turn is still working, so it outranks an in-flight
// set — a spinning foreground turn is Working, not Pending.
func TestPollSpinnerOutranksPending(t *testing.T) {
	c := crowdedSpinnerPane(1)
	s := hookPollSession(t, "claude", &c)
	seedHookRecord(t, s, hookRecord{State: hookStateReady, Inflight: []string{"aa"}})
	require.Equal(t, PaneWorking, s.Poll(), "tick 1: baseline change → spinner works")
	c = crowdedSpinnerPane(2) // animate
	require.Equal(t, PaneWorking, s.Poll(), "a live spinner outranks the in-flight (pending) record")
}

// TestPollNowSpinnerWorking: the detach-refresh path honors the spinner when there is no
// hook record to consult (e.g. before the first hook fires). With a record present PollNow
// already trusts the working latch / defers to ready — the spinner is the no-record fallback,
// consistent with Poll.
func TestPollNowSpinnerWorking(t *testing.T) {
	c := crowdedSpinnerPane(4)
	s := hookPollSession(t, "claude", &c) // no hook record seeded
	require.Equal(t, PaneWorking, s.PollNow(), "PollNow reads the live spinner as working when no hook record exists")
}
