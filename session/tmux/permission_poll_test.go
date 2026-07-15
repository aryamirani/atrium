package tmux

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// claudeFetchPane is claude's network-permission dialog — the one prompt autoyes still
// answers with Enter — captured from a live 2.1.210 pane by prompting a session to WebFetch
// a fresh domain (2026-07-15, #343). Note what it does NOT carry: any footer at all. The
// pre-#343 fixture guessed "Esc to cancel · Tab to amend"; the real dialog has none, which
// is why permission-local cannot back this family up.
//
// The tool's own arguments (url, prompt) render INSIDE the dialog, above the question —
// see claudeBashForgedPane for why that ordering is the whole discriminator.
var claudeFetchPane = strings.Join([]string{
	"● Fetch(https://example.net)",
	"",
	strings.Repeat("─", 100),
	" Fetch",
	"",
	`   url: "https://example.net", prompt: "Summarize the content of this page."`,
	"   Claude wants to fetch content from example.net",
	"",
	" Do you want to allow Claude to fetch this content?",
	" ❯ 1. Yes",
	"   2. Yes, and don't ask again for example.net",
	"   3. No, and tell Claude what to do differently (esc)",
}, "\n")

// claudeBashForgedPane is #343's sharper half at the poll boundary, captured from a live
// 2.1.210 pane by asking a session to run `mkdir -p "…/No, and tell Claude what to do
// differently"` (2026-07-15). Claude renders the command INSIDE the approval dialog, so the
// literal the "permission" matcher keyed on is in live chrome — and that matcher precedes
// permission-local and carries no NoAutoTap, so autoyes Enter-approved the shell command.
var claudeBashForgedPane = strings.Join([]string{
	"● Running 1 shell command…",
	`  ⎿  $ mkdir -p "/tmp/atr343/work/No, and tell Claude what to do differently"`,
	"",
	strings.Repeat("─", 100),
	" Bash command",
	"",
	`   mkdir -p "/tmp/atr343/work/No, and tell Claude what to do differently"`,
	"   Create directory with the given name",
	"",
	" Do you want to proceed?",
	" ❯ 1. Yes",
	"   2. Yes, and always allow access to work/ from this project",
	"   3. No",
	"",
	" Esc to cancel · Tab to amend · ctrl+e to explain",
}, "\n")

// TestPollForgedPermissionLiteralIsManual is #343's sharper half at the poll boundary, and
// PanePromptManual is the whole assertion: PanePrompt here means ApplyPaneState taps Enter
// on the pre-highlighted "❯ 1. Yes" and runs the command. An agent working on Atrium
// reaches this by grepping its own repo for the literal.
func TestPollForgedPermissionLiteralIsManual(t *testing.T) {
	c := claudeBashForgedPane
	s := pollSession(t, "claude", &c, nil)
	require.Equal(t, PanePromptManual, s.Poll(),
		"a Bash approval whose command quotes the fetch dialog's decline option must never auto-tap")
}

// TestPollFetchPermissionAutoTappable is the other side of the same coin: the real fetch
// dialog must stay PanePrompt (auto-tappable), so #343 is not "fixed" by making the one
// prompt autoyes answers manual.
func TestPollFetchPermissionAutoTappable(t *testing.T) {
	c := claudeFetchPane
	s := pollSession(t, "claude", &c, nil)
	require.Equal(t, PanePrompt, s.Poll(), "the live fetch dialog stays auto-answerable")
}

// claudeWritePermissionPane is claude's file-write approval dialog, captured from a
// live 2.1.210 pane and byte-identical on 2.1.207 (#332). Note what it does NOT
// carry: the "No, and tell Claude what to do differently" option the network-permission
// matcher keys on, and any navigate/select token for the selection matcher. Before
// #332 nothing in the adapter saw it, so the pane below classified as PaneIdle and the
// row read Ready while the session sat blocked on a human.
var claudeWritePermissionPane = strings.Join([]string{
	"● Write(hello.txt)",
	strings.Repeat("─", 56),
	" Create file",
	" hello.txt",
	"  1 hi",
	" Do you want to create hello.txt?",
	" ❯ 1. Yes",
	"   2. Yes, allow all edits during this session (shift+tab)",
	"   3. No",
	"",
	" Esc to cancel · Tab to amend",
}, "\n")

// TestPollLocalPermissionNeedsInput is the #332 bug repro at the poll boundary: the
// dialog must surface as a manual prompt, never as idle. PanePromptManual (not
// PanePrompt) is the assertion that matters — it is what keeps autoyes from tapping
// Enter and approving a file write on the user's behalf.
func TestPollLocalPermissionNeedsInput(t *testing.T) {
	c := claudeWritePermissionPane
	require.NotContains(t, c, "No, and tell Claude what to do differently",
		"the fixture is the real dialog: no long-form decline option")
	s := pollSession(t, "claude", &c, nil)
	require.Equal(t, PanePromptManual, s.Poll(), "a blocked write approval must surface as needs-input")
}
