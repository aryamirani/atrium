package tmux

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	cmd2 "github.com/ZviBaratz/atrium/cmd"
	"github.com/ZviBaratz/atrium/cmd/cmd_test"

	"github.com/stretchr/testify/require"
)

// SetContext pushes the @atrium_* user options in a batched command and caches the
// payload: an unchanged push issues no tmux command, while a changed value pushes
// again. This keeps the per-second metadata tick from spawning subprocesses when
// nothing moved.
func TestSetContext_CachesUnchanged(t *testing.T) {
	var ran []string
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(c *exec.Cmd) error {
			ran = append(ran, cmd2.ToString(c))
			return nil
		},
		OutputFunc: func(c *exec.Cmd) ([]byte, error) { return nil, nil },
	}
	sess := NewSessionWithDeps(context.Background(), "alpha", "claude", NewMockPtyFactory(t), cmdExec)

	require.NoError(t, sess.SetContext("alpha", "left"))
	require.Len(t, ran, 1, "first push should issue one batched command")

	// The batched command carries both options.
	for _, sub := range []string{"@atrium_name", "@atrium_left"} {
		require.True(t, strings.Contains(ran[0], sub), "batched command missing %q: %s", sub, ran[0])
	}

	require.NoError(t, sess.SetContext("alpha", "left"))
	require.Len(t, ran, 1, "identical push should be a no-op")

	require.NoError(t, sess.SetContext("alpha", "left-CHANGED"))
	require.Len(t, ran, 2, "changed push should issue a new command")
}

// A detached session is clientless, so SetContext must NOT issue refresh-client (it
// would error "no client") — only set the options, which always succeed and therefore
// cache. Regression guard for the lag bug: when refresh-client was always included, a
// detached session's push failed every time, never cached, and re-ran 11 synchronous
// tmux batches on the main event loop every metadata tick. A client gets the refresh.
func TestSetContext_RefreshOnlyWhenAttached(t *testing.T) {
	var ran []string
	cmdExec := cmd_test.MockCmdExec{
		RunFunc:    func(c *exec.Cmd) error { ran = append(ran, cmd2.ToString(c)); return nil },
		OutputFunc: func(c *exec.Cmd) ([]byte, error) { return nil, nil },
	}

	// Detached (the default): no refresh-client, and the push still caches.
	detached := NewSessionWithDeps(context.Background(), "det", "claude", NewMockPtyFactory(t), cmdExec)
	require.NoError(t, detached.SetContext("det", "left"))
	require.Len(t, ran, 1)
	require.NotContains(t, ran[0], "refresh-client", "detached: no client to refresh")
	require.Contains(t, ran[0], "@atrium_left")
	require.NoError(t, detached.SetContext("det", "left"))
	require.Len(t, ran, 1, "detached push must cache (the lag-bug regression guard)")

	// Attached: the live client is repainted.
	ran = nil
	attached := NewSessionWithDeps(context.Background(), "att", "claude", NewMockPtyFactory(t), cmdExec)
	attached.attached.Store(true)
	require.NoError(t, attached.SetContext("att", "left"))
	require.Len(t, ran, 1)
	require.Contains(t, ran[0], "refresh-client", "attached: repaint the live status line")
}
