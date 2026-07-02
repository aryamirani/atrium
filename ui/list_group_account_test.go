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
// and a Claude account. Each spec is "repoBase:accountName"; account "" means no
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
