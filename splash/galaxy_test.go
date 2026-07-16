package splash

import (
	"math"
	"testing"

	"github.com/muesli/termenv"
	"github.com/stretchr/testify/require"
)

// TestSplashGalaxyReachesItsOwnField is the same silent-failure guard the tunnel
// and ripple carry: splashFieldAt's switch falls through to the fallback, so a
// galaxy wired into the enum, rotation, names, ops and both test maps — but missing
// its one case in splashFieldAt — renders rain's field wearing galaxy's Pass-2
// policy, and every coverage loop is happy with that.
//
// It samples the point function, never two renders. Galaxy's ops differ from the
// fallback's on both fields (stars and lumRange), so two ops-applied renders differ
// whatever field is underneath them — the trap the tunnel's first version of this
// test fell into. Sampling rain is load-bearing: rain is what splashFieldAt's
// default arm returns, so this asks literally "am I getting the fallback's field".
// Move that arm elsewhere without moving this probe and the test passes while
// testing nothing.
func TestSplashGalaxyReachesItsOwnField(t *testing.T) {
	const phase = 5 * driftPerFrame
	sample := func(v Variant) []float64 {
		at := splashFieldAt(v, 96)
		out := make([]float64, 0, 2*21*31)
		for row := -10; row <= 10; row++ {
			for col := -15; col <= 15; col++ {
				val, aux := at(col, row, float64(col), float64(row)*cellAspect, phase)
				out = append(out, val, aux)
			}
		}
		return out
	}
	require.NotEqual(t, sample(Rain), sample(Galaxy),
		"galaxy must reach splashGalaxyAtFor — a variant with no case in splashFieldAt "+
			"silently falls through to the fallback's field")
}

// TestSplashGalaxyArmLODIsAnisotropic pins the one thing about the arm mip that is
// a trap rather than taste: it damps the vertical axis a factor vAspect harder than
// the horizontal, because a screen row covers vAspect in-plane units where a column
// covers one. vAspect is the grid's cellAspect times the inclination's 1/cos(galInc):
// a tilted disk foreshortens the vertical, packing the arms closer there. Get it
// isotropic and an arm crossing the top or bottom of the disk crawls (wagon-wheel)
// while the sides flow — the exposure the user caught in the tunnel twice, from
// motion alone and from no test.
//
// The property is asserted independently of the winding constants, so it survives
// the render round retuning the arm count or pitch. |∇ψ| has the same magnitude at
// every point of a given radius — it is sqrt(galWind²+galArms²)/r, rotation-free —
// so two points at one radius can be chosen where the phase gradient is purely
// horizontal (point A) and purely vertical (point B) with equal magnitude G. Then
// the only thing separating their LODs is the vAspect weight on the vertical term,
// so lod_A/lod_B is exactly vAspect. An isotropic mip makes it 1.
func TestSplashGalaxyArmLODIsAnisotropic(t *testing.T) {
	vAspect := cellAspect / math.Cos(galInc)
	d := math.Hypot(galWind, galArms) // |∇ψ|·r, the same at every point of radius r

	// A radius small enough that both LODs are below 1 (the regime where the ratio
	// is legible rather than clamped flat), derived from the constants so a retune
	// that pushes it out of that band fails here loudly instead of passing vacuously.
	r := 0.5 * galLODC * d / math.Pi

	// Point A: ∇ψ purely horizontal (its w-component vanishes on the (galWind,galArms)
	// ray). Point B: ∇ψ purely vertical (u-component vanishes on the (−galArms,galWind)
	// ray). Both at radius r, so both see |∇ψ| = d/r.
	ax, ay := r*galWind/d, r*galArms/d
	bx, by := -r*galArms/d, r*galWind/d

	lodA := splashGalaxyArmLOD(ax, ay, vAspect)
	lodB := splashGalaxyArmLOD(bx, by, vAspect)

	require.Greater(t, lodA, 0.0, "point A must not clamp to zero")
	require.Less(t, lodA, 1.0, "point A must be in the unclamped band, or the ratio is vacuous")
	require.Greater(t, lodB, 0.0, "point B must not clamp to zero")
	require.Less(t, lodB, 1.0, "point B must be in the unclamped band, or the ratio is vacuous")

	// The vertical gradient is damped vAspect harder. Reintroducing an isotropic step
	// (dropping the vAspect factor) makes both points read the same |∇ψ| and the ratio
	// collapses to 1 — which is what this catches.
	require.InEpsilon(t, vAspect, lodA/lodB, 1e-9,
		"the arm LOD must weight the vertical axis by vAspect (grid × inclination)")

	// And the mip points the right way: arms are resolved far out (lod 1) and fade
	// toward the crowded core (lod < 1). An inverted or unclamped mip fails one of
	// these.
	require.Equal(t, 1.0, splashGalaxyArmLOD(60, 0, vAspect), "arms must be fully resolved out in the disk")
	require.Less(t, splashGalaxyArmLOD(1, 0, vAspect), 1.0, "arms must fade toward the core")
	require.Equal(t, 0.0, splashGalaxyArmLOD(0, 0, vAspect), "the exact centre has no resolvable arms")
}

// TestSplashGalaxyCoreIsFinite guards the centre singularity. ln(r) and atan2 are
// undefined or meaningless at r == 0, and the field routes around them by returning
// the bulge alone below galCoreFrac·R — delete that guard and the exact centre
// computes cos(±Inf) == NaN, which clamp01 does not fix (NaN survives every
// comparison) and which then paints a garbage cell at the wordmark's centre.
func TestSplashGalaxyCoreIsFinite(t *testing.T) {
	at := splashFieldAt(Galaxy, 96)
	const phase = 3 * driftPerFrame
	for _, p := range []struct{ dx, dy float64 }{
		{0, 0}, {0.001, 0}, {0, 0.001}, {-0.5, 0.3}, {1, -1}, {2, 2},
	} {
		val, aux := at(0, 0, p.dx, p.dy, phase)
		require.Falsef(t, math.IsNaN(val) || math.IsInf(val, 0), "val at (%v,%v) must be finite", p.dx, p.dy)
		require.Falsef(t, math.IsNaN(aux) || math.IsInf(aux, 0), "aux at (%v,%v) must be finite", p.dx, p.dy)
		require.GreaterOrEqual(t, val, 0.0)
		require.LessOrEqual(t, val, 1.0)
		require.GreaterOrEqual(t, aux, 0.0)
		require.LessOrEqual(t, aux, 1.0)
	}
}

// galaxyMeasurable reports whether a rendered cell can be read as the galaxy's own
// brightness. The galaxy draws no starfield (see baseOps: a fixed star would punch a
// hole in the dense disk), so unlike ripple the only exclusion is the edge vignette,
// which dims the border cells for every variant and must stay out of any claim about
// this field's own falloff.
func galaxyMeasurable(col, row, w, h int) bool {
	mx := int(math.Max(1, float64(w)*edgeVignetteFrac)) + 1
	my := int(math.Max(1, float64(h)*edgeVignetteFrac)) + 1
	return col >= mx && col < w-mx && row >= my && row < h-my
}

// TestSplashGalaxyRendersABrightCoreAndDimmingArms is the Pass-2 half: brightness is
// the whole subject, and the claim is that it RENDERS as a radial gradient (a bright
// bulge grading out through dimming arms) with visible arm structure, not as a flat
// disc or a threshold. Asserted on the decoded luminance stops the terminal would
// receive, not on the field — the tunnel/rain lesson that Pass 1 being right never
// proved Pass 2 emits it.
func TestSplashGalaxyRendersABrightCoreAndDimmingArms(t *testing.T) {
	withColorProfile(t, termenv.TrueColor)
	const w, h, frame = 240, 60, 40
	stops, _ := shadeStopGrid(t, w, h, frame, splashTestPalette(), Galaxy)

	cx, cyFocal := float64(w-1)/2, float64((h-1)/2)
	maxD := math.Hypot(cx, cyFocal*cellAspect)
	rr := galExtent * maxD
	cosInc := math.Cos(galInc)
	// In-plane radius/angle: undo the inclination's vertical foreshortening so the
	// bands track the galaxy's actual (elliptical-on-screen) structure, not a screen
	// circle cutting across it.
	planeRA := func(col, row int) (rho, theta float64) {
		dx, dy := float64(col)-cx, (float64(row)-cyFocal)*cellAspect
		wy := dy / cosInc
		return math.Hypot(dx, wy) / rr, math.Atan2(wy, dx)
	}

	// Brightness density over a ρ band: the mean rendered stop across every
	// measurable cell, blanks counted as zero. That is deliberately not mean-over-lit
	// — lit cells in every band are dominated by saturated arm ridges, so a mean over
	// only-lit cells is nearly flat across radius and cannot see the envelope fade.
	// Counting the dark cells makes it an energy measure that falls as the arms dim
	// and the gaps open, which is what "brightness is the subject" has to render as.
	// Also returns the lit fraction, which the bulge and the arm gaps move.
	band := func(lo, hi float64) (dens, litFrac float64) {
		sum, lit, total := 0, 0, 0
		for row := 0; row < h; row++ {
			for col := 0; col < w; col++ {
				if !galaxyMeasurable(col, row, w, h) {
					continue
				}
				rho, _ := planeRA(col, row)
				if rho < lo || rho >= hi {
					continue
				}
				total++
				if s := stops[row][col]; s > 0 {
					lit++
					sum += s
				}
			}
		}
		if total == 0 {
			return 0, 0
		}
		return float64(sum) / float64(total), float64(lit) / float64(total)
	}

	coreDens, coreLit := band(0, 0.15)
	midDens, midLit := band(0.4, 0.62)

	require.Greater(t, coreLit, 0.9, "the bulge must render nearly solid")
	require.Greater(t, midDens, 0.0, "the mid-disk arms must render, not blank out")
	require.Greater(t, coreDens, midDens,
		"brightness must grade from a bright core to a dimmer disk, not render flat")

	// Arm structure: across a *thin* in-plane annulus the mean brightness must swing
	// with angle — brighter arms, darker dust lanes and inter-arm disk — rather than a
	// flat ring. It is a brightness swing, not a lit/blank one, because the disk is a
	// full glow (the inter-arm regions render rather than blanking, which is what keeps
	// the tight coil from weaving dark holes through it); the structure lives in *how
	// bright*, so measuring lit fraction would see a uniform 1.0 and miss it. The
	// annulus is thinner than one arm's radial period, so a given angle is mostly arm
	// or mostly lane; a wider band would let the coil cross an arm at every angle and
	// average the structure away.
	const bins = 24
	binSum := make([]int, bins)
	binTot := make([]int, bins)
	for row := 0; row < h; row++ {
		for col := 0; col < w; col++ {
			if !galaxyMeasurable(col, row, w, h) {
				continue
			}
			rho, theta := planeRA(col, row)
			if rho < 0.44 || rho >= 0.54 {
				continue
			}
			b := int((theta + math.Pi) / (2 * math.Pi) * bins)
			if b >= bins {
				b = bins - 1
			}
			binTot[b]++
			if s := stops[row][col]; s > 0 {
				binSum[b] += s
			}
		}
	}
	minMean, maxMean := math.Inf(1), math.Inf(-1)
	for b := 0; b < bins; b++ {
		if binTot[b] == 0 {
			continue
		}
		m := float64(binSum[b]) / float64(binTot[b])
		minMean = math.Min(minMean, m)
		maxMean = math.Max(maxMean, m)
	}
	require.Greater(t, maxMean-minMean, 1.5,
		"arms and dust lanes must render as an azimuthal brightness swing, not a flat ring: "+
			"mean stop ran %.2f..%.2f around the annulus", minMean, maxMean)
	require.Greater(t, midLit, 0.0)
}
