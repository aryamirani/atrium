package ui

import (
	"sort"

	"github.com/ZviBaratz/atrium/session"
)

// Within-group sort mode (creation vs status) and the manual-reorder ordering
// it toggles: applying the sort, persisting the manual order, and the slice
// helpers that keep same-repo sessions grouped.

// groupModeAccount is the account-clustering group mode (config.GroupModeAccount),
// compared as a bare literal so ui needs no config import — mirroring how sortMode
// uses "status"/"creation".
const groupModeAccount = "account"

// sortActive reports whether a non-creation within-group sort mode is in effect.
func (l *List) sortActive() bool {
	return l.sortMode != "" && l.sortMode != "creation"
}

// accountGrouped reports whether the top-level grouping clusters by Claude account.
func (l *List) accountGrouped() bool {
	return l.groupMode == groupModeAccount
}

// viewActive reports whether any display transform (within-group sort or account
// clustering) is deriving items from the manual snapshot. It generalizes the former
// sortActive() gate: manual is populated and items is a computed view whenever this
// is true.
func (l *List) viewActive() bool {
	return l.sortActive() || l.accountGrouped()
}

// ManualReorderEnabled reports whether J/K and { } manual reordering apply. Any
// active view (sort or account grouping) computes the order, so reordering is
// disabled there and the app surfaces a hint instead.
func (l *List) ManualReorderEnabled() bool {
	return !l.viewActive()
}

// AccountGrouped reports whether the list is clustering by Claude account. The app
// uses it to explain why whole-group reorder ({ / }) is inert: block order is owned
// by the clustering there, not the manual order. Group moves stay available under a
// status sort (see syncManualGroupOrder), so this is a narrower query than
// ManualReorderEnabled, which also bars J/K.
func (l *List) AccountGrouped() bool {
	return l.accountGrouped()
}

// InstancesForPersist returns the instances in canonical (manual) order so a view
// transform never overwrites the user's manual order on disk. With no view active
// the canonical order is simply items.
func (l *List) InstancesForPersist() []*session.Instance {
	if l.viewActive() {
		return l.manual
	}
	return l.items
}

// enterView snapshots the current items into manual the first time any view
// transform becomes active. Idempotent: a second transform activating while one is
// already active must not re-snapshot (manual already holds the canonical order).
func (l *List) enterView() {
	if l.manual == nil {
		l.manual = make([]*session.Instance, len(l.items))
		copy(l.manual, l.items)
	}
}

// exitViewInactive restores items from the manual snapshot and drops it once no view
// transform remains active. The selection is preserved by identity.
func (l *List) exitViewInactive() {
	if l.viewActive() || l.manual == nil {
		return
	}
	sel := l.GetSelectedInstance()
	l.items = l.manual
	l.manual = nil
	if sel != nil {
		l.SelectInstance(sel)
	}
}

// SetSortMode switches the within-group ordering. "" / "creation" restores the
// manual order; any other value sorts each repo group by action-priority. The
// selected session is preserved by identity.
func (l *List) SetSortMode(mode string) {
	if mode == "" {
		mode = "creation"
	}
	if mode == l.sortMode {
		return
	}
	l.sortMode = mode
	if l.viewActive() {
		l.enterView()
		l.rebuildView()
	} else {
		l.exitViewInactive()
	}
}

// SetGroupMode switches the top-level grouping. "" / "repo" restores repo groups in
// manual order; "account" clusters repo blocks by Claude account. The selected
// session is preserved by identity.
func (l *List) SetGroupMode(mode string) {
	if mode == "" {
		mode = "repo"
	}
	if mode == l.groupMode {
		return
	}
	l.groupMode = mode
	if l.viewActive() {
		l.enterView()
		l.rebuildView()
	} else {
		l.exitViewInactive()
	}
}

// ApplySort re-derives the status-sensitive view from the latest statuses; the
// metadata poll calls it each tick. Only the within-group status sort shifts as
// statuses update, so this is a no-op unless a sort mode is active: account
// clustering keys on each session's account and repo, both fixed at creation and
// changed only via Add/Kill (which rebuild the view directly), so re-clustering on
// every tick would allocate and recompute a result that never moved. Returns whether
// the order changed.
func (l *List) ApplySort() bool {
	if !l.sortActive() {
		return false
	}
	return l.rebuildView()
}

// rebuildView rebuilds items as the canonical manual order transformed by the active
// view(s): first clustered by account (if account-grouped), then sorted within each
// repo group (if a sort mode is active). It is the single writer of items while a
// view is active; the selected instance is preserved by identity. Returns whether
// items changed.
func (l *List) rebuildView() bool {
	if !l.viewActive() || l.manual == nil {
		return false
	}
	sel := l.GetSelectedInstance()
	next := make([]*session.Instance, len(l.manual))
	copy(next, l.manual)
	if l.accountGrouped() {
		next = clusterByAccount(next)
	}
	if l.sortActive() {
		sortWithinRepoGroups(next)
	}
	if sameOrder(l.items, next) {
		return false
	}
	l.items = next
	if sel != nil {
		l.SelectInstance(sel)
	} else {
		l.clampSelectionToNavigable()
	}
	return true
}

// forEachRepoBlock calls fn once per maximal contiguous run of items sharing a
// repoKey — the repo-block primitive the manual invariant guarantees. It is the one
// definition of "walk the repo blocks left to right"; sortWithinRepoGroups and
// clusterByAccount both build on it so the block-detection logic lives in a single
// place. (groupBounds walks outward from a known index, and String's render loop
// interleaves rendering, so those keep their own traversal.)
func forEachRepoBlock(items []*session.Instance, fn func(start, end int)) {
	for start := 0; start < len(items); {
		key := repoKey(items[start])
		end := start + 1
		for end < len(items) && repoKey(items[end]) == key {
			end++
		}
		fn(start, end)
		start = end
	}
}

// sortWithinRepoGroups stable-sorts each contiguous repo block of items by action-
// priority (NeedsInput first, then unread Ready, …), leaving block order and
// membership untouched. Extracted from the former applySort.
func sortWithinRepoGroups(items []*session.Instance) {
	forEachRepoBlock(items, func(start, end int) {
		grp := items[start:end]
		sort.SliceStable(grp, func(i, j int) bool {
			return session.StatusUrgency(grp[i].GetStatus(), grp[i].Unread()) <
				session.StatusUrgency(grp[j].GetStatus(), grp[j].Unread())
		})
	})
}

// clusterByAccount reorders whole repo blocks so blocks sharing a Claude account are
// contiguous, without disturbing any block's internal order. Input must be repo-
// contiguous (the manual invariant). Accounts are emitted in first-appearance order;
// the no-account ("") bucket trails last. Repo blocks within an account keep their
// first-appearance order. Pure function — the view deriver, not a state mutator.
func clusterByAccount(items []*session.Instance) []*session.Instance {
	type block struct {
		items []*session.Instance
		acct  string
	}
	var blocks []block
	forEachRepoBlock(items, func(start, end int) {
		blocks = append(blocks, block{items: items[start:end], acct: accountKey(items[start])})
	})
	order := make([]string, 0, len(blocks))
	seen := map[string]bool{}
	for _, b := range blocks {
		if b.acct != "" && !seen[b.acct] {
			seen[b.acct] = true
			order = append(order, b.acct)
		}
	}
	order = append(order, "") // no-account bucket trails last
	out := make([]*session.Instance, 0, len(items))
	for _, acct := range order {
		for _, b := range blocks {
			if b.acct == acct {
				out = append(out, b.items...)
			}
		}
	}
	return out
}

// sameOrder reports whether a and b hold the same instances in the same positions.
func sameOrder(a, b []*session.Instance) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// insertByRepo inserts inst into items immediately after the last existing entry
// sharing its repoKey (appending a new group if none), mirroring AddInstance's
// placement. Keeps the canonical manual order in step while a sort mode is active.
func insertByRepo(items []*session.Instance, inst *session.Instance) []*session.Instance {
	key := repoKey(inst)
	insertAt := len(items)
	for i, item := range items {
		if repoKey(item) == key {
			insertAt = i + 1
		}
	}
	items = append(items, nil)
	copy(items[insertAt+1:], items[insertAt:])
	items[insertAt] = inst
	return items
}

// removeInstance drops the first occurrence of target from items, returning the
// shortened slice (unchanged if target is absent).
func removeInstance(items []*session.Instance, target *session.Instance) []*session.Instance {
	for i, it := range items {
		if it == target {
			return append(items[:i], items[i+1:]...)
		}
	}
	return items
}

// regroupManualLike reorders manual's repo-group blocks to match the group order in
// like (the just-reordered display list), preserving manual's within-group order.
// Used after a whole-group move so the canonical order tracks the new group sequence
// without disturbing the within-group (manual) order.
//
// like and manual hold the same instances (a whole-group move only permutes group
// order), so every manual group is normally covered by like. The trailing pass that
// appends any group key absent from like is a safety net: should the two ever diverge
// on membership, those sessions are kept (appended in manual order) rather than
// silently dropped from the canonical order and then persisted away.
func regroupManualLike(manual, like []*session.Instance) []*session.Instance {
	byKey := map[string][]*session.Instance{}
	keyOrder := make([]string, 0)
	for _, it := range manual {
		k := repoKey(it)
		if _, ok := byKey[k]; !ok {
			keyOrder = append(keyOrder, k)
		}
		byKey[k] = append(byKey[k], it)
	}
	out := make([]*session.Instance, 0, len(manual))
	seen := map[string]bool{}
	emit := func(k string) {
		if seen[k] {
			return
		}
		seen[k] = true
		out = append(out, byKey[k]...)
	}
	for _, it := range like {
		emit(repoKey(it))
	}
	// Keep any manual group like never mentioned (see doc comment).
	for _, k := range keyOrder {
		emit(k)
	}
	return out
}
