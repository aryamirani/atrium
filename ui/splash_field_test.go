package ui

import (
	"math"
	"strings"
	"sync"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"
)

// TestSplashHashGolden pins exact hash outputs. The hash is pure integer math,
// so these goldens hold on every architecture — this is the one place where
// exact-value snapshots are safe (float-based field output can differ across
// arches via FMA contraction, so frame tests stay property-based).
func TestSplashHashGolden(t *testing.T) {
	cases := []struct {
		x, y int32
		seed uint32
		want uint32
	}{
		{0, 0, 0x0, 0x944FB554},
		{1, 0, 0x0, 0xB2FCF063},
		{0, 1, 0x0, 0xC67C684D},
		{-1, -1, 0x0, 0xCF737785},
		{13, -7, 0x9E3779B9, 0x85F59F37},
		{-200, 143, 0x85EBCA6B, 0x5CD9FA5C},
		{math.MaxInt32, math.MinInt32, 0xC2B2AE35, 0x688BDB26},
	}
	for _, c := range cases {
		require.Equalf(t, c.want, splashHash(c.x, c.y, c.seed),
			"splashHash(%d, %d, 0x%X)", c.x, c.y, c.seed)
	}
}

// TestSplashHashDecorrelates guards the hash's fitness for lattice noise:
// neighboring lattice points and different seeds must produce unrelated
// values (a weak hash here shows up as visible grid artifacts in the field).
func TestSplashHashDecorrelates(t *testing.T) {
	seen := map[uint32]bool{}
	for x := int32(-64); x <= 64; x++ {
		for y := int32(-64); y <= 64; y++ {
			h := splashHash(x, y, 0x9E3779B9)
			require.Falsef(t, seen[h], "collision in a 129x129 neighborhood at (%d,%d)", x, y)
			seen[h] = true
		}
	}
	require.NotEqual(t, splashHash(3, 4, 1), splashHash(3, 4, 2), "seed must change the value")
}

// TestSplashValNoiseRange checks output stays in [0,1) over a spread of
// coordinates, including negatives and non-integer positions.
func TestSplashValNoiseRange(t *testing.T) {
	for i := 0; i < 500; i++ {
		x := (float64(i) - 250) * 0.377
		y := (float64(i%37) - 18) * 1.713
		n := splashValNoise(x, y, 0x27D4EB2F)
		require.GreaterOrEqualf(t, n, 0.0, "noise(%f,%f) below 0", x, y)
		require.Lessf(t, n, 1.0, "noise(%f,%f) at or above 1", x, y)
	}
}

// TestSplashValNoiseContinuity checks the smoothstep interpolation: a small
// step in the domain must produce a small step in the value (no jumps at
// lattice boundaries), which is what keeps the rendered gas free of seams.
func TestSplashValNoiseContinuity(t *testing.T) {
	const eps = 1e-4
	for i := 0; i < 400; i++ {
		// March across several lattice cells, deliberately crossing integers.
		x := -3.0 + float64(i)*0.017
		y := 2.5 - float64(i)*0.011
		a := splashValNoise(x, y, 0x165667B1)
		b := splashValNoise(x+eps, y+eps, 0x165667B1)
		require.InDeltaf(t, a, b, 0.01, "discontinuity near (%f,%f)", x, y)
	}
}

// TestSplashValNoiseAnchorsLattice pins the interpolation contract: at exact
// lattice points the noise equals the lattice value itself.
func TestSplashValNoiseAnchorsLattice(t *testing.T) {
	for _, p := range [][2]int32{{0, 0}, {3, -2}, {-7, 5}} {
		require.InDelta(t, latticeVal(p[0], p[1], 42),
			splashValNoise(float64(p[0]), float64(p[1]), 42), 1e-12)
	}
}

// splashTestVariants enumerates every variant for contract loops, with names
// for failure messages.
func splashTestVariants() map[string]splashVariant {
	return map[string]splashVariant{
		"legacy":  splashVariantLegacy,
		"fbm":     splashVariantFBM,
		"braille": splashVariantBraille,
		"flow":    splashVariantFlow,
		"julia":   splashVariantJulia,
		"mandala": splashVariantMandala,
	}
}

// TestSplashVariantsContract loops every variant through the core field
// contract: determinism, exact w×h bounds, frame-to-frame animation, and
// fully-blank first/last rows (the edge vignette's by-construction guarantee,
// which dithering and any Pass-1 processing must never break).
func TestSplashVariantsContract(t *testing.T) {
	pal := splashTestPalette()
	w, h := 80, 30
	for name, v := range splashTestVariants() {
		a := renderSplashField(w, h, 5, pal, centeredClearing(h, 20, 4), v)
		b := renderSplashField(w, h, 5, pal, centeredClearing(h, 20, 4), v)
		require.Equalf(t, a, b, "%s: same inputs must render identically", name)
		next := renderSplashField(w, h, 6, pal, centeredClearing(h, 20, 4), v)
		require.NotEqualf(t, a, next, "%s: consecutive frames must differ", name)

		lines := stripLines(a)
		require.Lenf(t, lines, h, "%s: line count", name)
		for i, l := range lines {
			require.Equalf(t, w, len([]rune(l)), "%s: line %d width", name, i)
		}
		require.Equalf(t, strings.Repeat(" ", w), lines[0], "%s: top row must be blank", name)
		require.Equalf(t, strings.Repeat(" ", w), lines[h-1], "%s: bottom row must be blank", name)
	}
}

// TestSplashDitherBlankFloor locks the invariant that makes dithering safe
// under the vignette: with ditherAmp < 1 glyph step, a fully-dark cell
// (lit = 0) must quantize to glyph 0 for every cell position, so dither can
// never light a blank border row.
func TestSplashDitherBlankFloor(t *testing.T) {
	require.Less(t, ditherAmp, 1.0, "ditherAmp must stay below one glyph step")
	maxGlyph := len([]rune(splashRamp)) - 1
	for row := 0; row < 60; row++ {
		for col := 0; col < 200; col++ {
			gf := 0*float64(maxGlyph) + (splashDither(col, row)-0.5)*ditherAmp
			require.Zerof(t, clampInt(int(gf), 0, maxGlyph),
				"dark cell at (%d,%d) must stay blank", col, row)
		}
	}
}

// TestSplashDitherDistribution sanity-checks the dither noise: values in
// [0,1), roughly centered (so dithering is unbiased — it breaks banding
// without brightening or darkening the field on average).
func TestSplashDitherDistribution(t *testing.T) {
	sum, n := 0.0, 0
	for row := 0; row < 40; row++ {
		for col := 0; col < 120; col++ {
			v := splashDither(col, row)
			require.GreaterOrEqual(t, v, 0.0)
			require.Less(t, v, 1.0)
			sum += v
			n++
		}
	}
	require.InDelta(t, 0.5, sum/float64(n), 0.02, "dither must be unbiased")
}

// TestSplashFBMAtRange checks the fBm field evaluator's output contract over
// a spread of positions and phases: raw value and hue helper both in [0,1].
func TestSplashFBMAtRange(t *testing.T) {
	for i := 0; i < 600; i++ {
		dx := (float64(i%60) - 30) * 1.37
		dy := (float64(i/60) - 5) * 2.9
		phase := float64(i%17) * 1.3
		v, q := splashFBMAt(dx, dy, phase)
		require.GreaterOrEqualf(t, v, 0.0, "val at (%f,%f,p%f)", dx, dy, phase)
		require.LessOrEqualf(t, v, 1.0, "val at (%f,%f,p%f)", dx, dy, phase)
		require.GreaterOrEqualf(t, q, 0.0, "qLen at (%f,%f,p%f)", dx, dy, phase)
		require.LessOrEqualf(t, q, 1.0, "qLen at (%f,%f,p%f)", dx, dy, phase)
	}
}

// TestRenderSplashFieldConcurrent guards the two-panes case (preview and
// terminal both render the splash) and any future buffer pooling: concurrent
// renders of the same frame must all be byte-identical.
func TestRenderSplashFieldConcurrent(t *testing.T) {
	pal := splashTestPalette()
	want := renderSplashField(80, 30, 9, pal, centeredClearing(30, 20, 4), splashDefaultVariant)
	var wg sync.WaitGroup
	mismatch := make(chan string, 8)
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 20; i++ {
				if got := renderSplashField(80, 30, 9, pal, centeredClearing(30, 20, 4), splashDefaultVariant); got != want {
					select {
					case mismatch <- "concurrent render diverged":
					default:
					}
					return
				}
			}
		}()
	}
	wg.Wait()
	close(mismatch)
	require.Empty(t, <-mismatch)
}

// TestBrailleMaskLayout pins the dot-bit table to the Unicode braille layout:
// dots 1,2,3,7 run down the left column, dots 4,5,6,8 down the right — the
// bit order is NOT linear in the grid, so this is worth locking.
func TestBrailleMaskLayout(t *testing.T) {
	require.Equal(t, uint8(0x01), brailleBit[0][0], "dot 1: top-left")
	require.Equal(t, uint8(0x08), brailleBit[0][1], "dot 4: top-right")
	require.Equal(t, uint8(0x02), brailleBit[1][0], "dot 2")
	require.Equal(t, uint8(0x10), brailleBit[1][1], "dot 5")
	require.Equal(t, uint8(0x04), brailleBit[2][0], "dot 3")
	require.Equal(t, uint8(0x20), brailleBit[2][1], "dot 6")
	require.Equal(t, uint8(0x40), brailleBit[3][0], "dot 7: bottom-left")
	require.Equal(t, uint8(0x80), brailleBit[3][1], "dot 8: bottom-right")
	var all uint8
	for _, row := range brailleBit {
		all |= row[0] | row[1]
	}
	require.Equal(t, uint8(0xFF), all, "the 8 bits must cover the full mask")
}

// TestBrailleGlyphsWidthOne asserts every braille pattern renders at terminal
// width 1 (the column-alignment invariant every splash glyph must satisfy).
func TestBrailleGlyphsWidthOne(t *testing.T) {
	for m := 1; m <= 0xFF; m++ {
		g := string(rune(0x2800) | rune(m))
		require.Equalf(t, 1, ansi.StringWidth(g), "braille mask %#x must be width 1", m)
	}
}

// TestBrailleHalftoneInvariants locks the halftone's two structural
// relations: a fully dark sub-cell can never fire a dot (strict comparison
// against non-negative dither), and the scale exceeds the band top so a cell
// leaving the band doesn't out-weigh the ramp glyph it hands over to.
func TestBrailleHalftoneInvariants(t *testing.T) {
	for i := 0; i < 2000; i++ {
		require.False(t, 0.0 > splashDither(i%97, i/97)*brailleHalftoneScale,
			"a dark sub-cell must never fire a dot")
	}
	require.Greater(t, float64(brailleHalftoneScale), float64(brailleBandHi),
		"band-top cells must not light all 8 dots")
}

// TestSplashBrailleVariantOutput drives the braille variant end to end: the
// faint band must actually produce braille runes, and bare U+2800 (which some
// fonts draw as eight hollow circles) must never be emitted.
func TestSplashBrailleVariantOutput(t *testing.T) {
	pal := splashTestPalette()
	sawBraille := false
	for frame := 0; frame < 8; frame++ {
		out := ansi.Strip(renderSplashField(80, 30, frame*7, pal, centeredClearing(30, 20, 4), splashVariantBraille))
		for _, r := range out {
			require.NotEqual(t, rune(0x2800), r, "bare U+2800 must never be emitted")
			if r > 0x2800 && r <= 0x28FF {
				sawBraille = true
			}
		}
	}
	require.True(t, sawBraille, "the faint band must produce braille dots")
}

// TestSplashFractalRange checks both fractal evaluators' output contract
// over a spread of positions and phases: raw value and hue helper in [0,1].
func TestSplashFractalRange(t *testing.T) {
	for name, at := range map[string]func(dx, dy, phase float64) (float64, float64){
		"julia":   splashJuliaAt,
		"mandala": splashMandalaAt,
	} {
		for i := 0; i < 600; i++ {
			dx := (float64(i%60) - 30) * 1.37
			dy := (float64(i/60) - 5) * 2.9
			phase := float64(i%23) * 1.7
			v, aux := at(dx, dy, phase)
			require.GreaterOrEqualf(t, v, 0.0, "%s val at (%f,%f,p%f)", name, dx, dy, phase)
			require.LessOrEqualf(t, v, 1.0, "%s val at (%f,%f,p%f)", name, dx, dy, phase)
			require.GreaterOrEqualf(t, aux, 0.0, "%s aux at (%f,%f,p%f)", name, dx, dy, phase)
			require.LessOrEqualf(t, aux, 1.0, "%s aux at (%f,%f,p%f)", name, dx, dy, phase)
		}
	}
}

// TestSplashBloom checks the bloom's structural properties: it only ever
// brightens (additive), it spreads a bright spike into its neighborhood, and
// it leaves an all-dark buffer untouched (nothing above the threshold).
func TestSplashBloom(t *testing.T) {
	const w, h = 11, 9
	dark := splashField{vals: make([]float64, w*h)}
	splashBloom(dark, w, h)
	for i, v := range dark.vals {
		require.Zerof(t, v, "dark cell %d must stay dark", i)
	}

	spike := splashField{vals: make([]float64, w*h)}
	center := (h/2)*w + w/2
	spike.vals[center] = 1.0
	before := append([]float64(nil), spike.vals...)
	splashBloom(spike, w, h)
	for i, v := range spike.vals {
		require.GreaterOrEqualf(t, v, before[i], "bloom must never darken (cell %d)", i)
		require.LessOrEqual(t, v, 1.0)
	}
	require.Greater(t, spike.vals[center-1], 0.0, "bloom must spread to the left neighbor")
	require.Greater(t, spike.vals[center+w], 0.0, "bloom must spread to the row below")
	require.Zero(t, spike.vals[0], "bloom must not reach the far corner")
}

// flowTestField builds a 5×5 buffer from a linear function of (col, row) and
// returns the glyph choice at the center cell.
func flowTestField(t *testing.T, f func(col, row int) float64) (rune, bool) {
	t.Helper()
	const w, h = 5, 5
	vals := make([]float64, w*h)
	for r := 0; r < h; r++ {
		for c := 0; c < w; c++ {
			vals[r*w+c] = f(c, r)
		}
	}
	return splashFlowGlyph(vals, w, h, 2, 2)
}

// TestSplashFlowGlyphBins pins the angle→glyph mapping on synthetic linear
// fields, including the two traps: the y-down→y-up flip (getting it wrong
// swaps ╱ and ╲) and the aspect-corrected diagonals (a screen diagonal is a
// 2-rows-per-2-cols line, not 1:1).
func TestSplashFlowGlyphBins(t *testing.T) {
	// f increases rightward → vertical iso-lines.
	g, ok := flowTestField(t, func(c, _ int) float64 { return 0.1 * float64(c) })
	require.True(t, ok)
	require.Equal(t, '│', g)

	// f increases downward → horizontal iso-lines.
	g, ok = flowTestField(t, func(_, r int) float64 { return 0.1 * float64(r) })
	require.True(t, ok)
	require.Equal(t, '─', g)

	// f = 0.1c − 0.2r: iso-lines run 2 cols right per row down — a rendered
	// ╲ diagonal (one row is two visual units tall).
	g, ok = flowTestField(t, func(c, r int) float64 { return 0.1*float64(c) - 0.2*float64(r) })
	require.True(t, ok)
	require.Equal(t, '╲', g)

	// Mirrored: f = 0.1c + 0.2r → ╱.
	g, ok = flowTestField(t, func(c, r int) float64 { return 0.1*float64(c) + 0.2*float64(r) })
	require.True(t, ok)
	require.Equal(t, '╱', g)

	// A flat field is direction-free: no glyph.
	_, ok = flowTestField(t, func(_, _ int) float64 { return 0.5 })
	require.False(t, ok, "flat field must not stroke a contour")
}

// TestSplashFlowVariantOutput drives the flow variant end to end: the
// contour band must actually stroke line glyphs.
func TestSplashFlowVariantOutput(t *testing.T) {
	pal := splashTestPalette()
	saw := false
	for frame := 0; frame < 8 && !saw; frame++ {
		out := ansi.Strip(renderSplashField(80, 30, frame*7, pal, centeredClearing(30, 20, 4), splashVariantFlow))
		saw = strings.ContainsAny(out, "─╱│╲")
	}
	require.True(t, saw, "the contour band must produce flow glyphs")
}

func BenchmarkSplashValNoise(b *testing.B) {
	var sink float64
	for i := 0; i < b.N; i++ {
		sink += splashValNoise(float64(i)*0.13, float64(i%97)*0.29, 0x9E3779B9)
	}
	_ = sink
}

// BenchmarkRenderSplashVariants tracks the ≤3ms/frame budget per variant at
// the reference 80×30 pane (checked manually; never a timed assertion).
func BenchmarkRenderSplashVariants(b *testing.B) {
	pal := splashTestPalette()
	for name, v := range splashTestVariants() {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = renderSplashField(80, 30, i, pal, centeredClearing(30, 20, 4), v)
			}
		})
	}
}
