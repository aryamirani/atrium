package testutil

import (
	"os"
	"os/exec"
	"testing"
)

// requireTmuxEnv is the environment variable that flips RequireTmux from a
// friendly local skip into a hard failure. CI sets it to "1" on the jobs that
// install tmux (see .github/workflows/build.yml and issue #274).
const requireTmuxEnv = "ATRIUM_CI_REQUIRE_TMUX"

// RequireTmux gates a real-tmux integration test on tmux being available. When
// tmux is absent it skips the test — keeping local runs friendly on machines
// without tmux — unless ATRIUM_CI_REQUIRE_TMUX=1, in which case a missing tmux
// is a hard failure instead of a silent skip.
//
// The regression guards these tests provide (e.g. the atrium.conf bad-escape bug
// that shipped and broke every new session) are only useful if they actually
// run. CI installs tmux and sets ATRIUM_CI_REQUIRE_TMUX=1 so that a broken tmux
// install can never let those guards silently re-skip and go dark. See #274.
func RequireTmux(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		if os.Getenv(requireTmuxEnv) == "1" {
			t.Fatalf("tmux not found but %s=1: %v", requireTmuxEnv, err)
		}
		t.Skipf("tmux not available: %v", err)
	}
}
