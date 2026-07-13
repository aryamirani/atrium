package agent

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// rule is a box-border line wide enough to satisfy isHorizontalRule.
const rule = "──────────────────────────────────────────────────────────"

// workingPane builds a realistic claude working pane: some transcript, a blank,
// the live spinner status line, an optional task block, a blank, the input box,
// and the footer. footer lets a test vary the reflowed hint area.
func workingPane(spinner, footer string) string {
	return strings.Join([]string{
		"● Some earlier reasoning about the change.",
		"  ⎿  Ran 3 shell commands",
		"",
		spinner,
		"  ⎿  ✔ RED: unit tests for the helper",
		"     ◼ Open draft PR + CI green",
		"",
		rule,
		"❯ ",
		rule,
		"  " + footer,
	}, "\n")
}

// idlePane builds a settled claude pane: a past-tense summary status line above
// the box, no live timer.
func idlePane(summary, footer string) string {
	return strings.Join([]string{
		"● Final answer to the user's question.",
		"",
		summary,
		rule,
		"❯ ",
		rule,
		"  " + footer,
	}, "\n")
}

func TestAboveBoxBlock_IsolatesStatusBlock(t *testing.T) {
	content := workingPane("✽ Opening PR and running CI… (14m 24s · ↓ 34.6k tokens)",
		"⏵⏵ auto mode on (shift+tab to cycle) · PR #371 · ctrl+t to hide tasks · ← for agents")
	block, ok := aboveBoxBlock(content)
	require.True(t, ok, "a pane with a box returns its status block")
	require.Contains(t, block, "Opening PR and running CI…", "block includes the spinner line")
	require.Contains(t, block, "◼ Open draft PR", "block includes the task lines below the spinner")
	require.NotContains(t, block, "Some earlier reasoning", "block excludes scrollback above the blank")
	require.NotContains(t, block, "❯", "block is strictly above the box-top rule")
	require.NotContains(t, block, "auto mode on", "block excludes the below-box footer")
}

func TestAboveBoxBlock_ExcludesScrollbackAboveBlank(t *testing.T) {
	// A spinner-signature line quoted in the transcript, separated from the live
	// status block by a blank line, must not be in the returned block.
	content := strings.Join([]string{
		"● I added ✽ Cogitating… (5s · ↓ 1.2k tokens) as the marker.",
		"  ⎿  Ran 1 command",
		"",
		"✻ Worked for 3m 2s",
		rule,
		"❯ ",
		rule,
		"  ⏵⏵ auto mode on · ← for agents",
	}, "\n")
	block, ok := aboveBoxBlock(content)
	require.True(t, ok)
	require.Contains(t, block, "Worked for 3m 2s")
	require.NotContains(t, block, "Cogitating…", "the scrollback-quoted spinner above the blank is excluded")
}

func TestAboveBoxBlock_NoBoxReturnsNotOK(t *testing.T) {
	// The minimal fixture the poll tests use: an input box line but no box rule.
	_, ok := aboveBoxBlock("❯ \n⏵⏵ auto mode on (shift+tab to cycle) · ← for agents")
	require.False(t, ok, "no box-top rule → not ok (spinner inert)")

	_, ok = aboveBoxBlock("just some transcript\nwith no box at all")
	require.False(t, ok, "no input box → not ok")
}

func TestClaudeSpinnerWorking_PositiveAcrossVariants(t *testing.T) {
	// Glyph varies (✻ ✽ ✢ ✶ * ·); gerund varies; timer shape varies. All must fire.
	spinners := []string{
		"✽ Opening PR and running CI… (14m 24s · ↓ 34.6k tokens)",
		"✢ Billowing… (9m 36s · ↓ 23.3k tokens)",
		"· Metamorphosing… (27m 26s · ↓ 95.3k tokens · almost done thinking with xhigh effort)",
		"* Generating… (0s · ↓ 12 tokens)",
		"✻ Cogitating… (5s · esc to interrupt)", // pre-reflow footer-in-spinner form
	}
	for _, sp := range spinners {
		content := workingPane(sp, "⏵⏵ auto mode on · ctrl+t to hide tasks · ← for agents")
		require.True(t, claudeSpinnerWorking(content), "spinner %q must read as working", sp)
	}

	// Hours-long turn.
	require.True(t, claudeSpinnerWorking(workingPane("✻ Toiling… (1h 2m 3s · ↓ 500k tokens)", "⏵⏵ auto mode on · ← for agents")))
}

func TestClaudeSpinnerWorking_NegativeLookalikes(t *testing.T) {
	// Past-tense summary (settled turn): no parens, no middot.
	require.False(t, claudeSpinnerWorking(idlePane("✻ Worked for 20m 57s", "⏵⏵ auto mode on (shift+tab to cycle) · ← for agents")),
		"the past-tense done summary is not working")
	require.False(t, claudeSpinnerWorking(idlePane("✻ Brewed for 27m 18s", "⏵⏵ auto mode on · ← for agents")))

	// Bare tool duration in the status block: parens but no ' · '.
	require.False(t, claudeSpinnerWorking(idlePane("  done (2m 38s)", "⏵⏵ auto mode on · ← for agents")),
		"a bare tool duration is not working")

	// Sub-agent completion: has ' · ' but the duration is last, not right after '('.
	require.False(t, claudeSpinnerWorking(idlePane("  ⎿  Done (15 tool uses · 82.2k tokens · 2m 20s)", "⏵⏵ auto mode on · ← for agents")),
		"a sub-agent completion summary is not working")

	// Plain idle footer, nothing above the box but the answer.
	require.False(t, claudeSpinnerWorking(idlePane("● All set.", "⏵⏵ auto mode on (shift+tab to cycle) · ← for agents")))
}

func TestClaudeSpinnerWorking_NamedBoxBorder(t *testing.T) {
	// Claude renders the session's agent-context / branch name inside the box-TOP border
	// ("──── name ──"); the strict rule predicate rejects it, so aboveBoxBlock must anchor on
	// the loose border predicate or it misses the box entirely (regression from live panes).
	namedTop := rule + " zvi/agent-state ──"
	content := strings.Join([]string{
		"● Working on it.",
		"",
		"✶ Verifying and opening PR… (21m 4s · ↓ 81.4k tokens)",
		"",
		namedTop,
		"❯ ",
		rule,
		"  ⏵⏵ auto mode on · PR #371 · ctrl+t to hide tasks · ← for agents",
	}, "\n")
	block, ok := aboveBoxBlock(content)
	require.True(t, ok, "a named box-top border is still located")
	require.Contains(t, block, "Verifying and opening PR…")
	require.True(t, claudeSpinnerWorking(content), "spinner is detected above a named box border")
}

func TestClaudeSpinnerWorking_TaskSummaryIsNotWorking(t *testing.T) {
	// An idle pane whose status block is claude's task-count summary ("N tasks (…)") has
	// parens but no gerund ellipsis and no timer middot — it must not read as working.
	require.False(t, claudeSpinnerWorking(idlePane("  5 tasks (1 done, 1 in progress, 3 open)", "⏵⏵ auto mode on · ← for agents")))
}

func TestClaudeSpinnerWorking_ScrollbackQuoteIsNotWorking(t *testing.T) {
	// The critical FP guard: a spinner-signature line quoted in the transcript,
	// with a settled (past-tense) status block, must NOT read as working.
	content := strings.Join([]string{
		"● I added ✽ Cogitating… (5s · ↓ 1.2k tokens) as the marker.",
		"  ⎿  Ran 1 command",
		"",
		"✻ Worked for 3m 2s",
		rule,
		"❯ ",
		rule,
		"  ⏵⏵ auto mode on · ← for agents",
	}, "\n")
	require.False(t, claudeSpinnerWorking(content),
		"a spinner string quoted in scrollback (above the blank/box) must not pin working")
}
