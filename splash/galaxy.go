package splash

import "math"

// The galaxy is the roster's second single-object field, and the tunnel's
// structural sibling: one thing centred on the wordmark, scaled to the pane so a
// bigger window shows more of the *same* galaxy rather than more galaxies, with
// its hue coming from its own radial gradient rather than from screen position.
// Where the tunnel is a corridor receding into the wordmark, the galaxy is an
// inclined spiral turning around it.
//
// Three things about it are traps rather than taste, and all are commented where
// they bite. The arms are a density *wave*, not matter: rotating each point by a
// radius-dependent speed would wind them into an unrecognizable coil within a few
// turns, and the splash loops forever, so instead the whole self-similar log
// spiral is rotated *rigidly* (see the phase term in splashGalaxyAtFor). The disk
// is *inclined* — tilted away from face-on into an ellipse — which foreshortens the
// vertical axis and, together with the terminal grid, sets how hard the arms alias
// there. And that aliasing is real wherever the coil packs below the grid's Nyquist
// limit — the inner disk, and the foreshortened top and bottom — so a sampling-rate
// LOD fades the arms toward the smooth disk there (see splashGalaxyArmLOD).
const (
	// galExtent is the disk radius as a fraction of maxD (the focal-point-to-corner
	// radius), so R = galExtent*maxD is the one length that scales with the pane.
	// Everything else the field computes is either a ratio to R (the envelopes) or
	// dimensionless (the winding, the arm count), which is what makes the galaxy the
	// same galaxy at every size without a per-constant scale factor the way the
	// tunnel needs one — a log spiral under a uniform scale is only phase-shifted,
	// and a forever-rotating pattern does not care about a phase offset.
	galExtent = 0.94
	// galInc is the disk's inclination away from face-on, in radians. At 0 the galaxy
	// is a circle seen from directly above; at galInc the round disk projects to a
	// wide ellipse, foreshortened vertically by cos(galInc). It is what makes the
	// field read as a three-dimensional object caught mid-turn rather than a flat
	// target, and it is the biggest single reason a real galaxy photo looks the way
	// it does. The foreshortening also compresses the arms vertically, which is why
	// the LOD's vertical weight carries a 1/cos(galInc) on top of the grid's aspect.
	galInc = 0.40
	// galArms is the number of spiral arms, and it MUST be an integer. The arm
	// pattern is cos(galArms*θ − …); θ runs on atan2's branch cut at ±π, and only an
	// integer arm count crosses that cut continuously (cos(mπ) == cos(−mπ)). A
	// fractional count tears a seam straight across the disk, the galaxy's analogue
	// of the tunnel's angular-wrap tear. Two reads as a grand-design spiral.
	galArms = 2
	// galWind is how tightly the arms wind: along an arm θ = (galWind/galArms)·ln r,
	// so the pitch angle is atan(galArms/galWind) — larger galWind is a smaller pitch
	// and a tighter coil. It is a moderate, open grand-design pitch: enough wind to
	// read as a spiral (~1.5 turns from core to rim), loose enough that the two arms
	// stay traceable with dark wedges between them rather than packing into concentric
	// rings. The inner coil still passes below the grid's Nyquist limit, which is where
	// the LOD earns its keep — without it those inner arms would alias into crawling
	// spokes on the foreshortened axis.
	galWind = 6.0
	// galArmSharp sharpens the raised cosine into ridges: >1 narrows the bright arms
	// and widens the dark inter-arm gas between them.
	galArmSharp = 2.5
	// galArmFloor is the inter-arm floor: the diffuse disk still glows between the
	// arms rather than going fully black, so the arms sit on a disk instead of
	// hanging in the void. 0 would make the gaps as dark as deep space.
	galArmFloor = 0.38
	// galDiskScale sets how far the disk (and so the arms) reaches, as an
	// exponential falloff in ρ = r/R: diskEnv = exp(−ρ/galDiskScale). It is broad on
	// purpose — Pass 2's fixed Hermite contrast crushes low field values toward the
	// blank floor, so the arms have to stay bright well out into the disk to render
	// as more than a faint stipple.
	galDiskScale = 0.92
	// galBulgeAmp/galBulgeR are the central bulge: a bright, arm-free nucleus that is a
	// soft exponential glow, exp(−ρ/galBulgeR). The long tail is the point — it fades
	// gradually into the disk rather than ending in a hard-edged blob that reads as a
	// separate object — and it covers the inner radii where the arms wind too tightly
	// to resolve anyway. galBulgeR is the e-folding radius as a fraction of the disk.
	galBulgeAmp = 1.0
	galBulgeR   = 0.26
	// galHueGamma maps radius to gradient position: aux = ρ^galHueGamma, so the core
	// takes the palette's warm anchor and the rim its cool one. That is real
	// astrophysics (an old red bulge, young blue disk) and happens to be exactly how
	// splashAnchors already runs — warm at stop 0, cool at stop 19 — so aux paints
	// it for free through splashColorIdx with no arm to add.
	galHueGamma = 0.82
	// The dust lanes: dark bands offset from the bright arm ridges, subtracted from
	// the disk, the way a real spiral's dust threads the inside of each arm. galDustAmp
	// is their depth (0 disables them), galDustPhase places them relative to the ridge,
	// galDustSharp narrows them. Like the arms they are faded by the LOD, so they never
	// alias into a dark stipple where the arm itself has been smoothed away.
	galDustAmp   = 0.5
	galDustPhase = 0.55
	galDustSharp = 2.4
	// galLODC is where the arm LOD bites, in units of Nyquist — derived, not dialled
	// (a band-limited pattern aliases once it packs more than ~half a cycle into a
	// cell, i.e. stepPhase past π), so it is a small margin rather than a free knob.
	// See splashGalaxyArmLOD.
	galLODC = 1.1
	// galCoreFrac guards the exact centre: below r = galCoreFrac*R the field is the
	// bulge alone, which keeps ln(r) and atan2(0,0) away from r == 0 and matches the
	// physics — there are no arms inside the nucleus. Pane-relative so the guarded
	// disc stays the same fraction of the galaxy at every size.
	galCoreFrac = 0.05
	// galRotSpd is the rigid pattern rotation. A ridge holds ψ constant, so the
	// pattern turns at galRotSpd/galArms per unit phase — gentle, the way a galaxy
	// turns, and its sign is the spin direction. Settled from the render round.
	galRotSpd = 1.0
	// The turbulence: a static fBm the arms are mottled by and, at its peaks, studded
	// with bright knots. It is the detail — smooth raised-cosine arms read flat, while
	// a real spiral's arms are grainy, filamentary and knotted with star-forming
	// regions. galTurbFreq is the coarsest octave's spatial frequency (in in-plane
	// units), galTurbOct the octave count, galTurbGain the per-octave amplitude
	// falloff. galTurbAmp is how much the peaks brighten the arm — only the peaks: the
	// lows leave the arm alone rather than dimming it, or a dip under Pass 2's blank
	// floor would read as a hole. galKnotThr/galKnotAmp are where the turbulence tips
	// over into a knot, and how bright.
	//
	// It is deliberately fixed in screen space rather than turned with the disk: the
	// arms sweep *through* a static field, so its fine detail never moves and so can
	// never wagon-wheel, which is what lets the frequency be high enough to read as
	// texture without the per-octave mip the tunnel needs.
	galTurbFreq = 0.13
	galTurbOct  = 5
	galTurbGain = 0.55
	galTurbAmp  = 0.62
	galKnotThr  = 0.68
	galKnotAmp  = 0.7
	// Colour beyond the radial sweep: the arms lean cooler (galArmHue: young blue
	// stars) and the turbulence mottles the hue into warm/cool patches (galTurbHue), so
	// the disk works the whole palette instead of a clean gradient. Both are gentle —
	// enough variety to enrich the colour without carving the core into a distinct
	// patch — and they move only the hue, never the brightness, so neither opens a hole.
	galArmHue  = 0.12
	galTurbHue = 0.09
	// The near-side dust lane: a horizontal dark band silhouetting the disk's front
	// edge across the bright bulge (an inclined galaxy's real depth cue). galLaneAmp is
	// how far it darkens (to a floor, not to black), galLaneY its centre below the
	// mid-line and galLaneW its reach, both as fractions of the disk radius.
	galLaneAmp = 0.45
	galLaneY   = 0.10
	galLaneW   = 0.09
)

var (
	galTurbSeed = [galTurbOct]uint32{0x9E3779B1, 0x2545F491, 0xD1B54A33, 0x7F4A7C15, 0xB5297A4D}
	galTurbOff  = [galTurbOct]float64{0, 0.41, 0.77, 0.23, 0.59}
)

// galValNoise is bilinear value noise on the lattice hash, smoothstep-interpolated
// so it is C¹ (no lattice creases). It is the octave splashGalaxyTurbulence stacks.
func galValNoise(x, y float64, seed uint32) float64 {
	xi, yi := math.Floor(x), math.Floor(y)
	xf, yf := x-xi, y-yi
	su := xf * xf * (3 - 2*xf)
	sv := yf * yf * (3 - 2*yf)
	ix, iy := int32(xi), int32(yi)
	return splashLerp(
		splashLerp(latticeVal(ix, iy, seed), latticeVal(ix+1, iy, seed), su),
		splashLerp(latticeVal(ix, iy+1, seed), latticeVal(ix+1, iy+1, seed), su), sv)
}

// splashGalaxyTurbulence is the normalized fBm ([0,1], mean ~0.5) that mottles the
// arms. Per-octave offsets decorrelate the lattice positions; the seeds decorrelate
// the values.
func splashGalaxyTurbulence(x, y float64) float64 {
	sum, amp, norm := 0.0, 1.0, 0.0
	fx, fy := x*galTurbFreq, y*galTurbFreq
	for o := 0; o < galTurbOct; o++ {
		sum += amp * galValNoise(fx+galTurbOff[o], fy+galTurbOff[o], galTurbSeed[o])
		norm += amp
		amp *= galTurbGain
		fx *= 2
		fy *= 2
	}
	return sum / norm
}

// splashGalaxyArmLOD is the arm pattern's sampling-rate level-of-detail: 1 where
// the arms are resolved and falling toward 0 where they wind tighter than the grid
// can sample, so the caller can fade the arm modulation toward the smooth disk
// there instead of letting it alias into crawling wagon-wheel spokes.
//
// (u, w) are the in-plane coordinates — screen dx, and screen dy already un-inclined
// (dy/cos(galInc)) — and vAspect is how many in-plane units one screen row covers
// against one screen column: the grid's cellAspect times the inclination's 1/cos.
// The arm phase is ψ = galArms·θ − galWind·ln r over those coordinates, whose
// gradient is (∂ψ/∂u, ∂ψ/∂w) = (−(galWind·u+galArms·w)/r², (galArms·u−galWind·w)/r²).
// Phase advanced per screen cell is |∂ψ/∂u| horizontally and vAspect·|∂ψ/∂w|
// vertically — and the larger of the two is what aliases first. This is the same
// top-and-bottom exposure the tunnel's rings had (caught there twice, from motion
// alone); an isotropic version would leave the vertical arms crawling while the
// horizontal ones flow.
//
// Because both gradient components fall as 1/r, stepPhase grows without bound toward
// the centre, so the LOD reaches 0 there on its own — which is right, the arms are
// unresolvable in the nucleus and the bulge covers it. It is a pure function of the
// cell position, the winding constants and vAspect, so it can be exercised directly:
// at any matched radius the vertical axis must be damped a factor vAspect harder than
// the horizontal, which an isotropic mip cannot reproduce.
func splashGalaxyArmLOD(u, w, vAspect float64) float64 {
	r2 := u*u + w*w
	if r2 == 0 {
		return 0
	}
	dpsiDu := math.Abs(galWind*u+galArms*w) / r2
	dpsiDw := math.Abs(galArms*u-galWind*w) / r2
	// stepPhase is > 0 at every r > 0, so the divide below never needs a guard: the
	// two gradient forms are (galWind,galArms)·(u,w) and (galArms,−galWind)·(u,w),
	// whose matrix has determinant −(galWind²+galArms²) ≠ 0, so they cannot both
	// vanish off the origin — and r == 0 already returned above.
	stepPhase := math.Max(dpsiDu, vAspect*dpsiDw)
	return clamp01(math.Pi / (galLODC * stepPhase))
}

// splashGalaxyAtFor builds the evaluator mapping a cell to the galaxy's brightness
// and its radius-banded hue.
//
// val is a bright central bulge plus a spiral disk that fades outward, minus dust
// lanes; brightness is the whole subject (core to arms to dark lanes), so it rides
// ops.lumRange the way the tunnel's fog does — on this palette luminance is the only
// channel that can say how bright a cell is, never hue. aux is radius alone, which
// splashColorIdx spends directly as a gradient position: the galaxy is warm at the
// core and cool at the rim, and the arms turn *through* that fixed radial colour
// rather than carrying a hue of their own, so the palette sweep stays put while the
// brightness rotates.
//
// It is built for one pane rather than being a plain function because the galaxy is
// a single object and has to be the same object at every size (see splashFieldAt):
// R is the only length it measures against maxD. Unlike the tunnel it needs no other
// scale factor — the winding is scale-invariant and the LOD is in absolute cells,
// because Nyquist is a fact about cells and a larger pane simply resolves more of
// the arms, which is true rather than convenient.
func splashGalaxyAtFor(maxD float64) splashPointFn {
	R := galExtent * maxD
	if R <= 0 {
		// renderSplashField already returns early on a degenerate pane; this only
		// keeps a direct caller from dividing by zero below.
		R = 1
	}
	rMin := galCoreFrac * R
	cosInc := math.Cos(galInc)
	// The near-side dust lane's centre and reach in screen cells: a horizontal dark
	// band a little below the disk's mid-line, the front edge of the tilted disk
	// silhouetted across the bright bulge (see the closure).
	laneY, laneW := galLaneY*R, galLaneW*R
	// One screen row covers cellAspect in-plane units from the grid, then a further
	// 1/cos(galInc) from the disk's foreshortening — the total vertical anisotropy the
	// arm mip has to answer to.
	vAspect := cellAspect / cosInc

	return func(_, _ int, dx, dy, phase float64) (val, aux float64) {
		// Un-incline: the tilted disk foreshortens the vertical axis by cos(galInc),
		// so a screen row maps to dy/cos of the galaxy plane. Working in those in-plane
		// coordinates renders the round disk as the wide ellipse a tilted galaxy is.
		wy := dy / cosInc
		r := math.Hypot(dx, wy)
		rho := r / R
		auxBase := math.Pow(rho, galHueGamma)
		aux = clamp01(auxBase)

		// The bulge is a soft exponential glow, not a Gaussian: exp(−ρ/scale) has a long
		// tail, so the bright core fades *gradually* into the disk instead of ending in a
		// hard-edged blob that reads as a separate object. It is arm-free and defined
		// everywhere, so it also carries the core disc below rMin, where ln(r) and atan2
		// are undefined and there are no arms anyway.
		bulge := galBulgeAmp * math.Exp(-rho/galBulgeR)
		if r < rMin {
			return clamp01(bulge), aux
		}

		// The rigid density wave. ln(r) has a *constant* coefficient — it does not
		// grow with time or with the number of turns — so this is a rigid rotation of
		// a self-similar spiral and the arms keep their pitch forever, instead of the
		// material winding that would coil them shut in a handful of loops. galArms is
		// an integer, so the pattern crosses atan2's ±π branch cut without a seam.
		psi := galArms*math.Atan2(wy, dx) - galWind*math.Log(r) - phase*galRotSpd

		// Fade the *linear* raised cosine toward its mean (0.5) before sharpening, so
		// the LOD commutes with the nonlinearity: at lod 0 the modulation is flat and
		// pow leaves a constant, and pow's slope at 0.5 is ≤1 for galArmSharp ≥ 2, so
		// a partly-faded arm cannot be re-expanded the way a smoothstep-after-mip
		// would (the trap the tunnel's linear wall gain exists to avoid).
		lod := splashGalaxyArmLOD(dx, wy, vAspect)
		modMip := 0.5 + lod*(0.5*math.Cos(psi))
		arm := math.Pow(modMip, galArmSharp)

		// Turbulence brightens the arm into grain and filaments (the detail a smooth
		// ridge lacks), and where it peaks it studs the arm with a bright knot — a
		// star-forming region. Both ride the arm, so the texture lives on the arms and
		// fades out with them. It only ever *adds* light: the turbulence lows leave the
		// arm at its base value rather than dimming it, because a dip that fell under
		// Pass 2's blank floor would punch a dark hole in the disk. The dark structure
		// is the dust lanes' job — they follow the arms, so they read as lanes, not holes.
		turb := splashGalaxyTurbulence(dx, wy)
		bright := clamp01((turb - 0.5) * 2) // 0 below the mean, up to 1 at the peaks
		arm *= 1 + galTurbAmp*bright
		knot := galKnotAmp * arm * clamp01((turb-galKnotThr)/(1-galKnotThr))

		// Dark dust lanes offset from the ridge, also faded by the LOD so they never
		// outlive the arm they belong to.
		dust := galDustAmp * lod * math.Pow(0.5+0.5*math.Cos(psi+galDustPhase), galDustSharp)

		// The disk: arms on an inter-arm floor, all falling off with radius.
		diskEnv := math.Exp(-rho / galDiskScale)
		disk := diskEnv * (galArmFloor + (1-galArmFloor)*arm + knot)

		// The near-side dust lane: a horizontal dark band across the disk's front edge,
		// darkest just below centre where it cuts across the bright bulge. That
		// silhouette — disk in front of core — is the depth cue an inclined galaxy reads
		// by, and the one that survives this medium: geometric shape barely registers at
		// a cell's resolution, an occluding lane does. It multiplies down to a floor
		// rather than to black, so it dims into a lane instead of blanking holes.
		lane := 1 - galLaneAmp*math.Exp(-((dy-laneY)*(dy-laneY))/(laneW*laneW))

		// Hue: the radial warm-core→cool-rim sweep, softened by only a gentle arm-cool
		// lean and turbulence mottle — enough colour variety to work the palette without
		// carving the core into a distinct patch. Brightness is untouched here, so none
		// of it can open a hole.
		aux = clamp01(auxBase + galArmHue*lod*arm + galTurbHue*(turb-0.5))

		return clamp01((bulge + disk - dust*diskEnv) * lane), aux
	}
}
