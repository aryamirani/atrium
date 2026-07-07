package main

import (
	"os"
	"syscall"
	"testing"

	"github.com/ZviBaratz/atrium/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// SIGHUP must stay in the graceful-shutdown set. It is the whole point of the
// fix (issue #268): a terminal close / SSH disconnect sends SIGHUP, and unless
// it is registered Go's default disposition terminates the process without
// running the deferred autoyes-daemon handoff. SIGINT/SIGTERM round out the set.
// The real-signal mechanism test lives in main_sighup_test.go (non-Windows).
func TestQuitSignals(t *testing.T) {
	assert.Contains(t, quitSignals, os.Signal(syscall.SIGHUP))
	assert.Contains(t, quitSignals, os.Signal(syscall.SIGTERM))
	assert.Contains(t, quitSignals, os.Interrupt)
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
