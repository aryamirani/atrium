package tmux

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	"github.com/ZviBaratz/atrium/cmd/cmd_test"

	"github.com/stretchr/testify/require"
)

// recordingExec returns a MockCmdExec that appends every subprocess's argv to the
// returned slice pointer, so tests can assert which tmux commands a clientless
// geometry path issued (and, crucially, that it never spawns an attach-session
// client while detached). runErr is returned from every Run.
func recordingExec(runErr error) (*[]string, cmd_test.MockCmdExec) {
	argv := &[]string{}
	rec := func(c *exec.Cmd) { *argv = append(*argv, strings.Join(c.Args, " ")) }
	return argv, cmd_test.MockCmdExec{
		RunFunc:    func(c *exec.Cmd) error { rec(c); return runErr },
		OutputFunc: func(c *exec.Cmd) ([]byte, error) { rec(c); return []byte("x"), nil },
	}
}

func argvCount(argv []string, sub string) int {
	n := 0
	for _, a := range argv {
		if strings.Contains(a, sub) {
			n++
		}
	}
	return n
}

// Detached geometry is driven server-side by resize-window — never by spawning a
// phantom attach-session client — and an unchanged size is deduped so the per-layout
// fan-out over every session is free.
func TestSetDetachedSizeResizesWindowAndDedupes(t *testing.T) {
	argv, cmdExec := recordingExec(nil)
	s := NewSessionWithDeps(context.Background(), "geo", "claude", NewMockPtyFactory(t), cmdExec)

	require.NoError(t, s.SetDetachedSize(80, 24))
	require.Equal(t, 1, argvCount(*argv, "resize-window"), "first size issues one resize-window")
	require.Contains(t, strings.Join(*argv, "\n"), "-x 80 -y 24")
	require.Zero(t, argvCount(*argv, "attach-session"), "a detached session never holds a client")

	require.NoError(t, s.SetDetachedSize(80, 24))
	require.Equal(t, 1, argvCount(*argv, "resize-window"), "an unchanged size is deduped")

	require.NoError(t, s.SetDetachedSize(100, 30))
	require.Equal(t, 2, argvCount(*argv, "resize-window"), "a changed size resizes again")
	require.Contains(t, strings.Join(*argv, "\n"), "-x 100 -y 30")
}

// While attached, SetDetachedSize records the size but issues no resize-window: the
// live client owns geometry (window-size latest), and a server-side resize would flip
// the window back to manual and break the client's tracking.
func TestSetDetachedSizeRecordOnlyWhileAttached(t *testing.T) {
	argv, cmdExec := recordingExec(nil)
	s := NewSessionWithDeps(context.Background(), "geo-att", "claude", NewMockPtyFactory(t), cmdExec)
	s.attached.Store(true)

	require.NoError(t, s.SetDetachedSize(80, 24))
	require.Zero(t, argvCount(*argv, "resize-window"), "attached: record only, no resize-window")
}

// Restore re-applies the recorded detached geometry (and never spawns a client).
func TestRestoreReappliesGeometryClientlessly(t *testing.T) {
	argv, cmdExec := recordingExec(nil)
	s := NewSessionWithDeps(context.Background(), "geo-restore", "claude", NewMockPtyFactory(t), cmdExec)

	require.NoError(t, s.SetDetachedSize(70, 20))
	before := argvCount(*argv, "resize-window")
	require.NoError(t, s.Restore())
	require.Greater(t, argvCount(*argv, "resize-window"), before, "Restore re-applies geometry")
	require.Nil(t, s.ptmx, "Restore is clientless")
	require.Zero(t, argvCount(*argv, "attach-session"))
}

// PrepareLiveServer migrates a persisted server to the clientless options and detaches
// every stale phantom client a prior run left attached.
func TestPrepareLiveServerMigratesAndSweeps(t *testing.T) {
	var argv []string
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(c *exec.Cmd) error { argv = append(argv, strings.Join(c.Args, " ")); return nil },
		OutputFunc: func(c *exec.Cmd) ([]byte, error) {
			argv = append(argv, strings.Join(c.Args, " "))
			return []byte("/dev/pts/3\n/dev/pts/7\n"), nil
		},
	}

	PrepareLiveServer(context.Background(), cmdExec)

	joined := strings.Join(argv, "\n")
	require.Contains(t, joined, "set-option -g window-size manual")
	require.Contains(t, joined, "set-option -g aggressive-resize off")
	require.Equal(t, 2, argvCount(argv, "detach-client"), "each stale client is detached")
}
