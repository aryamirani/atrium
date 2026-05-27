package ui

import (
	"claude-squad/log"
	"claude-squad/session"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

const readyIcon = "● "
const pausedIcon = "⏸ "
const needsInputIcon = "◆ "

var readyStyle = lipgloss.NewStyle().
	Foreground(lipgloss.AdaptiveColor{Light: "#51bd73", Dark: "#51bd73"})

// workingStyle tints the busy spinner cyan: a cool, low-attention "in progress" that
// reads distinctly from the green ready dot without competing with it.
var workingStyle = lipgloss.NewStyle().
	Foreground(lipgloss.AdaptiveColor{Light: "#0e7490", Dark: "#39c5cf"})

// needsInputStyle (amber) marks a session blocked on a prompt — the one state that wants
// your attention, so it gets the attention color.
var needsInputStyle = lipgloss.NewStyle().
	Foreground(lipgloss.AdaptiveColor{Light: "#b8860b", Dark: "#d79921"})

var addedLinesStyle = lipgloss.NewStyle().
	Foreground(lipgloss.AdaptiveColor{Light: "#51bd73", Dark: "#51bd73"})

var removedLinesStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#de613e"))

// behindStyle (amber) is the only attention color in the git-context cluster: being
// behind the base implies an action (consider rebasing). commitsStyle/dirtyStyle are
// muted because they are status, not problems — this keeps the list scannable.
var behindStyle = lipgloss.NewStyle().
	Foreground(lipgloss.AdaptiveColor{Light: "#b8860b", Dark: "#d79921"})

var commitsStyle = lipgloss.NewStyle().
	Foreground(lipgloss.AdaptiveColor{Light: "#888888", Dark: "#999999"})

var dirtyStyle = lipgloss.NewStyle().
	Foreground(lipgloss.AdaptiveColor{Light: "#888888", Dark: "#999999"})

var pausedStyle = lipgloss.NewStyle().
	Foreground(lipgloss.AdaptiveColor{Light: "#888888", Dark: "#888888"})

var titleStyle = lipgloss.NewStyle().
	Padding(0, 1, 0, 1).
	Foreground(lipgloss.AdaptiveColor{Light: "#1a1a1a", Dark: "#dddddd"})

var listDescStyle = lipgloss.NewStyle().
	Padding(0, 1, 0, 1).
	Foreground(lipgloss.AdaptiveColor{Light: "#A49FA5", Dark: "#777777"})

var selectedTitleStyle = lipgloss.NewStyle().
	Padding(0, 1, 0, 1).
	Bold(true).
	Foreground(lipgloss.AdaptiveColor{Light: "#1a1a1a", Dark: "#ffffff"})

var selectedDescStyle = lipgloss.NewStyle().
	Padding(0, 1, 0, 1).
	Foreground(lipgloss.AdaptiveColor{Light: "#3a3a3a", Dark: "#bbbbbb"})

var mainTitle = lipgloss.NewStyle().
	Background(lipgloss.Color("62")).
	Foreground(lipgloss.Color("230"))

var autoYesStyle = lipgloss.NewStyle().
	Background(lipgloss.Color("#dde4f0")).
	Foreground(lipgloss.Color("#1a1a1a"))

var repoHeaderStyle = lipgloss.NewStyle().
	Padding(0, 1).
	Bold(true).
	Foreground(lipgloss.AdaptiveColor{Light: "#666666", Dark: "#9b9b9b"})

// repoRuleStyle renders the dim divider rule trailing a repo header.
var repoRuleStyle = lipgloss.NewStyle().
	Foreground(lipgloss.AdaptiveColor{Light: "#d7d7d7", Dark: "#3C3C3C"})

// selectedItemStyle draws a left accent bar down the selected item (reusing the
// panel's violet highlightColor from tabbed_window.go); unselectedItemStyle adds
// matching left padding so item text stays aligned as the selection moves.
var selectedItemStyle = lipgloss.NewStyle().
	Border(lipgloss.Border{Left: "▎"}, false, false, false, true).
	BorderForeground(highlightColor)

var unselectedItemStyle = lipgloss.NewStyle().PaddingLeft(1)

// repoKey returns the grouping key for an instance: its repo name once started,
// falling back to the base name of its repo path for not-yet-started instances.
func repoKey(i *session.Instance) string {
	if name, err := i.RepoName(); err == nil {
		return name
	}
	return filepath.Base(i.Path)
}

// distinctRepoCount returns how many distinct repo groups are present across the current
// items, computed from the items themselves via repoKey. Deriving it at render time (rather
// than tracking an incrementally-maintained counter) makes it impossible for the header
// decision to drift from what is actually displayed — the counter could fall out of sync
// during churn, since it was only updated when an instance was started.
func (l *List) distinctRepoCount() int {
	seen := make(map[string]struct{}, len(l.items))
	for _, item := range l.items {
		seen[repoKey(item)] = struct{}{}
	}
	return len(seen)
}

// renderRepoHeader renders a repo group header as a fold marker, the uppercased name (with a
// member count when collapsed), and a dim rule filling the rest of the panel width, so it
// reads as a section divider. A collapsed group's header doubles as its selectable row, so it
// gets the same left accent bar as a selected item when selected is true.
func (l *List) renderRepoHeader(key string, collapsed bool, count int, selected bool) string {
	marker := "▾ "
	if collapsed {
		marker = "▸ "
	}
	name := marker + strings.ToUpper(key)
	if collapsed {
		name = fmt.Sprintf("%s (%d)", name, count)
	}
	header := repoHeaderStyle.Render(name)
	// repoHeaderStyle pads the name with one space on each side; a selected header also gains
	// a one-cell left accent bar, so reserve for both when sizing the trailing rule.
	ruleLen := AdjustPreviewWidth(l.width) - runewidth.StringWidth(name) - 2
	if selected {
		ruleLen--
	}
	if ruleLen < 0 {
		ruleLen = 0
	}
	line := header + repoRuleStyle.Render(strings.Repeat("─", ruleLen))
	if selected {
		return selectedItemStyle.Render(line)
	}
	return line
}

type List struct {
	items         []*session.Instance
	selectedIdx   int
	height, width int
	renderer      *InstanceRenderer
	autoyes       bool
	// collapsed records which repo groups are folded, keyed by repoKey. It is a pure
	// display/navigation flag — never authoritative over membership or order, which stay
	// derived from items. All reads go through effectiveCollapsed so the "only meaningful
	// with >1 repo" rule is enforced in exactly one place.
	collapsed map[string]bool
}

func NewList(spinner *spinner.Model, autoYes bool) *List {
	return &List{
		items:     []*session.Instance{},
		renderer:  &InstanceRenderer{spinner: spinner},
		autoyes:   autoYes,
		collapsed: map[string]bool{},
	}
}

// SetSize sets the height and width of the list.
func (l *List) SetSize(width, height int) {
	l.width = width
	l.height = height
	l.renderer.setWidth(width)
}

// SetSessionPreviewSize sets the height and width for the tmux sessions. This makes the stdout line have the correct
// width and height.
func (l *List) SetSessionPreviewSize(width, height int) (err error) {
	for i, item := range l.items {
		if !item.Started() || item.Paused() {
			continue
		}

		if innerErr := item.SetPreviewSize(width, height); innerErr != nil {
			err = errors.Join(
				err, fmt.Errorf("could not set preview size for instance %d: %v", i, innerErr))
		}
	}
	return
}

func (l *List) NumInstances() int {
	return len(l.items)
}

// InstanceRenderer handles rendering of session.Instance objects
type InstanceRenderer struct {
	spinner *spinner.Model
	width   int
}

func (r *InstanceRenderer) setWidth(width int) {
	r.width = AdjustPreviewWidth(width)
}

// ɹ and ɻ are other options.
const branchIcon = "Ꮧ"

func (r *InstanceRenderer) Render(i *session.Instance, idx int, selected bool) string {
	prefix := fmt.Sprintf(" %d ", idx)
	titleS := selectedTitleStyle
	descS := selectedDescStyle
	if !selected {
		titleS = titleStyle
		descS = listDescStyle
	}

	// add spinner next to title if it's running
	var join string
	switch i.Status {
	case session.Running, session.Loading:
		join = fmt.Sprintf("%s ", workingStyle.Render(r.spinner.View()))
	case session.Ready:
		join = readyStyle.Render(readyIcon)
	case session.NeedsInput:
		join = needsInputStyle.Render(needsInputIcon)
	case session.Paused:
		join = pausedStyle.Render(pausedIcon)
	default:
	}

	// Cut the title if it's too long
	titleText := i.DisplayName()
	widthAvail := r.width - 3 - runewidth.StringWidth(prefix) - 1
	if widthAvail > 0 && runewidth.StringWidth(titleText) > widthAvail {
		titleText = runewidth.Truncate(titleText, widthAvail-3, "...")
	}
	title := titleS.Render(lipgloss.JoinHorizontal(
		lipgloss.Left,
		lipgloss.Place(r.width-3, 1, lipgloss.Left, lipgloss.Center, fmt.Sprintf("%s %s", prefix, titleText)),
		" ",
		join,
	))

	stat := i.GetDiffStats()

	var diff string
	var addedDiff, removedDiff string
	if stat == nil || stat.Error != nil || stat.IsEmpty() {
		// Don't show diff stats if there's an error or if they don't exist
		addedDiff = ""
		removedDiff = ""
		diff = ""
	} else {
		addedDiff = fmt.Sprintf("+%d", stat.Added)
		removedDiff = fmt.Sprintf("-%d ", stat.Removed)
		diff = lipgloss.JoinHorizontal(
			lipgloss.Center,
			addedLinesStyle.Background(descS.GetBackground()).Render(addedDiff),
			lipgloss.Style{}.Background(descS.GetBackground()).Foreground(descS.GetForeground()).Render(","),
			removedLinesStyle.Background(descS.GetBackground()).Render(removedDiff),
		)
	}

	// Build the git-context cluster (behind / commits ahead / dirty), shown just
	// left of the diff stats. Each segment appears only when meaningful, so the
	// common steady state is unchanged. ctxPlain mirrors ctxStyled without ANSI so
	// the width math below stays correct.
	var ctxPlain, ctxStyled string
	if stat != nil && stat.Error == nil {
		bg := descS.GetBackground()
		sep := lipgloss.NewStyle().Background(bg).Render(" ")
		if stat.Behind > 0 {
			s := fmt.Sprintf("⇣%d", stat.Behind)
			ctxPlain += s + " "
			ctxStyled += behindStyle.Background(bg).Render(s) + sep
		}
		if stat.Commits > 0 {
			s := fmt.Sprintf("⇡%d", stat.Commits)
			ctxPlain += s + " "
			ctxStyled += commitsStyle.Background(bg).Render(s) + sep
		}
		if stat.Dirty {
			s := "*"
			ctxPlain += s + " "
			ctxStyled += dirtyStyle.Background(bg).Render(s) + sep
		}
	}

	remainingWidth := r.width
	remainingWidth -= runewidth.StringWidth(prefix)
	remainingWidth -= runewidth.StringWidth(branchIcon)
	remainingWidth -= 2 // for the literal " " and "-" in the branchLine format string

	diffWidth := runewidth.StringWidth(addedDiff) + runewidth.StringWidth(removedDiff)
	if diffWidth > 0 {
		diffWidth += 1
	}

	// Use fixed width for diff stats to avoid layout issues
	remainingWidth -= diffWidth
	remainingWidth -= runewidth.StringWidth(ctxPlain)

	// If the context cluster doesn't fit, drop it rather than truncate the branch
	// to nothing.
	if remainingWidth < 0 {
		remainingWidth += runewidth.StringWidth(ctxPlain)
		ctxStyled = ""
	}

	branch := i.Branch
	// Don't show branch if there's no space for it. Or show ellipsis if it's too long.
	branchWidth := runewidth.StringWidth(branch)
	if remainingWidth < 0 {
		branch = ""
	} else if remainingWidth < branchWidth {
		if remainingWidth < 3 {
			branch = ""
		} else {
			// We know the remainingWidth is at least 4 and branch is longer than that, so this is safe.
			branch = runewidth.Truncate(branch, remainingWidth-3, "...")
		}
	}
	remainingWidth -= runewidth.StringWidth(branch)

	// Add spaces to fill the remaining width.
	spaces := ""
	if remainingWidth > 0 {
		spaces = strings.Repeat(" ", remainingWidth)
	}

	branchLine := fmt.Sprintf("%s %s-%s%s%s%s", strings.Repeat(" ", len(prefix)), branchIcon, branch, spaces, ctxStyled, diff)

	// join title and subtitle
	text := lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		descS.Render(branchLine),
	)

	// Selected items get a left accent bar; others get matching left padding so the
	// text stays aligned as the selection moves.
	if selected {
		return selectedItemStyle.Render(text)
	}
	return unselectedItemStyle.Render(text)
}

func (l *List) String() string {
	const titleText = " Instances "
	const autoYesText = " auto-yes "

	// Write the title.
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString("\n")

	// Write title line
	// add padding of 2 because the border on list items adds some extra characters
	titleWidth := AdjustPreviewWidth(l.width) + 2
	if !l.autoyes {
		b.WriteString(lipgloss.Place(
			titleWidth, 1, lipgloss.Left, lipgloss.Bottom, mainTitle.Render(titleText)))
	} else {
		title := lipgloss.Place(
			titleWidth/2, 1, lipgloss.Left, lipgloss.Bottom, mainTitle.Render(titleText))
		autoYes := lipgloss.Place(
			titleWidth-(titleWidth/2), 1, lipgloss.Right, lipgloss.Bottom, autoYesStyle.Render(autoYesText))
		b.WriteString(lipgloss.JoinHorizontal(
			lipgloss.Top, title, autoYes))
	}

	b.WriteString("\n")
	b.WriteString("\n")

	// Render the list group by group, in the user's existing (reorderable) order. Headers are
	// shown only with more than one repo, and are not selectable rows for expanded groups, so
	// selectedIdx stays a flat index into l.items. A collapsed group renders only its header
	// (which doubles as its anchor's row) and suppresses its members.
	showRepos := l.distinctRepoCount() > 1
	first := true
	for i := 0; i < len(l.items); {
		key := repoKey(l.items[i])
		start, end := l.groupBounds(i)
		collapsed := showRepos && l.collapsed[key]

		// Looser spacing before each group; tighter spacing between items is handled below.
		if !first {
			b.WriteString("\n\n")
		}
		first = false

		if showRepos {
			b.WriteString(l.renderRepoHeader(key, collapsed, end-start, collapsed && l.selectedIdx == start))
			if !collapsed {
				b.WriteString("\n")
			}
		}
		if !collapsed {
			for j := start; j < end; j++ {
				if j > start {
					b.WriteString("\n")
				}
				b.WriteString(l.renderer.Render(l.items[j], j+1, j == l.selectedIdx))
			}
		}
		i = end
	}
	return lipgloss.Place(l.width, l.height, lipgloss.Left, lipgloss.Top, b.String())
}

// Down selects the next visible item in the list, wrapping at the end and skipping the hidden
// members of collapsed groups.
func (l *List) Down() {
	if len(l.items) == 0 {
		return
	}
	idx := l.selectedIdx
	for i := 0; i < len(l.items); i++ {
		if idx < len(l.items)-1 {
			idx++
		} else {
			idx = 0
		}
		if !l.isHidden(idx) {
			l.selectedIdx = idx
			return
		}
	}
}

// Kill selects the next item in the list.
func (l *List) Kill() {
	if len(l.items) == 0 {
		return
	}
	targetInstance := l.items[l.selectedIdx]

	// Kill the tmux session
	if err := targetInstance.Kill(); err != nil {
		log.ErrorLog.Printf("could not kill instance: %v", err)
	}

	// If you delete the last one in the list, select the previous one.
	if l.selectedIdx == len(l.items)-1 {
		defer l.Up()
	}

	// Since there's items after this, the selectedIdx can stay the same.
	l.items = append(l.items[:l.selectedIdx], l.items[l.selectedIdx+1:]...)
	// Removing an item can shift the selection onto a now-hidden index (or off the end), so
	// re-establish the navigable-selection invariant.
	l.clampSelectionToNavigable()
}

func (l *List) Attach() (chan struct{}, error) {
	targetInstance := l.items[l.selectedIdx]
	return targetInstance.Attach()
}

// Up selects the prev visible item in the list, wrapping at the top and skipping the hidden
// members of collapsed groups.
func (l *List) Up() {
	if len(l.items) == 0 {
		return
	}
	idx := l.selectedIdx
	for i := 0; i < len(l.items); i++ {
		if idx > 0 {
			idx--
		} else {
			idx = len(l.items) - 1
		}
		if !l.isHidden(idx) {
			l.selectedIdx = idx
			return
		}
	}
}

// AddInstance adds a new instance to the list. It returns a finalizer function that callers
// invoke once the instance has started (restored/paused instances may call it immediately;
// the new-session flow calls it once the name is entered). The finalizer is currently a
// no-op — repo grouping is derived from the items at render time (see distinctRepoCount), so
// there is no per-instance state to register on start — but it is retained as the start-time
// lifecycle hook the new-session flow is built around.
//
// The instance is inserted immediately after the last existing item sharing its repoKey, so it becomes the next entry
// in that repo's group; if no item shares the key, it is appended as a new group. This keeps same-repo instances
// contiguous, which is the invariant String() relies on to avoid emitting a repo header more than once per group. It
// also self-migrates state loaded from storage, which is added one instance at a time through this method.
func (l *List) AddInstance(instance *session.Instance) (finalize func()) {
	key := repoKey(instance)
	insertAt := len(l.items)
	for i, item := range l.items {
		if repoKey(item) == key {
			insertAt = i + 1
		}
	}
	l.items = append(l.items, nil)
	copy(l.items[insertAt+1:], l.items[insertAt:])
	l.items[insertAt] = instance
	// A newly added session must never land hidden inside a folded group, so expand its group.
	// During startup restore the collapsed set is still empty (it is applied after this loop),
	// so this is a no-op there and doesn't clobber persisted folds.
	delete(l.collapsed, key)
	// Keep the selection on the same logical item if we inserted at or before it.
	if insertAt <= l.selectedIdx && len(l.items) > 1 {
		l.selectedIdx++
	}
	return func() {}
}

// GetSelectedInstance returns the currently selected instance
func (l *List) GetSelectedInstance() *session.Instance {
	if len(l.items) == 0 {
		return nil
	}
	return l.items[l.selectedIdx]
}

// SetSelectedInstance sets the selected index. Noop if the index is out of bounds.
func (l *List) SetSelectedInstance(idx int) {
	if idx >= len(l.items) {
		return
	}
	l.selectedIdx = idx
}

// SelectInstance finds and selects the given instance in the list. If the target sits inside a
// collapsed group, the selection snaps to that group's anchor so the cursor stays visible.
func (l *List) SelectInstance(target *session.Instance) {
	for i, inst := range l.items {
		if inst == target {
			l.SetSelectedInstance(i)
			l.clampSelectionToNavigable()
			return
		}
	}
}

// MoveUp swaps the selected instance with the one above it. The swap is confined to within a repo group: if the item
// above belongs to a different group, this is a no-op so the move cannot split a group and produce a duplicate header.
// (Single-repo lists share one key, so reordering stays unrestricted there.)
func (l *List) MoveUp() bool {
	if l.selectedIdx <= 0 || len(l.items) < 2 {
		return false
	}
	// A collapsed group shows no siblings to swap with, so within-group reorder is inert.
	if l.effectiveCollapsed(repoKey(l.items[l.selectedIdx])) {
		return false
	}
	if repoKey(l.items[l.selectedIdx]) != repoKey(l.items[l.selectedIdx-1]) {
		return false
	}
	l.items[l.selectedIdx], l.items[l.selectedIdx-1] = l.items[l.selectedIdx-1], l.items[l.selectedIdx]
	l.selectedIdx--
	return true
}

// MoveDown swaps the selected instance with the one below it. As with MoveUp, the swap is confined to within a repo
// group so it cannot split a group across a boundary.
func (l *List) MoveDown() bool {
	if l.selectedIdx >= len(l.items)-1 || len(l.items) < 2 {
		return false
	}
	if l.effectiveCollapsed(repoKey(l.items[l.selectedIdx])) {
		return false
	}
	if repoKey(l.items[l.selectedIdx]) != repoKey(l.items[l.selectedIdx+1]) {
		return false
	}
	l.items[l.selectedIdx], l.items[l.selectedIdx+1] = l.items[l.selectedIdx+1], l.items[l.selectedIdx]
	l.selectedIdx++
	return true
}

// groupBounds returns the [start, end) range of the contiguous run of items sharing the
// repoKey of the item at idx — i.e. the repo group idx belongs to. Returns an empty range
// for an out-of-bounds idx. This is the single primitive the whole-group operations build on.
func (l *List) groupBounds(idx int) (start, end int) {
	if idx < 0 || idx >= len(l.items) {
		return idx, idx
	}
	key := repoKey(l.items[idx])
	start = idx
	for start > 0 && repoKey(l.items[start-1]) == key {
		start--
	}
	end = idx + 1
	for end < len(l.items) && repoKey(l.items[end]) == key {
		end++
	}
	return start, end
}

// effectiveCollapsed reports whether a group is folded *and* folding is meaningful. Folding
// is only meaningful when more than one repo is present (headers don't render otherwise), so
// this guard lives here and every collapse read goes through it — preventing a stale flag from
// hiding the sole remaining group after others are killed.
func (l *List) effectiveCollapsed(key string) bool {
	return l.distinctRepoCount() > 1 && l.collapsed[key]
}

// isHidden reports whether the item at idx is suppressed from view: a member of a collapsed
// group other than its first item (the anchor), which stands in for the whole group.
func (l *List) isHidden(idx int) bool {
	if !l.effectiveCollapsed(repoKey(l.items[idx])) {
		return false
	}
	start, _ := l.groupBounds(idx)
	return idx != start
}

// clampSelectionToNavigable enforces the invariant that the selection always rests on a
// visible item. It snaps an out-of-bounds or hidden selectedIdx to its group anchor. This is
// the single place that invariant is maintained, so every mutation just calls it afterward.
func (l *List) clampSelectionToNavigable() {
	if len(l.items) == 0 {
		l.selectedIdx = 0
		return
	}
	if l.selectedIdx < 0 {
		l.selectedIdx = 0
	} else if l.selectedIdx >= len(l.items) {
		l.selectedIdx = len(l.items) - 1
	}
	if l.isHidden(l.selectedIdx) {
		l.selectedIdx, _ = l.groupBounds(l.selectedIdx)
	}
}

// ToggleCollapse folds or unfolds the selected session's repo group. It is a no-op (returns
// false) when fewer than two repos are present, since folding is meaningless there.
func (l *List) ToggleCollapse() bool {
	if len(l.items) == 0 || l.distinctRepoCount() <= 1 {
		return false
	}
	key := repoKey(l.items[l.selectedIdx])
	if l.collapsed[key] {
		delete(l.collapsed, key)
	} else {
		l.collapsed[key] = true
	}
	l.clampSelectionToNavigable()
	return true
}

// ToggleCollapseAll folds every group if any is currently expanded, otherwise unfolds every
// group. No-op (returns false) with fewer than two repos.
func (l *List) ToggleCollapseAll() bool {
	if len(l.items) == 0 || l.distinctRepoCount() <= 1 {
		return false
	}
	anyExpanded := false
	for i := 0; i < len(l.items); {
		_, end := l.groupBounds(i)
		if !l.collapsed[repoKey(l.items[i])] {
			anyExpanded = true
		}
		i = end
	}
	if anyExpanded {
		for i := 0; i < len(l.items); {
			_, end := l.groupBounds(i)
			l.collapsed[repoKey(l.items[i])] = true
			i = end
		}
	} else {
		l.collapsed = map[string]bool{}
	}
	l.clampSelectionToNavigable()
	return true
}

// CollapsedRepos returns the collapsed repo keys still present in the list, sorted for stable
// output. Pruning to live keys happens here (at save time) only — never on load, where the
// instance set is still being assembled.
func (l *List) CollapsedRepos() []string {
	present := map[string]struct{}{}
	for _, item := range l.items {
		present[repoKey(item)] = struct{}{}
	}
	keys := make([]string, 0, len(l.collapsed))
	for k := range l.collapsed {
		if _, ok := present[k]; ok {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys
}

// SetCollapsedRepos replaces the collapsed set (used to restore persisted state on startup).
func (l *List) SetCollapsedRepos(keys []string) {
	l.collapsed = make(map[string]bool, len(keys))
	for _, k := range keys {
		l.collapsed[k] = true
	}
	l.clampSelectionToNavigable()
}

// MoveGroupUp moves the selected session's entire repo group above the group immediately
// preceding it, as a unit, keeping the same session selected. It is a no-op when the group is
// already first (which also covers the single-group case). Returns whether anything moved.
func (l *List) MoveGroupUp() bool {
	start, end := l.groupBounds(l.selectedIdx)
	if start <= 0 {
		return false
	}
	prevStart, _ := l.groupBounds(start - 1)
	sel := l.items[l.selectedIdx]
	reordered := make([]*session.Instance, 0, len(l.items))
	reordered = append(reordered, l.items[:prevStart]...)
	reordered = append(reordered, l.items[start:end]...)
	reordered = append(reordered, l.items[prevStart:start]...)
	reordered = append(reordered, l.items[end:]...)
	l.items = reordered
	l.SelectInstance(sel)
	return true
}

// MoveGroupDown moves the selected session's entire repo group below the group immediately
// following it, as a unit, keeping the same session selected. It is a no-op when the group is
// already last (which also covers the single-group case). Returns whether anything moved.
func (l *List) MoveGroupDown() bool {
	start, end := l.groupBounds(l.selectedIdx)
	if end >= len(l.items) {
		return false
	}
	_, nextEnd := l.groupBounds(end)
	sel := l.items[l.selectedIdx]
	reordered := make([]*session.Instance, 0, len(l.items))
	reordered = append(reordered, l.items[:start]...)
	reordered = append(reordered, l.items[end:nextEnd]...)
	reordered = append(reordered, l.items[start:end]...)
	reordered = append(reordered, l.items[nextEnd:]...)
	l.items = reordered
	l.SelectInstance(sel)
	return true
}

// GetInstances returns all instances in the list
func (l *List) GetInstances() []*session.Instance {
	return l.items
}
