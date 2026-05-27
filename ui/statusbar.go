package ui

import (
	"claude-squad/ui/theme"
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
)

// StatusBar is the single-line top bar: the app title on the left and the
// global attention summary (working / waiting / paused) on the right. The
// waiting segment uses the attention color so blocked sessions draw the eye.
type StatusBar struct {
	width  int
	counts StatusCounts
}

func NewStatusBar() *StatusBar { return &StatusBar{} }

func (s *StatusBar) SetSize(width, _ int) { s.width = width }

func (s *StatusBar) SetCounts(c StatusCounts) { s.counts = c }

func (s *StatusBar) String() string {
	th := theme.Current()
	g := th.Glyphs

	leftPlain := " claude-squad"
	leftStyled := th.PurpleStyle().Bold(true).Render(leftPlain)

	// Right segments, each shown only when nonzero.
	var plain, styled []string
	add := func(p, st string) { plain = append(plain, p); styled = append(styled, st) }
	if s.counts.Working > 0 {
		add(fmt.Sprintf("%d working", s.counts.Working),
			th.DimStyle().Render(fmt.Sprintf("%d working", s.counts.Working)))
	}
	if s.counts.Waiting > 0 {
		p := fmt.Sprintf("%s %d waiting", g.Waiting, s.counts.Waiting)
		add(p, th.AttentionStyle().Render(p))
	}
	if s.counts.Paused > 0 {
		p := fmt.Sprintf("%s %d paused", g.Paused, s.counts.Paused)
		add(p, th.DimStyle().Render(p))
	}

	sepPlain := " · "
	rightPlain := strings.Join(plain, sepPlain)
	rightStyled := strings.Join(styled, th.DimStyle().Render(sepPlain))

	// Lay out: left … right, with a trailing space margin.
	gap := s.width - runewidth.StringWidth(leftPlain) - runewidth.StringWidth(rightPlain) - 1
	if gap < 1 {
		// Not enough room for both; keep the title and drop the summary.
		gap = 1
		rightStyled = ""
	}
	line := leftStyled + strings.Repeat(" ", gap) + rightStyled + " "
	if s.width > 0 && ansi.StringWidth(line) > s.width {
		line = ansi.Truncate(line, s.width, "")
	}
	return line
}
