package ui

import (
	"strings"
	"testing"

	"github.com/ZviBaratz/atrium/ui/theme"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
	"github.com/stretchr/testify/require"
)

// splashTestPalette is a fixed truecolor palette (tokyo-night's hues) so the
// splash tests never depend on the global active theme.
func splashTestPalette() theme.Palette {
	return theme.Palette{
		Danger: lipgloss.Color("#f7768e"),
		Purple: lipgloss.Color("#bb9af7"),
		Accent: lipgloss.Color("#7aa2f7"),
		Cyan:   lipgloss.Color("#7dcfff"),
		Fg:     lipgloss.Color("#c0caf5"),
	}
}

// stripLines strips SGR and splits into visible lines.
func stripLines(s string) []string {
	return strings.Split(ansi.Strip(s), "\n")
}

// centeredFocalRow is the focal row for a pane whose wordmark sits at its
// vertical centre — which is what splashScene builds, and what keeps the
// round-vignette assertions valid.
func centeredFocalRow(h int) int { return (h - 1) / 2 }

// overlayCenter drops fg onto the center of bg via the production overlayAt. A
// test-only convenience: the real render (renderSplashScene) positions the
// wordmark and message explicitly, so nothing outside tests needs centering.
func overlayCenter(bg, fg string) string {
	fgLines, fgWidth := splashLines(fg)
	bgLines, bgWidth := splashLines(bg)
	return overlayAt(bg, fg, (bgWidth-fgWidth)/2, (len(bgLines)-len(fgLines))/2)
}

// TestRenderSplashFieldDeterministic locks the pure-function contract: identical
// inputs must produce byte-identical output (so the field is snapshot-safe and
// the tick can drive it without hidden state).
func TestRenderSplashFieldDeterministic(t *testing.T) {
	pal := splashTestPalette()
	a := renderSplashField(80, 30, 5, pal, centeredFocalRow(30), splashDefaultVariant)
	b := renderSplashField(80, 30, 5, pal, centeredFocalRow(30), splashDefaultVariant)
	require.Equal(t, a, b, "same inputs must render identically")
	require.NotEmpty(t, a)
}

// TestRenderSplashFieldBounds is the view-bounds invariant: exactly h rows, each
// exactly w visible cells, across a spread of sizes (even/odd, wide/tall). This
// is what lets String() drop the field into the pane box without overflow (#251).
func TestRenderSplashFieldBounds(t *testing.T) {
	pal := splashTestPalette()
	sizes := [][2]int{{50, 18}, {66, 20}, {80, 30}, {120, 40}, {51, 19}, {73, 27}}
	for _, s := range sizes {
		w, h := s[0], s[1]
		field := renderSplashField(w, h, 3, pal, centeredFocalRow(h), splashDefaultVariant)
		lines := strings.Split(field, "\n")
		require.Lenf(t, lines, h, "%dx%d: line count", w, h)
		for i, l := range lines {
			require.Equalf(t, w, lipgloss.Width(l), "%dx%d: line %d width", w, h, i)
		}
	}
}

// TestRenderSplashFieldAnimates guards the drift wiring: advancing the frame must
// change the rendered field (otherwise the "slow drift" is dead).
func TestRenderSplashFieldAnimates(t *testing.T) {
	pal := splashTestPalette()
	f0 := renderSplashField(80, 30, 3, pal, centeredFocalRow(30), splashDefaultVariant)
	f1 := renderSplashField(80, 30, 4, pal, centeredFocalRow(30), splashDefaultVariant)
	require.NotEqual(t, f0, f1, "consecutive frames must differ")
}

// TestRenderSplashFieldVignetteCorners locks the edge vignette: the outermost
// rows and columns fade fully to blank at the pane border, so the full-bleed
// field softens into the edges instead of hard-clipping into a lit rectangle.
func TestRenderSplashFieldVignetteCorners(t *testing.T) {
	pal := splashTestPalette()
	w, h := 80, 30
	lines := stripLines(renderSplashField(w, h, 3, pal, centeredFocalRow(h), splashDefaultVariant))
	require.Len(t, lines, h)
	// The border rows fade to zero, so the first and last rows are entirely blank.
	require.Equal(t, strings.Repeat(" ", w), lines[0], "top row must be blank")
	require.Equal(t, strings.Repeat(" ", w), lines[h-1], "bottom row must be blank")
	// And each corner is blank regardless.
	for _, rc := range [][2]int{{0, 0}, {0, w - 1}, {h - 1, 0}, {h - 1, w - 1}} {
		require.Equalf(t, byte(' '), lines[rc[0]][rc[1]], "corner (%d,%d)", rc[0], rc[1])
	}
}

// TestOverlayIsOpaque is the fact the splash's whole text policy rests on: the
// text does not need the field cleared out from under it. overlayAt writes each
// overlaid line's cells wholesale — spaces included — so the text always covers
// its own footprint whatever the field draws underneath.
//
// This is why no variant takes a clearing. The splash carried one for a long
// time, and its name oversold it: it never prevented bleed-through, it only
// opened a margin of quiet *around* the text. That margin was charm on a field
// that faded into it and a defect on one that didn't — a band of missing streams
// with nothing drawn to account for them — and V5 retired the fields it flattered.
// If overlayAt ever became a fading or transparent composite, the field would
// start showing through the message's spaces and this policy would need
// revisiting.
func TestOverlayIsOpaque(t *testing.T) {
	bg := strings.Join([]string{
		strings.Repeat("#", 20),
		strings.Repeat("#", 20),
	}, "\n")
	// A foreground whose interior is a space: if overlays were transparent, the
	// background's # would survive in the middle.
	got := ansi.Strip(overlayAt(bg, "A B", 5, 0))
	first := strings.Split(got, "\n")[0]
	require.Equal(t, "#####A B############", first,
		"overlayAt must write the overlaid line's spaces over the background, not through it")
}

// TestBannerIsSolid pins the other half of that fact. The banner fills with ░
// rather than spaces, so it is opaque across its whole box on every row — there
// are no letter gaps for a field to show through even in principle. If a future
// banner introduced spaces, the field would start rendering inside the wordmark's
// counters and the no-clearing policy would need revisiting.
func TestBannerIsSolid(t *testing.T) {
	banner := ansi.Strip(trimBlankLines(FallbackBanner()))
	for i, line := range strings.Split(banner, "\n") {
		require.NotContainsf(t, line, " ",
			"banner row %d contains a space; the wordmark is assumed solid "+
				"(see TestOverlayIsOpaque)", i)
	}
}

// TestOverlayCenterComposites checks the fade-less compositor drops fg onto the
// center of the field while preserving the field's exact w×h bounds (the whole
// point of doing it before the #251 clamp).
func TestOverlayCenterComposites(t *testing.T) {
	pal := splashTestPalette()
	w, h := 60, 20
	field := renderSplashField(w, h, 3, pal, centeredFocalRow(h), splashDefaultVariant)
	fg := "ABCDEF"
	out := overlayCenter(field, fg)
	require.Contains(t, ansi.Strip(out), "ABCDEF", "fg must survive compositing")
	lines := strings.Split(out, "\n")
	require.Len(t, lines, h, "compositing must preserve height")
	for i, l := range lines {
		require.LessOrEqualf(t, lipgloss.Width(l), w, "composited line %d width", i)
	}
}

// fieldGlyphs are glyphs that only the splash field emits — none appear in the
// wordmark art (box-drawing + ░) or the onboarding message — so their presence in
// a stripped render proves the field engaged, and their absence proves the plain
// fallback did.
//
// It is the tunnel's mark, because the tunnel is what TestMain pins. That is a
// real coupling and worth stating: this probe reads whatever variant the
// String()-path tests resolve to, and each of the three has its own vocabulary —
// rain draws katakana, ripple the density ramp, and the tunnel exactly one glyph,
// since its lumRange is 1 and every lit cell is a full-weight mark shaded purely
// by colour. Re-pin TestMain and this must move with it. The tunnel is also the
// variant that makes the probe honest at every size: its wall lights 81–85% of
// the pane across the range these tests render (measured 80.8% at 50×18, 82.7% at
// 80×30, 85.3% at 240×60), so "no field glyphs" cannot quietly mean "the field
// rendered, somewhere else".
const fieldGlyphs = "@"

// TestPreviewSplashStringBounds drives the real idle path end to end
// (UpdateContent(nil) → setSplashState → String) and locks the #251 box
// contract at the String level: exactly h rows, each no wider than w, across a
// spread of sizes — with the wordmark and full onboarding message both present.
func TestPreviewSplashStringBounds(t *testing.T) {
	const msg = "No agents running yet"
	for _, s := range [][2]int{{50, 18}, {66, 20}, {80, 30}, {120, 40}, {51, 19}} {
		w, h := s[0], s[1]
		p := NewPreviewPane()
		p.SetSize(w, h)
		p.SetSplashFrame(6)
		require.NoError(t, p.UpdateContent(nil))
		require.True(t, p.previewState.splash, "%dx%d: idle screen must set splash", w, h)

		out := p.String()
		lines := strings.Split(out, "\n")
		require.Lenf(t, lines, h, "%dx%d: line count", w, h)
		for i, l := range lines {
			require.LessOrEqualf(t, lipgloss.Width(l), w, "%dx%d: line %d width", w, h, i)
		}
		stripped := ansi.Strip(out)
		require.Containsf(t, stripped, msg, "%dx%d: onboarding message must survive", w, h)
		require.Containsf(t, stripped, "█", "%dx%d: wordmark must survive", w, h)
		require.Truef(t, strings.ContainsAny(stripped, fieldGlyphs),
			"%dx%d: splash field must render behind the wordmark", w, h)
	}
}

// TestPreviewSplashFallbackBelowFloor guards the size gate: below the splashFits
// floor the idle screen must fall back to the plain centered placeholder —
// bounded, panic-free, and with no field glyphs — never a clipped field. The
// placeholder keeps the wordmark only where it fits; narrower than its 48 cols it
// is the message alone (see fallbackBlock), which is why this asserts the field is
// gone rather than that the wordmark is present.
func TestPreviewSplashFallbackBelowFloor(t *testing.T) {
	for _, s := range [][2]int{{49, 18}, {50, 17}, {40, 12}, {49, 17}, {10, 4}} {
		w, h := s[0], s[1]
		p := NewPreviewPane()
		p.SetSize(w, h)
		p.SetSplashFrame(6)
		require.NoError(t, p.UpdateContent(nil))

		out := p.String()
		lines := strings.Split(out, "\n")
		require.LessOrEqualf(t, len(lines), h, "%dx%d: too many lines", w, h)
		for _, l := range lines {
			require.LessOrEqualf(t, lipgloss.Width(l), w, "%dx%d: line too wide", w, h)
		}
		require.Falsef(t, strings.ContainsAny(ansi.Strip(out), fieldGlyphs),
			"%dx%d: below the floor must render the plain placeholder, not the field", w, h)
	}
}

// TestRenderSplashFieldExtremes is the panic-safety net: degenerate sizes must
// return "" or a bounded block, never panic (the caller gates on splashFits, but
// the generator must be robust on its own).
func TestRenderSplashFieldExtremes(t *testing.T) {
	pal := splashTestPalette()
	sizes := [][2]int{{0, 0}, {1, 1}, {2, 2}, {1, 40}, {40, 1}, {50, 1}, {3, 3}, {51, 19}}
	for _, s := range sizes {
		w, h := s[0], s[1]
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("renderSplashField(%d,%d) panicked: %v", w, h, r)
				}
			}()
			out := renderSplashField(w, h, 2, pal, centeredFocalRow(h), splashDefaultVariant)
			if out == "" {
				return
			}
			lines := strings.Split(out, "\n")
			require.LessOrEqualf(t, len(lines), h, "%dx%d: too many lines", w, h)
			for _, l := range lines {
				require.LessOrEqualf(t, lipgloss.Width(l), w, "%dx%d: line too wide", w, h)
			}
		}()
	}
}

// TestSplashLUTThemeAnchored locks the gradient to the theme: the ramp starts at
// the warm anchor (pink/Danger), ends at cyan, and the rim hue flows into the
// upper stops (so swapping cyan changes the colors). It asserts at the LUT level
// because lipgloss strips truecolor to plain text in a no-TTY test process, so
// the rendered field carries no color to compare.
func TestSplashLUTThemeAnchored(t *testing.T) {
	pal := splashTestPalette()
	lut := splashLUTFor(pal)
	require.Equal(t, pal.Danger, lut.colors[0], "core stop is the warm anchor")
	require.Equal(t, pal.Cyan, lut.colors[len(lut.colors)-1], "rim stop is theme cyan")

	other := pal
	other.Cyan = lipgloss.Color("#a6e3a1") // a distinctly different rim (catppuccin green)
	otherLUT := splashLUTFor(other)
	require.NotEqual(t, lut.colors, otherLUT.colors,
		"changing the rim hue must change the gradient")
	// The lower stops (warm→blue) are rim-independent; the upper stops must move.
	require.NotEqual(t, lut.colors[len(lut.colors)-4], otherLUT.colors[len(otherLUT.colors)-4],
		"the rim hue must reach the upper stops of the ramp")
}

// withColorProfile pins lipgloss's global color profile for one test and
// restores it afterward. The emitter's output is profile-dependent by
// construction, and a test binary's stdout is not a TTY — so the ambient
// profile is Ascii, and the SGR path never runs unless a test asks for it.
func withColorProfile(t *testing.T, prof termenv.Profile) {
	t.Helper()
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(prof)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
}

// TestSplashAffixBracketsMatchRender pins the invariant the whole emitter rests
// on: writing a style's cached prefix, then the cells, then its suffix must
// produce exactly what Style.Render would have produced for those cells.
// Splitting the SGR bracket out of lipgloss is only safe if the bracket really
// is a pure wrapper — for every stop, on every profile, including the colorless
// one where it has to collapse to plain text.
func TestSplashAffixBracketsMatchRender(t *testing.T) {
	profiles := map[string]termenv.Profile{
		"truecolor": termenv.TrueColor,
		"ansi256":   termenv.ANSI256,
		"ansi":      termenv.ANSI,
		"ascii":     termenv.Ascii,
	}
	// Spaces and a wide glyph included: the emitter brackets whatever it is given,
	// and lipgloss has space-sensitive paths for some attributes.
	contents := []string{"x", "▓▓▓", "   ", "⠿⠿", "·-=#"}
	for name, prof := range profiles {
		t.Run(name, func(t *testing.T) {
			withColorProfile(t, prof)
			// Built directly rather than via splashLUTFor: this asserts on the
			// affixes themselves; TestSplashLUTCacheTracksColorProfile covers
			// the cache's keying.
			lut := buildSplashLUT(splashTestPalette())
			require.Equal(t, len(lut.styles), lut.starIndex(),
				"the star sentinel must sit exactly one past the last gradient stop")
			for _, content := range contents {
				for i, st := range lut.styles {
					require.Equal(t, st.Render(content),
						lut.affix[i].prefix+content+lut.affix[i].suffix,
						"stop %d must bracket %q exactly as Render does", i, content)
				}
				require.Equal(t, lut.star.Render(content),
					lut.starAffix.prefix+content+lut.starAffix.suffix,
					"the star style must bracket %q exactly as Render does", content)
			}
			if prof == termenv.Ascii {
				require.Empty(t, lut.affix[1].prefix, "a colorless profile must degrade to plain")
				require.Empty(t, lut.affix[1].suffix, "a colorless profile must degrade to plain")
			}
		})
	}
}

// TestSplashLUTCacheTracksColorProfile guards the trap that baking the affixes
// introduces. Style.Render re-read the color profile on every call, so a LUT
// cached under one profile still rendered correctly under the next; the affixes
// are frozen at build time, so the cache has to key on the profile or it pins
// whichever one happened to build the entry. That matters most in tests: a test
// binary starts out colorless, so a stale entry would silently turn any later
// truecolor assertion into a test of plain text.
func TestSplashLUTCacheTracksColorProfile(t *testing.T) {
	pal := splashTestPalette()
	pal.Purple = lipgloss.Color("#123456") // a private cache entry, so other tests are unaffected

	withColorProfile(t, termenv.Ascii)
	require.Empty(t, splashLUTFor(pal).affix[1].prefix, "a colorless profile emits no SGR")

	withColorProfile(t, termenv.TrueColor)
	require.NotEmpty(t, splashLUTFor(pal).affix[1].prefix,
		"a LUT cached under Ascii must not pin the colorless path once the profile is truecolor")
}

// TestRenderSplashFieldColorByProfile pins the emitted bytes end-to-end: a
// truecolor terminal gets SGR-bracketed runs, a colorless one gets the very
// same glyphs with no escapes at all.
func TestRenderSplashFieldColorByProfile(t *testing.T) {
	pal := splashTestPalette()
	render := func() string {
		return renderSplashField(60, 20, 3, pal, centeredFocalRow(20), splashVariantTunnel)
	}

	withColorProfile(t, termenv.TrueColor)
	colored := render()
	require.Contains(t, colored, "\x1b[38;2;", "a truecolor profile must emit truecolor SGR")

	withColorProfile(t, termenv.Ascii)
	plain := render()
	require.NotContains(t, plain, "\x1b[", "a colorless profile must emit no escapes")

	require.Equal(t, plain, ansi.Strip(colored),
		"color must be the only difference — the glyphs are identical either way")
}

// TestSplashFitsExported pins the exported gate the screensaver entry uses —
// the same floor as the internal splashFits.
func TestSplashFitsExported(t *testing.T) {
	require.True(t, SplashFits(minSplashW, minSplashH))
	require.False(t, SplashFits(minSplashW-1, minSplashH))
	require.False(t, SplashFits(minSplashW, minSplashH-1))
}

// TestSplashScreensaverScene pins the full-window easter-egg scene: exact row
// count, rows within the pane width, deterministic over (size, frame), and no
// message line — the field flows uninterrupted below the wordmark instead of
// being blanked for guidance text nobody passed.
func TestSplashScreensaverScene(t *testing.T) {
	const w, h = 80, 30
	out := SplashScreensaver(w, h, 7)
	require.Equal(t, out, SplashScreensaver(w, h, 7), "same frame must render identically")

	lines := stripLines(out)
	require.Len(t, lines, h)
	for i, ln := range lines {
		require.LessOrEqual(t, lipgloss.Width(ln), w, "row %d overflows the window", i)
	}

	withMsg := splashScene(w, h, 7, "press n to start")
	require.NotContains(t, ansi.Strip(out), "press n", "the screensaver has no message line")
	require.NotEqual(t, out, withMsg,
		"the screensaver and the guided empty state must not render identically")
}

// TestFieldContrastIsStillHermite pins the distinction renderSplashField draws
// between a "full-range" contrast curve and an identity one. The curve every
// field now runs — smoothstep(0, 1, val) — clips nothing, which is the property a
// self-shading field needs. It does not pass values through untouched, and no
// window can: smoothstep is Hermite on the clamped parameter, so a {0,1} window
// still bends every interior value through t*t*(3-2t).
//
// The distinction is the point, and it is why the call is written out rather than
// dropped. "Identity" invites a reader to skip it, and skipping it would be a
// behaviour change — every surviving field carries its own gradient (rain's
// streams, the tunnel's fog, ripple's ring decay) and is tuned against this
// S-curved version of its output, not against its generator's raw values.
//
// It used to be a per-variant window in splashOps, ranging from the nebula's
// narrow 0.36–0.64 to this. V5 retired the fields that wanted a narrow one, all
// three survivors had independently chosen {0,1}, and the field became a constant
// and then a literal at the one call site.
func TestFieldContrastIsStillHermite(t *testing.T) {
	// It clips nothing: the endpoints come back exact.
	require.Zero(t, smoothstep(0, 1, 0), "the widest window must not lift the floor")
	require.Equal(t, 1.0, smoothstep(0, 1, 1), "the widest window must not clip the ceiling")

	// But the interior is bent. 0.5 is the fixed point an S-curve shares with the
	// identity, so it is the one probe that cannot witness anything.
	require.Equal(t, 0.5, smoothstep(0, 1, 0.5), "0.5 is Hermite's fixed point, not evidence")
	for _, x := range []float64{0.1, 0.25, 0.4, 0.6, 0.75, 0.9} {
		require.NotEqualf(t, x, smoothstep(0, 1, x),
			"a {0,1} window is not an identity at x=%v — it is still an S-curve", x)
	}
	// Concretely, and in the direction that matters: the curve pulls the dim half
	// down and the bright half up, which is contrast the caller did not ask for.
	require.Less(t, smoothstep(0, 1, 0.25), 0.25, "the S-curve must darken the dim half")
	require.Greater(t, smoothstep(0, 1, 0.75), 0.75, "the S-curve must brighten the bright half")

}

// TestFieldContrastReachesTheRender is the wiring half of the claim above, and
// both halves are needed. That test proves smoothstep(0, 1, ·) is an S-curve; it
// cannot prove Pass 2 *calls* it. Nothing else could either: the curve has one
// call site and every variant now takes the same one, so there is no second
// behaviour to compare against and no ops field left to pin. A reader who
// believes "full-range" means "identity" — which is exactly what the name invites,
// and why renderSplashField argues against it in a comment — could drop the call
// and every other test in this package would still pass.
//
// That is this package's signature bug: ops.dimToRim shipped declared and never
// read, and the guard that should have caught it was measuring the ops table
// instead of the wiring. So this decodes the emitted SGR back to a luminance stop
// and compares it against the documented pipeline evaluated independently —
// smoothstep, then splashShade's split — in rippleMeasurable's region, where the
// envelope is exactly 1 and the arithmetic is therefore complete.
//
// The second assertion is what makes it able to fail rather than merely pass: it
// counts the cells an identity contrast would land on a *different* stop, so the
// test is only trusted where it can tell the two apart.
func TestFieldContrastReachesTheRender(t *testing.T) {
	withColorProfile(t, termenv.TrueColor)
	const w, h, frame = 240, 60, 300
	stops, _ := shadeStopGrid(t, w, h, frame, splashTestPalette(), splashVariantRipple)
	vals := rippleFieldVals(w, h, frame)
	lumRange := splashVariantRipple.ops().lumRange

	// The pipeline renderSplashField documents, from val to emitted luminance stop.
	stopFor := func(intensity float64) int {
		_, lumT := splashShade(intensity, lumRange)
		return clampInt(int(lumT*float64(splashLumStops-1)), 0, splashLumStops-1)
	}

	checked, separable := 0, 0
	for row := 0; row < h; row++ {
		for col := 0; col < w; col++ {
			if !rippleMeasurable(col, row, w, h) || stops[row][col] <= 0 {
				continue
			}
			v := vals[row][col]
			checked++
			require.Equalf(t, stopFor(smoothstep(0, 1, v)), stops[row][col],
				"cell (%d,%d) at val %.4f rendered a stop the contrast curve does not "+
					"predict; Pass 2 is not running smoothstep(0, 1, val)", col, row, v)
			if stopFor(clamp01(v)) != stopFor(smoothstep(0, 1, v)) {
				separable++
			}
		}
	}
	require.Greaterf(t, checked, 2000, "only %d measurable lit cells", checked)
	require.Greaterf(t, separable, 500,
		"only %d of %d cells would render a different stop under an identity contrast, "+
			"so this test cannot tell an S-curve from a passthrough", separable, checked)
}

func BenchmarkRenderSplash(b *testing.B) {
	pal := splashTestPalette()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = renderSplashField(80, 30, i, pal, centeredFocalRow(30), splashDefaultVariant)
	}
}
