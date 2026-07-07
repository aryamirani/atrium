package tmux

import (
	"context"
	"fmt"
	"os/exec"
	"slices"
	"testing"

	"github.com/ZviBaratz/atrium/cmd/cmd_test"

	"github.com/stretchr/testify/require"
)

// TestCloseTreatsDeadSessionAsSuccess verifies Close does not report a spurious
// error when kill-session fails because the session was already gone (external
// kill, crashed/absent server). It still attempts the kill — the classification is
// on the failure, not a pre-check that could skip a live session.
func TestCloseTreatsDeadSessionAsSuccess(t *testing.T) {
	for _, msg := range []string{"can't find session: x", "no server running on /tmp/sock", "session not found"} {
		t.Run(msg, func(t *testing.T) {
			var attempted bool
			cmdExec := cmd_test.MockCmdExec{
				RunFunc: func(cmd *exec.Cmd) error {
					if slices.Contains(cmd.Args, "kill-session") {
						attempted = true
						return fmt.Errorf("%s", msg)
					}
					return nil
				},
				OutputFunc: func(cmd *exec.Cmd) ([]byte, error) { return []byte(""), nil },
			}
			s := NewSessionWithDeps(context.Background(), "dead", "claude", NewMockPtyFactory(t), cmdExec)

			require.NoError(t, s.Close(), "an already-dead session must not surface a spurious teardown error")
			require.True(t, attempted, "Close should still attempt kill-session, not silently skip it")
		})
	}
}

// TestCloseSurfacesRealTeardownFailure verifies a kill-session failure that is NOT
// an already-dead session (a hung/unresponsive server) is surfaced, so the caller
// never reports a clean kill while a live agent keeps running.
func TestCloseSurfacesRealTeardownFailure(t *testing.T) {
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			if slices.Contains(cmd.Args, "kill-session") {
				return fmt.Errorf("server is wedged")
			}
			return nil
		},
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) { return []byte(""), nil },
	}
	s := NewSessionWithDeps(context.Background(), "hung", "claude", NewMockPtyFactory(t), cmdExec)

	err := s.Close()
	require.Error(t, err, "a real kill-session failure must surface")
	require.Contains(t, err.Error(), "wedged")
}
