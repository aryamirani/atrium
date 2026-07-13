package ui

// The empty-state splash: a slow-drifting radial-ripple field that appears to
// emanate from the ATRIUM wordmark and fades out at a round vignette. The field
// is a cheap sum-of-sines interference pattern sampled per character cell,
// modulated by a radial envelope, colored by a theme-anchored gradient, and
// composited *behind* the existing wordmark+message block (which is left
// untouched, so its styling survives). Only the idle "no agents" screen uses
// it; every other empty state keeps the plain FallbackBanner.

import (
	"math"
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

	// driftPerFrame is the outward phase advance per animation frame. Small, so
	// the rings breathe slowly rather than strobe.
	driftPerFrame = 0.18

	rippleFreq1 = 0.60 // primary ring spacing
	rippleFreq2 = 0.30 // secondary interference band (drifts at a different rate)
	rippleArms  = 5.0  // angular tendrils modulating the secondary band

	// splashRamp maps intensity to a glyph, light→heavy. Index 0 (space) is
	// "nothing here"; every glyph is terminal-width 1 (downsample-safe).
	splashRamp = " .·:*oO0"

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

// splashLUT is a precomputed gradient: parallel color/style stops from the
// theme's core hue (purple) to its rim hue (cyan). Built once per palette so the
// per-cell hot loop only does index lookups, never color math.
type splashLUT struct {
	colors []lipgloss.Color
	styles []lipgloss.Style
}

var (
	splashLUTMu    sync.Mutex
	splashLUTCache = map[string]*splashLUT{}
)

// splashLUTFor returns the memoized gradient for a palette, keyed by its three
// anchor hues. Bubble Tea renders on a single goroutine, but the mutex is cheap
// insurance since both the preview and (future) terminal panes render here.
func splashLUTFor(pal theme.Palette) *splashLUT {
	key := string(pal.Purple) + "|" + string(pal.Accent) + "|" + string(pal.Cyan)
	splashLUTMu.Lock()
	defer splashLUTMu.Unlock()
	if lut, ok := splashLUTCache[key]; ok {
		return lut
	}
	lut := buildSplashLUT(pal)
	splashLUTCache[key] = lut
	return lut
}

// buildSplashLUT blends purple→accent→cyan in HCL for a perceptually even ramp.
// The endpoints are pinned to the exact palette colors so the core reads as the
// brand hue and the rim as the theme's cyan. If any anchor is not parseable hex
// (an unusual theme), the whole ramp degrades to flat purple rather than
// emitting broken colors.
func buildSplashLUT(pal theme.Palette) *splashLUT {
	lut := &splashLUT{
		colors: make([]lipgloss.Color, splashLUTSize),
		styles: make([]lipgloss.Style, splashLUTSize),
	}
	core, err1 := colorful.Hex(string(pal.Purple))
	mid, err2 := colorful.Hex(string(pal.Accent))
	rim, err3 := colorful.Hex(string(pal.Cyan))
	for i := range lut.colors {
		c := pal.Purple
		if err1 == nil && err2 == nil && err3 == nil {
			t := float64(i) / float64(splashLUTSize-1) // 0 at core, 1 at rim
			var blended colorful.Color
			if t < 0.5 {
				blended = core.BlendHcl(mid, t/0.5).Clamped()
			} else {
				blended = mid.BlendHcl(rim, (t-0.5)/0.5).Clamped()
			}
			c = lipgloss.Color(blended.Hex())
		}
		lut.colors[i] = c
		lut.styles[i] = lipgloss.NewStyle().Foreground(c)
	}
	// Pin the endpoints exactly (HCL round-tripping can nudge the hex a hair).
	lut.colors[0], lut.styles[0] = pal.Purple, lipgloss.NewStyle().Foreground(pal.Purple)
	last := splashLUTSize - 1
	lut.colors[last], lut.styles[last] = pal.Cyan, lipgloss.NewStyle().Foreground(pal.Cyan)
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

// renderSplashField builds the colored ripple+vignette background: exactly h
// rows of exactly w visible cells, with the clearing ellipses blanked out for the
// composited text. The rings and the round vignette both emanate from the
// wordmark's center (clear.wordCenterRow), not the raw pane center, so the field
// stays anchored on the wordmark even as the message below shifts the visual
// weight down. Pure over its inputs (deterministic, snapshot-testable); returns
// "" when the pane is too small to inscribe a disc.
func renderSplashField(w, h, frame int, pal theme.Palette, clear splashClearing) string {
	if w <= 0 || h <= 0 {
		return ""
	}
	cx := float64(w-1) / 2
	cyFocal := float64(clear.wordCenterRow)
	// Inscribe the vignette disc to the nearest edge from the focal row
	// (aspect-corrected) so the corners and side-margins fall outside it, stay
	// blank, and the disc never spills past the pane.
	vEdge := math.Min(cyFocal, float64(h-1)-cyFocal)
	radius := math.Min(cx, vEdge*cellAspect)
	if radius <= 0 {
		return ""
	}

	lut := splashLUTFor(pal)
	nColors := len(lut.styles)
	ramp := []rune(splashRamp)
	maxGlyph := len(ramp) - 1
	phase := float64(frame) * driftPerFrame

	var sb strings.Builder
	var run strings.Builder
	for row := 0; row < h; row++ {
		if row > 0 {
			sb.WriteByte('\n')
		}
		dy := (float64(row) - cyFocal) * cellAspect
		curIdx := -1 // -1 marks a blank (uncolored) run
		for col := 0; col < w; col++ {
			dx := float64(col) - cx
			idx, ch := -1, ' '

			d := math.Hypot(dx, dy)
			r := d / radius
			if r < 1 && !clear.blanks(dx, row) {
				theta := math.Atan2(dy, dx)
				v := math.Sin(d*rippleFreq1-phase) +
					0.5*math.Sin(d*rippleFreq2-phase*0.6+theta*rippleArms)
				intensity := clamp01((v/1.5 + 1) * 0.5)
				// Keep the mid-field vivid, then fade to the round rim. Fading
				// only past r≈0.3 (rather than immediately) lets the rings just
				// outside the clearing read as bright, so the field emanates with
				// energy instead of a faint haze.
				envelope := 1 - smoothstep(0.30, 1.02, r)
				lit := intensity * envelope
				if g := clampInt(int(lit*float64(maxGlyph)), 0, maxGlyph); g > 0 {
					ch = ramp[g]
					idx = clampInt(int(r*float64(nColors-1)), 0, nColors-1)
				}
			}

			if idx != curIdx {
				flushSplashRun(&sb, &run, curIdx, lut)
				curIdx = idx
			}
			run.WriteRune(ch)
		}
		flushSplashRun(&sb, &run, curIdx, lut)
	}
	return sb.String()
}

// flushSplashRun emits an accumulated run of same-color cells with a single SGR
// (or raw, for a blank run), then resets the buffer. Coalescing runs keeps the
// per-frame ANSI compact instead of one color code per cell.
func flushSplashRun(sb, run *strings.Builder, styleIdx int, lut *splashLUT) {
	if run.Len() == 0 {
		return
	}
	if styleIdx < 0 {
		sb.WriteString(run.String())
	} else {
		sb.WriteString(lut.styles[styleIdx].Render(run.String()))
	}
	run.Reset()
}

// overlayCenter composites fg centered over bg. Retained as the centered
// convenience over overlayAt (and exercised directly by tests).
func overlayCenter(bg, fg string) string {
	fgLines, fgWidth := splashLines(fg)
	bgLines, bgWidth := splashLines(bg)
	return overlayAt(bg, fg, (bgWidth-fgWidth)/2, (len(bgLines)-len(fgLines))/2)
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
