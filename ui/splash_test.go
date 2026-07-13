package ui

import (
	"strings"
	"testing"

	"github.com/ZviBaratz/atrium/ui/theme"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"
)

// splashTestPalette is a fixed truecolor palette (tokyo-night's hues) so the
// splash tests never depend on the global active theme.
func splashTestPalette() theme.Palette {
	return theme.Palette{
		Purple: lipgloss.Color("#bb9af7"),
		Accent: lipgloss.Color("#7aa2f7"),
		Cyan:   lipgloss.Color("#7dcfff"),
	}
}

// stripLines strips SGR and splits into visible lines.
func stripLines(s string) []string {
	return strings.Split(ansi.Strip(s), "\n")
}

// TestRenderSplashFieldDeterministic locks the pure-function contract: identical
// inputs must produce byte-identical output (so the field is snapshot-safe and
// the tick can drive it without hidden state).
func TestRenderSplashFieldDeterministic(t *testing.T) {
	pal := splashTestPalette()
	a := renderSplashField(80, 30, 5, pal, 20, 4)
	b := renderSplashField(80, 30, 5, pal, 20, 4)
	require.Equal(t, a, b, "same inputs must render identically")
	require.NotEmpty(t, a)
}

// TestRenderSplashFieldBounds is the view-bounds invariant: exactly h rows, each
// exactly w visible cells, across a spread of sizes (even/odd, wide/tall). This
// is what lets String() drop the field into the pane box without overflow (#251).
func TestRenderSplashFieldBounds(t *testing.T) {
	pal := splashTestPalette()
	sizes := [][2]int{{50, 18}, {66, 20}, {80, 30}, {120, 40}, {51, 19}, {73, 27}}
	for _, s := range sizes {
		w, h := s[0], s[1]
		field := renderSplashField(w, h, 3, pal, w/4, h/6)
		lines := strings.Split(field, "\n")
		require.Lenf(t, lines, h, "%dx%d: line count", w, h)
		for i, l := range lines {
			require.Equalf(t, w, lipgloss.Width(l), "%dx%d: line %d width", w, h, i)
		}
	}
}

// TestRenderSplashFieldAnimates guards the drift wiring: advancing the frame must
// change the rendered field (otherwise the "slow drift" is dead).
func TestRenderSplashFieldAnimates(t *testing.T) {
	pal := splashTestPalette()
	f0 := renderSplashField(80, 30, 3, pal, 20, 4)
	f1 := renderSplashField(80, 30, 4, pal, 20, 4)
	require.NotEqual(t, f0, f1, "consecutive frames must differ")
}

// TestRenderSplashFieldVignetteCorners locks the round vignette: the corners (and
// the height-limited top/bottom rows) fall outside the inscribed disc and stay
// blank, so the field reads as a circle, not a rectangle.
func TestRenderSplashFieldVignetteCorners(t *testing.T) {
	pal := splashTestPalette()
	w, h := 80, 30
	lines := stripLines(renderSplashField(w, h, 3, pal, 20, 4))
	require.Len(t, lines, h)
	// At 80x30 the disc is height-limited, so the first and last rows are fully
	// outside it — entirely blank.
	require.Equal(t, strings.Repeat(" ", w), lines[0], "top row must be blank")
	require.Equal(t, strings.Repeat(" ", w), lines[h-1], "bottom row must be blank")
	// And each corner is blank regardless.
	for _, rc := range [][2]int{{0, 0}, {0, w - 1}, {h - 1, 0}, {h - 1, w - 1}} {
		require.Equalf(t, byte(' '), lines[rc[0]][rc[1]], "corner (%d,%d)", rc[0], rc[1])
	}
}

// TestRenderSplashFieldClearing verifies the center clearing is blank, so the
// composited wordmark+message always lands on emptiness (never over field glyphs).
func TestRenderSplashFieldClearing(t *testing.T) {
	pal := splashTestPalette()
	w, h := 80, 30
	chw, chh := 20, 4
	lines := stripLines(renderSplashField(w, h, 3, pal, chw, chh))
	centerRow := (h - 1) / 2
	cx := (w - 1) / 2
	// Along the center row the clearing spans |dx| < chw — those cells are blank.
	for col := cx - (chw - 1); col <= cx+(chw-1); col++ {
		require.Equalf(t, byte(' '), lines[centerRow][col],
			"clearing cell (%d,%d) must be blank", centerRow, col)
	}
}

// TestOverlayCenterComposites checks the fade-less compositor drops fg onto the
// center of the field while preserving the field's exact w×h bounds (the whole
// point of doing it before the #251 clamp).
func TestOverlayCenterComposites(t *testing.T) {
	pal := splashTestPalette()
	w, h := 60, 20
	field := renderSplashField(w, h, 3, pal, 8, 3)
	fg := "ABCDEF"
	out := overlayCenter(field, fg)
	require.Contains(t, ansi.Strip(out), "ABCDEF", "fg must survive compositing")
	lines := strings.Split(out, "\n")
	require.Len(t, lines, h, "compositing must preserve height")
	for i, l := range lines {
		require.LessOrEqualf(t, lipgloss.Width(l), w, "composited line %d width", i)
	}
}

// TestRenderSplashFieldExtremes is the panic-safety net: degenerate sizes must
// return "" or a bounded block, never panic (the caller gates on splashFits, but
// the generator must be robust on its own).
func TestRenderSplashFieldExtremes(t *testing.T) {
	pal := splashTestPalette()
	sizes := [][2]int{{0, 0}, {1, 1}, {2, 2}, {1, 40}, {40, 1}, {50, 1}, {3, 3}, {51, 19}}
	for _, s := range sizes {
		w, h := s[0], s[1]
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("renderSplashField(%d,%d) panicked: %v", w, h, r)
				}
			}()
			out := renderSplashField(w, h, 2, pal, maxInt(1, w/4), maxInt(1, h/4))
			if out == "" {
				return
			}
			lines := strings.Split(out, "\n")
			require.LessOrEqualf(t, len(lines), h, "%dx%d: too many lines", w, h)
			for _, l := range lines {
				require.LessOrEqualf(t, lipgloss.Width(l), w, "%dx%d: line too wide", w, h)
			}
		}()
	}
}

// TestSplashLUTThemeAnchored locks the gradient to the theme: the ramp starts at
// the palette's purple (core), ends at its cyan (rim), and the rim hue actually
// flows into the upper half of the ramp (so swapping cyan changes the colors).
// It asserts at the LUT level because lipgloss strips truecolor to plain text in
// a no-TTY test process, so the rendered field carries no color to compare.
func TestSplashLUTThemeAnchored(t *testing.T) {
	pal := splashTestPalette()
	lut := splashLUTFor(pal)
	require.Equal(t, pal.Purple, lut.colors[0], "core stop is theme purple")
	require.Equal(t, pal.Cyan, lut.colors[len(lut.colors)-1], "rim stop is theme cyan")

	other := pal
	other.Cyan = lipgloss.Color("#a6e3a1") // a distinctly different rim (catppuccin green)
	otherLUT := splashLUTFor(other)
	require.NotEqual(t, lut.colors, otherLUT.colors,
		"changing the rim hue must change the gradient")
	// The lower half (core→accent) is rim-independent; the upper half must move.
	require.NotEqual(t, lut.colors[len(lut.colors)-4], otherLUT.colors[len(otherLUT.colors)-4],
		"the rim hue must reach the upper stops of the ramp")
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func BenchmarkRenderSplash(b *testing.B) {
	pal := splashTestPalette()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = renderSplashField(80, 30, i, pal, 20, 4)
	}
}
