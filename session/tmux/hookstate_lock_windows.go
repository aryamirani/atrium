//go:build windows

package tmux

// hookStateLock is a no-op on Windows, which has no flock. Atrium's tmux-based runtime
// doesn't target Windows (the build exists only for completeness), and the whole hook
// mechanism requires a wrapped tmux `claude`, so concurrent hook writers never actually
// run there — mirroring the no-op single-instance lock in lock_windows.go and
// daemon/daemon_windows.go.
func hookStateLock(stateFile string) (unlock func(), err error) {
	return func() {}, nil
}
