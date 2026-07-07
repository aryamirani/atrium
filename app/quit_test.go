package app

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/ZviBaratz/atrium/config"
	"github.com/ZviBaratz/atrium/session"
	"github.com/ZviBaratz/atrium/ui"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pressQ sends the global quit key from the default list view.
func pressQ(t *testing.T, h *home) tea.Cmd {
	t.Helper()
	_, cmd := h.handleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	return cmd
}

// isQuit reports whether running cmd yields the tea.QuitMsg that tea.Quit produces.
func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

// newQuitTestHome builds a home wired enough to drive handleQuit and
// handleInstanceStarted: a real list, a working in-memory storage, auto-attach
// disabled (so a completed start never tries to attach), and the overlays those
// paths touch. HOME is already sandboxed by the package TestMain.
func newQuitTestHome(t *testing.T) *home {
	t.Helper()
	spin := spinner.New(spinner.WithSpinner(spinner.MiniDot))
	list := ui.NewList(&spin)

	appState := config.DefaultState()
	storage, err := session.NewStorage(appState)
	require.NoError(t, err)

	noAutoAttach := false
	cfg := config.DefaultConfig()
	cfg.AutoAttach = &noAutoAttach

	return &home{
		ctx:          context.Background(),
		state:        stateDefault,
		appConfig:    cfg,
		appState:     appState,
		storage:      storage,
		list:         list,
		menu:         ui.NewMenu(),
		tabbedWindow: ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewDiffPane(), ui.NewTerminalPane(context.Background())),
	}
}

// addLoadingInstance appends a session in the Loading phase — the state the
// new-session flow leaves it in while its background Start goroutine runs.
func addLoadingInstance(t *testing.T, h *home, title string) *session.Instance {
	t.Helper()
	inst, err := session.NewInstance(session.InstanceOptions{
		Title: title, Path: t.TempDir(), Program: "echo",
	})
	require.NoError(t, err)
	h.list.AddInstance(inst)
	inst.SetStatus(session.Loading)
	return inst
}

// Fix 2: quitting while a session is still Loading must not drop it. handleQuit
// defers, and the exit completes once the in-flight Start reports done.
func TestQuitWaitsForLoadingSessionThenExits(t *testing.T) {
	h := newQuitTestHome(t)
	inst := addLoadingInstance(t, h, "loading-one")

	cmd := pressQ(t, h)
	assert.False(t, isQuit(cmd), "must not quit while a session is Loading")
	assert.True(t, h.quitRequested, "quit should be armed for after startup")

	// Start completes -> the deferred quit fires.
	_, cmd = h.Update(instanceStartedMsg{instance: inst})
	assert.Equal(t, session.Running, inst.GetStatus())
	assert.True(t, isQuit(cmd), "quit should complete once nothing is Loading")
}

// Fix 2 regression guard: with nothing Loading, quit exits immediately.
func TestQuitWithNothingLoadingExitsImmediately(t *testing.T) {
	h := newQuitTestHome(t)

	cmd := pressQ(t, h)
	assert.False(t, h.quitRequested)
	assert.True(t, isQuit(cmd))
}

// Fix 2: a deferred quit must wait for EVERY Loading session, not just the first
// to finish — otherwise the still-Loading sibling would be dropped.
func TestQuitWaitsForAllLoadingSessions(t *testing.T) {
	h := newQuitTestHome(t)
	first := addLoadingInstance(t, h, "loading-first")
	second := addLoadingInstance(t, h, "loading-second")

	cmd := pressQ(t, h)
	require.False(t, isQuit(cmd))
	require.True(t, h.quitRequested)

	_, cmd = h.Update(instanceStartedMsg{instance: first})
	assert.False(t, isQuit(cmd), "second session still Loading -> keep waiting")
	assert.True(t, h.quitRequested, "quit stays armed")

	_, cmd = h.Update(instanceStartedMsg{instance: second})
	assert.True(t, isQuit(cmd), "last Loading session done -> quit")
}

// failingStore is a config.InstanceStorage whose SaveInstances always fails,
// simulating a full disk / read-only data dir.
type failingStore struct{}

func (failingStore) SaveInstances(json.RawMessage) error {
	return errors.New("no space left on device")
}
func (failingStore) GetInstances() json.RawMessage { return nil }
func (failingStore) DeleteAllInstances() error     { return nil }

// Fix 3: when state can't be saved, quit must not trap the user in an unquittable
// TUI — it opens a confirm modal, and confirming exits anyway.
func TestQuitOnSaveFailureOpensConfirmModalAndCanForceQuit(t *testing.T) {
	h := newQuitTestHome(t)
	st, err := session.NewStorage(failingStore{})
	require.NoError(t, err)
	h.storage = st

	cmd := pressQ(t, h)
	assert.False(t, isQuit(cmd), "a save failure must not quit outright")
	assert.Equal(t, stateConfirm, h.state, "it should open the quit-anyway confirm modal")
	require.NotNil(t, h.confirmationOverlay)
	require.NotNil(t, h.pendingConfirmAction)

	// Confirm -> quit anyway.
	_, cmd = h.handleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	assert.Equal(t, stateDefault, h.state)
	assert.True(t, isQuit(cmd), "confirming must quit despite the save failure")
}

// Fix 3: cancelling the save-failure confirm returns to the list without quitting.
func TestQuitOnSaveFailureCancelDoesNotQuit(t *testing.T) {
	h := newQuitTestHome(t)
	st, err := session.NewStorage(failingStore{})
	require.NoError(t, err)
	h.storage = st

	require.False(t, isQuit(pressQ(t, h)))
	require.Equal(t, stateConfirm, h.state)

	_, cmd := h.handleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	assert.Equal(t, stateDefault, h.state)
	assert.False(t, isQuit(cmd), "cancelling must not quit")
}
