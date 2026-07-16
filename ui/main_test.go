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
	//
	// Deliberately not the fallback (splashDefaultVariant): an unrecognized value
	// resolves to that with ok=true, so a pin naming it would keep "working" while
	// saying nothing — which is exactly what "a" did until the variant it named was
	// deleted. TestSplashEnvOverrideTrumpsSelection is what notices.
	_ = os.Setenv("ATRIUM_SPLASH_VARIANT", "tunnel")
	os.Exit(testutil.SandboxHomeMain(m))
}
