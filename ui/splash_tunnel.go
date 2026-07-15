package ui

import "math"

// The tunnel is the roster's depth variant: a textured wall flying past a
// vanishing point that sits on the wordmark. Its geometry is the classic
// demoscene mapping — screen position → (depth, angle) = (K/r, atan2(y,x)) —
// which turns a plain 2D texture lookup into an infinite corridor.
//
// Three of its constants encode traps rather than taste, and are commented where
// they are used: u is clamped (int32(+Inf) is implementation-defined in Go), the
// angular axis wraps on an exact power-of-two period (a detuned lacunarity would
// tear the seam), and a sampling-rate LOD fades the wall toward its mean near the
// centre (|du/dr| = K/r² diverges there).
const (
	// tunDepthK scales depth: u = K/r. With tunFreqU it sets the only thing about
	// this variant that has to be right — how far apart the rings land on screen.
	//
	// One texture cell spans r²/(tunDepthK*tunFreqU) screen cells, so the product
	// is the knob and the two constants are one decision. Perspective guarantees
	// that spacing varies as r², i.e. by 10x across a pane, so no product makes
	// rings ideal everywhere; the choice is which radius they read at. Measured at
	// the product 72 they were legible only around r=10–30 while the pane runs to
	// r≈90, and the outer 80% of the view — nearly all of the cells — was a single
	// stretched texture cell, which rendered as coloured haze with no tunnel in it.
	// At 400 the rings read from the mip boundary out to the corners.
	tunDepthK = 200.0
	// tunUMax is the single guard on the singularity at the vanishing point, and
	// it does two jobs at once. It caps how far the texture may compress —
	// without it, near-centre cells sample lattice points hundreds apart and
	// alias into a boiling white-noise storm that moves with the vanishing point.
	// And because math.Min(+Inf, c) == c, the same clamp is what keeps r == 0
	// from reaching int32(+Inf), which is implementation-defined in Go (amd64
	// gives MinInt32, arm64 saturates) and would differ per architecture rather
	// than fail anywhere.
	//
	// A separate r clamp is the obvious companion and would be dead code: it
	// could only bind if tunDepthK/rMin < tunUMax, and it must not, or the
	// texture would alias in exactly the band this cap exists to quiet. The
	// clamp has to live on u, where the aliasing does. Keep the math.Min form —
	// dividing first and clamping after is what makes the +Inf harmless.
	tunUMax = 25.0
	// tunFogA is the z-fog half-distance: fog = r/(r+A) passes 0.5 at r == A, so
	// A is the half-lit radius in aspect-corrected cells. It is large because
	// screen area grows as r²: most of the pane is at large r, so a small A puts
	// nearly every cell in the saturated bright end of the hyperbola and leaves no
	// gradient across the region that dominates the view. Measured at A=7 the fog
	// only ran 0.53→0.92 from r=8 to r=80, which rendered as a flat dim wash.
	tunFogA = 30.0
	// tunFogGain is exposure, and it is what makes the fog usable on a pane rather
	// than on a number line. r/(r+A) is a hyperbola: it cannot be both dark at the
	// core and bright at the rim, because the same A sets both ends. The gain
	// saturates it before the rim, which is the physically right reading — past
	// some distance the wall is simply fully lit — while leaving the property that
	// actually matters untouched, since anything times 0 is still 0 and the centre
	// stays exactly black.
	tunFogGain = 1.35
	// tunWallLo/tunWallHi open the fBm's middle. Three octaves average toward
	// their mean, so the raw field spans roughly 0.2–0.8 and a wall built straight
	// from it has no contrast to fly past. This is the same trick the nebula's
	// contrast window plays, applied here rather than in ops — where it would hit
	// wall*fog and eat the depth along with the flatness.
	tunWallLo = 0.36
	tunWallHi = 0.64
	// tunWallFloor is the wall's unlit reflectance: the surface is lit, and its
	// texture modulates down from full rather than all the way to black. At 0 the
	// texture's dark patches would be as black as the far end of the corridor, and
	// brightness would stop meaning distance — which is the whole variant.
	tunWallFloor = 0.5
	// tunLODC is where the mip bites, in units of Nyquist — and it is derived
	// rather than dialled in, which is why it is not the free constant the brief
	// expected. A texture cell spans r²/(tunDepthK*tunFreqU) screen cells, and a
	// band-limited signal aliases once that falls below ~2, so the wall must start
	// fading toward its mean at spacing == tunLODC and be fully flat as spacing
	// reaches 0. Expressing the lod as spacing/tunLODC makes the constant mean
	// "fade in at N cells per ring" instead of naming an opaque radius, and keeps
	// it correct for free when the ring spacing is retuned.
	tunLODC = 2.5
	// tunFreqU is the wall texture's frequency along depth. It only ever acts on
	// screen through its product with tunDepthK — see there. The angular axis
	// deliberately has no frequency of its own — see splashTunnelFBM.
	tunFreqU = 2.0
	// tunHueF is how fast hue cycles with depth: one full gradient sweep every
	// 1/tunHueF units of u, which is what makes the corridor read as coloured
	// rings receding rather than as a single wash.
	tunHueF = 1.0

	// Motion. u carries the fly, a carries the roll, and the centre banks.
	tunFlySpd   = 1.4
	tunRotSpd   = 0.12
	tunHueSpd   = 0.18
	tunDriftX   = 3.0
	tunDriftY   = 1.5
	tunDriftSpX = 0.31
	tunDriftSpY = 0.23

	// tunRefD is the pane radius the length-scale constants above were tuned
	// against — maxD for a 160x44 pane. It is a unit, not a preference: every
	// scaled constant means "this many cells on a pane of this size", and the
	// scale factor is maxD/tunRefD. Retuning any of them means retuning them for
	// this reference pane, whatever pane you happen to be looking at.
	tunRefD = 96.0

	// tunWrapP is the angular lattice period: the number of lattice cells one
	// turn around the tunnel spans, and so the octave-0 spoke count. It must be
	// an integer (it is a lattice index period) and untyped so it can serve as
	// both the float64 divisor in v and the int32 period in the noise.
	tunWrapP = 8

	tunOctaves = 3
	tunGain    = 0.55
)

var (
	// tunSeedOct decorrelates the octaves' values.
	tunSeedOct = [tunOctaves]uint32{0x3B1E5F07, 0xA4D91C6B, 0x7E2F84D3}
	// tunLacU is the depth axis's lacunarity, kept detuned off 2 exactly as the
	// shared fBm does (IQ: avoids octave self-alignment). Only the depth axis may
	// be detuned — the angular axis must double exactly or the wrap tears.
	tunLacU = [tunOctaves - 1]float64{2.01, 2.02}
	// tunVOff offsets each octave along the angular axis. The angular lacunarity
	// is pinned to exactly 2, so the octaves would otherwise share lattice
	// alignment on every spoke and stack into one hard ribbing; a constant offset
	// decorrelates their lattice *positions* while preserving periodicity for
	// free. Seeds cannot do this — they decorrelate values, not positions.
	tunVOff = [tunOctaves]float64{0, 0.37, 0.71}
)

// splashTunnelFBM is the wall texture: a wrapped, rotation-free fBm over
// (depth, angle). It is deliberately not splashFBMBody, which is unusable here
// three times over — its ring term closes on math.Hypot(x,y) and is not periodic
// in v, its per-octave rotation mixes fx into fy and destroys the wrap outright,
// and its fbmLacun is detuned off 2 (octave 1 would span 2.01·P against a period
// of 2·P, so the seam survives and tears ~0.5% per octave).
//
// Dropping the rotation is a feature, not a concession. Value noise's
// axis-aligned lattice artifacts are the thing a rotation normally exists to
// hide; mapped through (u,v) = (depth, angle) they become concentric rings and
// radial spokes in screen space — which is to say, tunnel ribbing. We want them.
//
// No frequency multiplier on the angular axis, and this is load-bearing: v
// arrives already in lattice units (it spans exactly tunWrapP per turn), so any
// tunFreqV would make the per-turn span tunWrapP*tunFreqV — non-integer for
// almost every choice, at which point the lattice period and the angular period
// stop coinciding and the seam silently returns. Frequency belongs to u alone.
//
// It takes no phase: u already carries the fly, and each octave scales it, so the
// octaves parallax against each other for free — which is also the physically
// right answer, since a rigid wall flies past as one piece.
func splashTunnelFBM(u, v float64) float64 {
	sum, norm, amp := 0.0, 0.0, 1.0
	fu, fv := u*tunFreqU, v
	period := int32(tunWrapP)
	for o := 0; o < tunOctaves; o++ {
		sum += amp * splashValNoiseWrapY(fu, fv+tunVOff[o], period, tunSeedOct[o])
		norm += amp
		amp *= tunGain
		if o < tunOctaves-1 {
			fu *= tunLacU[o]
			// Exactly 2, matching the period's doubling: the angular axis is the
			// one that must tile.
			fv *= 2
			period *= 2
		}
	}
	return sum / norm
}

// splashTunnelAt maps a cell to the tunnel wall's brightness and its depth-banded
// hue.
//
// val is wall*fog, and the product is the physically right one: a far wall is
// both fogged and dim. aux is depth alone, which splashColorIdx's tunnel arm
// spends directly as a gradient position — hue says which ring, never how near,
// because on this palette (hue-adjacent by construction, all four anchors inside
// L* 65–80) hue cannot encode distance at all. Luminance is the only cue the eye
// reads as depth, which is why the fog rides ops.lumRange.
//
// The vanishing point sits on the wordmark for free: dx and dy are already
// focal-relative, so ATRIUM ends up at the end of an infinite corridor with the
// fog's black core around it.
// It is built for one pane rather than being a plain function, because the
// tunnel is a single object and has to be the same object at every size — see
// splashFieldAt. Everything with a length scale is measured against maxD: the
// depth constant, so the rings land at the same fraction of the pane; the fog
// distance, so the black core stays ~18% of the radius; and the banking, so the
// corridor sways by the same proportion rather than by a fixed number of cells.
//
// tunUMax deliberately does not scale — u = K/r with K ∝ maxD and r ∝ maxD makes
// u scale-invariant already. Nor does the lod: Nyquist is an absolute fact about
// cells, and expressing the mip in cells means a larger pane simply resolves more
// of the wall, which is true rather than convenient.
func splashTunnelAtFor(maxD float64) splashPointFn {
	s := maxD / tunRefD
	if s <= 0 {
		// renderSplashField already returns early on a degenerate pane; this only
		// keeps a direct caller from dividing by zero below.
		s = 1
	}
	k := tunDepthK * s
	fogA := tunFogA * s
	driftX, driftY := tunDriftX*s, tunDriftY*s

	return func(_, _ int, dx, dy, phase float64) (val, aux float64) {
		// The centre banks on two detuned sines, so the corridor never reads as a
		// fixed hole punched in the pane.
		x := dx - driftX*math.Sin(phase*tunDriftSpX)
		y := dy - driftY*math.Sin(phase*tunDriftSpY)

		// r is deliberately unclamped: at the exact centre it is 0, and every use
		// below is total there — atan2(0,0) is 0, fog is 0, the lod is 0, and the
		// division's +Inf dies in tunUMax's math.Min. See tunUMax.
		r := math.Hypot(x, y)
		a := math.Atan2(y, x) + phase*tunRotSpd
		u := math.Min(k/r, tunUMax) + phase*tunFlySpd
		v := a * tunWrapP / (2 * math.Pi)

		// The mip. |du/dr| = K/r², so the wall's sampling rate diverges toward the
		// vanishing point — precisely where the eye is drawn — and would alias into
		// shimmering rings. Fading a band-limited function toward its mean as its
		// rate outruns the grid is what a mip does; 0.5 is this texture's mean.
		//
		// It is anisotropic because the grid is. A column step moves dx by 1 but a
		// row step moves dy by cellAspect, so the vertical axis samples the wall at
		// half the horizontal rate and aliases at twice the radius. An isotropic mip
		// tuned for the horizontal rate therefore leaves a band around the vertical
		// axis — the top and bottom of the pane — 1.6-2x over Nyquist, where the
		// rings crawl (wagon-wheel) instead of flowing outward. Measured before the
		// fix at 240x60: 0.41 cycles/step horizontally at r=37 against 0.81
		// vertically, and still 0.55 vertically out at r=45.
		//
		// step is the largest |dr| one cell step can cover here, so spacing —
		// r³/(K*freq*step) — is cells-per-ring along whichever screen axis is
		// currently worst. On the horizontal axis step == r and it reduces to the
		// isotropic r²/(K*freq); on the vertical it is 2r and the mip bites a factor
		// sqrt(cellAspect) further out, which is exactly the radius that axis can
		// actually resolve.
		step := math.Max(math.Abs(x), cellAspect*math.Abs(y))
		lod := 0.0
		if step > 0 {
			// step is 0 only at the exact centre, where the wall is fully mipped
			// anyway and the fog has already taken the cell to black. Guarded rather
			// than epsilon'd because r³/step is 0/0 there, and NaN survives every
			// comparison below it.
			lod = clamp01(r * r * r / (tunLODC * k * tunFreqU * step))
		}
		tex := smoothstep(tunWallLo, tunWallHi, splashTunnelFBM(u, v))
		tex = 0.5 + lod*(tex-0.5)
		// A lit surface whose texture modulates it downward, never to black — see
		// tunWallFloor.
		wall := tunWallFloor + (1-tunWallFloor)*tex

		// Real z-fog: fog = r/(r+A) is 1/(1+z/D) with z = K/r, and it reaches
		// exactly 0, so the centre goes black on its own — no envelope term
		// required, which matters because the only one available (dimToRim) would
		// invert depth.
		fog := clamp01(tunFogGain * r / (r + fogA))

		// Hue takes the same lod, and that is not decoration: u = K/r compresses
		// the hue bands without bound toward the centre exactly as it does the
		// wall, and the wall's mip does nothing for a channel it does not touch.
		// The fog blanks most of that region, but the band where fog is small and
		// non-zero would alias into dim rainbow confetti. One mip, both channels.
		hue := 0.5 + lod*(splashTri(u*tunHueF+phase*tunHueSpd)-0.5)

		return clamp01(wall * fog), clamp01(hue)
	}
}

// splashWrapIdx folds a lattice index into [0, p). Go's % keeps the dividend's
// sign, so a bare v % p returns a negative index for every row above the focal
// point — and splashU32 keeps negative coordinates deliberately distinct, so
// those rows would hash to a different field than their positive counterparts.
// The tunnel would still seam; the seam would just move off a = ±π to a = 0,
// which is the same bug wearing a different angle. Same fold, same reason, as
// splashRotationIdx.
func splashWrapIdx(v, p int32) int32 { return ((v % p) + p) % p }

// splashValNoiseWrapY is splashValNoise made periodic on the y axis: the lattice
// row index is folded mod period, so the field tiles seamlessly every `period`
// units of y while x stays unbounded. It exists for the tunnel, whose y axis is
// an angle — one turn must arrive back where it started or a radial tear runs
// down the wall at a = ±π.
//
// Seamless under bilinear, by construction rather than by tuning. Approaching
// the wrap from below (y → P⁻) gives iy = P-1 and yf → 1, so the v fade selects
// the iy+1 row, which folds to row 0; arriving from above (y → P⁺) gives iy = 0
// and yf → 0, so the fade selects the iy row, which is also row 0. Same lattice
// values, same limit, no discontinuity.
//
// Two conditions, and missing either leaves a seam rather than an obvious break.
// The fold is applied to iy and iy+1 *independently after the increment* — a
// wrap(iy)+1 would step off the end of the period at the last row instead of
// returning to its start. And it goes through splashWrapIdx rather than a raw %
// for the sign reason documented there.
func splashValNoiseWrapY(x, y float64, period int32, seed uint32) float64 {
	xi, yi := math.Floor(x), math.Floor(y)
	xf, yf := x-xi, y-yi
	u := xf * xf * (3 - 2*xf)
	v := yf * yf * (3 - 2*yf)
	ix, iy := int32(xi), int32(yi)
	iy0 := splashWrapIdx(iy, period)
	iy1 := splashWrapIdx(iy+1, period)
	return splashLerp(
		splashLerp(latticeVal(ix, iy0, seed), latticeVal(ix+1, iy0, seed), u),
		splashLerp(latticeVal(ix, iy1, seed), latticeVal(ix+1, iy1, seed), u),
		v)
}
