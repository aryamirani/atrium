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

// typeInto sends each rune of s to the overlay as individual key messages.
func typeInto(o *AccountsOverlay, s string) {
	for _, r := range s {
		o.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
}

func TestAccountsOverlay_AddAppendsOnCommit(t *testing.T) {
	cfg := &config.Config{}
	o := NewAccountsOverlay(cfg)
	o.SetSize(80, 24)

	o.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}}) // new
	require.Equal(t, modeEdit, o.mode)
	typeInto(o, "work")                            // Name
	o.HandleKeyPress(tea.KeyMsg{Type: tea.KeyTab}) // → Config dir
	typeInto(o, "~/.claude-work")
	_, dirty := o.HandleKeyPress(tea.KeyMsg{Type: tea.KeyEnter}) // commit

	assert.True(t, dirty)
	assert.Equal(t, modeList, o.mode)
	require.Len(t, cfg.ClaudeAccounts, 1)
	assert.Equal(t, "work", cfg.ClaudeAccounts[0].Name)
	assert.Equal(t, "~/.claude-work", cfg.ClaudeAccounts[0].ConfigDir)
}

func TestAccountsOverlay_ValidationRejectsEmptyAndDuplicateName(t *testing.T) {
	cfg := &config.Config{ClaudeAccounts: []config.ClaudeAccount{{Name: "work"}}}
	o := NewAccountsOverlay(cfg)
	o.SetSize(80, 24)

	o.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	_, dirty := o.HandleKeyPress(tea.KeyMsg{Type: tea.KeyEnter}) // empty name
	assert.False(t, dirty)
	assert.Equal(t, modeEdit, o.mode, "stays in edit on validation error")
	assert.NotEmpty(t, o.lastErr)
	assert.Len(t, cfg.ClaudeAccounts, 1, "config not mutated")

	typeInto(o, "work") // duplicate of the existing account
	_, dirty = o.HandleKeyPress(tea.KeyMsg{Type: tea.KeyEnter})
	assert.False(t, dirty)
	assert.Equal(t, modeEdit, o.mode)
	assert.Len(t, cfg.ClaudeAccounts, 1)
}

func TestAccountsOverlay_CancelDiscardsEdits(t *testing.T) {
	cfg := &config.Config{ClaudeAccounts: []config.ClaudeAccount{
		{Name: "work", RemoteMatches: []string{"github.com/acme"}},
	}}
	o := NewAccountsOverlay(cfg)
	o.SetSize(80, 24)

	o.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}}) // edit row 0
	require.Equal(t, modeEdit, o.mode)
	typeInto(o, "-extra")                          // mutate the Name field
	o.HandleKeyPress(tea.KeyMsg{Type: tea.KeyEsc}) // cancel

	assert.Equal(t, modeList, o.mode)
	assert.Equal(t, "work", cfg.ClaudeAccounts[0].Name, "esc discards edits")
	assert.Equal(t, []string{"github.com/acme"}, cfg.ClaudeAccounts[0].RemoteMatches)
}

func TestAccountsOverlay_DeleteWithConfirm(t *testing.T) {
	cfg := &config.Config{ClaudeAccounts: []config.ClaudeAccount{{Name: "a"}, {Name: "b"}}}
	o := NewAccountsOverlay(cfg)
	o.SetSize(80, 24)
	o.cursor = 1

	o.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	require.Equal(t, modeConfirmDelete, o.mode)
	_, dirty := o.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	assert.True(t, dirty)
	require.Len(t, cfg.ClaudeAccounts, 1)
	assert.Equal(t, "a", cfg.ClaudeAccounts[0].Name)
	assert.Equal(t, 0, o.cursor, "cursor clamped after delete")
}

func TestAccountsOverlay_GHCommitIncludesTokenEnv(t *testing.T) {
	cfg := &config.Config{}
	o := NewAccountsOverlay(cfg)
	o.SetSize(80, 24)
	o.selectTab(tabGH)

	o.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	typeInto(o, "gh-work")
	// jump to the Token env field (index fldToken) via tab presses
	for i := 0; i < fldToken; i++ {
		o.HandleKeyPress(tea.KeyMsg{Type: tea.KeyTab})
	}
	typeInto(o, "GH_TOKEN")
	o.HandleKeyPress(tea.KeyMsg{Type: tea.KeyEnter})

	require.Len(t, cfg.GHAccounts, 1)
	assert.Equal(t, []string{"GH_TOKEN"}, cfg.GHAccounts[0].TokenEnv)
}

func TestAccountsOverlay_PreviewResolves(t *testing.T) {
	cfg := &config.Config{ClaudeAccounts: []config.ClaudeAccount{
		{Name: "work", ConfigDir: "~/.claude-work", RemoteMatches: []string{"github.com/acme"}},
		{Name: "personal", ConfigDir: "~/.claude"}, // catch-all
	}}
	o := NewAccountsOverlay(cfg)
	o.SetSize(80, 24)

	o.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	require.Equal(t, modePreview, o.mode)
	typeInto(o, "github.com/acme/widgets")
	assert.Contains(t, o.renderPreview(), "work", "remote matches the work account")

	o.HandleKeyPress(tea.KeyMsg{Type: tea.KeyEsc})
	assert.Equal(t, modeList, o.mode)
}

func TestAccountsOverlay_PreviewEmptyAndRuleOnlyInheritAmbient(t *testing.T) {
	// 0 accounts
	o := NewAccountsOverlay(&config.Config{})
	o.SetSize(80, 24)
	o.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	typeInto(o, "github.com/acme")
	out := o.renderPreview()
	assert.Contains(t, out, "inherit")
	assert.NotContains(t, out, "Claude → \n", "no blank name")

	// rule-only (no catch-all), unmatched input
	cfg := &config.Config{ClaudeAccounts: []config.ClaudeAccount{
		{Name: "work", RemoteMatches: []string{"github.com/acme"}},
	}}
	o2 := NewAccountsOverlay(cfg)
	o2.SetSize(80, 24)
	o2.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	typeInto(o2, "github.com/other")
	assert.Contains(t, o2.renderPreview(), "inherit", "no-match with no catch-all inherits ambient")
}
