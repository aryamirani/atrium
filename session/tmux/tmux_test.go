package tmux

import (
	cmd2 "claude-squad/cmd"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"claude-squad/cmd/cmd_test"

	"github.com/stretchr/testify/require"
)

type MockPtyFactory struct {
	t *testing.T

	// Array of commands and the corresponding file handles representing PTYs.
	cmds  []*exec.Cmd
	files []*os.File
}

func (pt *MockPtyFactory) Start(cmd *exec.Cmd) (*os.File, error) {
	filePath := filepath.Join(pt.t.TempDir(), fmt.Sprintf("pty-%s-%d", pt.t.Name(), rand.Int31()))
	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_RDWR, 0644)
	if err == nil {
		pt.cmds = append(pt.cmds, cmd)
		pt.files = append(pt.files, f)
	}
	return f, err
}

func (pt *MockPtyFactory) Close() {}

func NewMockPtyFactory(t *testing.T) *MockPtyFactory {
	return &MockPtyFactory{
		t: t,
	}
}

func TestSanitizeName(t *testing.T) {
	session := NewTmuxSession("asdf", "program")
	require.Equal(t, TmuxPrefix+"asdf", session.sanitizedName)

	session = NewTmuxSession("a sd f . . asdf", "program")
	require.Equal(t, TmuxPrefix+"asdf__asdf", session.sanitizedName)
}

func TestIsReadyForPrompt(t *testing.T) {
	cases := []struct {
		name    string
		program string
		content string
		want    bool
	}{
		{
			name:    "claude trust screen is not ready",
			program: "claude",
			content: "Do you trust the files in this folder?\n  Yes  No",
			want:    false,
		},
		{
			name:    "claude new MCP server screen is not ready",
			program: "claude",
			content: "new MCP server detected. Approve?",
			want:    false,
		},
		{
			name:    "empty pane is not ready",
			program: "claude",
			content: "   \n\t\n",
			want:    false,
		},
		{
			name:    "claude idle input box is ready",
			program: "claude",
			content: "╭───╮\n│ > │  ? for shortcuts\n╰───╯",
			want:    true,
		},
		{
			name:    "non-claude doc-url gate is not ready",
			program: "aider",
			content: "Open documentation url for more info? (Y)es/(N)o",
			want:    false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ptyFactory := NewMockPtyFactory(t)
			cmdExec := cmd_test.MockCmdExec{
				RunFunc: func(cmd *exec.Cmd) error { return nil }, // session exists
				OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
					return []byte(tc.content), nil
				},
			}
			session := newTmuxSession("ready-test", tc.program, ptyFactory, cmdExec)
			require.Equal(t, tc.want, session.IsReadyForPrompt())
		})
	}
}

// Regression: after extracting containsStartupGate (shared by CheckAndHandleTrustPrompt
// and IsReadyForPrompt), it must still recognize each gate string.
func TestContainsStartupGate(t *testing.T) {
	cases := []struct {
		name    string
		program string
		content string
		want    bool
	}{
		{"claude trust folder", "claude", "Do you trust the files in this folder?", true},
		{"claude new MCP server", "claude", "A new MCP server was found", true},
		{"non-claude doc url", "aider", "Open documentation url for more info", true},
		{"claude idle box has no gate", "claude", "│ > │  ? for shortcuts", false},
		{"claude ignores non-claude gate string", "claude", "Open documentation url for more info", false},
		{"non-claude ignores claude gate string", "aider", "Do you trust the files in this folder?", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, containsStartupGate(tc.program, tc.content))
		})
	}
}

// A dead/missing tmux session must not be probed: the pollers should short-circuit
// without ever running capture-pane, so a single dead session can't flood the log
// and error box with "error capturing pane content: exit status 1" every tick.
func TestPollersSkipCaptureWhenSessionDead(t *testing.T) {
	ptyFactory := NewMockPtyFactory(t)
	captured := false
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			// has-session fails => the session no longer exists.
			return fmt.Errorf("can't find session")
		},
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			captured = true
			return nil, fmt.Errorf("error capturing pane content: exit status 1")
		},
	}
	session := newTmuxSession("dead", "claude", ptyFactory, cmdExec)

	updated, hasPrompt := session.HasUpdated()
	require.False(t, updated)
	require.False(t, hasPrompt)
	require.False(t, session.IsReadyForPrompt())
	require.False(t, captured, "capture-pane must not run when the tmux session is dead")
}

// The happy path must keep working: an alive session still captures and reports
// freshly seen content as updated.
func TestHasUpdatedCapturesWhenSessionAlive(t *testing.T) {
	ptyFactory := NewMockPtyFactory(t)
	cmdExec := cmd_test.MockCmdExec{
		RunFunc:    func(cmd *exec.Cmd) error { return nil }, // session exists
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) { return []byte("hello"), nil },
	}
	session := newTmuxSession("alive", "claude", ptyFactory, cmdExec)
	session.monitor = newStatusMonitor() // normally set by Start/Restore

	updated, _ := session.HasUpdated()
	require.True(t, updated, "first capture of new content should report updated")
}

// TestSessionDeathStopsProbing drives a REAL tmux session (not mocks) to reproduce the
// production flood: once a started session's pane is killed out from under cs, the
// pollers must report "not alive" and stop capturing, instead of running capture-pane
// and getting "exit status 1" on every tick.
func TestSessionDeathStopsProbing(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available")
	}

	name := fmt.Sprintf("death-%s-%d", t.Name(), rand.Int31())
	session := NewTmuxSession(name, "sleep 300")
	require.NoError(t, session.Start(t.TempDir()))
	t.Cleanup(func() { _ = session.Close() })

	// While alive: detectable, and a probe runs without panicking.
	require.True(t, session.DoesSessionExist())
	_, _ = session.HasUpdated()

	// Kill the session out from under cs (simulates a crash / external kill).
	require.NoError(t, exec.Command("tmux", "kill-session", "-t", session.sanitizedName).Run())

	// The pollers must now short-circuit cleanly rather than erroring every tick.
	require.False(t, session.DoesSessionExist())
	updated, hasPrompt := session.HasUpdated()
	require.False(t, updated)
	require.False(t, hasPrompt)
	require.False(t, session.IsReadyForPrompt())
}

func TestStartTmuxSession(t *testing.T) {
	ptyFactory := NewMockPtyFactory(t)

	created := false
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			if strings.Contains(cmd.String(), "has-session") && !created {
				created = true
				return fmt.Errorf("session already exists")
			}
			return nil
		},
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			return []byte("output"), nil
		},
	}

	workdir := t.TempDir()
	session := newTmuxSession("test-session", "claude", ptyFactory, cmdExec)

	err := session.Start(workdir)
	require.NoError(t, err)
	require.Equal(t, 2, len(ptyFactory.cmds))
	require.Equal(t, fmt.Sprintf("tmux new-session -d -s claudesquad_test-session -c %s claude", workdir),
		cmd2.ToString(ptyFactory.cmds[0]))
	require.Equal(t, "tmux attach-session -t claudesquad_test-session",
		cmd2.ToString(ptyFactory.cmds[1]))

	require.Equal(t, 2, len(ptyFactory.files))

	// File should be closed.
	_, err = ptyFactory.files[0].Stat()
	require.Error(t, err)
	// File should be open
	_, err = ptyFactory.files[1].Stat()
	require.NoError(t, err)
}
