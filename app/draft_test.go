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

func TestDraft_ReopenRestoresStash(t *testing.T) {
	h := newCreateFormHome(t)

	h.handleKeyPress(draftRunes("n"))
	h.handleKeyPress(draftRunes("my-draft"))
	h.handleKeyPress(tea.KeyMsg{Type: tea.KeyEsc})
	require.NotNil(t, h.stashedDraft)

	h.handleKeyPress(draftRunes("n")) // reopen
	require.NotNil(t, h.textInputOverlay)
	assert.Equal(t, "my-draft", h.textInputOverlay.GetTitle(), "the draft is restored")
	assert.Nil(t, h.stashedDraft, "the stash is consumed into the live overlay")
}

func TestDraft_DoubleCtrlRRebuildsFresh(t *testing.T) {
	h := newCreateFormHome(t)

	h.handleKeyPress(draftRunes("n"))
	h.handleKeyPress(draftRunes("my-draft"))
	h.handleKeyPress(tea.KeyMsg{Type: tea.KeyEsc})
	h.handleKeyPress(draftRunes("n")) // reopen with the restored draft
	require.Equal(t, "my-draft", h.textInputOverlay.GetTitle())

	h.handleKeyPress(tea.KeyMsg{Type: tea.KeyCtrlR}) // arm
	h.handleKeyPress(tea.KeyMsg{Type: tea.KeyCtrlR}) // confirm

	require.NotNil(t, h.textInputOverlay)
	assert.Equal(t, "", h.textInputOverlay.GetTitle(), "the form is rebuilt fresh")
	assert.Nil(t, h.stashedDraft, "the stash is dropped on clear")
}
