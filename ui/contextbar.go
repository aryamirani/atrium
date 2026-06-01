package ui

import (
	"fmt"
	"strings"

	"github.com/ZviBaratz/atrium/session"
	"github.com/ZviBaratz/atrium/ui/theme"
	"github.com/mattn/go-runewidth"
)

// contextbar.go composes the strings Atrium pushes into each session's tmux user
// options for the in-session context bar (see session/tmux/context.go). Styling
// uses tmux #[fg=...] markup rather than ANSI/lipgloss because tmux renders the
// status line itself. The composition mirrors the session-list theme so the bar and
// the list agree.

// RepoKey exposes the repo-grouping key (repo name, else base dir) used to build the
// session list's groups, so app.go can form the same groups when pushing per-session
// context (each session's sibling strip is its repo group).
func RepoKey(i *session.Instance) string { return repoKey(i) }

// tmuxEsc escapes '#' so dynamic text (names, branches) can't be misread as a tmux
// format directive (#[...] / #{...}).
func tmuxEsc(s string) string { return strings.ReplaceAll(s, "#", "##") }

// barState returns a session's chip glyph, status word, and tmux color. Unlike the
// list — which animates a spinner for Running/Loading — the pushed status line is
// static, so working states get a steady filled marker.
func barState(s session.Status, th *theme.Theme) (glyph, word, color string) {
	switch s {
	case session.Running:
		return "●", "working", string(th.Palette.Working)
	case session.Loading:
		return "●", "starting", string(th.Palette.Working)
	case session.Ready:
		return th.Glyphs.Ready, "ready", string(th.Palette.Success)
	case session.NeedsInput:
		return th.Glyphs.Waiting, "waiting", string(th.Palette.Attention)
	case session.Paused:
		return th.Glyphs.Paused, "paused", string(th.Palette.FgDim)
	default:
		return " ", "", string(th.Palette.FgDim)
	}
}

// ComposeSessionContext renders the three strings pushed to a session's tmux user
// options for the context bar:
//   - name:  the plain display name (drives the terminal title via set-titles-string)
//   - left:  styled identity — state glyph + name, repo, branch, state word
//   - right: styled chips for every session in the repo group, the current one accented
//
// group is the repo group's sessions in list order (including paused ones, shown
// dimmed); current is the session the bar belongs to.
func ComposeSessionContext(current *session.Instance, repo string, group []*session.Instance) (name, left, right string) {
	th := theme.Current()
	name = current.DisplayName()

	glyph, word, color := barState(current.GetStatus(), th)
	dim := string(th.Palette.FgDim)

	branch := current.Branch
	if runewidth.StringWidth(branch) > 40 {
		branch = runewidth.Truncate(branch, 40, "…")
	}

	var b strings.Builder
	fmt.Fprintf(&b, "#[fg=%s]%s %s#[default]", color, glyph, tmuxEsc(name))
	if repo != "" {
		fmt.Fprintf(&b, "  #[fg=%s]%s#[default]", dim, tmuxEsc(repo))
	}
	if branch != "" {
		fmt.Fprintf(&b, "  #[fg=%s]%s %s#[default]", dim, th.Glyphs.Branch, tmuxEsc(branch))
	}
	fmt.Fprintf(&b, "  #[fg=%s]%s#[default]", color, word)
	left = b.String()

	right = composeChips(current, group, th)
	return name, left, right
}

// composeChips renders the sibling strip: one chip per session in the repo group,
// each "<glyph> <name>" tinted by status, with the current session bracketed and
// accented so it stands out.
func composeChips(current *session.Instance, group []*session.Instance, th *theme.Theme) string {
	chips := make([]string, 0, len(group))
	for _, s := range group {
		glyph, _, color := barState(s.GetStatus(), th)
		label := tmuxEsc(runewidth.Truncate(s.DisplayName(), 16, "…"))
		if s == current {
			chips = append(chips, fmt.Sprintf("#[fg=%s,bold][%s %s]#[default]", string(th.Palette.Accent), glyph, label))
			continue
		}
		chips = append(chips, fmt.Sprintf("#[fg=%s]%s %s#[default]", color, glyph, label))
	}
	return strings.Join(chips, "  ")
}
