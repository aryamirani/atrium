package app

import (
	"context"
	"testing"

	"github.com/ZviBaratz/atrium/ui"

	"github.com/charmbracelet/bubbles/spinner"
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
