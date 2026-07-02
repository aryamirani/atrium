package app

import (
	"testing"

	"github.com/ZviBaratz/atrium/config"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAccountsPanel_OpenAddPersistClose(t *testing.T) {
	resetSettingsTestState(t) // restores config.json on cleanup
	h := newSettingsTestHome()

	_, _ = h.handleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("@")})
	require.Equal(t, stateAccounts, h.state)
	require.NotNil(t, h.accountsOverlay)
	assert.False(t, h.menuVisible(), "the modal renders its own hints")

	// n → type a name → tab → config dir → enter commits + persists.
	_, _ = h.handleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	for _, r := range "work" {
		_, _ = h.handleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	_, _ = h.handleKeyPress(tea.KeyMsg{Type: tea.KeyTab})
	for _, r := range "~/.claude-work" {
		_, _ = h.handleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	_, _ = h.handleKeyPress(tea.KeyMsg{Type: tea.KeyEnter})

	require.Len(t, h.appConfig.ClaudeAccounts, 1)
	assert.Equal(t, "work", h.appConfig.ClaudeAccounts[0].Name)
	assert.Len(t, config.LoadConfig().ClaudeAccounts, 1, "the change reached disk immediately")

	_, _ = h.handleKeyPress(tea.KeyMsg{Type: tea.KeyEsc})
	assert.Equal(t, stateDefault, h.state)
	assert.Nil(t, h.accountsOverlay)
}
