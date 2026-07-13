package ui

// The empty-state splash: a slow-drifting radial-ripple field that appears to
// emanate from the ATRIUM wordmark and fades out at a round vignette. The field
// is a cheap sum-of-sines interference pattern sampled per character cell,
// modulated by a radial envelope, colored by a theme-anchored gradient, and
// composited *behind* the existing wordmark+message block (which is left
// untouched, so its styling survives). Only the idle "no agents" screen uses
// it; every other empty state keeps the plain FallbackBanner.

import (
	"strings"
	"sync"

	"github.com/ZviBaratz/atrium/ui/theme"

	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"
	colorful "github.com/lucasb-eyer/go-colorful"
)

const (
	// cellAspect corrects for terminal cells being ~2:1 (tall): vertical
	// distance is weighted up by this factor so the rings and the round
	// vignette render circular rather than oval.
	cellAspect = 2.0

	// driftPerFrame is the outward phase advance per animation frame (~30fps,
	// see the app's splash tick) — ~0.9 phase units/second, the same visual
	// speed the field had at its original 5Hz push (0.18/frame), just six
	// times smoother along the way.
	driftPerFrame = 0.03

	// The field is a small sum of sines evaluated per cell: two domain-warped
	// concentric ring octaves + rotationally-symmetric petals + an isotropic
	// fine texture. Every term is direction-free (radial, or a set of plane waves
	// whose directions cancel), so the plasma reads rich but never skewed.
	rippleFreq1 = 0.55  // primary ring spacing
	rippleFreq2 = 0.31  // second ring octave (drifts at a different rate)
	rippleFreq3 = 0.14  // slow ring that pulses the angular petals
	petalCount  = 6.0   // even → rotationally symmetric petals, no directional lean
	rippleWarp  = 3.6   // domain-warp amplitude in cells: wavy, organic filaments
	rippleWarpF = 0.055 // domain-warp spatial frequency
	// isoFreq/isoSpeed drive three plane waves 120° apart (iso*Cos/Sin below);
	// their directions sum to zero, so the fine texture shimmers isotropically
	// with no diagonal grain — the fix for the field looking skewed.
	isoFreq   = 0.13
	isoSpeed  = 0.8
	isoWeight = 0.20
	iso1Cos   = -0.5
	iso1Sin   = 0.8660254037844386
	iso2Cos   = -0.5
	iso2Sin   = -0.8660254037844386
	// rippleAmp is the sum of the term weights below; normalizes v into [0,1].
	rippleAmp = 1.0 + 0.55 + 0.40 + 3*isoWeight

	// edgeVignetteFrac is the fraction of each dimension over which the full-bleed
	// field fades to black at the pane border, so it softens into the edges
	// instead of hard-clipping into a rectangle.
	edgeVignetteFrac = 0.16
	// radialDim is how much the field dims from the wordmark (core) out to the
	// farthest corner — enough to read as a glow emanating from ATRIUM while the
	// field still reaches the edges.
	radialDim = 0.42

	// Contrast curve applied to intensity: values below Lo fade toward blank,
	// above Hi saturate — so bright ridges read as filaments against darker
	// voids instead of a uniform mid-tone wash, and the top of the ramp is used.
	splashContrastLo = 0.20
	splashContrastHi = 0.86
	// Color mixing: how much the gradient follows radius vs. a slow angular
	// swirl. A lower radius weight makes the hue wander so the field reads as a
	// multi-hued nebula rather than one flat band.
	colorRadialMix  = 0.50
	colorSwirlF     = 0.045
	colorSwirlSpeed = 0.30

	// The starfield: sparse, fixed, twinkling points scattered through the plasma
	// (including its dark voids) for depth. starThreshold sets rarity (higher →
	// fewer stars), starRamp maps twinkle→glyph, and stars are drawn in a bright
	// near-white so they read as starlight in front of the colored gas.
	starThreshold    = 0.986
	starTwinkleSpeed = 1.7
	starPhaseScatter = 137.0 // desyncs twinkles so stars don't pulse in unison
	starRamp         = " ·+*"

	// Breathing: a slow global brightness swell so the whole nebula feels alive
	// (inhaling) rather than only drifting outward.
	breatheDepth = 0.16
	breatheSpeed = 0.33

	// splashRamp maps intensity to a glyph, light→heavy. Index 0 (space) is
	// "nothing here"; every glyph is terminal-width 1 (downsample-safe). A longer
	// ramp gives finer density steps, so gradients read smooth instead of banded.
	splashRamp = " .·:;+=*oO0@"

	// splashLUTSize is the number of gradient color stops from core to rim.
	splashLUTSize = 20

	// minSplashW/minSplashH gate the effect: below this the pane is too small
	// for the field to read, so String() falls back to the plain wordmark. The
	// width floor sits just above the 48-col wordmark; the height floor leaves
	// room for a few ring rows above and below the ~10-row text block.
	minSplashW = 50
	minSplashH = 18
)

// splashFits reports whether a pane is large enough to render the splash field
// legibly. Callers fall back to the plain centered wordmark when it is not.
func splashFits(w, h int) bool { return w >= minSplashW && h >= minSplashH }

// splashScene composites the idle empty screen: the animated nebula field with
// the wordmark centered on top and the message tucked just below it. The wordmark
// and message are overlaid separately at their own widths (not one padded block)
// so the field's rings hug the narrow wordmark rather than being pushed out by
// the wider message; each gets its own tight clearing so no glyphs bleed through
// the text. The outer clamp honors the pane box (#251). Shared by the preview and
// terminal panes so their idle empty states match. Callers gate on splashFits.
func splashScene(width, height, frame int, message string) string {
	word := trimBlankLines(FallbackBanner())
	msg := theme.Current().FgStyle().Render(message)
	wordW, wordH := lipgloss.Width(word), lipgloss.Height(word)
	msgW, msgH := lipgloss.Width(msg), lipgloss.Height(msg)

	const gap = 2 // blank rows between the wordmark and the message
	cy := (height - 1) / 2
	wordX := (width - wordW) / 2
	wordY := max(0, cy-wordH/2) // wordmark centered on the pane
	msgX := (width - msgW) / 2
	msgY := wordY + wordH + gap

	clearing := splashClearing{
		wordHalfW:     wordW/2 + 2,
		wordHalfH:     wordH/2 + 1,
		wordCenterRow: wordY + wordH/2,
		msgHalfW:      msgW/2 + 2,
		msgHalfH:      msgH/2 + 2,
		msgCenterRow:  msgY + msgH/2,
	}
	field := renderSplashField(width, height, frame, theme.Current().Palette, clearing, splashActiveVariant())
	scene := overlayAt(field, word, wordX, wordY)
	scene = overlayAt(scene, msg, msgX, msgY)
	return lipgloss.NewStyle().MaxWidth(width).MaxHeight(height).Render(scene)
}

// splashLUT is a precomputed gradient: parallel color/style stops from the
// warm core hue to the cool rim, plus a bright star color. Built once per
// palette so the per-cell hot loop only does index lookups, never color math.
type splashLUT struct {
	colors []lipgloss.Color
	styles []lipgloss.Style
	star   lipgloss.Style
}

var (
	splashLUTMu    sync.Mutex
	splashLUTCache = map[string]*splashLUT{}
)

// splashLUTFor returns the memoized gradient for a palette, keyed by every
// anchor it draws from. Bubble Tea renders on a single goroutine, but the mutex
// is cheap insurance since both the preview and (future) terminal panes render
// here.
func splashLUTFor(pal theme.Palette) *splashLUT {
	key := strings.Join([]string{
		string(pal.Danger), string(pal.Purple), string(pal.Accent),
		string(pal.Cyan), string(pal.Fg),
	}, "|")
	splashLUTMu.Lock()
	defer splashLUTMu.Unlock()
	if lut, ok := splashLUTCache[key]; ok {
		return lut
	}
	lut := buildSplashLUT(pal)
	splashLUTCache[key] = lut
	return lut
}

// splashAnchors is the warm→cool nebula gradient (pink → purple → blue → cyan),
// drawn from theme tokens so it tracks the active theme. Consecutive anchors are
// hue-adjacent, so HCL blending between them stays smooth (no muddy backtrack).
func splashAnchors(pal theme.Palette) []lipgloss.Color {
	return []lipgloss.Color{pal.Danger, pal.Purple, pal.Accent, pal.Cyan}
}

// buildSplashLUT blends the anchors across splashLUTSize stops in HCL for smooth,
// non-muddy hue steps, pins the exact endpoints, and adds a bright near-white
// star color. If any anchor is not parseable hex (an unusual theme), the ramp
// degrades to flat purple rather than emitting broken colors.
func buildSplashLUT(pal theme.Palette) *splashLUT {
	anchorCols := splashAnchors(pal)
	lut := &splashLUT{
		colors: make([]lipgloss.Color, splashLUTSize),
		styles: make([]lipgloss.Style, splashLUTSize),
		star:   lipgloss.NewStyle().Foreground(pal.Fg),
	}
	anchors := make([]colorful.Color, len(anchorCols))
	ok := true
	for i, c := range anchorCols {
		cc, err := colorful.Hex(string(c))
		if err != nil {
			ok = false
			break
		}
		anchors[i] = cc
	}
	segs := len(anchorCols) - 1
	for i := range lut.colors {
		c := pal.Purple
		if ok {
			// Map stop i to a position along the multi-segment anchor path.
			t := float64(i) / float64(splashLUTSize-1) * float64(segs)
			seg := clampInt(int(t), 0, segs-1)
			c = lipgloss.Color(anchors[seg].BlendHcl(anchors[seg+1], t-float64(seg)).Clamped().Hex())
		}
		lut.colors[i] = c
		lut.styles[i] = lipgloss.NewStyle().Foreground(c)
	}
	// Pin the endpoints exactly (HCL round-tripping can nudge the hex a hair).
	lut.colors[0], lut.styles[0] = anchorCols[0], lipgloss.NewStyle().Foreground(anchorCols[0])
	last := splashLUTSize - 1
	lut.colors[last], lut.styles[last] = anchorCols[segs], lipgloss.NewStyle().Foreground(anchorCols[segs])
	return lut
}

// splashClearing marks the cells to leave blank for the composited text: a tight
// ellipse hugging the wordmark, plus a shorter, wider one around the message,
// each centered on its own row. Keeping them separate (rather than one clearing
// sized to the whole text block) is what lets the rings hug the narrow wordmark
// instead of being pushed out by the wider message. Half-extents are in cells; a
// zero half-extent disables that ellipse. Both are centered on the field's
// horizontal axis, since the text is centered horizontally.
type splashClearing struct {
	wordHalfW, wordHalfH, wordCenterRow int
	msgHalfW, msgHalfH, msgCenterRow    int
}

// blanks reports whether the cell at horizontal offset dx (from the field axis)
// and absolute row lies inside either clearing ellipse. Uses raw cell distance
// (not the aspect-corrected dy) so each ellipse hugs its text rectangle —
// deliberately distinct from the round vignette metric.
func (c splashClearing) blanks(dx float64, row int) bool {
	inEllipse := func(halfW, halfH, centerRow int) bool {
		if halfW <= 0 || halfH <= 0 {
			return false
		}
		dy := float64(row - centerRow)
		return (dx*dx)/float64(halfW*halfW)+(dy*dy)/float64(halfH*halfH) < 1
	}
	return inEllipse(c.wordHalfW, c.wordHalfH, c.wordCenterRow) ||
		inEllipse(c.msgHalfW, c.msgHalfH, c.msgCenterRow)
}

// flushSplashRun emits an accumulated run of same-color cells with a single SGR
// (or raw, for a blank run), then resets the buffer. Coalescing runs keeps the
// per-frame ANSI compact instead of one color code per cell.
func flushSplashRun(sb, run *strings.Builder, styleIdx int, lut *splashLUT) {
	if run.Len() == 0 {
		return
	}
	switch {
	case styleIdx < 0: // blank run — raw spaces, no color
		sb.WriteString(run.String())
	case styleIdx >= len(lut.styles): // star run — bright near-white
		sb.WriteString(lut.star.Render(run.String()))
	default: // gradient run
		sb.WriteString(lut.styles[styleIdx].Render(run.String()))
	}
	run.Reset()
}

// starHash is a deterministic per-cell pseudo-random value in [0,1), so the
// starfield is fixed in place (and snapshot-stable) while the plasma drifts
// behind it. Built on the integer lattice hash: exact on every architecture,
// unlike the sin-fract hash it replaced.
func starHash(col, row int) float64 {
	return splashCellHash(col, row, seedStar)
}

// overlayAt composites fg over bg (the ripple field) at cell (placeX, placeY),
// splicing width-correctly around bg's ANSI escapes. Adapted from
// overlay.PlaceOverlay (ui/overlay/overlay.go) but deliberately WITHOUT its
// background fade — the gradient must show through, not be dimmed — and without
// the whitespace-option plumbing (plain spaces fill any gap). The field carves a
// blank clearing under fg, so nothing colored bleeds through the text.
func overlayAt(bg, fg string, placeX, placeY int) string {
	fgLines, fgWidth := splashLines(fg)
	bgLines, bgWidth := splashLines(bg)
	fgHeight, bgHeight := len(fgLines), len(bgLines)

	if fgWidth >= bgWidth && fgHeight >= bgHeight {
		return fg
	}

	placeX = clampInt(placeX, 0, max(0, bgWidth-fgWidth))
	placeY = clampInt(placeY, 0, max(0, bgHeight-fgHeight))

	var b strings.Builder
	for i, bgLine := range bgLines {
		if i > 0 {
			b.WriteByte('\n')
		}
		if i < placeY || i >= placeY+fgHeight {
			b.WriteString(bgLine)
			continue
		}

		pos := 0
		if placeX > 0 {
			left := xansi.Truncate(bgLine, placeX, "")
			pos = xansi.StringWidth(left)
			b.WriteString(left)
			if pos < placeX {
				b.WriteString(strings.Repeat(" ", placeX-pos))
				pos = placeX
			}
		}

		fgLine := fgLines[i-placeY]
		b.WriteString(fgLine)
		pos += xansi.StringWidth(fgLine)

		right := xansi.TruncateLeft(bgLine, pos, "")
		bgLineWidth := xansi.StringWidth(bgLine)
		rightWidth := xansi.StringWidth(right)
		if rightWidth <= bgLineWidth-pos {
			b.WriteString(strings.Repeat(" ", bgLineWidth-rightWidth-pos))
		}
		b.WriteString(right)
	}
	return b.String()
}

// splashLines splits s into lines and returns the widest visible (ANSI-aware)
// line width. Mirrors overlay.getLines.
func splashLines(s string) (lines []string, widest int) {
	lines = strings.Split(s, "\n")
	for _, l := range lines {
		if wdt := xansi.StringWidth(l); wdt > widest {
			widest = wdt
		}
	}
	return lines, widest
}

// trimBlankLines drops leading/trailing all-whitespace lines (the wordmark art
// is padded with blank rows) so the composited block is exactly its glyph rows —
// letting the clearing hug the wordmark tightly.
func trimBlankLines(s string) string {
	lines := strings.Split(s, "\n")
	start, end := 0, len(lines)
	for start < end && strings.TrimSpace(xansi.Strip(lines[start])) == "" {
		start++
	}
	for end > start && strings.TrimSpace(xansi.Strip(lines[end-1])) == "" {
		end--
	}
	return strings.Join(lines[start:end], "\n")
}

// smoothstep is the classic Hermite ease between edges a and b, clamped to [0,1].
func smoothstep(a, b, x float64) float64 {
	if a == b {
		if x < a {
			return 0
		}
		return 1
	}
	t := clamp01((x - a) / (b - a))
	return t * t * (3 - 2*t)
}

func clamp01(x float64) float64 {
	if x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
