package tmux

import (
	"errors"
	"os/exec"
)

// ErrNotInstalled is the user-facing sentinel returned by Available when tmux is
// not on PATH. Atrium runs every session inside tmux, so its absence is fatal to
// session creation; the message names the dependency and the fix rather than
// leaking a raw exec-not-found error. It is intentionally multi-clause so the
// TUI's error surfacing routes it to the persistent info modal (see the app's
// handleError) instead of a truncated toast.
var ErrNotInstalled = errors.New(
	"tmux is not installed — Atrium runs each session inside tmux. " +
		"Install it and retry (macOS: brew install tmux; Debian/Ubuntu: sudo apt install tmux). " +
		"Run `atrium doctor` to check dependencies")

// lookPath is the exec.LookPath seam so tests can simulate tmux present/absent
// without a real binary on PATH (mirrors notify.Notifier's lookPath field).
var lookPath = exec.LookPath

// Available reports whether tmux is usable: it returns ErrNotInstalled when the
// binary is not on PATH, nil otherwise. It is the pre-flight gate the new-session
// flow (the create form and smart dispatch) consults before building a worktree,
// so a missing tmux surfaces one clean, actionable message up front instead of the
// raw "exec: \"tmux\": executable file not found" at pty launch.
func Available() error {
	if _, err := lookPath("tmux"); err != nil {
		return ErrNotInstalled
	}
	return nil
}
