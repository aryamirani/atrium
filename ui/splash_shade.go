package ui

// The luminance channel: a second axis the field can shade along, so brightness
// and glyph identity stop being one instruction.
//
// The palette cannot carry brightness. splashAnchors is chosen hue-adjacent so
// HCL blending never backtracks muddy, which lands all four anchors inside
// L* 65-80 — it can say what colour a cell is, never how bright. Brightness is
// therefore carried entirely by the glyph density ramp, which means a dim cell is
// necessarily a *small* one: the faint end of every field degenerates into "."
// and "·", and a scatter of dots is confetti rather than dimming.
//
// This file owns the split (splashShade) and the hue x luminance grid it indexes.
// Rain's own ramp is deliberately NOT folded in here — see buildShadeGrid.

import (
	"math"

	"github.com/ZviBaratz/atrium/ui/theme"

	"github.com/charmbracelet/lipgloss"
	colorful "github.com/lucasb-eyer/go-colorful"
)

// splashLumStops is the number of luminance steps each hue is shaded across. It
// matches splashRainStops only by coincidence of taste — the two axes are
// independent, and this one is free to grow (see buildShadeGrid) without touching
// rain.
const splashLumStops = 16

// splashShade splits a cell's brightness between the density channel (which glyph)
// and the luminance channel (how bright its colour is).
//
// dens*lum == lit for every lumRange: the split MOVES brightness between the two
// channels, it never adds any. That is what keeps an opted-in variant as bright
// overall as it was before, so lumRange tunes *how* a field shades rather than how
// much of it there is.
//
// The two endpoints are exact and transcendental-free, and that is load-bearing
// twice over. They are the shading every shipped variant already uses — lumRange 0
// is the density ramp, lumRange 1 is rain — so reproducing them by construction,
// rather than by a float landing where it should, is what makes "byte-identical
// until opted in" a property instead of a hope. It also means nothing shipped pays
// for a channel it does not use.
//
// The interior is a gamma split: dens = lit^(1-lumRange), lum = lit^lumRange. It
// is written as one Log and one Exp rather than two math.Pow because each Pow does
// its own Frexp/Log/Exp internally — this is half the work, and recovering dens by
// division makes the product identity true to ~1 ulp by construction rather than
// as a claim about two independently-rounded results.
//
// Gamma rather than a linear split (lum = 1-r(1-lit)) because of the shape at the
// faint end, which is the whole point: gamma's density has an infinite slope at 0
// (lit^(1-r)), so it keeps lifting however faint the cell gets, while a linear
// split's is O(lit) and collapses to a dot anyway — just later.
func splashShade(lit, lumRange float64) (dens, lumT float64) {
	switch {
	case lumRange <= 0:
		return lit, 1 // density-only: today's shading, untouched
	case lumRange >= 1:
		return 1, lit // luminance-only: a constant-weight glyph, as rain wants
	case lit <= 0:
		// Load-bearing, not defensive: Log(0) is -Inf, so the division below would
		// be 0/0 = NaN, and int(NaN) is implementation-defined in Go (amd64 gives
		// minint, arm64 gives 0) — it would differ per architecture rather than
		// fail anywhere.
		return 0, 0
	default:
		lumT = math.Exp(lumRange * math.Log(lit))
		return lit / lumT, lumT
	}
}

// shadeAt resolves a cell's hue to the run index the emitter should open, and
// reports whether the cell carries any ink at all.
//
// At lumRange 0 it returns the plain gradient index and never false — the shade
// grid is not consulted, no arithmetic runs, and the emitted bytes are the ones
// every variant produced before this channel existed. That short-circuit is both
// the performance guard and the byte-identity guarantee, which is why it is one
// branch rather than a multiply that happens to come out to the top stop.
//
// The false is the luminance gate, and at lumRange > 0 it is load-bearing in a way
// worth naming: it, not ditherAmp < 1, is what keeps a dark cell blank. A lifted
// density reaches glyph 2 or 3 even at lit ≈ 0, so the density ramp alone would
// paint the vignette's edges; what blanks them is that stop 0 is near-black ink on
// a dark pane and never worth emitting.
func shadeAt(hue int, lumT float64, ops splashOps, lut *splashLUT) (int, bool) {
	if ops.lumRange == 0 {
		return hue, true
	}
	l := clampInt(int(lumT*float64(splashLumStops-1)), 0, splashLumStops-1)
	if l == 0 {
		return -1, false
	}
	return lut.shadeIndex() + hue*splashLumStops + l, true
}

// splashLumHexAt is the luminance curve, and it is the piece rain's ramp and the
// shade grid genuinely share: hold the hue, walk L* from a near-black floor up to
// the colour itself at u == 1, and let chroma fall alongside it.
//
// Chroma falling with luminance is a gamut concession rather than a look: a dim
// cell cannot hold its hue's full chroma in sRGB. It does mean the bottom of every
// column desaturates toward grey (8% of the hue's chroma at u=0.08, 33% at 0.33) —
// measurably more than the gamut alone requires, so this is the first knob to
// reach for if a shaded field's dim end reads grey rather than dark.
//
// The floor is deliberately not black. A cell that dim renders blank (see the
// l == 0 gate in Pass 2), so the floor is never itself emitted — what it does is
// anchor the low end's shape, and so set how dark the dimmest stop that *does*
// render is. That one has to stay off true black: terminals with a
// minimum-contrast feature rewrite true black to something legible, which would
// speckle a field with the very artifact this curve removes.
func splashLumHexAt(base colorful.Color, u float64) string {
	hue, chroma, lum := base.Hcl()
	return colorful.Hcl(hue, chroma*u, lum*(rainRampFloor+(1-rainRampFloor)*u)).Clamped().Hex()
}

// splashShadeParse resolves a gradient stop to a colour the curve can walk down,
// degrading exactly as buildSplashLUT and rainRampHexAt do rather than emitting
// broken colour for an unusual theme.
func splashShadeParse(c lipgloss.Color) colorful.Color {
	cc, err := colorful.Hex(string(c))
	if err != nil {
		return colorful.Color{R: 0.49, G: 0.81, B: 1}
	}
	return cc
}

// splashShadeHexAt is hue h's colour at luminance stop l. Split out from
// buildShadeGrid for the same reason rainRampHexAt is split out of buildRainRamp:
// the property that matters is that a column climbs in *luminance*, and asserting
// that on colours is honest where parsing it back out of an SGR affix is not.
//
// Note the top stop is recomputed here and *pinned* in buildShadeGrid. The two
// agree to within an HCL round-trip, which is precisely the discrepancy the pin
// exists to remove — so this is the shape of the axis, and the grid is what
// renders.
func splashShadeHexAt(pal theme.Palette, h, l int) string {
	lut := splashGradientColors(pal)
	return splashLumHexAt(splashShadeParse(lut[h]), float64(l)/float64(splashLumStops-1))
}

// buildShadeGrid builds the hue x luminance grid: every gradient stop, shaded down
// its own luminance axis. Flat rather than [][]splashAffix so the emitter's clamp
// stays one expression and the whole grid is one allocation.
//
// It is deliberately NOT rain's ramp generalized, and the difference is the design.
// Rain's axis has to reach pal.Fg, because a head reads as a head only by the step
// down to the cell behind it. This one has to stop at the hue, because a field
// whose colour is the point cannot have its brightest cells blow out to white. And
// the two cannot be one axis: the climb from an anchor to Fg is 2.1 L* from Cyan
// but 16.4 from Danger, so a shared blow-out would be an 8x stronger gesture at the
// warm end of the hue axis than at the cool end. Two tables, one curve.
//
// Being headless is also what buys the resolution: all 15 steps go to the tail
// rather than 12, so the axis steps ~5.0 L* instead of rain's 6.1.
func buildShadeGrid(colors []lipgloss.Color, affix []splashAffix) []splashAffix {
	grid := make([]splashAffix, len(colors)*splashLumStops)
	top := splashLumStops - 1
	for h, c := range colors {
		base := splashShadeParse(c)
		for l := 0; l < top; l++ {
			hex := splashLumHexAt(base, float64(l)/float64(top))
			grid[h*splashLumStops+l] = splashAffixFor(lipgloss.NewStyle().Foreground(lipgloss.Color(hex)))
		}
		// Pin the top exactly, as buildSplashLUT pins its gradient endpoints: at
		// u == 1 the curve returns the hue, but by way of an HCL round-trip that can
		// nudge the hex a digit. A fully-lit shaded cell has to be the same colour
		// as an unshaded one, not a hair off it.
		grid[h*splashLumStops+top] = affix[h]
	}
	return grid
}
