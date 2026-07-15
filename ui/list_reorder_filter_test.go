package ui

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// A filter is a scope boundary: navigation skips filter-hidden rows and the *InView
// scopes drop them. Reordering never learned that, so every move walked the unfiltered
// items and could swap the selection past a neighbor the filter hides — changing, and
// persisting, an order with nothing on screen moving (#339).
//
// The gate is the *neighbor*, not the filter: a swap whose neighbor is on screen is
// visible and correct, and stays allowed. These tests pin both halves — refusing the
// invisible swap is worthless if it also kills the visible one.

// The core of #339, at row level: the sibling J would swap with sits between two matches
// and is filtered away, so the move would reorder nothing the user can see.
func TestFilter_MoveDownRefusesPastHiddenSibling(t *testing.T) {
	l, _ := newFilterList(t, "api-one", "zzz-hidden", "api-two")
	l.SetFilter("api") // zzz-hidden sits between the two matches in items

	require.True(t, l.MoveNeighborHidden(false), "the guard sees the hidden sibling")
	require.False(t, l.MoveDown(), "and the move refuses for that same reason")
	require.Equal(t, []string{"api-one", "zzz-hidden", "api-two"}, titlesOf(l.GetInstances()),
		"the refused move must leave the order untouched")
}

// The mirror direction, with the hidden sibling above the selection.
func TestFilter_MoveUpRefusesPastHiddenSibling(t *testing.T) {
	l, _ := newFilterList(t, "api-one", "zzz-hidden", "api-two")
	l.SetFilter("api")
	l.SetSelectedInstance(2) // api-two, whose neighbor above is hidden

	require.True(t, l.MoveNeighborHidden(true))
	require.False(t, l.MoveUp())
	require.Equal(t, []string{"api-one", "zzz-hidden", "api-two"}, titlesOf(l.GetInstances()))
}

// The other half of the contract: when the neighbor is on screen the swap is visible and
// correct, so it must still happen. Refusing here would trade #339 for "J stopped
// working" — a filter-then-tidy workflow that works today.
func TestFilter_MoveDownStillMovesWhenTheNeighborIsVisible(t *testing.T) {
	l, _ := newFilterList(t, "api-one", "api-two", "zzz-other")
	l.SetFilter("api") // both neighbors match and render; only zzz is hidden

	require.False(t, l.MoveNeighborHidden(false), "a rendered neighbor is not hidden")
	require.True(t, l.MoveDown(), "a swap the user can see must still be allowed")
	require.Equal(t, []string{"api-two", "api-one", "zzz-other"}, titlesOf(l.GetInstances()))
}

// Running out of siblings is a plain no-op, not a hidden neighbor: the guard must stay
// false at the edges so the app says nothing rather than blaming the filter.
func TestFilter_MoveNeighborHiddenIsFalseAtTheGroupEdges(t *testing.T) {
	l, _ := newFilterList(t, "api-one", "api-two")
	l.SetFilter("api")

	require.False(t, l.MoveNeighborHidden(true), "nothing above the first row")
	require.False(t, l.MoveUp())
}

// A different repo above the selection is the group-boundary no-op, which J/K has always
// refused silently. It must not be reported as a filter problem.
func TestFilter_MoveNeighborHiddenIgnoresADifferentRepo(t *testing.T) {
	l := newGroupList(t, "/x/repoA", "/x/repoB")
	l.GetInstances()[0].SetDisplayName("alpha")
	l.GetInstances()[1].SetDisplayName("bravo")
	l.SetSelectedInstance(1) // repoB, whose neighbor above is another repo

	require.False(t, l.MoveNeighborHidden(true), "a group boundary is not a hidden neighbor")
}

// A filter can empty a whole repo block — List.String skips a block with no visible rows
// entirely — so { / } can transpose the selected block with one that renders nothing.
func TestFilter_GroupMoveDownRefusesPastEmptiedGroup(t *testing.T) {
	l := newGroupList(t, "/x/repoA", "/x/repoB")
	l.GetInstances()[0].SetDisplayName("alpha")
	l.GetInstances()[1].SetDisplayName("bravo")
	l.SetFilter("alpha") // the entire repoB block renders nothing

	require.True(t, l.GroupMoveNeighborHidden(false))
	require.False(t, l.MoveGroupDown(), "a transpose with an emptied block must be refused")
	require.Equal(t, []string{"repoA", "repoB"}, repoKeys(l))
}

// The mirror direction, with the emptied block above.
func TestFilter_GroupMoveUpRefusesPastEmptiedGroup(t *testing.T) {
	l := newGroupList(t, "/x/repoA", "/x/repoB")
	l.GetInstances()[0].SetDisplayName("alpha")
	l.GetInstances()[1].SetDisplayName("bravo")
	l.SetFilter("bravo") // the entire repoA block renders nothing

	require.True(t, l.GroupMoveNeighborHidden(true))
	require.False(t, l.MoveGroupUp())
	require.Equal(t, []string{"repoA", "repoB"}, repoKeys(l))
}

// A block that merely loses *some* rows to the filter still renders, so transposing with
// it is visible and stays allowed.
func TestFilter_GroupMoveStillMovesWhenTheNeighborBlockRenders(t *testing.T) {
	l := newGroupList(t, "/x/repoA", "/x/repoB", "/x/repoB")
	l.GetInstances()[0].SetDisplayName("api-a")
	l.GetInstances()[1].SetDisplayName("api-b")
	l.GetInstances()[2].SetDisplayName("zzz-gone")
	l.SetFilter("api") // repoB keeps api-b, so the block still renders

	require.False(t, l.GroupMoveNeighborHidden(false))
	require.True(t, l.MoveGroupDown())
	require.Equal(t, []string{"repoB", "repoB", "repoA"}, repoKeys(l))
}

// The issue's exact repro. AccountReorderEnabled counts clusters in items regardless of
// row visibility, so it reports the move as available while the filter empties a whole
// cluster; before the gate the move then reported success and rewrote accountOrder with
// the rendered list unchanged. The guard is left alone — its count is right for its own
// question — and the move refuses on the emptied neighbor instead.
func TestFilter_AccountMoveRefusesPastEmptiedCluster(t *testing.T) {
	l := acctList(t, "api|work", "sideproj|personal")
	l.GetInstances()[0].SetDisplayName("api")
	l.GetInstances()[1].SetDisplayName("sideproj")
	l.SetGroupMode("account")
	l.SetFilter("api") // empties the whole personal cluster

	require.True(t, l.AccountReorderEnabled(), "precondition: the guard still counts two clusters")
	require.True(t, l.AccountMoveNeighborHidden(false), "but the neighbor cluster renders nothing")
	require.False(t, l.MoveAccountDown(), "so the move refuses")
	require.Equal(t, []string{"work", "personal"}, accountsOf(l), "the cluster order is untouched")
	require.Empty(t, l.AccountOrder(), "nothing may be recorded for the app to persist")
}

// The mirror direction, so neither bracket can write an invisible order.
func TestFilter_AccountMoveUpRefusesPastEmptiedCluster(t *testing.T) {
	l := acctList(t, "api|work", "sideproj|personal")
	l.GetInstances()[0].SetDisplayName("api")
	l.GetInstances()[1].SetDisplayName("sideproj")
	l.SetGroupMode("account")
	l.SetFilter("sideproj") // empties the whole work cluster

	require.True(t, l.AccountMoveNeighborHidden(true))
	require.False(t, l.MoveAccountUp())
	require.Equal(t, []string{"work", "personal"}, accountsOf(l))
	require.Empty(t, l.AccountOrder())
}

// Both clusters keeping a visible row means the swap is visible, so it stays allowed and
// still persists its order.
func TestFilter_AccountMoveStillMovesWhenBothClustersRender(t *testing.T) {
	l := acctList(t, "api|work", "sideproj|personal")
	l.GetInstances()[0].SetDisplayName("keep-api")
	l.GetInstances()[1].SetDisplayName("keep-sideproj")
	l.SetGroupMode("account")
	l.SetFilter("keep") // both clusters keep a row

	require.False(t, l.AccountMoveNeighborHidden(false))
	require.True(t, l.MoveAccountDown())
	require.Equal(t, []string{"personal", "work"}, accountsOf(l))
}

// The gate must be exactly as wide as the hiding, not wider: clearing the filter restores
// the move. A refusal that outlived the filter would be a worse bug than the one fixed.
func TestFilter_ReorderResumesOnceTheFilterIsCleared(t *testing.T) {
	l, _ := newFilterList(t, "api-one", "zzz-hidden", "api-two")
	l.SetFilter("api")
	require.False(t, l.MoveDown(), "precondition: refused while the sibling is hidden")

	l.ClearFilter()

	require.True(t, l.MoveDown(), "the move works again once nothing is hidden")
	require.Equal(t, []string{"zzz-hidden", "api-one", "api-two"}, titlesOf(l.GetInstances()))
}

// isHidden covers both ways to hide, so the same predicate catches a folded sibling —
// which is why J/K's collapsed-group refusal (a fold shows no siblings to swap with) can
// finally be explained instead of left as a silent dead key. The selection is the anchor,
// since clampSelectionToNavigable never rests on a folded member.
func TestCollapse_MoveNeighborHiddenSeesAFoldedSibling(t *testing.T) {
	l := newGroupList(t, "/x/repoA", "/x/repoA", "/x/repoB")
	require.True(t, l.Collapse()) // folds repoA onto its anchor

	require.False(t, l.Filtering(), "no filter is live, so the fold is the reason")
	require.True(t, l.MoveNeighborHidden(false), "the sibling below is folded away")
	require.False(t, l.MoveDown(), "and the move refuses")
}

// A filter overrides a fold in the render, so a folded group reads as expanded while
// filtering. The reason the app reports must follow the screen, not the stale flag.
func TestFilter_OverridesTheFoldAsTheReportedReason(t *testing.T) {
	l := newGroupList(t, "/x/repoA", "/x/repoA", "/x/repoB")
	l.GetInstances()[0].SetDisplayName("api-one")
	l.GetInstances()[1].SetDisplayName("api-two")
	l.GetInstances()[2].SetDisplayName("bravo")
	require.True(t, l.Collapse())
	l.SetFilter("api") // both repoA rows surface despite the fold

	require.True(t, l.Filtering(), "the filter is the live reason, not the overridden fold")
	require.False(t, l.MoveNeighborHidden(false), "both siblings render, so nothing is hidden")
	require.True(t, l.MoveDown(), "and the visible swap is allowed")
}

// The guards are read on every reorder keypress, including before any session exists.
func TestReorderGuards_EmptyListIsSafe(t *testing.T) {
	l, _ := newFilterList(t)

	require.False(t, l.Filtering())
	require.False(t, l.MoveNeighborHidden(true), "an empty list must not index items")
	require.False(t, l.MoveNeighborHidden(false))
	require.False(t, l.GroupMoveNeighborHidden(true))
	require.False(t, l.GroupMoveNeighborHidden(false))
	require.False(t, l.AccountMoveNeighborHidden(true))
	require.False(t, l.AccountMoveNeighborHidden(false))
}
