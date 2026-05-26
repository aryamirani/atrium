package overlay

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

func TestRenameOverlay_PrefillAndSubmit(t *testing.T) {
	o := NewRenameOverlay("current")
	assert.Equal(t, "current", o.Value(), "overlay should be pre-filled with the current label")

	o.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("-x")})
	shouldClose := o.HandleKeyPress(tea.KeyMsg{Type: tea.KeyEnter})

	assert.True(t, shouldClose)
	assert.True(t, o.IsSubmitted())
	assert.False(t, o.IsCanceled())
	assert.Equal(t, "current-x", o.Value())
}

func TestRenameOverlay_Cancel(t *testing.T) {
	o := NewRenameOverlay("current")
	shouldClose := o.HandleKeyPress(tea.KeyMsg{Type: tea.KeyEsc})

	assert.True(t, shouldClose)
	assert.True(t, o.IsCanceled())
	assert.False(t, o.IsSubmitted())
}

func TestRenameOverlay_TrimsValue(t *testing.T) {
	o := NewRenameOverlay("")
	o.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("  spaced  ")})
	assert.Equal(t, "spaced", o.Value())
}

func TestRenameOverlay_EnforcesCharLimit(t *testing.T) {
	o := NewRenameOverlay("")
	o.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(strings.Repeat("a", 40))})
	assert.LessOrEqual(t, len(o.Value()), 32, "the title input is capped at 32 characters")
}
