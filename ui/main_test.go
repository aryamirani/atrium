package ui

import (
	"os"
	"testing"

	"github.com/ZviBaratz/atrium/internal/testutil"
)

func TestMain(m *testing.M) {
	// Pin the splash variant: with no override the splash rotates per launch
	// (time-seeded), which would make the String()-path splash tests
	// nondeterministic across runs.
	_ = os.Setenv("ATRIUM_SPLASH_VARIANT", "a")
	os.Exit(testutil.SandboxHomeMain(m))
}
