package ui

import (
	"math"
	"strings"
	"sync"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
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

// splashTestVariants enumerates every variant for contract loops, with names
// for failure messages. Hand-maintained — TestSplashTestVariantsCoversEnum is
// what keeps it honest.
func splashTestVariants() map[string]splashVariant {
	return map[string]splashVariant{
		"rain":   splashVariantRain,
		"tunnel": splashVariantTunnel,
		"ripple": splashVariantRipple,
	}
}

// TestSplashTestVariantsCoversEnum guards the map above, which is the entry
// point to this package's two per-variant sweeps: TestSplashVariantsContract
// (determinism, bounds, blank borders, frame-to-frame animation) and
// BenchmarkRenderSplashVariants (the frame budget). Both iterate the map, so a
// variant left out of it is not partially covered — it is invisible to both,
// and nothing fails to say so. Adding one is exactly when a contract breach is
// most likely and least likely to be noticed.
func TestSplashTestVariantsCoversEnum(t *testing.T) {
	seen := make(map[splashVariant]bool, len(splashTestVariants()))
	for name, v := range splashTestVariants() {
		require.Falsef(t, seen[v], "%q duplicates a variant already in the map", name)
		seen[v] = true
	}
	for v := splashVariant(0); v < splashVariantCount; v++ {
		require.Truef(t, seen[v],
			"variant %d is missing from splashTestVariants, so it escapes both "+
				"the contract loop and the benchmark", int(v))
	}
}

// TestSplashVariantsContract loops every variant through the core field
// contract: determinism, exact w×h bounds, frame-to-frame animation, and
// fully-blank first/last rows (the edge vignette's by-construction guarantee,
// which no Pass-1 processing may break).
func TestSplashVariantsContract(t *testing.T) {
	pal := splashTestPalette()
	w, h := 80, 30
	for name, v := range splashTestVariants() {
		a := renderSplashField(w, h, 5, pal, centeredFocalRow(h), v)
		b := renderSplashField(w, h, 5, pal, centeredFocalRow(h), v)
		require.Equalf(t, a, b, "%s: same inputs must render identically", name)
		next := renderSplashField(w, h, 6, pal, centeredFocalRow(h), v)
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

// TestRenderSplashFieldConcurrent guards the two-panes case (preview and
// terminal both render the splash) and any future buffer pooling: concurrent
// renders of the same frame must all be byte-identical.
func TestRenderSplashFieldConcurrent(t *testing.T) {
	pal := splashTestPalette()
	want := renderSplashField(80, 30, 9, pal, centeredFocalRow(30), splashDefaultVariant)
	var wg sync.WaitGroup
	mismatch := make(chan string, 8)
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 20; i++ {
				if got := renderSplashField(80, 30, 9, pal, centeredFocalRow(30), splashDefaultVariant); got != want {
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

// TestSplashPointFnRange checks every variant's point evaluator over a spread
// of cells and phases.
//
// Driven off splashFieldAt rather than a list of evaluators, so a variant is
// covered the moment it is dispatched — this used to be two hand-maintained
// lists (one for fBm, one for the fractals) that a new variant had to be
// remembered into, which is why the gap below went unnoticed for so long.
//
// val is [0,1] for every variant: Pass 2's contrast curve, glyph quantization
// and envelope all assume it, and a breach silently clips rather than crashing.
//
// aux is [0,1] for every variant too, and that is newly universal. It is the
// field's hue helper, and splashColorIdx now spends it straight as the gradient
// position, so a field that leaves the range renders a flat end of the gradient.
// The range used to be a contract only where the hue mix *weighted* aux — the
// retired legacy plasma passed an atan2 angle into a sine, which is meaningful
// for any real — and that carve-out went with the variant.
func TestSplashPointFnRange(t *testing.T) {
	for name, v := range splashTestVariants() {
		at := splashFieldAt(v, 96)
		for i := 0; i < 600; i++ {
			col, row := i%60, i/60
			dx := (float64(col) - 30) * 1.37
			dy := (float64(row) - 5) * 2.9
			phase := float64(i%23) * 1.7
			val, aux := at(col, row, dx, dy, phase)
			require.GreaterOrEqualf(t, val, 0.0, "%s val at (%d,%d,p%f)", name, col, row, phase)
			require.LessOrEqualf(t, val, 1.0, "%s val at (%d,%d,p%f)", name, col, row, phase)
			require.GreaterOrEqualf(t, aux, 0.0, "%s aux at (%d,%d,p%f)", name, col, row, phase)
			require.LessOrEqualf(t, aux, 1.0, "%s aux at (%d,%d,p%f)", name, col, row, phase)
		}
	}
}

// TestSplashRotationPickInBounds guards against a negative-index panic: a clock
// set before the Unix epoch yields a negative UnixNano(), and Go's signed % of
// that would produce a negative slice index. The pick must stay in [0, len) for
// every int64 seed, including negative and boundary values.
func TestSplashRotationPickInBounds(t *testing.T) {
	seeds := []int64{
		0, 1, -1, 42, -42,
		math.MinInt64, math.MaxInt64,
		-1234567890123456789, 1234567890123456789,
	}
	for _, seed := range seeds {
		require.NotPanics(t, func() { splashRotationPick(seed) },
			"splashRotationPick must not panic for seed %d", seed)
	}
}

// resetSplashSelection restores the process-wide splash selection after a test
// pins or re-rolls it, so package siblings see the pristine lazy-random state.
// (Rendering is shielded anyway — TestMain pins the env override, which trumps
// the selection — but the package state should stay canonical regardless.)
func resetSplashSelection(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		splashRandomMode, splashPicked, splashPick = true, false, splashDefaultVariant
	})
}

// TestSplashVariantNamesCoverAllVariants guards the settings vocabulary: every
// rotation variant must be pinnable by name, so a future variant can't ship
// unreachable from the settings panel. It used to add the legacy baseline by
// hand, which was the one pinnable variant outside the pool; V5 retired it, and
// TestSplashRotationCoversEveryVariant now pins that the pool is the whole enum.
func TestSplashVariantNamesCoverAllVariants(t *testing.T) {
	named := make(map[splashVariant]bool, len(splashVariantNames))
	for _, v := range splashVariantNames {
		named[v] = true
	}
	for _, v := range splashRotation {
		require.True(t, named[v], "rotation variant %d has no config name", v)
	}
}

// TestSetSplashVariantPinsKnownNames locks the name→generator mapping and that
// a pinned name leaves random mode.
func TestSetSplashVariantPinsKnownNames(t *testing.T) {
	resetSplashSelection(t)
	for name, want := range splashVariantNames {
		SetSplashVariant(name)
		require.False(t, splashRandomMode, "%q must pin, not stay random", name)
		require.True(t, splashPicked)
		require.Equal(t, want, splashPick, "name %q", name)
	}
}

// TestSetSplashVariantRandomRerolls pins random-mode semantics: anything that
// isn't a known name (config.SplashRandom, junk) enters random mode with an
// immediate rotation draw, and RerollSplashVariant only re-draws in that mode —
// a pinned pattern survives a screensaver activation untouched.
func TestSetSplashVariantRandomRerolls(t *testing.T) {
	resetSplashSelection(t)

	SetSplashVariant("ripple")
	RerollSplashVariant()
	require.Equal(t, splashVariantRipple, splashPick, "reroll must not disturb a pinned pattern")

	SetSplashVariant("random")
	require.True(t, splashRandomMode)
	require.True(t, splashPicked, "random mode draws immediately so both panes agree")
	require.Contains(t, splashRotation, splashPick, "the draw must come from the rotation pool")

	// Every re-roll must land somewhere in the pool *and* move: the screensaver
	// re-rolls per activation, so a draw that repeats the current pattern reads
	// as a dead keypress. Looping catches a seed-dependent repeat that a single
	// call would only hit ~1/len of the time.
	for i := 0; i < 40; i++ {
		prev := splashPick
		RerollSplashVariant()
		require.Contains(t, splashRotation, splashPick)
		require.NotEqual(t, prev, splashPick, "a re-roll must change the pattern (iteration %d)", i)
	}
}

// TestSplashRotationRerollUniformOverRemainder pins the exclusion arithmetic
// directly, seed by seed: every consecutive seed maps to some variant other
// than cur, and sweeping a full period hits each of the other variants exactly
// once — i.e. skipping cur's slot doesn't bias the draw toward its neighbour.
func TestSplashRotationRerollUniformOverRemainder(t *testing.T) {
	for _, cur := range splashRotation {
		counts := map[splashVariant]int{}
		for seed := int64(0); seed < int64(len(splashRotation)-1); seed++ {
			got := splashRotationReroll(seed, cur)
			require.NotEqual(t, cur, got, "cur=%d seed=%d must be excluded", cur, seed)
			counts[got]++
		}
		require.Len(t, counts, len(splashRotation)-1, "cur=%d: every other variant must be reachable", cur)
		for v, n := range counts {
			require.Equal(t, 1, n, "cur=%d variant=%d drawn %d times over one period", cur, v, n)
		}
	}
}

// TestSplashRotationRerollFallsBackOutsidePool: a cur the pool doesn't contain
// has nothing to exclude, so it degrades to a plain draw rather than looping or
// panicking. Negative seeds must stay in bounds here too.
//
// Reaching it takes a cast now: the out-of-pool cur used to occur naturally (the
// legacy baseline was pinnable but never drawn), and since V5 every shipped
// variant is in the pool and the unpicked zero value is rain.
//
// "Stays in the pool" is not the assertion, because it cannot fail — deleting the
// ci < 0 arm keeps every result in the pool, since idx >= -1 always holds and the
// increment lands inside the slice regardless. What breaks without the arm is the
// *distribution*: the draw steps past a slot that isn't there and can never
// return splashRotation[0] again. So the claim is that an out-of-pool cur excludes
// nothing, and it takes a sweep to see it.
func TestSplashRotationRerollFallsBackOutsidePool(t *testing.T) {
	notAVariant := splashVariantCount
	seen := map[splashVariant]bool{}
	for seed := int64(0); seed < int64(len(splashRotation))*20; seed++ {
		got := splashRotationReroll(seed, notAVariant)
		require.Containsf(t, splashRotation, got, "out-of-pool cur, seed %d", seed)
		seen[got] = true
	}
	require.Lenf(t, seen, len(splashRotation),
		"an out-of-pool cur has nothing to exclude, so every variant must still be "+
			"drawable; only %d of %d were", len(seen), len(splashRotation))

	// Negative and boundary seeds must stay in bounds here too.
	for _, seed := range []int64{-1, -(1 << 62), 1 << 62} {
		require.Containsf(t, splashRotation, splashRotationReroll(seed, notAVariant),
			"out-of-pool cur, seed %d", seed)
	}
}

// TestSplashSelectionConcurrent is TestRenderSplashFieldConcurrent's counterpart
// for the mutable selection: renderSplashField is pure and provably safe, but
// SetSplashVariant / RerollSplashVariant write process-wide state that a
// sync.OnceValue used to make safe by construction. Under -race this pins
// splashSelMu actually covering them; without it the package's own
// concurrent-render posture would rest on a comment.
//
// splashActiveVariant short-circuits on the env override TestMain pins, so this
// exercises the writers plus a locked read rather than the lazy-seed path.
func TestSplashSelectionConcurrent(t *testing.T) {
	resetSplashSelection(t)

	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				switch (g + i) % 3 {
				case 0:
					SetSplashVariant("random")
				case 1:
					SetSplashVariant("tunnel")
				default:
					RerollSplashVariant()
				}
				_ = splashActiveVariant()
			}
		}(g)
	}
	wg.Wait()

	splashSelMu.Lock()
	defer splashSelMu.Unlock()
	require.True(t, splashPicked, "every path above picks")
	require.Contains(t, splashRotation, splashPick,
		"the selection must never be torn into an out-of-pool value")
}

// TestSplashEnvOverrideTrumpsSelection relies on TestMain pinning
// ATRIUM_SPLASH_VARIANT=tunnel: whatever the config pins, the dev override wins
// in splashActiveVariant — which is exactly what keeps every String()-path test
// deterministic even if a future test seeds a config with a splash value.
//
// It is also the only thing that notices if that pin stops being honoured, and
// only because the pin does not name the fallback. parseSplashEnvVariant answers
// an unrecognized value with (splashDefaultVariant, true), so a pin reading
// "rain" would be indistinguishable from a pin the parse never understood. This
// is why TestMain names a variant that is not rain.
func TestSplashEnvOverrideTrumpsSelection(t *testing.T) {
	resetSplashSelection(t)
	SetSplashVariant("ripple")
	require.Equal(t, splashVariantTunnel, splashActiveVariant(),
		"the env override (pinned to \"tunnel\" in TestMain) must trump the config selection")
}

// BenchmarkRenderSplashVariants tracks the ≤3ms/frame budget per variant
// (checked manually; never a timed assertion — a timed assertion would flake on
// shared CI).
//
// Truecolor is forced because the profile decides what this measures. A
// benchmark binary's stdout is not a TTY, so termenv resolves to Ascii, every
// SGR affix is the empty string, and the emitter is timed with nothing to emit.
// That skews less than it used to — the affix cache made emission cheap either
// way — but it is exactly why the colorless default is the wrong budget check:
// the Render-per-run cost the cache replaced measured 3.7ms/frame at 240×60
// under Ascii and 6.3ms in a real terminal. Time the frame the user actually
// renders, or a regression of that shape reads ~40% cheaper than it is.
//
// Two sizes, because they answer different questions. 80×30 is the reference
// preview pane. 240×60 is the *screensaver*, which renders full-window: it is
// ~6× the cells but ~8× the cost, and it is measured against a 16.7ms/60fps
// frame — so a variant can sit comfortably inside the 80×30 budget and still be
// a slideshow full-screen. Benchmark both before shipping one.
func BenchmarkRenderSplashVariants(b *testing.B) {
	forceBenchTrueColor(b)
	pal := splashTestPalette()
	sizes := []struct {
		name string
		w, h int
	}{
		{"80x30", 80, 30},
		{"240x60", 240, 60}, // the full-window screensaver
	}
	for _, s := range sizes {
		for name, v := range splashTestVariants() {
			b.Run(s.name+"/"+name, func(b *testing.B) {
				b.ReportAllocs()
				focalRow := centeredFocalRow(s.h)
				for i := 0; i < b.N; i++ {
					_ = renderSplashField(s.w, s.h, i, pal, focalRow, v)
				}
			})
		}
	}
}

// forceBenchTrueColor pins the color profile for a benchmark so the SGR path
// runs, and asserts it took — a silent degrade would turn every number the
// benchmark reports into a measurement of the wrong code.
func forceBenchTrueColor(b *testing.B) {
	b.Helper()
	prof := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	b.Cleanup(func() { lipgloss.SetColorProfile(prof) })

	// The tunnel is the canary because it cannot be blank: every cell of its wall
	// is lit at every frame. A sparse field could emit no SGR at frame 1 for an
	// honest reason and fail this for the wrong one.
	out := renderSplashField(80, 30, 1, splashTestPalette(),
		centeredFocalRow(30), splashVariantTunnel)
	if !strings.Contains(out, "\x1b[38;2;") {
		b.Fatal("no truecolor SGR emitted: benchmark would measure the colorless path")
	}
}
