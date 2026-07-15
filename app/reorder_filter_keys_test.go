package app

import (
	"testing"

	"github.com/ZviBaratz/atrium/config"
	"github.com/ZviBaratz/atrium/session"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A reorder key whose neighbor the filter hides used to move — and persist — an order
// with nothing on screen changing (#339). Every scope now refuses and explains itself,
// and (the other half of the contract) a swap the user can actually see still happens.

// filterReorderHome builds a stateDefault home with working in-memory storage whose
// sessions are (displayName, repo, account) triples, so a filter can hide the neighbor of
// any reorder scope. The menu is visible in stateDefault, so the refusal notices land.
func filterReorderHome(t *testing.T, specs ...[3]string) *home {
	t.Helper()
	h := newCreateFormHome(t)
	st := config.DefaultState()
	storage, err := session.NewStorage(st)
	require.NoError(t, err)
	h.appState = st
	h.storage = storage
	for _, spec := range specs {
		inst, err := session.NewInstance(session.InstanceOptions{
			Title: spec[0], Path: "/tmp/" + spec[1], Program: "echo",
		})
		require.NoError(t, err)
		if spec[2] != "" {
			inst.SetClaudeAccount(spec[2], "", false)
		}
		h.list.AddInstance(inst)
	}
	h.state = stateDefault
	return h
}

// J toward a filtered-out sibling must explain itself rather than silently rewriting the
// persisted order.
func TestReorderKeys_SessionMoveRefusesPastFilterHiddenSibling(t *testing.T) {
	h := filterReorderHome(t,
		[3]string{"api-one", "repoA", ""},
		[3]string{"zzz-hidden", "repoA", ""},
		[3]string{"api-two", "repoA", ""})
	h.list.SetFilter("api") // zzz-hidden sits between the two matches
	before := instanceTitles(h)

	pressKey(h, 'J') // KeyMoveDown

	require.True(t, h.menu.HasNotice(), "the refusal must explain itself")
	assert.Contains(t, h.menu.String(), "filter-hidden session")
	assert.Equal(t, before, instanceTitles(h), "and must not touch the persisted order")
}

// The other half: a sibling that renders is a swap the user can see, so it must still
// move and persist. Refusing here would trade #339 for "J stopped working".
func TestReorderKeys_SessionMoveStillMovesWhenTheNeighborIsVisible(t *testing.T) {
	h := filterReorderHome(t,
		[3]string{"api-one", "repoA", ""},
		[3]string{"api-two", "repoA", ""},
		[3]string{"zzz-other", "repoA", ""})
	h.list.SetFilter("api") // both neighbors render

	pressKey(h, 'J')

	assert.False(t, h.menu.HasNotice(), "a visible swap needs no explanation")
	assert.Equal(t, []string{"api-two", "api-one", "zzz-other"}, instanceTitles(h),
		"the visible swap still happens and persists")
}

// } toward a block the filter has emptied renders nothing, so the transpose would be
// invisible.
func TestReorderKeys_GroupMoveRefusesPastEmptiedGroup(t *testing.T) {
	h := filterReorderHome(t,
		[3]string{"api-one", "repoA", "work"},
		[3]string{"zzz-hidden", "repoB", "work"})
	h.list.SetFilter("api") // the whole repoB block renders nothing
	before := instanceTitles(h)

	pressKey(h, '}') // KeyMoveGroupDown

	require.True(t, h.menu.HasNotice())
	assert.Contains(t, h.menu.String(), "filter-hidden group")
	assert.Equal(t, before, instanceTitles(h))
}

// The issue's repro, end-to-end through handleKeyPress: the guard counts two clusters
// (it counts them in items, regardless of visibility), so before the fix ] reported
// success and wrote account_order to state with the rendered list unchanged.
func TestReorderKeys_AccountMoveRefusesPastEmptiedCluster(t *testing.T) {
	h := accountGroupedHome(t) // api|work, infra|personal
	h.state = stateDefault
	h.list.SetFilter("api") // empties the whole personal cluster

	require.True(t, h.list.AccountReorderEnabled(), "precondition: the guard still says available")
	pressKey(h, ']') // KeyMoveAccountDown

	require.True(t, h.menu.HasNotice())
	assert.Contains(t, h.menu.String(), "filter-hidden cluster")
	assert.Empty(t, h.appState.GetAccountOrder(),
		"nothing may reach state.json when the screen cannot change")
}

// A folded sibling is the same shape reached without a filter: J inside a fold was a
// silent dead key (the move refused, ManualReorderEnabled said otherwise, and
// moveAndPersist swallows a false). It now names the fold and the key that undoes it.
func TestReorderKeys_SessionMoveRefusesPastFoldedSiblingAndSaysSo(t *testing.T) {
	h := filterReorderHome(t,
		[3]string{"alpha", "repoA", ""},
		[3]string{"apex", "repoA", ""},
		[3]string{"bravo", "repoB", ""})
	h.list.SetSelectedInstance(0)
	require.True(t, h.list.Collapse(), "precondition: repoA is folded")

	pressKey(h, 'J')

	require.True(t, h.menu.HasNotice(), "a folded refusal must not stay silent")
	assert.Contains(t, h.menu.String(), "folded session")
	assert.Contains(t, h.menu.String(), "expand")
}

// Notice precedence. A status sort owns within-block order whether or not a filter is
// live, so clearing the filter would not restore J — naming the filter would promise a
// fix that does not arrive.
func TestReorderKeys_SortRefusalOutranksTheFilterRefusal(t *testing.T) {
	h := filterReorderHome(t,
		[3]string{"api-one", "repoA", ""},
		[3]string{"zzz-hidden", "repoA", ""},
		[3]string{"api-two", "repoA", ""})
	h.list.SetSortMode("status")
	h.list.SetFilter("api")

	pressKey(h, 'J')

	assert.Contains(t, h.menu.String(), "sorting by status",
		"the durable reason wins over the transient one")
	assert.NotContains(t, h.menu.String(), "esc to clear")
	assert.Contains(t, h.menu.String(), "session reorder",
		"the refusal names the one ladder the sort disables (#346)")
}

// The status-sort refusal names the session ladder, and that scoping has to stay true:
// the sort owns within-group order only, so { / } keeps working under it. Nothing pinned
// the two halves together, which is how the hint drifted to the unscoped "manual reorder
// is off" — a claim the settings screen contradicts ("Group order stays manual ({ / })").
// ui.TestGroupMode_StatusSortGroupMoveCrossesAccountsFreely pins the move; this pins that
// the notice does not disown it.
func TestReorderKeys_StatusSortRefusalScopesItselfToSessions(t *testing.T) {
	h := filterReorderHome(t,
		[3]string{"api-one", "repoA", ""},
		[3]string{"api-two", "repoA", ""},
		[3]string{"infra-one", "repoB", ""})
	h.list.SetSortMode("status")
	h.list.SetSelectedInstance(0)

	pressKey(h, 'J')
	require.True(t, h.menu.HasNotice(), "J under a status sort must explain itself")
	notice := h.menu.String()
	assert.Contains(t, notice, "session reorder", "only the session ladder is off")
	assert.NotContains(t, notice, "manual reorder",
		"'manual' is the settings screen's word for group order too, so it over-claims")
	assert.Contains(t, notice, ", to switch",
		"the only fixable refusal that named no key; , opens the sort setting (#346)")

	// The other half: the ladders the notice does not disown still move, silently.
	h.menu.ClearNotice()
	pressKey(h, '}') // KeyMoveGroupDown — repoA past repoB, under the same status sort
	assert.Equal(t, []string{"infra-one", "api-one", "api-two"}, instanceTitles(h),
		"a status sort owns within-group order only, so { / } still moves")
	assert.False(t, h.menu.HasNotice(), "a group move that works needs no explanation")
}

// Same rule at the block level: the account boundary is filter-independent, and its
// advice ([ / ]) would itself be refused while filtering — so it must be named first.
func TestReorderKeys_AccountBoundaryRefusalOutranksTheFilterRefusal(t *testing.T) {
	h := accountGroupedHome(t) // api|work, infra|personal
	h.state = stateDefault
	h.list.SetSelectedInstance(1) // infra|personal, whose neighbor above is the work cluster
	h.list.SetFilter("infra")     // and which the filter has also emptied

	pressKey(h, '{') // KeyMoveGroupUp — crosses into work, which is also hidden

	assert.Contains(t, h.menu.String(), "within an account",
		"the boundary a cleared filter would not lift is named first")
}
