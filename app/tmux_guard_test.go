package app

import (
	"testing"

	"github.com/ZviBaratz/atrium/session/tmux"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

// When tmux is not installed, pressing n must NOT open the create form. Instead
// the friendly sentinel is surfaced (routed to the persistent info modal), so the
// user never fills in a form only to hit the raw exec-not-found error at launch.
func TestOpenCreateForm_BlockedWhenTmuxMissing(t *testing.T) {
	orig := tmuxAvailable
	t.Cleanup(func() { tmuxAvailable = orig })
	tmuxAvailable = func() error { return tmux.ErrNotInstalled }

	h := newCreateFormHome(t)

	h.handleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})

	assert.NotEqual(t, statePrompt, h.state, "a missing tmux must block the create form from opening")
	assert.Nil(t, h.textInputOverlay, "no form overlay should be built when tmux is missing")
}

// The smart-dispatch auto-create path bypasses the create form, so it has its own
// tmux gate: autoDispatch must decline (return false) when tmux is missing, so the
// caller falls through to the form guard's friendly message instead of launching a
// session that would fail async with the raw exec error.
func TestAutoDispatch_DeclinesWhenTmuxMissing(t *testing.T) {
	orig := tmuxAvailable
	t.Cleanup(func() { tmuxAvailable = orig })
	tmuxAvailable = func() error { return tmux.ErrNotInstalled }

	h := newCreateFormHome(t)

	cmd, ok := h.autoDispatch(PrefillResult{Title: "x", Path: t.TempDir(), Confident: true})

	assert.False(t, ok, "autoDispatch must decline when tmux is missing")
	assert.Nil(t, cmd, "no launch command should be produced when tmux is missing")
}
