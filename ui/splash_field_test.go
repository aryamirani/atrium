package ui

import (
	"math"
	"testing"

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

// TestSplashValNoiseRange checks output stays in [0,1) over a spread of
// coordinates, including negatives and non-integer positions.
func TestSplashValNoiseRange(t *testing.T) {
	for i := 0; i < 500; i++ {
		x := (float64(i) - 250) * 0.377
		y := (float64(i%37) - 18) * 1.713
		n := splashValNoise(x, y, 0x27D4EB2F)
		require.GreaterOrEqualf(t, n, 0.0, "noise(%f,%f) below 0", x, y)
		require.Lessf(t, n, 1.0, "noise(%f,%f) at or above 1", x, y)
	}
}

// TestSplashValNoiseContinuity checks the smoothstep interpolation: a small
// step in the domain must produce a small step in the value (no jumps at
// lattice boundaries), which is what keeps the rendered gas free of seams.
func TestSplashValNoiseContinuity(t *testing.T) {
	const eps = 1e-4
	for i := 0; i < 400; i++ {
		// March across several lattice cells, deliberately crossing integers.
		x := -3.0 + float64(i)*0.017
		y := 2.5 - float64(i)*0.011
		a := splashValNoise(x, y, 0x165667B1)
		b := splashValNoise(x+eps, y+eps, 0x165667B1)
		require.InDeltaf(t, a, b, 0.01, "discontinuity near (%f,%f)", x, y)
	}
}

// TestSplashValNoiseAnchorsLattice pins the interpolation contract: at exact
// lattice points the noise equals the lattice value itself.
func TestSplashValNoiseAnchorsLattice(t *testing.T) {
	for _, p := range [][2]int32{{0, 0}, {3, -2}, {-7, 5}} {
		require.InDelta(t, latticeVal(p[0], p[1], 42),
			splashValNoise(float64(p[0]), float64(p[1]), 42), 1e-12)
	}
}

func BenchmarkSplashValNoise(b *testing.B) {
	var sink float64
	for i := 0; i < b.N; i++ {
		sink += splashValNoise(float64(i)*0.13, float64(i%97)*0.29, 0x9E3779B9)
	}
	_ = sink
}
