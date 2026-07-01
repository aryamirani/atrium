package app

import (
	"testing"

	"github.com/ZviBaratz/atrium/config"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/require"
)

// stubDetect swaps the package detection seam for the test's duration.
func stubDetect(t *testing.T, profiles []config.Profile) {
	t.Helper()
	orig := detectAgents
	detectAgents = func() []config.Profile { return profiles }
	t.Cleanup(func() { detectAgents = orig })
}

func TestWelcome_FirstRunConfirmPersistsProgram(t *testing.T) {
	stubDetect(t, []config.Profile{
		{Name: "claude", Program: "claude"},
		{Name: "codex", Program: "codex"},
	})
	h := newCreateFormHome(t)

	// First WindowSizeMsg opens the welcome and fires the detect cmd.
	model, cmd := h.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	h = model.(*home)
	require.Equal(t, stateWelcome, h.state, "first launch enters stateWelcome")
	require.NotNil(t, h.welcomeOverlay)
	require.NotNil(t, cmd, "detection cmd should be returned")

	// Run the detect cmd and feed its message back.
	msg := cmd()
	detected, ok := msg.(agentsDetectedMsg)
	require.True(t, ok, "cmd should yield agentsDetectedMsg")
	model, _ = h.Update(detected)
	h = model.(*home)

	// Move to codex and confirm.
	h.Update(tea.KeyMsg{Type: tea.KeyDown})
	model, _ = h.Update(tea.KeyMsg{Type: tea.KeyEnter})
	h = model.(*home)

	require.Equal(t, stateDefault, h.state, "confirm closes the welcome")
	require.Equal(t, "codex", h.appConfig.DefaultProgram, "confirm persists the picked program")
	require.Equal(t, "codex", h.program, "confirm applies the program to the run")
	require.NotZero(t, h.appState.GetHelpScreensSeen()&(helpTypeWelcome{}.mask()), "confirm sets the welcome seen-bit")
}

func TestWelcome_SkipLeavesSeenBitUnset(t *testing.T) {
	stubDetect(t, []config.Profile{{Name: "claude", Program: "claude"}})
	h := newCreateFormHome(t)

	model, cmd := h.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	h = model.(*home)
	model, _ = h.Update(cmd().(agentsDetectedMsg))
	h = model.(*home)

	model, _ = h.Update(tea.KeyMsg{Type: tea.KeyEsc})
	h = model.(*home)

	require.Equal(t, stateDefault, h.state, "esc closes the welcome")
	require.Zero(t, h.appState.GetHelpScreensSeen()&(helpTypeWelcome{}.mask()), "skip must not set the seen-bit")
}

// While the welcome is up, menuVisible() is false and the panes are sized
// full-height by maybeShowWelcome's recomputeLayout(). Closing the welcome
// flips menuVisible back on (the hint bar is on by default), so unless the
// close path also recomputes, the hint bar's row lands on top of still
// full-height panes and the frame is one row taller than the terminal — on
// first-ever launch, before any other resize fixes it. TestViewFitsTerminalBounds
// (view_bounds_test.go) enforces the general "frame never exceeds the terminal"
// invariant across the app; these two mirror that same fit check specifically
// across a welcome close, on both the confirm and skip paths.
func TestWelcome_ConfirmFrameFitsAfterClose(t *testing.T) {
	stubDetect(t, []config.Profile{{Name: "claude", Program: "claude"}})
	h := newCreateFormHome(t)

	model, cmd := h.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	h = model.(*home)
	require.Equal(t, stateWelcome, h.state, "first launch enters stateWelcome")

	model, _ = h.Update(cmd().(agentsDetectedMsg))
	h = model.(*home)

	model, _ = h.Update(tea.KeyMsg{Type: tea.KeyEnter})
	h = model.(*home)

	require.Equal(t, stateDefault, h.state, "confirm closes the welcome")
	require.Equal(t, 40, lipgloss.Height(h.View()),
		"frame must be exactly the terminal height after the welcome closes via confirm")
}

func TestWelcome_SkipFrameFitsAfterClose(t *testing.T) {
	stubDetect(t, []config.Profile{{Name: "claude", Program: "claude"}})
	h := newCreateFormHome(t)

	model, cmd := h.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	h = model.(*home)
	require.Equal(t, stateWelcome, h.state, "first launch enters stateWelcome")

	model, _ = h.Update(cmd().(agentsDetectedMsg))
	h = model.(*home)

	model, _ = h.Update(tea.KeyMsg{Type: tea.KeyEsc})
	h = model.(*home)

	require.Equal(t, stateDefault, h.state, "esc closes the welcome")
	require.Equal(t, 40, lipgloss.Height(h.View()),
		"frame must be exactly the terminal height after the welcome closes via skip")
}

// markWelcomeSeen flips the welcome seen-bit so maybeShowWelcome takes the
// returning-user branch instead of showing the welcome.
func markWelcomeSeen(t *testing.T, h *home) {
	t.Helper()
	seen := h.appState.GetHelpScreensSeen()
	require.NoError(t, h.appState.SetHelpScreensSeen(seen|(helpTypeWelcome{}.mask())))
}

func TestWarn_ReturningUserMissingProgram(t *testing.T) {
	h := newCreateFormHome(t)
	markWelcomeSeen(t, h)
	h.program = "definitely-not-a-real-binary-xyzzy"

	// Size the app so the menu/errBox are laid out, then trigger the startup path.
	h.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	require.Equal(t, stateDefault, h.state, "no welcome for a returning user")
	require.True(t, h.pathWarned, "a missing program must trigger the one-shot warning")
}

func TestWarn_ReturningUserInstalledProgram(t *testing.T) {
	h := newCreateFormHome(t)
	markWelcomeSeen(t, h)
	h.program = "sh" // present on PATH

	h.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	require.False(t, h.pathWarned, "an installed program must not warn")
}

func TestWarn_SuppressedWhenWelcomeShows(t *testing.T) {
	stubDetect(t, []config.Profile{{Name: "claude", Program: "claude"}})
	h := newCreateFormHome(t)
	h.program = "definitely-not-a-real-binary-xyzzy" // would warn, but welcome shows

	h.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	require.Equal(t, stateWelcome, h.state, "first run shows the welcome")
	require.False(t, h.pathWarned, "the standalone warning is suppressed while the welcome shows")
}
