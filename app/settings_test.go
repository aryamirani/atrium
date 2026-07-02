package app

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/ZviBaratz/atrium/config"
	"github.com/ZviBaratz/atrium/session"
	"github.com/ZviBaratz/atrium/ui"
	"github.com/ZviBaratz/atrium/ui/theme"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newSettingsTestHome builds the minimal home model the settings paths touch.
// HOME is sandboxed by TestMain, so config persistence stays hermetic.
func newSettingsTestHome() *home {
	return &home{
		ctx:       context.Background(),
		state:     stateDefault,
		appConfig: config.DefaultConfig(),
		menu:      ui.NewMenu(),
	}
}

// resetSettingsTestState restores the on-disk config and active theme that
// settings tests mutate, so sibling tests in the package see defaults.
func resetSettingsTestState(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		_ = config.SaveConfig(config.DefaultConfig())
		theme.Set(theme.DefaultThemeName)
	})
}

func TestSettingsPanel_OpenEditPersistClose(t *testing.T) {
	resetSettingsTestState(t)
	h := newSettingsTestHome()

	// ',' opens the settings panel.
	_, _ = h.handleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(",")})
	require.Equal(t, stateSettings, h.state)
	require.NotNil(t, h.settingsOverlay)

	// Toggling a value persists it to config.json immediately, not on close.
	require.True(t, h.settingsOverlay.SelectRow("auto_attach"))
	_, _ = h.handleKeyPress(tea.KeyMsg{Type: tea.KeySpace})
	assert.False(t, h.appConfig.GetAutoAttach())
	assert.False(t, config.LoadConfig().GetAutoAttach(),
		"a change must reach disk immediately so it survives a crash")

	// Esc closes the panel and returns to the list.
	_, _ = h.handleKeyPress(tea.KeyMsg{Type: tea.KeyEsc})
	assert.Equal(t, stateDefault, h.state)
	assert.Nil(t, h.settingsOverlay)
}

func TestSettingsPanel_ThemeChangeAppliesLive(t *testing.T) {
	resetSettingsTestState(t)
	h := newSettingsTestHome()

	_, _ = h.handleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(",")})
	require.Equal(t, stateSettings, h.state)
	require.True(t, h.settingsOverlay.SelectRow("theme"))

	_, cmd := h.handleKeyPress(tea.KeyMsg{Type: tea.KeyRight})
	assert.NotEqual(t, theme.DefaultThemeName, h.appConfig.Theme)
	assert.Equal(t, h.appConfig.Theme, theme.Current().Name,
		"the active theme must follow the config change without a restart")
	assert.Equal(t, h.appConfig.Theme, config.LoadConfig().Theme)
	assert.NotNil(t, cmd, "a repaint command must be issued for the new palette")
}

func TestSettingsPanel_AutoYesTogglePropagatesToHomeFlag(t *testing.T) {
	resetSettingsTestState(t)
	h := newSettingsTestHome()

	_, _ = h.handleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(",")})
	require.True(t, h.settingsOverlay.SelectRow("auto_yes"))
	_, _ = h.handleKeyPress(tea.KeyMsg{Type: tea.KeySpace})

	assert.True(t, h.autoYes, "the home flag gates AutoYes on newly created instances")
	assert.True(t, config.LoadConfig().AutoYes,
		"the persisted flag is what the exit-time daemon decision reads")
}

// TestGroupModeChange_ClustersList proves the "group_mode" settings-changed case
// (app_layout.go) reaches the live list end-to-end: opening the panel, cycling
// the row to "account", and reading the list back out. Mirrors
// TestSettingsPanel_ThemeChangeAppliesLive's open/select/KeyRight dispatch, but
// builds the home via assembleHome (see TestAssembleHomeWiring) so the list
// carries real instances to cluster, rather than newSettingsTestHome's list-less
// shell.
func TestGroupModeChange_ClustersList(t *testing.T) {
	resetSettingsTestState(t)

	cfg := config.DefaultConfig()
	st := config.DefaultState()
	storage, err := session.NewStorage(st)
	require.NoError(t, err)

	newInst := func(repoBase, account string) *session.Instance {
		inst, err := session.NewInstance(session.InstanceOptions{
			Title: repoBase + "-" + account, Path: "/tmp/" + repoBase, Program: "echo",
		})
		require.NoError(t, err)
		if account != "" {
			inst.SetClaudeAccount(account, "", false)
		}
		return inst
	}
	// Interleaved input: work, personal, work — two repos share the "work"
	// account and must end up adjacent once account-clustering applies.
	instances := []*session.Instance{
		newInst("api", "work"),
		newInst("sideproj", "personal"),
		newInst("infra", "work"),
	}

	h := assembleHome(context.Background(), "claude", false, "v", "atr", cfg, st, storage, instances)

	_, _ = h.handleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(",")})
	require.Equal(t, stateSettings, h.state)
	require.True(t, h.settingsOverlay.SelectRow("group_mode"))

	_, _ = h.handleKeyPress(tea.KeyMsg{Type: tea.KeyRight})
	assert.Equal(t, config.GroupModeAccount, h.appConfig.GetGroupMode(),
		"must report its row key so home can persist")

	got := h.list.GetInstances()
	require.Len(t, got, 3)
	repos := make([]string, len(got))
	for i, inst := range got {
		repos[i] = filepath.Base(inst.Path)
	}
	assert.Equal(t, []string{"api", "infra", "sideproj"}, repos,
		"the two work-account repos (api, infra) must be adjacent after clustering")
}

func TestSettingsPanel_HidesHintBarLikeOtherModals(t *testing.T) {
	resetSettingsTestState(t)
	h := newSettingsTestHome()

	_, _ = h.handleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(",")})
	require.Equal(t, stateSettings, h.state)
	assert.False(t, h.menuVisible(), "the panel renders its own key hints; the bar would be redundant")
}
