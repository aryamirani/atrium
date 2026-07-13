package testutil

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestRequireTmuxHelper is not a standalone test: it is the child process that
// TestRequireTmux re-execs. It runs its body only when GO_WANT_REQUIRE_TMUX_HELPER=1;
// the parent launches it with tmux hidden from PATH so RequireTmux hits its
// tmux-absent branch, and asserts on whether this process skips or fails.
func TestRequireTmuxHelper(t *testing.T) {
	if os.Getenv("GO_WANT_REQUIRE_TMUX_HELPER") != "1" {
		t.Skip("helper process; invoked only by TestRequireTmux")
	}
	RequireTmux(t)
}

func TestRequireTmux(t *testing.T) {
	// A non-empty dir with no tmux in it: the child inherits this as its whole
	// PATH so exec.LookPath("tmux") fails there, whether or not tmux is installed
	// on this machine (it is, locally and in CI).
	emptyDir := t.TempDir()

	// run re-execs this test binary against only TestRequireTmuxHelper, with a
	// scrubbed env. PATH and ATRIUM_CI_REQUIRE_TMUX are always replaced (never
	// duplicated) so an inherited value from a CI job that sets them can't leak
	// in and flip the outcome.
	run := func(requireValue string) (string, error) {
		extra := []string{
			"GO_WANT_REQUIRE_TMUX_HELPER=1",
			"PATH=" + emptyDir,
			requireTmuxEnv + "=" + requireValue,
		}
		cmd := exec.Command(os.Args[0], "-test.run=^TestRequireTmuxHelper$", "-test.v")
		cmd.Env = childEnv(extra...)
		out, err := cmd.CombinedOutput()
		return string(out), err
	}

	t.Run("skips when tmux absent and env unset", func(t *testing.T) {
		out, err := run("")
		if err != nil {
			t.Fatalf("want skip (exit 0), got error %v\noutput:\n%s", err, out)
		}
		if !strings.Contains(out, "tmux not available") {
			t.Fatalf("want skip message, got:\n%s", out)
		}
	})

	t.Run("fatals when tmux absent and env set", func(t *testing.T) {
		out, err := run("1")
		if err == nil {
			t.Fatalf("want failure (non-zero exit), got success\noutput:\n%s", out)
		}
		if !strings.Contains(out, "tmux not found but "+requireTmuxEnv+"=1") {
			t.Fatalf("want fatal message, got:\n%s", out)
		}
	})
}

// childEnv returns os.Environ() with every key named in extra removed, then
// extra appended — so the caller's values are the only occurrence of those keys
// and win regardless of platform getenv dup-resolution order.
func childEnv(extra ...string) []string {
	drop := make(map[string]bool, len(extra))
	for _, e := range extra {
		if i := strings.IndexByte(e, '='); i >= 0 {
			drop[e[:i]] = true
		}
	}
	env := make([]string, 0, len(os.Environ())+len(extra))
	for _, e := range os.Environ() {
		if i := strings.IndexByte(e, '='); i >= 0 && drop[e[:i]] {
			continue
		}
		env = append(env, e)
	}
	return append(env, extra...)
}
