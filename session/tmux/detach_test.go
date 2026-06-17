package tmux

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"testing"

	"github.com/ZviBaratz/atrium/cmd/cmd_test"

	"github.com/stretchr/testify/require"
)

// attachedSession builds a Session in the "currently attached" state without going
// through the real-stdin Attach goroutines: it installs the one interactive client
// (ptmx + attachCmd) and the per-attach fields directly, mirroring what Attach
// establishes. In the clientless model a detached session has no client, so the
// pty/cmd exist ONLY in this attached state.
func attachedSession(t *testing.T) (*Session, *MockPtyFactory) {
	t.Helper()
	ptyFactory := NewMockPtyFactory(t)
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error { return nil }, // session exists / is alive
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			return []byte("output"), nil
		},
	}
	s := NewSessionWithDeps(context.Background(), "detach-test", "claude", ptyFactory, cmdExec)
	ptmx, attachCmd, err := ptyFactory.Start(exec.CommandContext(context.Background(), "true"))
	require.NoError(t, err)
	s.ptmx = ptmx
	s.attachCmd = attachCmd
	s.attachCh = make(chan struct{})
	s.wg = &sync.WaitGroup{}
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.attached.Store(true) // mirror Attach's poll guard
	return s, ptyFactory
}

// assertClosed fails unless ch is already closed (a receive that does not block).
func assertClosed(t *testing.T, ch chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	default:
		t.Fatal("attachCh not closed")
	}
}

// T1: a clean detach reaps the interactive client, records no error, and tears down
// the per-attach state, leaving the session clientless (ptmx nil).
func TestDetachCleanByDefault(t *testing.T) {
	s, _ := attachedSession(t)
	ch := s.attachCh

	require.NotPanics(t, s.Detach)

	require.NoError(t, s.AttachExitError())
	require.Nil(t, s.ptmx, "detach is clientless: ptmx stays nil")
	require.Nil(t, s.attachCmd, "the interactive client cmd is cleared (and reaped)")
	require.Nil(t, s.attachCh)
	require.Nil(t, s.cancel)
	require.Nil(t, s.wg)
	require.Nil(t, s.ctx)
	require.False(t, s.attached.Load())
	assertClosed(t, ch)
}

// T2: a failing pty close is recorded in detachErr, not fatal; the session is still
// left clientless (ptmx nil).
func TestDetachPtyCloseErrorDoesNotPanic(t *testing.T) {
	s, _ := attachedSession(t)
	ch := s.attachCh
	require.NoError(t, s.ptmx.Close()) // the next Close inside Detach now errors

	require.NotPanics(t, s.Detach)

	require.Error(t, s.AttachExitError())
	require.Contains(t, s.AttachExitError().Error(), "closing attach pty")
	require.Nil(t, s.ptmx, "detach is clientless: ptmx stays nil even on a close error")
	require.Nil(t, s.attachCh)
	assertClosed(t, ch)
}

// T3: detaching when the tmux session has vanished is clean — the re-baseline
// (resize-window) failing is best-effort, NOT a detach error — and leaves the session
// clientless without breaking polling.
func TestDetachWhenSessionGoneStaysClean(t *testing.T) {
	ptyFactory := NewMockPtyFactory(t)
	cmdExec := cmd_test.MockCmdExec{
		RunFunc:    func(cmd *exec.Cmd) error { return fmt.Errorf("no server running") },
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) { return nil, fmt.Errorf("no server running") },
	}
	s := NewSessionWithDeps(context.Background(), "detach-gone", "claude", ptyFactory, cmdExec)
	ptmx, attachCmd, err := ptyFactory.Start(exec.CommandContext(context.Background(), "true"))
	require.NoError(t, err)
	s.ptmx, s.attachCmd = ptmx, attachCmd
	s.attachCh = make(chan struct{})
	s.wg = &sync.WaitGroup{}
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.attached.Store(true)
	ch := s.attachCh

	require.NotPanics(t, s.Detach)

	// The geometry re-baseline failed (session gone) but that is logged, not surfaced:
	// the detach itself succeeded.
	require.NoError(t, s.AttachExitError())
	require.Nil(t, s.ptmx, "detach is clientless: ptmx stays nil")
	require.Nil(t, s.attachCh)
	require.False(t, s.attached.Load())
	assertClosed(t, ch)

	// A nil ptmx must not break polling — Poll never reads ptmx, only cmdExec.
	require.NotPanics(t, func() { _ = s.Poll() })
}

// T4: clientless Restore allocates NO pty; it returns nil when the session exists (so
// it can re-baseline geometry + swap the monitor) and errors when it is gone (so
// instance Resume can fall back to kill-and-recreate).
func TestRestoreClientlessExistenceContract(t *testing.T) {
	exists := true
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			if strings.Contains(strings.Join(cmd.Args, " "), "has-session") && !exists {
				return fmt.Errorf("no such session")
			}
			return nil
		},
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) { return []byte("x"), nil },
	}
	s := NewSessionWithDeps(context.Background(), "restore-test", "claude", NewMockPtyFactory(t), cmdExec)

	require.NoError(t, s.Restore(), "Restore succeeds when the session exists")
	require.Nil(t, s.ptmx, "clientless Restore never allocates a pty")

	exists = false
	require.Error(t, s.Restore(), "Restore errors when the session is gone")
	require.Nil(t, s.ptmx)
}

// T5: DetachSafely tears down without re-creating a client (leaving ptmx nil), and a
// second call is a no-op (no double-close panic).
func TestDetachSafelyUnchangedAndIdempotent(t *testing.T) {
	s, _ := attachedSession(t)
	ch := s.attachCh

	require.NoError(t, s.DetachSafely())
	require.Nil(t, s.ptmx, "DetachSafely leaves the session clientless")
	require.Nil(t, s.attachCh)
	assertClosed(t, ch)

	require.NoError(t, s.DetachSafely(), "second DetachSafely must be a no-op")
}

// T6: mixing the two detach paths in either order never panics or double-closes.
func TestDoubleDetachNoPanic(t *testing.T) {
	s, _ := attachedSession(t)
	require.NotPanics(t, s.Detach)
	require.NotPanics(t, func() { _ = s.DetachSafely() })

	s2, _ := attachedSession(t)
	require.NotPanics(t, func() { _ = s2.DetachSafely() })
	require.NotPanics(t, s2.Detach)
}
