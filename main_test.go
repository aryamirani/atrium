package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"testing"
	"time"

	"github.com/ZviBaratz/atrium/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// SIGHUP must stay in the graceful-shutdown set. It is the whole point of the
// fix (issue #268): a terminal close / SSH disconnect sends SIGHUP, and unless
// it is registered Go's default disposition terminates the process without
// running the deferred autoyes-daemon handoff. SIGINT/SIGTERM round out the set.
func TestQuitSignals(t *testing.T) {
	assert.Contains(t, quitSignals, os.Signal(syscall.SIGHUP))
	assert.Contains(t, quitSignals, os.Signal(syscall.SIGTERM))
	assert.Contains(t, quitSignals, os.Interrupt)
}

// Beyond asserting membership, prove the mechanism: a real SIGHUP delivered to
// this process cancels a context wired with quitSignals (rather than killing the
// test binary). NotifyContext registers before Kill, so Go catches the signal.
// Not parallel — it raises a process-global signal.
func TestQuitSignalsCancelContextOnSIGHUP(t *testing.T) {
	ctx, stop := signal.NotifyContext(context.Background(), quitSignals...)
	defer stop()

	require.NoError(t, syscall.Kill(syscall.Getpid(), syscall.SIGHUP))

	select {
	case <-ctx.Done():
		// pass: SIGHUP cancelled the lifecycle context, so defers (the daemon
		// handoff) get to run — it did not hard-kill the process.
	case <-time.After(2 * time.Second):
		t.Fatal("SIGHUP did not cancel the lifecycle context wired with quitSignals")
	}
}

// The exit-time daemon decision must re-read the persisted config rather than
// the autoyes value merged at startup, so an auto_yes toggle made in the
// settings panel during the run takes effect when the TUI exits.
func TestShouldLaunchDaemonOnExit(t *testing.T) {
	// Sandbox HOME so config reads/writes never touch the real data dir.
	t.Setenv("HOME", t.TempDir())

	t.Run("config off, no flag: no daemon", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.AutoYes = false
		require.NoError(t, config.SaveConfig(cfg))
		assert.False(t, shouldLaunchDaemonOnExit(false))
	})

	t.Run("config toggled on during the run wins over the startup value", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.AutoYes = true
		require.NoError(t, config.SaveConfig(cfg))
		assert.True(t, shouldLaunchDaemonOnExit(false))
	})

	t.Run("the -y flag wins for the run that received it", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.AutoYes = false
		require.NoError(t, config.SaveConfig(cfg))
		assert.True(t, shouldLaunchDaemonOnExit(true))
	})
}
