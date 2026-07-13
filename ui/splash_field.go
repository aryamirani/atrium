package ui

// The splash field generator: deterministic noise primitives and the two-pass
// renderer built on them. Pass 1 evaluates the raw (pre-contrast,
// pre-envelope) scalar field into a buffer; Pass 2 applies contrast, the
// envelopes (edge vignette, radial dim, breathing), glyph and color
// quantization, the starfield, and emits run-coalesced ANSI. Everything is
// free of time/rand dependence so renderSplashField stays pure and
// snapshot-testable — animation enters only through the frame counter.
//
// The scene composition (wordmark/message overlay, clearing, gradient LUT)
// lives in splash.go; this file owns the field math and the per-cell loops.

import (
	"math"
	"os"
	"strings"
	"sync"

	"github.com/ZviBaratz/atrium/ui/theme"
)

// splashVariant selects the field generator + glyph technique. The non-legacy
// variants land behind the dev-only ATRIUM_SPLASH_VARIANT switch so they can
// be screenshot-compared live; the losers (and this type, if only one
// survives) are removed once a winner is picked.
type splashVariant int

const (
	// splashVariantLegacy is the PR #314 sum-of-sines plasma — the comparison
	// baseline while the noise-based variants are tuned.
	splashVariantLegacy splashVariant = iota
)

// splashDefaultVariant is what production renders when no override is set.
const splashDefaultVariant = splashVariantLegacy

// splashActiveVariant resolves the variant once per process: the dev-only
// ATRIUM_SPLASH_VARIANT env override, else the default. Only splashScene
// consults it — renderSplashField takes the variant as a parameter and stays
// pure over its inputs.
var splashActiveVariant = sync.OnceValue(func() splashVariant {
	switch os.Getenv("ATRIUM_SPLASH_VARIANT") {
	case "legacy":
		return splashVariantLegacy
	}
	return splashDefaultVariant
})

// splashHash is a deterministic 32-bit lattice hash: seed, y, and x are folded
// in through *nested* avalanche passes (lowbias32-style mixer at each level).
// Nesting matters: folding the coordinates linearly (x*A ^ y*B) collides on
// every point-symmetric pair (x,y)/(-x,-y), which would mirror the noise
// around the field's origin — the wordmark center. Being pure integer math it
// is exact on every architecture, unlike the classic sin-fract hash, whose
// platform-dependent sin precision can make hash-derived features differ
// across builds.
func splashHash(x, y int32, seed uint32) uint32 {
	return lowbias32(splashU32(x) ^ lowbias32(splashU32(y)^lowbias32(seed^0x9E3779B9)))
}

// lowbias32 is Chris Wellons' low-bias 32-bit avalanche mixer: every input bit
// flips ~half the output bits, in two multiplies and three shifts.
func lowbias32(h uint32) uint32 {
	h ^= h >> 16
	h *= 0x7FEB352D
	h ^= h >> 15
	h *= 0x846CA68B
	h ^= h >> 16
	return h
}

// splashU32 reinterprets a signed lattice coordinate as its two's-complement
// bit pattern for hashing; the value change on negatives is the point (it
// keeps negative coordinates distinct without branching).
func splashU32(v int32) uint32 { return uint32(v) } //nolint:gosec // G115: intentional bit reinterpretation for hashing

// latticeVal maps the lattice hash to a float in [0,1).
func latticeVal(x, y int32, seed uint32) float64 {
	return float64(splashHash(x, y, seed)) * (1.0 / 4294967296.0)
}

// splashValNoise is bilinear value noise with a smoothstep fade: continuous,
// deterministic, in [0,1). Frequency is applied by the caller (pass x*freq).
// Different seeds give statistically independent fields, which is how the
// warp vector and each fBm octave decorrelate.
func splashValNoise(x, y float64, seed uint32) float64 {
	xi, yi := math.Floor(x), math.Floor(y)
	xf, yf := x-xi, y-yi
	u := xf * xf * (3 - 2*xf)
	v := yf * yf * (3 - 2*yf)
	ix, iy := int32(xi), int32(yi)
	return splashLerp(
		splashLerp(latticeVal(ix, iy, seed), latticeVal(ix+1, iy, seed), u),
		splashLerp(latticeVal(ix, iy+1, seed), latticeVal(ix+1, iy+1, seed), u),
		v)
}

func splashLerp(a, b, t float64) float64 { return a + (b-a)*t }

// splashField is Pass 1's output: the raw scalar field (pre-contrast,
// pre-envelope, in [0,1]) plus a variant-defined hue helper per cell. For the
// legacy field the helper is the warped-coordinate angle its color swirl has
// always used; noise variants will carry their warp-vector magnitude instead
// (the "free" nebula-coloring byproduct).
type splashField struct {
	vals []float64
	aux  []float64
}

// splashEvalField runs Pass 1: the raw field for every cell, in focal-relative
// aspect-corrected coordinates (dx, dy) — the same frame the envelopes and
// color gradient use in Pass 2. Buffers are per-call allocations; at splash
// sizes (19 KB at 80×30) that is cheaper than any pooling would be worth.
func splashEvalField(w, h int, cx, cyFocal, phase float64, v splashVariant) splashField {
	f := splashField{vals: make([]float64, w*h), aux: make([]float64, w*h)}
	i := 0
	for row := 0; row < h; row++ {
		dy := (float64(row) - cyFocal) * cellAspect
		for col := 0; col < w; col++ {
			dx := float64(col) - cx
			f.vals[i], f.aux[i] = splashLegacyAt(dx, dy, phase)
			i++
		}
	}
	return f
}

// splashLegacyAt is the PR #314 sum-of-sines plasma, evaluated at one point:
// two domain-warped ring octaves + rotationally-symmetric petals + an
// isotropic fine texture (three plane waves 120° apart, so no diagonal
// grain). Returns the raw value and the warped angle (its swirl input).
func splashLegacyAt(dx, dy, phase float64) (val, theta float64) {
	wx := dx + rippleWarp*math.Sin(dy*rippleWarpF-phase*0.4)
	wy := dy + rippleWarp*math.Sin(dx*rippleWarpF-phase*0.4)
	d := math.Hypot(wx, wy)
	theta = math.Atan2(wy, wx)
	tex := math.Sin(dx*isoFreq-phase*isoSpeed) +
		math.Sin((dx*iso1Cos+dy*iso1Sin)*isoFreq-phase*isoSpeed) +
		math.Sin((dx*iso2Cos+dy*iso2Sin)*isoFreq-phase*isoSpeed)
	v := math.Sin(d*rippleFreq1-phase) +
		0.55*math.Sin(d*rippleFreq2-phase*0.7) +
		0.40*math.Sin(d*rippleFreq3-phase*0.5)*math.Cos(theta*petalCount) +
		isoWeight*tex
	return clamp01((v/rippleAmp + 1) * 0.5), theta
}

// renderSplashField builds the colored plasma background: exactly h rows of
// exactly w visible cells, with the clearing ellipses blanked out for the
// composited text. The field fills the whole pane and softens only near the
// four borders (an edge vignette), rather than being a single disc inscribed
// to the shorter axis. The pattern emanates from the wordmark's center
// (clearing.wordCenterRow) and the color gradient / gentle radial dim are
// normalized to the farthest corner, so the field stays visually anchored on
// the wordmark while still reaching the edges. Pure over its inputs
// (deterministic, snapshot-testable); returns "" on a degenerate pane.
func renderSplashField(w, h, frame int, pal theme.Palette, clearing splashClearing, variant splashVariant) string {
	if w <= 0 || h <= 0 {
		return ""
	}
	cx := float64(w-1) / 2
	cyFocal := float64(clearing.wordCenterRow)
	// Distance from the focal point to the farthest corner: the denominator for
	// the color gradient and the core→rim dim, so both span the whole pane.
	maxD := math.Hypot(
		math.Max(cx, float64(w-1)-cx),
		math.Max(cyFocal, float64(h-1)-cyFocal)*cellAspect)
	if maxD <= 0 {
		return ""
	}
	// Border-fade reach in cells (min 1 so a tiny pane still fades, never /0).
	marginX := math.Max(1, float64(w)*edgeVignetteFrac)
	marginY := math.Max(1, float64(h)*edgeVignetteFrac)
	phase := float64(frame) * driftPerFrame

	fld := splashEvalField(w, h, cx, cyFocal, phase, variant)

	lut := splashLUTFor(pal)
	nColors := len(lut.styles)
	starIdx := nColors // flushSplashRun renders any index >= len(styles) as a star
	ramp := []rune(splashRamp)
	maxGlyph := len(ramp) - 1
	starRampR := []rune(starRamp)
	starMax := len(starRampR) - 1
	// Slow global brightness swell (breathing), computed once per frame.
	breathe := 1 - breatheDepth*(0.5-0.5*math.Sin(phase*breatheSpeed))

	var sb strings.Builder
	var run strings.Builder
	for row := 0; row < h; row++ {
		if row > 0 {
			sb.WriteByte('\n')
		}
		dy := (float64(row) - cyFocal) * cellAspect
		// edgeY is exactly 0 on the first/last rows, and the envelope multiplies
		// the field *after* any Pass-1 processing — that construction (not
		// tuning) is what keeps the border rows blank.
		edgeY := smoothstep(0, 1, clamp01(math.Min(float64(row), float64(h-1-row))/marginY))
		curIdx := -1 // -1 marks a blank (uncolored) run
		for col := 0; col < w; col++ {
			dx := float64(col) - cx
			idx, ch := -1, ' '

			if edgeY > 0 && !clearing.blanks(dx, row) {
				cell := row*w + col
				dRaw := math.Hypot(dx, dy)
				// Contrast: push mid-tones apart so bright ridges read as
				// filaments against darker voids.
				intensity := smoothstep(splashContrastLo, splashContrastHi, fld.vals[cell])
				edgeX := smoothstep(0, 1, clamp01(math.Min(float64(col), float64(w-1-col))/marginX))
				radial := 1 - radialDim*clamp01(dRaw/maxD)
				lit := intensity * edgeX * edgeY * radial * breathe
				if g := clampInt(int(lit*float64(maxGlyph)), 0, maxGlyph); g > 0 {
					ch = ramp[g]
					// Hue swirls across the field (radius + a slow angular sweep)
					// so the gradient reads as a drifting multi-hued nebula.
					swirl := 0.5 + 0.5*math.Sin(fld.aux[cell]+dRaw*colorSwirlF-phase*colorSwirlSpeed)
					colorT := clamp01(colorRadialMix*(dRaw/maxD) + (1-colorRadialMix)*swirl)
					idx = clampInt(int(colorT*float64(nColors-1)), 0, nColors-1)
				}
				// Starfield on top: a fixed, twinkling point can light even a void
				// the plasma left dark. Fades with the same border vignette.
				if sh := starHash(col, row); sh > starThreshold {
					tw := 0.7 + 0.3*math.Sin(phase*starTwinkleSpeed+sh*starPhaseScatter)
					if sg := clampInt(int(tw*edgeX*edgeY*float64(starMax)), 0, starMax); sg > 0 {
						ch = starRampR[sg]
						idx = starIdx
					}
				}
			}

			if idx != curIdx {
				flushSplashRun(&sb, &run, curIdx, lut)
				curIdx = idx
			}
			run.WriteRune(ch)
		}
		flushSplashRun(&sb, &run, curIdx, lut)
	}
	return sb.String()
}
