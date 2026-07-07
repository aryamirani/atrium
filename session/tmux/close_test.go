package tmux

import (
	"context"
	"fmt"
	"os/exec"
	"slices"
	"strings"
	"testing"

	"github.com/ZviBaratz/atrium/cmd/cmd_test"

	"github.com/stretchr/testify/require"
)

// TestCloseTargetsSessionByExactName guards the kill target form. A bare "-t" is a
// tmux prefix match, so killing an already-gone session could match and kill a live
// sibling whose name this one is a prefix of ("sess" vs "sess2"). Close must target
// by exact name with "-t=".
func TestCloseTargetsSessionByExactName(t *testing.T) {
	var killArgs []string
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			if slices.Contains(cmd.Args, "kill-session") {
				killArgs = slices.Clone(cmd.Args)
			}
			return nil
		},
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) { return []byte(""), nil },
	}
	s := NewSessionWithDeps(context.Background(), "sess", "claude", NewMockPtyFactory(t), cmdExec)

	require.NoError(t, s.Close())
	require.NotEmpty(t, killArgs, "Close must issue kill-session")
	require.NotContains(t, killArgs, "-t", "bare -t is a tmux prefix match and can kill the wrong session")
	require.True(t, slices.ContainsFunc(killArgs, func(a string) bool { return strings.HasPrefix(a, "-t=") }),
		"kill-session must target by exact name (-t=<name>)")
}

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
						// Model real tmux: the diagnostic lands on stderr and the
						// process exits non-zero. This exercises Close's stderr
						// classification (its production path), not just the
						// error-string fallback that test fakes would otherwise hit.
						if cmd.Stderr != nil {
							fmt.Fprintln(cmd.Stderr, msg)
						}
						return fmt.Errorf("exit status 1")
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
				// Real tmux: diagnostic on stderr, generic non-zero exit. Close must
				// fold the stderr text into the surfaced error.
				if cmd.Stderr != nil {
					fmt.Fprintln(cmd.Stderr, "server is wedged")
				}
				return fmt.Errorf("exit status 1")
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
