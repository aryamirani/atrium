package tmux

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

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
