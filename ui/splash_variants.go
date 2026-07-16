package ui

// The splash variant vocabulary: which field generators exist, what users call
// them, and which one is currently selected. Kept apart from splash_field.go so
// that file is only the noise core and the renderer — this is the part that
// changes whenever a variant is added, retired, or renamed.
//
// Note the two directions of "name" here. splashVariantNames is the pinnable
// vocabulary shared with config (see SplashVariantNames); ATRIUM_SPLASH_VARIANT
// additionally accepts historical dev letters and trumps config, which is what
// keeps screenshot A/B runs and the test suites deterministic.

import (
	"math"
	"os"
	"slices"
	"sort"
	"strconv"
	"sync"
	"time"
)

// splashVariant selects the field generator + glyph technique. Users pin one
// (or keep the random per-launch rotation) via the "splash" config setting;
// the dev-only ATRIUM_SPLASH_VARIANT env override still trumps it for
// screenshot A/B runs and test pinning.
type splashVariant int

const (
	// splashVariantRain ("f") is Matrix-style digital rain: per-column streams
	// with bright heads and fading tails, layered at three depths. The roster's
	// motion entry — and the only variant that shades by luminance alone, with no
	// hue of its own to spend (see buildRainRamp).
	//
	// It is also the package's fallback: an unrecognized override name and a
	// variant with no case in splashFieldAt or baseOps all land here.
	splashVariantRain splashVariant = iota
	// splashVariantTunnel ("g") is a textured wall flying past a vanishing point
	// that sits on the wordmark: screen position maps to (depth, angle), so a
	// plain noise lookup becomes an infinite corridor. The roster's depth entry —
	// z-fog carries distance in luminance, and hue bands by depth into coloured
	// rings receding down the wall.
	splashVariantTunnel
	// splashVariantRipple ("h") is drops falling on a dark pool: each one flashes
	// where it lands and expands into a ring that shifts hue as it ages, and the
	// rings interfere where they cross. The roster's event entry — the only field
	// with a birth and a death in it rather than a steady state.
	splashVariantRipple

	// splashVariantCount is the enum's cardinality, not a variant — it must
	// stay last. It exists so the tests can prove they cover every variant:
	// the contract loop and the benchmark both walk a hand-maintained map, and
	// a variant missing from it escapes both without failing anything. Safe to
	// key off the iota here because nothing persists these ordinals — config
	// stores the pattern's name, not its number.
	splashVariantCount
)

// splashDefaultVariant is the fallback for an unrecognized override value; an
// unset override rotates instead (see splashActiveVariant).
//
// Rain, so that the package has exactly one fallback rather than three: this,
// splashFieldAt's default arm and baseOps' default arm all resolve to it, and a
// variant that is mis-wired in any of them renders rain rather than something
// that merely looks plausible. Deliberately NOT what the test suites pin — see
// parseSplashEnvVariant, whose fall-through is invisible to a pin that names the
// fallback.
const splashDefaultVariant = splashVariantRain

// splashRotation is the pool random mode draws from: every shipped variant.
var splashRotation = []splashVariant{
	splashVariantRain, splashVariantTunnel, splashVariantRipple,
}

// splashVariantNames maps the user-facing pattern names (config.SplashVariants,
// cycled in the settings panel) onto the variant enum. ui deliberately takes
// the name as a plain string (SetSplashVariant) so it needs no config import.
var splashVariantNames = map[string]splashVariant{
	"rain":   splashVariantRain,
	"ripple": splashVariantRipple,
	"tunnel": splashVariantTunnel,
}

// splashOps is a variant's Pass-2 policy: how its raw field is turned into
// glyphs and colour.
//
// It is deliberately small. It carried five more fields — a contrast window, a
// dither, a radial dim, a breathing swell — that existed for the organic fields
// retired in V5, and not one of them was a knob a surviving variant turned. A new
// variant that wants one re-adds it with its own justification, which is healthier
// than inheriting an unowned one: this package has already shipped that bug, with
// dimToRim declared and never read, so rain silently took a 42% dim it had opted
// out of. git show 0734403 is the archive.
type splashOps struct {
	// stars draws the fixed twinkling starfield over the field.
	stars bool
	// lumRange is the share of a cell's brightness that rides its colour's
	// luminance rather than its glyph's density.
	//
	// 0 is the shading every field had before there was a choice: density carries
	// all of it, so a dim cell is necessarily a *small* one and a fading field
	// degenerates into a scatter of dots. 1 is rain's: a constant-weight glyph with
	// all brightness in the colour, which is the only thing that works when the
	// glyph vocabulary has no light end to fade into. Between them the two channels
	// split it, so density can carry texture while luminance carries brightness.
	//
	// Both endpoints are reproduced exactly and for free (see splashShade). No
	// shipped variant sits at 0 any more — the fields that wanted their stipple were
	// the organic ones — so that endpoint survives for the dev override and for a
	// future variant rather than for the roster.
	lumRange float64
}

// ops returns a variant's Pass-2 policy.
func (v splashVariant) ops() splashOps {
	o := v.baseOps()
	if r, ok := splashLumRangeOverride(); ok {
		o.lumRange = r
	}
	return o
}

// baseOps is the shipped policy, before the dev-only lumRange override.
//
// Every variant states both fields as a literal, and the roster table
// (TestShippedVariantsOps) pins them per variant, so a new one has to make an
// explicit choice rather than inherit whatever the zero value happens to be. Rain
// and the tunnel currently agree on both and are still written out separately on
// purpose: merging them would silently move one when the other is retuned.
func (v splashVariant) baseOps() splashOps {
	switch v {
	case splashVariantTunnel:
		return splashOps{
			// Screen-fixed stars punching through a moving wall destroy vection —
			// the reflex that makes a receding texture read as self-motion rather
			// than as a pattern. This is the variant's whole premise, so it is not
			// a preference.
			stars: false,
			// The fog is a gradient with no stipple to spend, so it is exactly what
			// the luminance channel was built for. Density cannot carry depth here:
			// "dim" would mean "small", and a far wall would read as a scatter of
			// dots rather than as distance. Chosen from a rendered sweep of
			// {0, 0.5, 0.75, 1}, not from the arithmetic — at 1 the glyph is a
			// constant '@' and every bit of the corridor is drawn in colour.
			lumRange: 1,
		}
	case splashVariantRipple:
		return splashOps{
			// The one variant that keeps the starfield, because for once fixed
			// points are *right*: this field is a dark pool, and a still pool
			// reflects a still sky. Rain and the tunnel had to drop the stars because
			// the eye tracks their motion and a fixed point in a moving field reads
			// as a stuck pixel — but nothing here travels except the rings, and the
			// pool between them is empty enough to want the company.
			stars: true,
			// Ripple's ring decay is a gradient with no stipple to spend, so the
			// luminance channel is exactly what it was built for — but this one stops
			// short of rain's and the tunnel's 1, and the sweep is what decided it.
			//
			// At 1 every lit cell is a solid '@' and the rings read as a bitmap of
			// bubbles: correct, and less like something a terminal drew. At 0.75 the
			// crests get the density ramp back (a band steps o → O → 0 → @ across its
			// width) while the tail keeps enough of its brightness in the colour to
			// stay dim rather than breaking into the scatter of dots that is this
			// palette's whole defect. Below that the ramp reaches into the packet's
			// faint halo and the halo is most of the field's area — at 0.5 it renders
			// as confetti around every ring.
			lumRange: 0.75,
		}
	default:
		// Rain, and the fallback for a variant with no case here (see
		// splashDefaultVariant).
		return splashOps{
			// Stars are fixed points; rain is moving ones. Together the fixed ones
			// read as stuck pixels, and rain has its own highlight anyway.
			stars: false,
			// All brightness rides the colour; the glyph stays a constant mark.
			// This is the whole difference between a stream and a column of dots —
			// see buildRainRamp.
			lumRange: 1,
		}
	}
}

// SplashVariantNames lists every pinnable pattern name, sorted.
//
// It exists to be checked against config.SplashVariants(). The two lists are
// hand-maintained in packages that cannot import each other — config knows
// nothing of ui, and ui takes the name as a plain string precisely so it needs
// no config import — and for a long time nothing tied them. The failure is
// silent in the worst direction: a name in config but not here is offered by
// the settings panel, falls through SetSplashVariant's lookup, and quietly
// means "random", so the pattern the user pinned simply never appears. app
// imports both and asserts they agree.
func SplashVariantNames() []string {
	names := make([]string, 0, len(splashVariantNames))
	for name := range splashVariantNames {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// splashEnvVariant resolves the dev-only ATRIUM_SPLASH_VARIANT override once
// per process. It trumps the config setting so screenshot A/B runs and the test
// suites (which pin a variant in TestMain against rotation nondeterminism) stay
// deterministic whatever the config under test says. The second value is false
// when the variable is unset.
var splashEnvVariant = sync.OnceValues(func() (splashVariant, bool) {
	return parseSplashEnvVariant(os.Getenv("ATRIUM_SPLASH_VARIANT"))
})

// parseSplashEnvVariant reads the override: the variant to render, and whether
// the variable was set at all.
//
// Split from the env read for one reason — it is the only way any of this is
// testable. The resolution lives in a sync.OnceValues closure and TestMain pins
// the variable, which defeats even a subprocess probe, so the letters had no test
// at all and the fall-through here had no way to be stated. Its sibling knob
// (parseSplashLumRange) already had this shape.
//
// The two knobs answer the same question differently, and the asymmetry is
// deliberate. Junk gives parseSplashLumRange ok=false (no override), but gives
// this ok=true: the bool means "an override was set", not "it was understood", so
// a typo, a retirement and a deletion all resolve to splashDefaultVariant rather
// than falling back to the rotation. That keeps a mispinned suite deterministic —
// wrong, but not flaky, which is the failure worth having.
//
// It is also why the understanding is a separate function rather than a fifth
// case here. Collapsing them makes the letter that names the fallback untestable:
// "f" resolves to rain and so does every unrecognized string, so deleting case
// "f" cannot be observed at this boundary and the whole letter vocabulary sits on
// a guard that passes either way. And it is why TestMain pins a variant that is
// not the fallback, for the same reason one level up.
func parseSplashEnvVariant(s string) (splashVariant, bool) {
	if s == "" {
		return splashDefaultVariant, false
	}
	if v, ok := lookupSplashVariant(s); ok {
		return v, true
	}
	return splashDefaultVariant, true
}

// lookupSplashVariant answers only "does this build know that name", which is the
// question the fall-through above destroys. It accepts the user-facing names and
// the historical dev letters, kept as they were: f/g/h are what the screenshot
// recipes, the notes and the muscle memory all use, and re-lettering to a/b/c
// would buy tidiness and break every recipe. a–e and "legacy" named the organic
// fields retired in V5; next free letter is i.
func lookupSplashVariant(s string) (splashVariant, bool) {
	if v, ok := splashVariantNames[s]; ok {
		return v, true
	}
	switch s {
	case "f":
		return splashVariantRain, true
	case "g":
		return splashVariantTunnel, true
	case "h":
		return splashVariantRipple, true
	}
	return splashDefaultVariant, false
}

// splashLumRange is the dev-only ATRIUM_SPLASH_LUMRANGE override: it replaces
// every variant's shipped lumRange, so a screenshot round can sweep the knob
// without a rebuild per value. Unset (the shipped path) leaves each variant's own.
//
// It applies to every variant, rain included, and on rain a low value is a
// deliberate absurdity rather than a bug: rain's glyphs have constant weight, so
// taking brightness off the colour leaves it nowhere to go and the pane fills with
// white katakana. That is the whole thesis of this file, rendered. Pin the variant
// you mean to look at (ATRIUM_SPLASH_VARIANT) when sweeping.
//
// A plain var behind splashSelMu rather than a sync.OnceValue, for two reasons.
// The benchmarks have to drive the shaded path, and a OnceValue cannot be driven
// from one — it would resolve on whatever the first test touched it with and pin
// that for the binary. And splashSelMu already guards exactly this shape of state
// (see splashPick below), at one uncontended lock per *frame* — ops() is called
// once per render, not once per cell.
var (
	splashLumRangeSet bool
	splashLumRangeVal float64
)

func init() {
	splashLumRangeVal, splashLumRangeSet = parseSplashLumRange(os.Getenv("ATRIUM_SPLASH_LUMRANGE"))
}

func splashLumRangeOverride() (float64, bool) {
	splashSelMu.Lock()
	defer splashSelMu.Unlock()
	return splashLumRangeVal, splashLumRangeSet
}

// parseSplashLumRange reads the override, rejecting anything that would not be a
// share of brightness.
//
// The NaN and infinity rejections are not hygiene. strconv.ParseFloat accepts
// "nan" and "+Inf" happily, and a NaN would pass *both* of splashShade's endpoint
// guards (every comparison against NaN is false), reach the interior, and land in
// an int() conversion of a NaN — which is implementation-defined in Go: amd64
// gives minint, arm64 gives 0. That is a silent per-architecture difference in
// rendered output, which is the one property this whole field is careful about.
func parseSplashLumRange(s string) (float64, bool) {
	if s == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, false
	}
	return math.Min(math.Max(v, 0), 1), true
}

// The process-wide variant selection: lazily a per-launch rotation draw, or
// whatever SetSplashVariant last pinned or re-rolled. splashRandomMode records
// whether the config asked for random, which is what lets the screensaver
// re-roll per showing while a pinned choice stays put.
//
// splashSelMu guards them. Bubble Tea drives every caller from its single
// update/view goroutine, so the lock is uncontended insurance rather than a
// live need — but it is the same insurance splashLUTFor buys one file over,
// and TerminalPane.String() already takes its own mutex before reaching
// splashScene, i.e. that pane is treated as multi-goroutine-reachable. These
// vars replaced a sync.OnceValue that was safe by construction; a leaf mutex
// keeps the invariant enforced instead of merely documented, at one
// uncontended lock per frame.
var (
	splashSelMu      sync.Mutex
	splashRandomMode = true
	splashPicked     bool
	splashPick       splashVariant
)

// splashActiveVariant resolves the variant splashScene renders: the dev env
// override if set, else the current selection (seeded lazily from the launch
// time, so an unconfigured launch gets a fresh look each time). Resolving to
// one process-wide value is what keeps the preview and terminal panes in
// agreement; only splashScene consults it — renderSplashField takes the
// variant as a parameter and stays pure over its inputs.
func splashActiveVariant() splashVariant {
	if v, ok := splashEnvVariant(); ok {
		return v
	}
	splashSelMu.Lock()
	defer splashSelMu.Unlock()
	if !splashPicked {
		splashPick, splashPicked = splashRotationPick(time.Now().UnixNano()), true
	}
	return splashPick
}

// SetSplashVariant applies the config's splash mode (config.GetSplash): a
// known pattern name pins that generator; anything else (config.SplashRandom,
// or an unknown value) re-rolls a fresh random pick. Called at startup and on
// a live settings change — with the settings panel open over the idle empty
// state, cycling the enum previews each pattern in place.
func SetSplashVariant(name string) {
	splashSelMu.Lock()
	defer splashSelMu.Unlock()
	if v, ok := splashVariantNames[name]; ok {
		splashRandomMode, splashPick, splashPicked = false, v, true
		return
	}
	splashRandomMode = true
	splashPick, splashPicked = splashRotationPick(time.Now().UnixNano()), true
}

// RerollSplashVariant draws a fresh random pick when in random mode (a pinned
// config keeps its pattern). The screensaver calls it on activation so each
// showing within a launch differs from the last: the draw deliberately excludes
// the current pattern, since a re-roll that lands back on it reads as a dead
// keypress rather than a fresh look.
func RerollSplashVariant() {
	splashSelMu.Lock()
	defer splashSelMu.Unlock()
	if splashRandomMode {
		splashPick, splashPicked = splashRotationReroll(time.Now().UnixNano(), splashPick), true
	}
}

// splashRotationPick maps a launch-time nanosecond seed to a rotation variant.
func splashRotationPick(nano int64) splashVariant {
	return splashRotation[splashRotationIdx(nano, len(splashRotation))]
}

// splashRotationReroll maps a seed to a rotation variant other than cur, so a
// re-roll always visibly changes the pattern. It draws over the pool minus cur
// (len-1 slots) and steps past cur's slot, which keeps the result uniform over
// the remaining variants rather than biased toward cur's neighbour.
//
// A cur outside the pool has nothing to exclude, so it falls back to a plain
// draw over the whole pool. Every shipped variant is in the pool now — the
// retired legacy baseline used to be the one that wasn't — so that arm is
// defensive, and it is worth being exact about what it defends: not an index
// panic. slices.Index returns -1, every idx is >= -1, so the step-past-cur
// increment fires unconditionally and still lands inside the slice. What is lost
// without it is slot 0 — the draw silently becomes uniform over the pool *minus
// its first variant*, which is a bias no caller asked for and nothing would
// notice. (The len < 2 arm is the one that stops a panic: splashRotationIdx
// would take nano % 0.)
func splashRotationReroll(nano int64, cur splashVariant) splashVariant {
	ci := slices.Index(splashRotation, cur)
	if ci < 0 || len(splashRotation) < 2 {
		return splashRotationPick(nano)
	}
	idx := splashRotationIdx(nano, len(splashRotation)-1)
	if idx >= ci {
		idx++
	}
	return splashRotation[idx]
}

// splashRotationIdx folds a nanosecond seed into [0, n). Go's % preserves the
// dividend's sign, so a clock set before the Unix epoch (a negative UnixNano)
// would otherwise yield a negative index and panic.
func splashRotationIdx(nano int64, n int) int {
	idx := nano % int64(n)
	if idx < 0 {
		idx += int64(n)
	}
	return int(idx)
}
