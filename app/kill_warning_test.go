package app

import (
	"strings"
	"testing"

	"github.com/ZviBaratz/atrium/session"
	"github.com/ZviBaratz/atrium/session/git"

	"github.com/stretchr/testify/require"
)

// flattenOverlay strips the confirmation overlay's box-drawing and line wrapping so
// a test can assert on the logical message regardless of where Render wraps it.
func flattenOverlay(s string) string {
	r := strings.NewReplacer("│", " ", "╭", " ", "╮", " ", "╰", " ", "╯", " ", "─", " ")
	return strings.Join(strings.Fields(r.Replace(s)), " ")
}

// TestKillDataWarning covers the kill-confirmation suffix across the four states of
// the branch: clean, dirty-only, committed-only (the paused auto-WIP case reads
// here, Dirty=false), and both. The count is pluralized and the consequence is
// spelled out so the danger is explicit.
func TestKillDataWarning(t *testing.T) {
	require.Equal(t, "", killDataWarning(false, 0))
	require.Equal(t, " (has uncommitted changes)", killDataWarning(true, 0))
	require.Equal(t, " (has 1 unmerged commit — deleting discards them)", killDataWarning(false, 1))
	require.Equal(t, " (has 3 unmerged commits — deleting discards them)", killDataWarning(false, 3))
	require.Equal(t,
		" (has uncommitted changes and 2 unmerged commits — deleting discards them)",
		killDataWarning(true, 2))
}

// TestSessionsWithUnmergedWork verifies the batch-kill aggregate counts a session
// once whether it is dirty, ahead of base, or both, and ignores clean and
// never-polled (nil-stats) sessions.
func TestSessionsWithUnmergedWork(t *testing.T) {
	mk := func(t *testing.T, stats *git.DiffStats) *session.Instance {
		inst, err := session.NewInstance(session.InstanceOptions{Title: "t", Path: t.TempDir(), Program: "echo"})
		require.NoError(t, err)
		if stats != nil {
			inst.SetDiffStats(stats)
		}
		return inst
	}

	insts := []*session.Instance{
		mk(t, &git.DiffStats{Dirty: true}),             // dirty only
		mk(t, &git.DiffStats{Commits: 2}),              // committed only (e.g. paused WIP)
		mk(t, &git.DiffStats{Dirty: true, Commits: 1}), // both — still one session
		mk(t, &git.DiffStats{}),                        // clean
		mk(t, nil),                                     // never polled
	}

	require.Equal(t, 3, sessionsWithUnmergedWork(insts))
	require.Equal(t, 0, sessionsWithUnmergedWork(nil))
}

// TestConfirmKill_WarnsUnmergedCommits verifies the single-kill confirmation
// actually renders the unmerged-work warning (the paused-with-WIP and unpushed-
// commits case both surface here as Commits>0).
func TestConfirmKill_WarnsUnmergedCommits(t *testing.T) {
	h := newCreateFormHome(t)
	h.windowWidth = 120
	inst := addActive(t, h, "alpha")
	inst.SetDiffStats(&git.DiffStats{Commits: 2})

	h.confirmKill(inst)

	require.Equal(t, stateConfirm, h.state)
	require.NotNil(t, h.confirmationOverlay)
	require.Contains(t, flattenOverlay(h.confirmationOverlay.Render()), "has 2 unmerged commits — deleting discards them")
}

// TestKillMarked_AggregatesUnmergedWork drives the real x-over-marked path and
// verifies the batch confirmation names how many of the marked sessions carry work
// (dirty or ahead of base), counting each such session once.
func TestKillMarked_AggregatesUnmergedWork(t *testing.T) {
	h := newCreateFormHome(t)
	h.windowWidth = 120
	a := addActive(t, h, "alpha")
	b := addActive(t, h, "bravo")
	c := addActive(t, h, "charlie")
	a.SetDiffStats(&git.DiffStats{Commits: 1})
	b.SetDiffStats(&git.DiffStats{Dirty: true})
	// charlie (c) is left clean — it is marked below but must not be counted.

	pressRune(h, 'v')
	h.list.ToggleMark(a)
	h.list.ToggleMark(b)
	h.list.ToggleMark(c)

	pressRune(h, 'x')

	require.Equal(t, stateConfirm, h.state)
	require.NotNil(t, h.confirmationOverlay)
	require.Contains(t, flattenOverlay(h.confirmationOverlay.Render()), "2 of 3 have unmerged work")
}
