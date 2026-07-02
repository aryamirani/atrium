package ui

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ZviBaratz/atrium/session"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"
)

// acctList builds a list whose instances carry a Path (→ repo group key = base)
// and a Claude account. Each spec is "repoBase|account"; account "" means no
// account. Titles are unique so selection can be asserted by identity.
func acctList(t *testing.T, specs ...string) *List {
	t.Helper()
	s := spinner.New()
	l := NewList(&s)
	for i, spec := range specs {
		base, acct := splitSpec(spec) // spec form "repoBase|account"
		inst, err := session.NewInstance(session.InstanceOptions{
			Title: string(rune('a' + i)), Path: "/tmp/" + base, Program: "echo",
		})
		require.NoError(t, err)
		if acct != "" {
			inst.SetClaudeAccount(acct, "", false)
		}
		l.AddInstance(inst)
	}
	l.SetSize(80, 40)
	return l
}

func splitSpec(s string) (repo, acct string) {
	for i := 0; i < len(s); i++ {
		if s[i] == '|' {
			return s[:i], s[i+1:]
		}
	}
	return s, ""
}

// orderKeys returns "repoBase|account" per item in list order.
func orderKeys(l *List) []string {
	out := make([]string, 0, len(l.items))
	for _, it := range l.items {
		out = append(out, filepath.Base(it.Path)+"|"+it.ClaudeAccountName())
	}
	return out
}

// persistKeys returns "repoBase|account" per item in InstancesForPersist order —
// the canonical (manual) order that must never reflect a view transform.
func persistKeys(l *List) []string {
	out := make([]string, 0, len(l.items))
	for _, it := range l.InstancesForPersist() {
		out = append(out, filepath.Base(it.Path)+"|"+it.ClaudeAccountName())
	}
	return out
}

func TestGroupMode_ClustersByAccount(t *testing.T) {
	// Interleaved input: work, personal, work.
	l := acctList(t, "api|work", "sideproj|personal", "infra|work")
	l.SetGroupMode("account")
	// work cluster (first appearance) then personal; repos keep first-seen order.
	require.Equal(t, []string{"api|work", "infra|work", "sideproj|personal"}, orderKeys(l))
}

func TestGroupMode_NoAccountBucketTrailsLast(t *testing.T) {
	l := acctList(t, "legacy|", "api|work")
	l.SetGroupMode("account")
	require.Equal(t, []string{"api|work", "legacy|"}, orderKeys(l))
}

func TestGroupMode_SingleAccountLeavesOrderUnchanged(t *testing.T) {
	l := acctList(t, "api|work", "infra|work")
	before := orderKeys(l)
	l.SetGroupMode("account")
	require.Equal(t, before, orderKeys(l), "≤1 distinct account is a no-op reorder")
	require.Equal(t, 1, l.distinctAccountCount())
}

func TestGroupMode_DoesNotOverwritePersistedOrder(t *testing.T) {
	l := acctList(t, "api|work", "sideproj|personal", "infra|work")
	l.SetGroupMode("account")
	persist := make([]string, 0, 3)
	for _, it := range l.InstancesForPersist() {
		persist = append(persist, filepath.Base(it.Path)+"|"+it.ClaudeAccountName())
	}
	// Persisted (canonical/manual) order stays the interleaved input order.
	require.Equal(t, []string{"api|work", "sideproj|personal", "infra|work"}, persist)
}

func TestGroupMode_RoundTripRestoresRepoOrder(t *testing.T) {
	l := acctList(t, "api|work", "sideproj|personal", "infra|work")
	before := orderKeys(l)
	l.SetGroupMode("account")
	l.SetGroupMode("repo")
	require.Equal(t, before, orderKeys(l), "switching back restores canonical order")
}

func TestGroupMode_PreservesSelectionByIdentity(t *testing.T) {
	l := acctList(t, "api|work", "sideproj|personal", "infra|work")
	l.SelectInstance(l.items[1]) // the personal session
	sel := l.GetSelectedInstance()
	l.SetGroupMode("account")
	require.Same(t, sel, l.GetSelectedInstance())
}

func TestGroupMode_DisablesManualReorder(t *testing.T) {
	l := acctList(t, "api|work", "infra|personal")
	require.True(t, l.ManualReorderEnabled())
	l.SetGroupMode("account")
	require.False(t, l.ManualReorderEnabled())
}

func TestGroupMode_GroupMovesAreNoOpInAccountMode(t *testing.T) {
	l := acctList(t, "api|work", "sideproj|personal", "infra|work")
	l.SetGroupMode("account")
	l.SelectInstance(l.items[0])
	require.False(t, l.MoveGroupDown(), "group moves are disabled while account-grouped")
}

func TestGroupMode_RendersAccountDividers(t *testing.T) {
	l := acctList(t, "api|work", "sideproj|personal")
	l.SetGroupMode("account")
	out := ansi.Strip(l.String())
	require.Contains(t, out, "── work", "work divider present")
	require.Contains(t, out, "── personal", "personal divider present")
}

func TestGroupMode_SuppressesRowAccountBadgeWhenGrouped(t *testing.T) {
	// Two work sessions in one repo + a personal repo → grouping active (2 accounts).
	// No name here contains the substring "work" except the account name itself, so
	// counting "work" cleanly separates row badges (repo mode) from the divider.
	l := acctList(t, "api|work", "api|work", "sideproj|personal")
	require.Equal(t, 2, strings.Count(ansi.Strip(l.String()), "work"), "two row badges in repo mode")
	l.SetGroupMode("account")
	// Badges suppressed; "work" now survives only in the single work divider.
	require.Equal(t, 1, strings.Count(ansi.Strip(l.String()), "work"), "badges gone, one divider remains")
}

func TestGroupMode_NoDividerWithSingleAccount(t *testing.T) {
	l := acctList(t, "api|work", "infra|work")
	l.SetGroupMode("account")
	out := ansi.Strip(l.String())
	require.NotContains(t, out, "── work", "no divider when only one account")
	require.Equal(t, 2, strings.Count(out, "work"), "row badges kept when grouping is a no-op")
}

// The divider is emitted once per contiguous account *cluster*, not once per repo:
// two distinct work repos (api, infra) must share a single "work" divider once
// clustering makes them adjacent, proving the divider tracks account-block
// boundaries rather than repo-group boundaries.
func TestGroupMode_DividerOncePerAccountClusterSpanningRepos(t *testing.T) {
	l := acctList(t, "api|work", "sideproj|personal", "infra|work")
	l.SetGroupMode("account")
	out := ansi.Strip(l.String())

	require.Equal(t, 1, strings.Count(out, "── work"), "one work divider spans both work repos")
	require.Equal(t, 1, strings.Count(out, "── personal"), "one personal divider")
	require.Contains(t, out, "API", "api repo header present")
	require.Contains(t, out, "INFRA", "infra repo header present")
	require.Equal(t, []string{"api|work", "infra|work", "sideproj|personal"}, orderKeys(l),
		"work repos (api, infra) are adjacent, then personal")
}

// Combining account clustering with status sorting must produce the same
// clustered-then-status-ordered result whichever order the two view transforms
// are toggled in (sort→group or group→sort), and InstancesForPersist must stay
// the canonical creation order in both cases — the snapshot never captures a
// transformed order regardless of toggle sequence. This is because rebuildView
// always derives items from the canonical l.manual snapshot rather than from
// whatever l.items happened to be, so order of activation cannot matter.
func TestGroupMode_SortAndAccountCombineRegardlessOfToggleOrder(t *testing.T) {
	// Repo-contiguous insertion (via AddInstance) turns the interleaved specs into
	// canonical creation order api,api,sideproj,sideproj,infra — titled a,d,b,e,c
	// respectively (title reflects spec position, not final placement).
	build := func(t *testing.T) (l *List, a, b, c, d, e *session.Instance) {
		t.Helper()
		l = acctList(t, "api|work", "sideproj|personal", "infra|work", "api|work", "sideproj|personal")
		byTitle := func(title string) *session.Instance {
			for _, it := range l.items {
				if it.Title == title {
					return it
				}
			}
			t.Fatalf("no instance titled %q", title)
			return nil
		}
		a, b, c, d, e = byTitle("a"), byTitle("b"), byTitle("c"), byTitle("d"), byTitle("e")
		a.SetStatus(session.Running)
		d.SetStatus(session.NeedsInput)
		b.SetStatus(session.Running)
		e.SetStatus(session.NeedsInput)
		c.SetStatus(session.Running)
		return
	}

	// Account-clustered (work: api then infra; personal: sideproj), and within each
	// repo group NeedsInput sorts ahead of Running.
	want := func(a, b, c, d, e *session.Instance) []*session.Instance {
		return []*session.Instance{d, a, c, e, b}
	}
	// Canonical creation order (repo-contiguous), untouched by either view transform.
	canonical := func(a, b, c, d, e *session.Instance) []*session.Instance {
		return []*session.Instance{a, d, b, e, c}
	}

	t.Run("sort then group", func(t *testing.T) {
		l, a, b, c, d, e := build(t)
		l.SetSortMode("status")
		l.SetGroupMode("account")
		require.Equal(t, want(a, b, c, d, e), l.items, "sort-then-group order")
		require.Equal(t, canonical(a, b, c, d, e), l.InstancesForPersist(), "persisted order stays canonical")
	})

	t.Run("group then sort", func(t *testing.T) {
		l, a, b, c, d, e := build(t)
		l.SetGroupMode("account")
		l.SetSortMode("status")
		require.Equal(t, want(a, b, c, d, e), l.items, "group-then-sort order")
		require.Equal(t, canonical(a, b, c, d, e), l.InstancesForPersist(), "persisted order stays canonical")
	})
}

// AddInstance while account-grouped must place the new session contiguous with
// its account's cluster in the display order, while InstancesForPersist keeps the
// canonical repo-contiguous order (the new session lands next to its repo's
// existing entry, not wherever the clustered view happened to put it).
func TestGroupMode_AddInstanceLandsInAccountCluster(t *testing.T) {
	l := acctList(t, "api|work", "sideproj|personal", "infra|work")
	l.SetGroupMode("account")
	require.Equal(t, []string{"api|work", "infra|work", "sideproj|personal"}, orderKeys(l))

	newInst, err := session.NewInstance(session.InstanceOptions{
		Title: "z", Path: "/tmp/api", Program: "echo",
	})
	require.NoError(t, err)
	newInst.SetClaudeAccount("work", "", false)
	l.AddInstance(newInst)

	require.Equal(t, []string{"api|work", "api|work", "infra|work", "sideproj|personal"}, orderKeys(l),
		"new work session lands inside the contiguous work cluster")
	require.Equal(t, []string{"api|work", "api|work", "sideproj|personal", "infra|work"}, persistKeys(l),
		"persisted order stays canonical (repo-contiguous), new session next to its repo's existing entry")
}

// KillInstance while account-grouped must keep the display clustered and drop the
// killed session from InstancesForPersist while keeping that order canonical
// (repo-contiguous). The killed target here is an unstarted NewInstance, so
// target.Kill() is a harmless no-op (no live tmux session or worktree to tear
// down), matching the pattern already used by TestSessionSort_KillReflectedInCanonicalOrder.
func TestGroupMode_KillInstanceKeepsClusteringAndCanonicalPersist(t *testing.T) {
	l := acctList(t, "api|work", "sideproj|personal", "infra|work")
	l.SetGroupMode("account")
	require.Equal(t, []string{"api|work", "infra|work", "sideproj|personal"}, orderKeys(l))

	target := l.items[0] // api|work, the first work-cluster entry
	l.KillInstance(target)

	// Removing api leaves canonical order sideproj,infra: personal now leads the
	// first-appearance account order, so the cluster order flips relative to
	// before the kill — a further check that clustering is recomputed from the
	// post-removal canonical order, not patched in place.
	require.Equal(t, []string{"sideproj|personal", "infra|work"}, orderKeys(l),
		"clustering stays consistent with the killed session gone")
	require.Equal(t, []string{"sideproj|personal", "infra|work"}, persistKeys(l),
		"persisted order drops the killed session and stays canonical")
}

// MoveGroupUp mirrors the existing MoveGroupDown no-op guard: whole-group moves
// are disabled while account-grouped, since account clustering — not manual group
// order — owns block order there.
func TestGroupMode_MoveGroupUpNoOpInAccountMode(t *testing.T) {
	l := acctList(t, "api|work", "sideproj|personal", "infra|work")
	l.SetGroupMode("account")
	l.SelectInstance(l.items[2])
	require.False(t, l.MoveGroupUp(), "group moves are disabled while account-grouped")
}
