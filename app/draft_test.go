package app

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runes is a small helper to type text into the focused field.
func draftRunes(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func TestDraft_EscapeStashesDirtyForm(t *testing.T) {
	h := newCreateFormHome(t)

	h.handleKeyPress(draftRunes("n")) // open, focus on title
	require.NotNil(t, h.textInputOverlay)
	h.handleKeyPress(draftRunes("my-draft"))
	h.handleKeyPress(tea.KeyMsg{Type: tea.KeyEsc})

	assert.Nil(t, h.textInputOverlay, "the live overlay is cleared on cancel")
	require.NotNil(t, h.stashedDraft, "a dirty form is stashed")
	assert.Equal(t, "my-draft", h.stashedDraft.GetTitle())
	assert.Equal(t, stateDefault, h.state)
}

func TestDraft_EscapeDiscardsCleanForm(t *testing.T) {
	h := newCreateFormHome(t)

	h.handleKeyPress(draftRunes("n")) // open, type nothing
	h.handleKeyPress(tea.KeyMsg{Type: tea.KeyEsc})

	assert.Nil(t, h.stashedDraft, "an untouched form leaves no stash")
}
