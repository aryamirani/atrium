package overlay

import (
	"testing"

	"github.com/ZviBaratz/atrium/config"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func twoTabCfg() *config.Config {
	return &config.Config{
		ClaudeAccounts: []config.ClaudeAccount{
			{Name: "work", ConfigDir: "~/.claude-work", RemoteMatches: []string{"github.com/acme"}},
			{Name: "personal", ConfigDir: "~/.claude"},
		},
		GHAccounts: []config.GHAccount{
			{Name: "gh-work", ConfigDir: "~/.config/gh-work", RemoteMatches: []string{"github.com/acme"}},
		},
	}
}

func TestAccountsOverlay_NavAndTabSwitchClampsCursor(t *testing.T) {
	o := NewAccountsOverlay(twoTabCfg())
	o.SetSize(80, 24)
	require.Equal(t, tabClaude, o.tab)

	o.HandleKeyPress(tea.KeyMsg{Type: tea.KeyDown})
	assert.Equal(t, 1, o.cursorIndex())

	// Claude tab has 2 rows, cursor=1; GitHub tab has 1 row → cursor must clamp to 0.
	o.HandleKeyPress(tea.KeyMsg{Type: tea.KeyTab})
	assert.Equal(t, tabGH, o.tab)
	assert.Equal(t, 0, o.cursorIndex(), "cursor clamped into the shorter tab (no panic later)")
}

func TestAccountsOverlay_EmptyTabIsSafe(t *testing.T) {
	o := NewAccountsOverlay(&config.Config{})
	o.SetSize(80, 24)
	// No accounts on either tab; nav/tab/render must not panic.
	o.HandleKeyPress(tea.KeyMsg{Type: tea.KeyDown})
	o.HandleKeyPress(tea.KeyMsg{Type: tea.KeyTab})
	assert.Equal(t, 0, o.cursorIndex())
	assert.Contains(t, o.Render(), "No GitHub accounts")
}

func TestAccountsOverlay_EscCloses(t *testing.T) {
	o := NewAccountsOverlay(twoTabCfg())
	o.SetSize(80, 24)
	closed, dirty := o.HandleKeyPress(tea.KeyMsg{Type: tea.KeyEsc})
	assert.True(t, closed)
	assert.False(t, dirty)
}

func TestAccountsOverlay_BadgesMarkCatchAllAndUnreachable(t *testing.T) {
	cfg := &config.Config{ClaudeAccounts: []config.ClaudeAccount{
		{Name: "a"}, // first rule-less → default
		{Name: "b"}, // second rule-less → unreachable
		{Name: "c", RemoteMatches: []string{"github.com/x"}}, // routed
	}}
	o := NewAccountsOverlay(cfg)
	o.SetSize(80, 24)
	out := o.Render()
	assert.Contains(t, out, "default")
	assert.Contains(t, out, "unreachable")
	assert.Contains(t, out, "routed")
}
