package ui

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestShippedVariantsFieldPolicy pins the per-variant decisions that render
// differently but break no invariant — the ones nothing else can fail on.
//
// It is the sibling of TestShippedVariantsLumRange, and it exists because that
// test's shape turned out to cover only one of four such decisions. The other
// three were reachable by mutation on a green tree: flipping ripple's stars or
// dither, or dropping it from structured(), each left `go test ./...` entirely
// passing. That is the exact failure mode the lumRange table was written for —
// "a variant opted in by accident six months from now" — so the answer is the
// same table, extended to the rest of the policy.
//
// structured() is the sharpest of the three, and it is worth naming why it needs
// pinning *here* rather than where it is spent. TestStructuredClearingLeavesNoUncoveredRow
// is the test that cares, but it opens with `if !v.structured() { continue }` —
// it checks that variants which *claim* to be structured have no uncovered rows,
// so a variant that stops claiming is not caught by it, it is excused from it. A
// field that quietly reverts to organic takes the text clearing back, and on
// ripple that means an ellipse biting an arc out of every ring that crosses the
// wordmark — see structured()'s own comment on why a broken circle is not
// sparser weather.
//
// Literals rather than reads of baseOps, for the same reason the lumRange table
// spells its values out: a table that asks the code what it does and then agrees
// is not a change-detector, it is a mirror.
func TestShippedVariantsFieldPolicy(t *testing.T) {
	type policy struct {
		structured bool // takes no text clearing
		stars      bool // draws the fixed twinkling starfield
		dither     bool // sub-glyph-step noise against banding
		hueIsAux   bool // spends aux straight as the gradient position
	}
	want := map[splashVariant]policy{
		// The organic fields: gas that drifts and fades, so a clearing reads as
		// gas parting, stars read as sky behind it, and dither smooths the wash.
		// Their hue is the swirl mix over screen position.
		splashVariantLegacy:  {structured: false, stars: true, dither: false, hueIsAux: false},
		splashVariantFBM:     {structured: false, stars: true, dither: true, hueIsAux: false},
		splashVariantBraille: {structured: false, stars: true, dither: true, hueIsAux: false},
		splashVariantFlow:    {structured: false, stars: true, dither: true, hueIsAux: false},
		splashVariantJulia:   {structured: false, stars: true, dither: true, hueIsAux: false},
		splashVariantMandala: {structured: false, stars: true, dither: true, hueIsAux: false},
		// Rain and the tunnel are the moving fields: the eye tracks them, so a
		// fixed star reads as a stuck pixel, and their marks are lines rather than
		// a wash — dither eats them instead of smoothing them. Rain's false here is
		// the one entry that is not a claim about a render: it never reaches
		// splashColorIdx at all, so the predicate is unread for it and false is what
		// "no opinion" spells. See hueIsAux on why that is a predicate and not an
		// ops field.
		splashVariantRain:   {structured: true, stars: false, dither: false, hueIsAux: false},
		splashVariantTunnel: {structured: true, stars: false, dither: false, hueIsAux: true},
		// Ripple is structured like them and dithers like them — a ring is a thin
		// closed line — but it is the one field that is both structured and starry,
		// because nothing in it travels except the rings and a still pool reflects a
		// still sky. That combination is unique, so nothing else's table entry would
		// notice it changing.
		splashVariantRipple: {structured: true, stars: true, dither: false, hueIsAux: true},
	}
	require.Len(t, want, int(splashVariantCount),
		"every variant needs an explicit policy here — a new one must make these "+
			"choices, not inherit whatever the zero value happens to be")
	for v := splashVariant(0); v < splashVariantCount; v++ {
		w, ok := want[v]
		require.Truef(t, ok, "variant %d is missing from the table", int(v))
		require.Equalf(t, w.structured, v.structured(),
			"variant %d's shipped structured(): it decides whether the text clearing "+
				"punches a hole in this field", int(v))
		require.Equalf(t, w.stars, v.ops().stars, "variant %d's shipped ops.stars", int(v))
		require.Equalf(t, w.dither, v.ops().dither, "variant %d's shipped ops.dither", int(v))
		require.Equalf(t, w.hueIsAux, v.hueIsAux(),
			"variant %d's shipped hueIsAux(): it decides whether hue follows the field "+
				"or the cell's address", int(v))
	}
}
