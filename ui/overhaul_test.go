package ui

import (
	"claude-squad/session"
	"claude-squad/session/git"
	"claude-squad/ui/theme"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/stretchr/testify/require"
)

func instWithStatus(t *testing.T, title string, st session.Status) *session.Instance {
	t.Helper()
	inst, err := session.NewInstance(session.InstanceOptions{Title: title, Path: ".", Program: "echo"})
	require.NoError(t, err)
	inst.Status = st
	return inst
}

func TestList_Counts(t *testing.T) {
	s := spinner.New()
	l := NewList(&s, false)
	l.AddInstance(instWithStatus(t, "a", session.Running))()
	l.AddInstance(instWithStatus(t, "b", session.Loading))()
	l.AddInstance(instWithStatus(t, "c", session.NeedsInput))()
	l.AddInstance(instWithStatus(t, "d", session.Paused))()
	l.AddInstance(instWithStatus(t, "e", session.Ready))()

	c := l.Counts()
	require.Equal(t, 2, c.Working, "Running + Loading are 'working'")
	require.Equal(t, 1, c.Waiting)
	require.Equal(t, 1, c.Paused)
}

func TestStatusBar_ShowsNonzeroCounts(t *testing.T) {
	t.Cleanup(theme.Set("unicode"))
	sb := NewStatusBar()
	sb.SetSize(80, 1)
	sb.SetCounts(StatusCounts{Working: 3, Waiting: 1, Paused: 0})
	out := sb.String()
	require.Contains(t, out, "claude-squad")
	require.Contains(t, out, "3 working")
	require.Contains(t, out, "1 waiting")
	require.NotContains(t, out, "paused", "zero paused is omitted")
}

func TestRender_AutoBadge(t *testing.T) {
	t.Cleanup(theme.Set("unicode")) // AutoBadge glyph is empty here, so the chip is plain "AUTO"
	s := spinner.New()
	r := &InstanceRenderer{spinner: &s}
	r.setWidth(60)

	working := instWithStatus(t, "auto", session.Running)
	working.AutoYes = true
	require.Contains(t, r.Render(working, 1, false), "AUTO", "auto-accepting session shows the badge")

	paused := instWithStatus(t, "auto-paused", session.Paused)
	paused.AutoYes = true
	require.NotContains(t, r.Render(paused, 1, false), "AUTO", "paused session never shows the badge")
}

func TestRender_StateWords(t *testing.T) {
	t.Cleanup(theme.Set("unicode"))
	s := spinner.New()
	r := &InstanceRenderer{spinner: &s}
	r.setWidth(60)
	cases := map[session.Status]string{
		session.Ready:      "ready",
		session.NeedsInput: "waiting",
		session.Paused:     "paused",
	}
	for st, word := range cases {
		out := r.Render(instWithStatus(t, "s", st), 1, false)
		require.Contains(t, out, word, "status %v should render the word %q", st, word)
	}
}

// guard the row keeps its diff stat right-aligned within the inner width
func TestRender_DiffStatPresent(t *testing.T) {
	t.Cleanup(theme.Set("unicode"))
	s := spinner.New()
	r := &InstanceRenderer{spinner: &s}
	r.setWidth(60)
	inst := instWithStatus(t, "s", session.Ready)
	inst.SetDiffStats(&git.DiffStats{Added: 9, Removed: 2})
	out := r.Render(inst, 1, false)
	require.True(t, strings.Contains(out, "+9") && strings.Contains(out, "-2"), "diff stat should render")
}

// TestListGolden is a deterministic snapshot of the panelized list (no color,
// unicode glyphs, fixed size) guarding against unintended layout/content
// changes. Regenerate with CS_UPDATE_GOLDEN=1 go test ./ui/ -run TestListGolden.
func TestListGolden(t *testing.T) {
	t.Cleanup(theme.Set("unicode"))
	prof := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii) // strip color → stable bytes
	t.Cleanup(func() { lipgloss.SetColorProfile(prof) })

	s := spinner.New()
	l := NewList(&s, false)
	mk := func(title, branch string, st session.Status, stats *git.DiffStats) {
		inst := instWithStatus(t, title, st)
		inst.Branch = branch
		inst.SetDiffStats(stats)
		l.AddInstance(inst)()
	}
	mk("overhaul", "zvi/visual-overhaul", session.Ready, &git.DiffStats{Added: 142, Removed: 31, Commits: 3, Dirty: true})
	mk("bounds", "fix-bounds", session.NeedsInput, nil)
	mk("markers", "pane-markers", session.Paused, nil)
	l.SetSize(40, 14)

	got := l.String()
	golden := filepath.Join("testdata", "list_golden.txt")
	if os.Getenv("CS_UPDATE_GOLDEN") != "" {
		require.NoError(t, os.MkdirAll("testdata", 0o755))
		require.NoError(t, os.WriteFile(golden, []byte(got), 0o644))
	}
	want, err := os.ReadFile(golden)
	require.NoError(t, err, "missing golden; regenerate with CS_UPDATE_GOLDEN=1")
	require.Equal(t, string(want), got)
}
