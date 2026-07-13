package ui

// Throwaway dev harness for the splash iteration loop: dumps rendered frames
// so structure can be judged in CI-less text (piped through ansi.Strip) and
// color in a real terminal (raw truecolor SGR — lipgloss would strip it in a
// no-TTY test process without the forced profile). Guarded by an env var so
// the suite never prints it. Deleted before merge.
//
// Usage:
//   ATRIUM_SPLASH_DUMP=1 go test ./ui -run TestSplashDump -v            # color, in a terminal
//   ATRIUM_SPLASH_DUMP=strip go test ./ui -run TestSplashDump -v       # structure only
//   ATRIUM_SPLASH_DUMP_FRAMES=0,40 ATRIUM_SPLASH_DUMP_SIZE=100x34 ...  # overrides

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

func TestSplashDump(t *testing.T) {
	mode := os.Getenv("ATRIUM_SPLASH_DUMP")
	if mode == "" {
		t.Skip("set ATRIUM_SPLASH_DUMP=1 (color) or =strip (structure) to dump frames")
	}
	lipgloss.SetColorProfile(termenv.TrueColor)

	w, h := 100, 34
	if s := os.Getenv("ATRIUM_SPLASH_DUMP_SIZE"); s != "" {
		if parts := strings.SplitN(s, "x", 2); len(parts) == 2 {
			if pw, err := strconv.Atoi(parts[0]); err == nil {
				w = pw
			}
			if ph, err := strconv.Atoi(parts[1]); err == nil {
				h = ph
			}
		}
	}
	frames := []int{0, 24}
	if s := os.Getenv("ATRIUM_SPLASH_DUMP_FRAMES"); s != "" {
		frames = frames[:0]
		for _, p := range strings.Split(s, ",") {
			if f, err := strconv.Atoi(strings.TrimSpace(p)); err == nil {
				frames = append(frames, f)
			}
		}
	}

	pal := splashTestPalette()
	for _, vc := range []struct {
		name string
		v    splashVariant
	}{
		{"legacy", splashVariantLegacy},
		{"a-fbm", splashVariantFBM},
		{"b-braille", splashVariantBraille},
	} {
		for _, frame := range frames {
			out := renderSplashField(w, h, frame, pal, centeredClearing(h, 20, 4), vc.v)
			if mode == "strip" {
				out = ansi.Strip(out)
			}
			fmt.Printf("──── %s frame %d (%dx%d) ────\n%s\n", vc.name, frame, w, h, out)
		}
	}
}
