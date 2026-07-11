//go:build !windows

package tmux

import (
	"os"
	"syscall"
)

// hookStateLock takes a blocking exclusive lock on stateFile's sidecar ".lock" file and
// returns an unlock func. It serializes the concurrent hook subprocesses (and the
// watchdog) that read-modify-write the state record, across processes — the kernel
// releases the flock on close (and on process death), so a crashed hook never wedges the
// next one.
//
// The lock lives in a SEPARATE, never-renamed file rather than the state file itself:
// UpdateHookState replaces the state file by atomic rename, which swaps its inode, so a
// flock held on the state inode would protect the wrong (unlinked) inode after the first
// write. The ".lock" file's inode is stable, so all writers contend on the same lock.
//
// LOCK_EX blocks (not LOCK_NB): a hook must wait its turn and apply its mutation, not
// give up — dropping a SubagentStart/Stop would corrupt the in-flight set. Mirrors the
// flock idiom in lock_unix.go / daemon/daemon_unix.go, minus the non-blocking flag.
func hookStateLock(stateFile string) (unlock func(), err error) {
	f, err := os.OpenFile(stateFile+".lock", os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, err
	}
	// Closing the descriptor releases the flock.
	return func() { _ = f.Close() }, nil
}
