package ui

import (
	"math"
	"testing"

	"github.com/muesli/termenv"
	"github.com/stretchr/testify/require"
)

// TestSplashWrapIdxFoldsNegatives pins the trap that makes the whole seam fix
// work. Go's % keeps the dividend's sign, so a bare iy % period returns a
// negative index above the focal row — and splashU32 deliberately keeps negative
// lattice coordinates distinct, so those cells would hash to a different field
// than their positive counterparts. The tunnel would still seam; the seam would
// just move off a = ±π. This is the same fold splashRotationIdx documents.
func TestSplashWrapIdxFoldsNegatives(t *testing.T) {
	const p = 8
	for v := int32(-3 * p); v <= 3*p; v++ {
		got := splashWrapIdx(v, p)
		require.GreaterOrEqualf(t, got, int32(0), "splashWrapIdx(%d, %d) must be non-negative", v, p)
		require.Lessf(t, got, int32(p), "splashWrapIdx(%d, %d) must be < period", v, p)
		// Congruent to v, which is what makes it a fold rather than a clamp.
		require.Zerof(t, (int64(got)-int64(v))%p, "splashWrapIdx(%d, %d) must stay congruent", v, p)
	}
	// The exact cases a naive v % p gets wrong.
	require.Equal(t, int32(7), splashWrapIdx(-1, 8))
	require.Equal(t, int32(0), splashWrapIdx(-8, 8))
	require.Equal(t, int32(1), splashWrapIdx(-7, 8))
}

// TestSplashValNoiseWrapYIsPeriodic is the property the angular seam rests on:
// one turn around the tunnel must land on exactly the lattice row it started
// from. Exact equality is deliberate — y and y+period share a fractional part,
// so every lattice read and both fades are bit-identical. An approximate
// assertion here would pass on a field that merely happens to be smooth.
func TestSplashValNoiseWrapYIsPeriodic(t *testing.T) {
	const (
		p    = int32(8)
		seed = uint32(0x1234567)
	)
	for _, x := range []float64{-3.7, -0.5, 0, 0.25, 1.5, 9.9} {
		for _, y := range []float64{-2.25, -0.5, 0, 0.5, 3.75, 7.5} {
			base := splashValNoiseWrapY(x, y, p, seed)
			for _, k := range []float64{1, 2, -1, -3} {
				got := splashValNoiseWrapY(x, y+k*float64(p), p, seed)
				require.Equalf(t, base, got,
					"splashValNoiseWrapY(%v, %v) must equal its value one period away (k=%v)", x, y, k)
			}
		}
	}
}

// TestSplashValNoiseWrapYClosesTheSeam is the geometric statement of the same
// fact, and the one that would actually be visible: approaching the wrap from
// below must arrive at the value the field starts with, so no radial tear shows
// down the tunnel at a = ±π.
//
// Mutation-tested: swapping splashValNoiseWrapY's body for plain splashValNoise
// fails this, and dropping the negative fold moves the failure to y ≈ 0 rather
// than removing it.
func TestSplashValNoiseWrapYClosesTheSeam(t *testing.T) {
	const (
		p    = int32(8)
		seed = uint32(0xC0FFEE)
		eps  = 1e-9
	)
	for _, x := range []float64{-2.5, -0.25, 0, 1.75, 4.5} {
		atZero := splashValNoiseWrapY(x, 0, p, seed)
		require.InDeltaf(t, atZero, splashValNoiseWrapY(x, float64(p)-eps, p, seed), 1e-6,
			"the field must be continuous arriving at the wrap from below (x=%v)", x)
		require.InDeltaf(t, atZero, splashValNoiseWrapY(x, -eps, p, seed), 1e-6,
			"the field must be continuous arriving at zero from below (x=%v)", x)
	}
}

// TestSplashValNoiseWrapYRange keeps the wrap inside the contract every field
// consumer assumes.
func TestSplashValNoiseWrapYRange(t *testing.T) {
	const p = int32(6)
	for i := -40; i <= 40; i++ {
		for j := -40; j <= 40; j++ {
			x, y := float64(i)*0.37, float64(j)*0.41
			got := splashValNoiseWrapY(x, y, p, 0xABCDEF)
			require.GreaterOrEqualf(t, got, 0.0, "wrapped noise at (%v,%v)", x, y)
			require.Lessf(t, got, 1.0, "wrapped noise at (%v,%v)", x, y)
			require.Falsef(t, math.IsNaN(got), "wrapped noise must never be NaN at (%v,%v)", x, y)
		}
	}
}

// TestSplashValNoiseWrapYMatchesUnwrappedInside proves the wrap only changes the
// field where it must. Away from any multiple of the period the wrapped and
// plain noises index the same lattice rows, so they must agree exactly — which
// is what makes "wrapping" a statement about the seam rather than a different
// field.
func TestSplashValNoiseWrapYMatchesUnwrappedInside(t *testing.T) {
	const (
		p    = int32(16)
		seed = uint32(0x5EED)
	)
	// y in [0, p-1) keeps both iy and iy+1 inside [0, p), where the fold is the
	// identity.
	for _, y := range []float64{0.5, 3.25, 9.9, 14.5} {
		for _, x := range []float64{-1.5, 0.25, 6.75} {
			require.Equalf(t, splashValNoise(x, y, seed), splashValNoiseWrapY(x, y, p, seed),
				"inside the period the wrap must be the identity (x=%v, y=%v)", x, y)
		}
	}
}

// tunnelTestAt is the tunnel built at its reference pane, so the scale factor is
// exactly 1 and the constants in the file are the constants under test. Every
// radius named in this file is in those units.
var tunnelTestAt = splashTunnelAtFor(tunRefD)

// tunnelPolarAt samples the tunnel at a polar position. Phase 0 is deliberate in
// every caller that uses it for geometry: sin(0) == 0, so the vanishing point's
// drift vanishes and the field is exactly centred on the focal point, which is
// what lets a test name an angle and mean it.
func tunnelPolarAt(r, theta, phase float64) (val, aux float64) {
	return tunnelTestAt(0, 0, r*math.Cos(theta), r*math.Sin(theta), phase)
}

// TestSplashTunnelAtRange holds the point-fn contract: both returns in [0,1].
// splashColorIdx's tunnel arm reads aux as a gradient position and Pass 2 reads
// val as a brightness, so an out-of-range value is a wrong colour rather than a
// crash — which is exactly the kind of thing that survives to a screenshot round.
func TestSplashTunnelAtRange(t *testing.T) {
	for _, phase := range []float64{0, 0.015, 1.7, 42.5} {
		for row := -40; row <= 40; row++ {
			for col := -60; col <= 60; col++ {
				dx, dy := float64(col), float64(row)*cellAspect
				val, aux := tunnelTestAt(col, row, dx, dy, phase)
				require.GreaterOrEqualf(t, val, 0.0, "val at (%v,%v) phase %v", dx, dy, phase)
				require.LessOrEqualf(t, val, 1.0, "val at (%v,%v) phase %v", dx, dy, phase)
				require.GreaterOrEqualf(t, aux, 0.0, "aux at (%v,%v) phase %v", dx, dy, phase)
				require.LessOrEqualf(t, aux, 1.0, "aux at (%v,%v) phase %v", dx, dy, phase)
			}
		}
	}
}

// TestSplashTunnelAtIsFiniteAtTheVanishingPoint is the T5 guard, and it is about
// portability rather than robustness. With u = K/r, the centre gives u = +Inf,
// and int32(+Inf) is implementation-defined in Go — amd64 yields MinInt32, arm64
// saturates — so the lattice index, and then a NaN through the lerp, would differ
// per architecture instead of failing anywhere. That breaks the cross-arch
// determinism the whole golden strategy rests on.
//
// Mutation-tested, and the first version of this test was vacuous: it also passed
// with the r clamp deleted, because math.Min(+Inf, tunUMax) is tunUMax and the u
// clamp was silently doing all the work. That is why there is no r clamp — it was
// dead code. Removing tunUMax's math.Min is what this now fails on.
func TestSplashTunnelAtIsFiniteAtTheVanishingPoint(t *testing.T) {
	for _, phase := range []float64{0, 0.9, 13.2} {
		for _, d := range []float64{0, 1e-12, 1e-9, 1e-6, 1e-3} {
			for _, p := range [][2]float64{{d, 0}, {0, d}, {d, d}, {-d, -d}} {
				val, aux := tunnelTestAt(0, 0, p[0], p[1], phase)
				require.Falsef(t, math.IsNaN(val), "val NaN at (%v,%v) phase %v", p[0], p[1], phase)
				require.Falsef(t, math.IsInf(val, 0), "val Inf at (%v,%v) phase %v", p[0], p[1], phase)
				require.Falsef(t, math.IsNaN(aux), "aux NaN at (%v,%v) phase %v", p[0], p[1], phase)
				require.Falsef(t, math.IsInf(aux, 0), "aux Inf at (%v,%v) phase %v", p[0], p[1], phase)
			}
		}
	}
}

// TestSplashTunnelClosesTheAngularSeam is the T3/T4 guard and the reason the
// tunnel has its own fBm at all. v jumps +P/2 → −P/2 at a = ±π, so any texture
// that is not exactly periodic in v tears a radial seam down the wall. The shared
// splashFBMBody fails this three ways: its ring term is not periodic in v, its
// per-octave rotation mixes the axes, and fbmLacun is detuned off 2.
//
// Sampled at phase 0 so the vanishing point's drift is exactly zero and a = ±π is
// exactly the −x ray.
func TestSplashTunnelClosesTheAngularSeam(t *testing.T) {
	const eps = 1e-7
	for _, r := range []float64{6, 12, 25, 40, 90} {
		above, _ := tunnelPolarAt(r, math.Pi-eps, 0)
		below, _ := tunnelPolarAt(r, -math.Pi+eps, 0)
		require.InDeltaf(t, above, below, 1e-4,
			"the wall must be continuous across a = ±π at r=%v (a radial seam is the whole T3/T4 failure)", r)
	}
}

// TestSplashTunnelHueClosesTheAngularSeam extends the same guard to the hue
// channel. aux is a function of depth alone, so it is seam-free by construction —
// this pins that, because a future aux that mixed in the angle would tear a
// colour seam the brightness tests could not see.
func TestSplashTunnelHueClosesTheAngularSeam(t *testing.T) {
	const eps = 1e-7
	for _, r := range []float64{6, 25, 90} {
		_, above := tunnelPolarAt(r, math.Pi-eps, 0)
		_, below := tunnelPolarAt(r, -math.Pi+eps, 0)
		require.InDeltaf(t, above, below, 1e-4, "hue must be continuous across a = ±π at r=%v", r)
	}
}

// tunnelWallVariance estimates the wall texture's variance around a ring by
// dividing out the fog, which depends only on r and so is constant along it. It
// is the wall, not val, that the mip acts on.
func tunnelWallVariance(t *testing.T, r float64) float64 {
	t.Helper()
	fog := r / (r + tunFogA)
	require.Greaterf(t, fog, 0.0, "fog must be positive at r=%v for this estimate to mean anything", r)
	const n = 720
	walls := make([]float64, 0, n)
	sum := 0.0
	for i := 0; i < n; i++ {
		val, _ := tunnelPolarAt(r, 2*math.Pi*float64(i)/n, 0)
		w := val / fog
		walls = append(walls, w)
		sum += w
	}
	mean := sum / float64(n)
	varSum := 0.0
	for _, w := range walls {
		varSum += (w - mean) * (w - mean)
	}
	return varSum / float64(n)
}

// TestSplashTunnelMipQuietsTheVanishingPoint is the T11 guard. |du/dr| = K/r²
// diverges toward the vanishing point, so the wall's sampling rate outruns the
// cell grid exactly where the eye is drawn and aliases into shimmering rings. The
// LOD fades the wall toward its mean as the rate diverges — a real mip (a
// filtered band function converges to its mean), not a fudge.
//
// Mutation-tested: pinning lod to 1 fails this.
func TestSplashTunnelMipQuietsTheVanishingPoint(t *testing.T) {
	near := tunnelWallVariance(t, 1.5)
	far := tunnelWallVariance(t, 34)
	require.Greaterf(t, far, 20*near,
		"the mip must flatten the wall toward its mean near the vanishing point "+
			"(near-variance %.6g vs far %.6g)", near, far)
}

// hueSpreadOver walks radially between two radii and reports how much of the
// gradient the hue channel sweeps. Radially, because hue is a function of depth
// alone: its aliasing shows up as r moves, never as theta does.
func hueSpreadOver(rLo, rHi float64) float64 {
	lo, hi := math.Inf(1), math.Inf(-1)
	const n = 4000
	for i := 0; i <= n; i++ {
		rr := rLo + (rHi-rLo)*float64(i)/n
		_, aux := tunnelPolarAt(rr, 0.7, 0)
		lo, hi = math.Min(lo, aux), math.Max(hi, aux)
	}
	return hi - lo
}

// TestSplashTunnelHueMipQuietsTheVanishingPoint covers the trap the brief and its
// appendix both miss: aux = splashTri(u*tunHueF) compresses hue bands without
// bound as r → 0, exactly as the wall texture does, and the wall's mip does
// nothing for a channel it does not touch. The fog blanks most of that region,
// but the band where fog is small and non-zero would alias into dim rainbow
// confetti. One lod, both channels.
//
// The sampled band is not arbitrary and the test is worthless outside it. Below
// r = tunDepthK/tunUMax the u clamp pins hue flat on its own, so a test there
// passes with the mip deleted — which is exactly what the first version of this
// test did. Above the radius where the lod saturates there is no mip left to
// observe. The band is derived from the constants rather than written down, so
// retuning either moves the probe or fails loudly instead of quietly hollowing
// this out.
func TestSplashTunnelHueMipQuietsTheVanishingPoint(t *testing.T) {
	const probeLo, probeHi = 9.0, 14.0
	uClampR := tunDepthK / tunUMax                    // below this, u is pinned
	mipR := math.Sqrt(tunLODC * tunDepthK * tunFreqU) // above this, lod == 1
	require.Lessf(t, uClampR, probeLo,
		"the u clamp (r<%.1f) must stay below the probe band, or hue is flat there for the wrong reason", uClampR)
	require.Greaterf(t, mipR, probeHi,
		"the mip must still be active (r<%.1f) across the probe band, or there is nothing to observe", mipR)

	// Across the band the depth term sweeps ~8 full gradient cycles, so an
	// unmipped hue would cover essentially the whole gradient.
	near := hueSpreadOver(probeLo, probeHi)
	require.Lessf(t, near, 0.35,
		"hue must stay near its mean where the bands compress (spread %.3f); "+
			"unmipped this sweeps the full gradient and reads as confetti", near)

	// And the mip must not be quietly killing hue everywhere: out where the lod
	// saturates, the full sweep has to survive or there are no colour rings.
	far := hueSpreadOver(40, 90)
	require.Greaterf(t, far, 0.8, "the far field must still sweep the gradient (spread %.3f)", far)
}

// TestSplashTunnelIsNotTheNebula is the only guard on the nastiest silent
// failure in the variant surface. splashFieldAt's switch falls through to
// splashFBMAt, so a tunnel registered in the enum, the rotation, the names, the
// ops and both test maps — but missing that one case — renders as the nebula
// while every other test in the package still passes. The contract loop only
// checks determinism, bounds and animation, all of which the nebula satisfies
// perfectly.
//
// It is asserted against the ops-applied render rather than the point function
// because that is the failure's shape: the field would be right and unreachable.
func TestSplashTunnelIsNotTheNebula(t *testing.T) {
	withColorProfile(t, termenv.TrueColor)
	const w, h = 80, 30
	clearing := splashClearing{wordCenterRow: (h - 1) / 2}
	tunnel := renderSplashField(w, h, 5, splashTestPalette(), clearing, splashVariantTunnel)
	nebula := renderSplashField(w, h, 5, splashTestPalette(), clearing, splashVariantFBM)
	require.NotEqual(t, nebula, tunnel,
		"tunnel must reach splashTunnelAt — an unregistered variant silently renders as the nebula")
}

// TestSplashTunnelRendersDepthAsLuminance is the Pass-2 half of
// TestSplashTunnelFogReadsAsDepth, and both are needed. It decodes the emitted
// SGR back to a luminance stop rather than recomputing the field, because Pass 1
// being right has never proved Pass 2 emits it: every rain brightness test
// asserted the layer table and was structurally blind to a real Pass-2 bug that
// shipped (ops.dimToRim declared and never read).
//
// The claim is the variant's reason to exist — distance must arrive at the eye as
// brightness — so it is asserted on the bytes a terminal would receive.
func TestSplashTunnelRendersDepthAsLuminance(t *testing.T) {
	withColorProfile(t, termenv.TrueColor)
	const w, h = 160, 44
	pal := splashTestPalette()
	stops, _ := shadeStopGrid(t, w, h, 5, pal, splashVariantTunnel)

	cx, cyFocal := float64(w-1)/2, float64((h-1)/2)
	// Mean luminance stop in a near band and a far band, by radius from the
	// vanishing point. Blank cells (-1) are excluded: they are the fog's black
	// core, and counting them as stop 0 would prove the claim by construction.
	bandMean := func(lo, hi float64) (float64, int) {
		sum, n := 0.0, 0
		for row := 0; row < h; row++ {
			for col := 0; col < w; col++ {
				dx, dy := float64(col)-cx, (float64(row)-cyFocal)*cellAspect
				d := math.Hypot(dx, dy)
				if d < lo || d >= hi || stops[row][col] < 0 {
					continue
				}
				sum += float64(stops[row][col])
				n++
			}
		}
		return sum / float64(n), n
	}
	near, nNear := bandMean(8, 16)
	far, nFar := bandMean(34, 60)
	require.Greaterf(t, nNear, 40, "the near band needs enough lit cells to mean anything (got %d)", nNear)
	require.Greaterf(t, nFar, 40, "the far band needs enough lit cells to mean anything (got %d)", nFar)
	require.Greaterf(t, far, near+1.5,
		"the rendered wall must brighten with distance from the vanishing point "+
			"(near band mean stop %.2f over %d cells, far band %.2f over %d)", near, nNear, far, nFar)
}

// TestSplashTunnelFogReadsAsDepth is the design's core claim, stated as a
// property: brightness must fall toward the vanishing point. fog = r/(r+A) is
// classic z-fog (1/(1+z/D) with z = K/r) and reaches exactly 0, so the centre
// goes black regardless of the envelope. This asserts the field; the rendered
// counterpart lives in TestSplashTunnelRendersDepthAsLuminance, and both are
// needed — Pass 1 being right has never proved Pass 2 emits it.
func TestSplashTunnelFogReadsAsDepth(t *testing.T) {
	meanAt := func(r float64) float64 {
		const n = 360
		sum := 0.0
		for i := 0; i < n; i++ {
			val, _ := tunnelPolarAt(r, 2*math.Pi*float64(i)/n, 0)
			sum += val
		}
		return sum / n
	}
	prev := meanAt(1)
	for _, r := range []float64{3, 6, 12, 24, 48} {
		cur := meanAt(r)
		require.Greaterf(t, cur, prev, "mean brightness must rise with distance from the vanishing point (r=%v)", r)
		prev = cur
	}
}
