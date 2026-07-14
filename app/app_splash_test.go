package app

import (
	"context"
	"strings"
	"testing"

	"github.com/ZviBaratz/atrium/ui"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"
)

// splashTestHome builds the minimal home the splash tick needs: a tabbed
// window to push frames into and an empty list (the idle splash's condition).
func splashTestHome() *home {
	tw := ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewDiffPane(), ui.NewTerminalPane(context.Background()))
	sp := spinner.New()
	return &home{tabbedWindow: tw, list: ui.NewList(&sp)}
}

// TestSplashTickFrozenWhileOverlayUp locks the "don't animate behind a
// dialog" behavior: outside the default state the loop neither arms nor
// re-arms, so the welcome dialog (and any other overlay) the user is reading
// has a still background, not churning motion.
func TestSplashTickFrozenWhileOverlayUp(t *testing.T) {
	m := splashTestHome()
	m.state = stateWelcome

	require.Nil(t, m.armSplashTick(), "the loop must not arm behind an overlay")

	// A live loop dies (and clears its flag) the moment an overlay owns the
	// screen, without advancing the frame.
	m.splashTicking = true
	_, cmd := m.handleSplashTick()
	require.Nil(t, cmd, "the loop must die behind an overlay")
	require.False(t, m.splashTicking, "a dead loop must clear its flag")
	require.Zero(t, m.splashFrame, "splash must not advance behind an overlay")
}

// TestSplashTickAnimatesWhenIdle locks the animation loop's contract in the
// idle empty state: arming is single-flight, and each tick advances exactly
// one frame and re-arms.
func TestSplashTickAnimatesWhenIdle(t *testing.T) {
	m := splashTestHome()
	m.state = stateDefault

	require.NotNil(t, m.armSplashTick(), "the idle empty state must arm the loop")
	require.True(t, m.splashTicking)
	require.Nil(t, m.armSplashTick(), "a live loop must not be armed twice")

	_, cmd := m.handleSplashTick()
	require.NotNil(t, cmd, "an animating loop must re-arm itself")
	require.Equal(t, 1, m.splashFrame, "each tick advances one frame")
	_, _ = m.handleSplashTick()
	require.Equal(t, 2, m.splashFrame)
}

// screensaverTestHome is splashTestHome at a window size the splash fits.
func screensaverTestHome() *home {
	m := splashTestHome()
	m.windowWidth, m.windowHeight = 80, 30
	return m
}

// TestScreensaverEntersAndAnyKeyExits locks the easter egg's core loop:
// backtick enters (arming the animation), and the next key wakes the screen
// while being consumed — a stray 'n' must not open the new-session form.
func TestScreensaverEntersAndAnyKeyExits(t *testing.T) {
	m := screensaverTestHome()
	m.state = stateDefault

	_, cmd := m.handleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("`")})
	require.Equal(t, stateScreensaver, m.state)
	require.NotNil(t, cmd, "entering must arm the splash tick")
	require.True(t, m.splashTicking)

	_, _ = m.handleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	require.Equal(t, stateDefault, m.state)
	require.Nil(t, m.textInputOverlay, "the waking key must be consumed, not acted on")
}

// TestScreensaverConsumesQuitKeys guards against rage-quits: q / ctrl+c wake
// the screen instead of tearing the app down.
func TestScreensaverConsumesQuitKeys(t *testing.T) {
	for _, k := range []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune("q")},
		{Type: tea.KeyCtrlC},
	} {
		m := screensaverTestHome()
		m.state = stateScreensaver
		_, cmd := m.handleKeyPress(k)
		require.Equal(t, stateDefault, m.state, "key %q must wake", k.String())
		require.Nil(t, cmd, "key %q must not quit", k.String())
	}
}

// TestScreensaverIgnoredBelowSplashFloor: with the window too small for the
// field to read there is nothing to show, so the key is silently inert.
func TestScreensaverIgnoredBelowSplashFloor(t *testing.T) {
	m := splashTestHome()
	m.windowWidth, m.windowHeight = 40, 10
	m.state = stateDefault

	_, _ = m.handleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("`")})
	require.Equal(t, stateDefault, m.state)
}

// TestScreensaverAnimatesRegardlessOfSessions pins the short-circuit in
// splashAnimating: the screensaver animates whatever the session count — the
// nil list here would panic if the state ever fell through to the idle
// branch's NumInstances check.
func TestScreensaverAnimatesRegardlessOfSessions(t *testing.T) {
	m := &home{state: stateScreensaver}
	require.True(t, m.splashAnimating())
}

// TestScreensaverViewIsFullWindow: the view replaces the whole frame at the
// window size (the pane sizes here are zero — the screensaver must not use
// them).
func TestScreensaverViewIsFullWindow(t *testing.T) {
	m := screensaverTestHome()
	m.state = stateScreensaver
	m.splashFrame = 5

	lines := strings.Split(ansi.Strip(m.View()), "\n")
	require.Len(t, lines, m.windowHeight)
	for i, ln := range lines {
		require.LessOrEqual(t, lipgloss.Width(ln), m.windowWidth, "row %d overflows", i)
	}
}

// TestScreensaverMouse: a click wakes the screen; wheel and motion don't, so
// a nudged mouse doesn't tear it down.
func TestScreensaverMouse(t *testing.T) {
	m := screensaverTestHome()
	m.state = stateScreensaver

	_, _ = m.handleMouse(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelUp})
	require.Equal(t, stateScreensaver, m.state, "wheel must not wake")
	_, _ = m.handleMouse(tea.MouseMsg{Action: tea.MouseActionMotion})
	require.Equal(t, stateScreensaver, m.state, "motion must not wake")

	_, _ = m.handleMouse(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	require.Equal(t, stateDefault, m.state, "a click wakes")
}

// TestScreensaverExitsWhenResizedBelowFloor: shrinking the window under the
// splash floor mid-screensaver wakes rather than rendering a degenerate field.
func TestScreensaverExitsWhenResizedBelowFloor(t *testing.T) {
	h := newCreateFormHome(t)
	h.welcomeChecked = true // keep maybeShowWelcome out of the resize path
	h.state = stateScreensaver

	model, _ := h.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	h = model.(*home)
	require.Equal(t, stateScreensaver, h.state, "a comfortable resize keeps the screensaver")

	model, _ = h.Update(tea.WindowSizeMsg{Width: 40, Height: 10})
	h = model.(*home)
	require.Equal(t, stateDefault, h.state)
}
