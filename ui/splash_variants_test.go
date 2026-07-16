package ui

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestShippedVariantsOps pins the per-variant decisions that render differently
// but break no invariant — the ones nothing else can fail on.
//
// This is the roster table, and it is the shape a new variant inherits. It exists
// because the alternative shape does not work: the tests that *care* about a
// policy field all open with a filter on it, so they check that variants which
// claim a property honour it, and a variant that stops claiming is not caught —
// it is excused. Every such field was reachable by mutation on a fully green
// tree: flipping ripple's stars, or dropping it from the structured set, left
// `go test ./...` entirely passing. Both of those knobs are gone now, but the
// shape that catches them is not, and the two that remain are pinned here.
//
// Literals rather than reads of baseOps: a table that asks the code what it does
// and then agrees is not a change-detector, it is a mirror. The risk is real
// rather than theoretical — ops.dimToRim once shipped declared-and-never-read
// because a scripted edit silently no-op'd, and a lumRange that quietly reverted
// to 0 would look exactly like a variant that was never tuned.
//
// It replaces two tables, one per surviving field. V5 retired six variants and
// with them five ops fields (a contrast window, a dither, a radial dim, a
// breathing swell), and the two tables' other columns went with those: every
// survivor was structured, so that predicate became a constant and then nothing;
// every survivor spent aux as its hue, so hueIsAux became the universal rule in
// splashColorIdx rather than a per-variant claim.
func TestShippedVariantsOps(t *testing.T) {
	type policy struct {
		stars    bool    // draws the fixed twinkling starfield
		lumRange float64 // share of brightness on colour rather than glyph density
	}
	want := map[splashVariant]policy{
		// Rain and the tunnel are the moving fields: the eye tracks them, so a fixed
		// star reads as a stuck pixel. Both put all their brightness on the colour —
		// rain because a constant-weight katakana cannot shade by size, the tunnel
		// because its fog is a gradient with no stipple to spend on the trade.
		splashVariantRain:   {stars: false, lumRange: 1},
		splashVariantTunnel: {stars: false, lumRange: 1},
		// Ripple is the one field that is both in motion and starry, because nothing
		// in it travels except the rings and a still pool reflects a still sky. It is
		// also the only one at neither end of the luminance split: its crests keep the
		// density ramp while the tail rides the colour. The value came from a rendered
		// sweep; see baseOps.
		splashVariantRipple: {stars: true, lumRange: 0.75},
	}
	require.Len(t, want, int(splashVariantCount),
		"every variant needs an explicit policy here — a new one must make these "+
			"choices, not inherit whatever the zero value happens to be")
	for v := splashVariant(0); v < splashVariantCount; v++ {
		w, ok := want[v]
		require.Truef(t, ok, "variant %d is missing from the table", int(v))
		require.Equalf(t, w.stars, v.ops().stars, "variant %d's shipped ops.stars", int(v))
		require.Equalf(t, w.lumRange, v.ops().lumRange, "variant %d's shipped ops.lumRange", int(v))
	}
}

// TestSplashRotationCoversEveryVariant closes the hole the roster tables leave.
//
// Three comments in this package now assert that the pool is "every shipped
// variant", and until V5 nothing could pin it: the legacy baseline was pinnable
// by name and deliberately never drawn, so the pool was legitimately a subset. It
// isn't any more, and the failure it leaves is silent in the way this package
// keeps getting caught by — a variant added to the enum, the names, the ops table
// and both test maps but forgotten here is pinnable, passes every other guard,
// and simply never comes up at random. splashRotation is the one hand-maintained
// list nothing walked.
func TestSplashRotationCoversEveryVariant(t *testing.T) {
	require.Len(t, splashRotation, int(splashVariantCount),
		"the random pool must offer every variant")
	for v := splashVariant(0); v < splashVariantCount; v++ {
		require.Containsf(t, splashRotation, v,
			"variant %d is pinnable but never drawn at random", int(v))
	}
}

// TestParseSplashEnvVariant covers the dev override's parse, which had no test at
// all until it was split out of the sync.OnceValues closure it resolves in — the
// closure memoizes process-wide and TestMain pins the variable, which defeats
// even a subprocess probe.
//
// The fall-through is the interesting half. It answers an unrecognized value with
// ok=true, so ok means "an override was set", not "it was understood": a typo, a
// retired name and a deleted letter all resolve to splashDefaultVariant rather
// than falling back to the rotation, which keeps a mispinned suite deterministic
// instead of flaky. That is also why TestMain pins a variant that is not the
// fallback — a pin naming rain would be indistinguishable from a pin this never
// understood.
func TestParseSplashEnvVariant(t *testing.T) {
	// Unset is the only case that reports "no override".
	v, ok := parseSplashEnvVariant("")
	require.False(t, ok, "an unset override must not claim to be one")
	require.Equal(t, splashDefaultVariant, v)

	for s, want := range map[string]splashVariant{
		"rain": splashVariantRain, "tunnel": splashVariantTunnel, "ripple": splashVariantRipple,
		// The historical dev letters. They are kept as they are — f/g/h is what the
		// screenshot recipes and the notes use — so a–e are simply gone with the
		// fields they named, and i is next.
		"f": splashVariantRain, "g": splashVariantTunnel, "h": splashVariantRipple,
	} {
		v, ok := parseSplashEnvVariant(s)
		require.Truef(t, ok, "%q must set an override", s)
		require.Equalf(t, want, v, "%q", s)
	}

	// Junk, a retired name and a retired letter are indistinguishable *here*, by
	// design: all of them mean "an override was set" and resolve to the fallback.
	for _, s := range []string{"banana", "nebula", "plasma", "a", "e", "legacy", "Rain", " rain"} {
		v, ok := parseSplashEnvVariant(s)
		require.Truef(t, ok, "%q must report an override was set", s)
		require.Equalf(t, splashDefaultVariant, v, "%q must fall through to the fallback", s)
	}
}

// TestLookupSplashVariantKnowsOnlyWhatItShips is the half of the parse the
// fall-through cannot express, and it is the only place the letters are really
// pinned.
//
// The test above cannot do it. parseSplashEnvVariant answers an unknown string
// with (splashDefaultVariant, true), and the fallback is rain — so "f" and
// "banana" return exactly the same pair, and deleting case "f" leaves that test
// green while breaking the letter every screenshot recipe in the notes uses. The
// vocabulary needs a boundary that distinguishes "known" from "resolved to the
// fallback", which is what lookupSplashVariant is for.
func TestLookupSplashVariantKnowsOnlyWhatItShips(t *testing.T) {
	for s, want := range map[string]splashVariant{
		"rain": splashVariantRain, "tunnel": splashVariantTunnel, "ripple": splashVariantRipple,
		"f": splashVariantRain, "g": splashVariantTunnel, "h": splashVariantRipple,
	} {
		v, ok := lookupSplashVariant(s)
		require.Truef(t, ok, "%q is a name this build ships", s)
		require.Equalf(t, want, v, "%q", s)
	}
	// Retired names and letters are not known — including the ones that would
	// resolve to the fallback anyway, which is exactly what the layer above hides.
	for _, s := range []string{"banana", "", "nebula", "braille", "contours", "julia",
		"mandala", "plasma", "legacy", "a", "b", "c", "d", "e", "i", "Rain", " rain"} {
		_, ok := lookupSplashVariant(s)
		require.Falsef(t, ok, "%q must not be a known name", s)
	}
}
