package tmux

import (
	"os"
	"os/exec"

	"github.com/creack/pty"
)

// PtyFactory creates the pseudo-terminal a tmux client (attach) runs in. It
// exists so tests can substitute a fake pty instead of allocating a real one.
//
// Start also returns the started *exec.Cmd so the caller can Wait() on it once the
// pty master is closed — closing the master makes the tmux client exit, and without
// a Wait() that process lingers as a zombie. The cmd is the same value passed in
// (creack's pty.Start has already forked it).
type PtyFactory interface {
	Start(cmd *exec.Cmd) (*os.File, *exec.Cmd, error)
	Close()
}

// Pty starts a "real" pseudo-terminal (PTY) using the creack/pty package.
type Pty struct{}

// Start launches cmd inside a new pty and returns its master end plus the started
// cmd (for reaping via Wait()).
func (pt Pty) Start(cmd *exec.Cmd) (*os.File, *exec.Cmd, error) {
	f, err := pty.Start(cmd)
	return f, cmd, err
}

// Close is a no-op: the real pty's lifetime is tied to the file returned by
// Start, which callers close themselves.
func (pt Pty) Close() {}

// MakePtyFactory returns the real pty-backed PtyFactory.
func MakePtyFactory() PtyFactory {
	return Pty{}
}
