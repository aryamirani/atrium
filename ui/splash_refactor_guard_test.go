package ui

// Temporary refactor guard: SHA-256 fingerprints of the pre-refactor renderer
// output, captured at the commit before the two-pass restructure. Proves the
// restructure is byte-identical for the legacy variant. Deleted before merge
// (fingerprints are arch-sensitive via FMA, so this must never reach CI).

import (
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSplashRefactorFingerprint(t *testing.T) {
	pal := splashTestPalette()
	for _, c := range []struct {
		w, h, frame int
		want        string
	}{
		{80, 30, 0, "e2a87f7f7497f05bf6da7c3613f942920304ff2a0d428641ee69b76e34d0e5b1"},
		{80, 30, 7, "59ae3eb61080ef3c2aeb80b39179dc8686d928ef31a73cacf06c6aee7899a291"},
		{120, 40, 13, "2827b6ce611303edc890debef387817ebcc2f63cb62934b572216a36968dfeec"},
		{50, 18, 3, "ae7e1b2eb588936ee87bb1e48a58fd23c722f03b53025310c865950bff382bfa"},
		{73, 27, 999, "8999f16e6f1c2efb004a1a3b403cb1dc5b91d4f3832fa7addd63e7f084f6a7f3"},
	} {
		out := renderSplashField(c.w, c.h, c.frame, pal, centeredClearing(c.h, c.w/4, c.h/6), splashVariantLegacy)
		got := fmt.Sprintf("%x", sha256.Sum256([]byte(out)))
		require.Equalf(t, c.want, got, "legacy output changed at %dx%d frame %d", c.w, c.h, c.frame)
	}
}
