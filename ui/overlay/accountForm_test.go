package overlay

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

func TestParseList_TrimsAndDropsBlanks(t *testing.T) {
	assert.Equal(t, []string{"a", "b"}, parseList("a ,, b,"))
	assert.Nil(t, parseList("   "), "whitespace-only → nil, never a blank token")
	assert.Nil(t, parseList(""), "empty → nil")
	assert.Equal(t, []string{"github.com/acme"}, parseList(" github.com/acme "))
}

func TestAccountForm_SeedAndParse(t *testing.T) {
	f := newAccountForm(false, "work", "~/.claude-work", "github.com/acme, gh.com/x", "~/work/", "")
	assert.Equal(t, "work", f.Name())
	assert.Equal(t, "~/.claude-work", f.ConfigDir())
	assert.Equal(t, []string{"github.com/acme", "gh.com/x"}, f.RemoteMatches())
	assert.Equal(t, []string{"~/work/"}, f.PathMatches())
	assert.Nil(t, f.TokenEnv(), "Claude form has no token field")
	assert.Len(t, f.inputs, 4)
}

func TestAccountForm_GHHasTokenField(t *testing.T) {
	f := newAccountForm(true, "gh", "~/.config/gh-work", "", "", "GH_TOKEN, GITHUB_TOKEN")
	assert.Len(t, f.inputs, 5)
	assert.Equal(t, []string{"GH_TOKEN", "GITHUB_TOKEN"}, f.TokenEnv())
}

func TestAccountForm_NavAndSubmitCancel(t *testing.T) {
	f := newAccountForm(false, "", "", "", "", "")
	assert.Equal(t, fldName, f.focus)
	f.HandleKeyPress(tea.KeyMsg{Type: tea.KeyTab})
	assert.Equal(t, fldConfigDir, f.focus, "tab advances focus")
	f.HandleKeyPress(tea.KeyMsg{Type: tea.KeyShiftTab})
	assert.Equal(t, fldName, f.focus, "shift+tab retreats focus")

	assert.True(t, f.HandleKeyPress(tea.KeyMsg{Type: tea.KeyEnter}))
	assert.True(t, f.Submitted())

	g := newAccountForm(false, "", "", "", "", "")
	assert.True(t, g.HandleKeyPress(tea.KeyMsg{Type: tea.KeyEsc}))
	assert.True(t, g.Canceled())
}
