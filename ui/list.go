package ui

import (
	"claude-squad/log"
	"claude-squad/session"
	"errors"
	"fmt"
	"path/filepath"
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

// renderRepoHeader renders a repo group header as an uppercased name followed by a
// dim rule filling the rest of the panel width, so it reads as a section divider.
func (l *List) renderRepoHeader(key string) string {
	name := strings.ToUpper(key)
	header := repoHeaderStyle.Render(name)
	// repoHeaderStyle pads the name with one space on each side.
	ruleLen := AdjustPreviewWidth(l.width) - runewidth.StringWidth(name) - 2
	if ruleLen < 0 {
		ruleLen = 0
	}
	return header + repoRuleStyle.Render(strings.Repeat("─", ruleLen))
}

type List struct {
	items         []*session.Instance
	selectedIdx   int
	height, width int
	renderer      *InstanceRenderer
	autoyes       bool

	// map of repo name to number of instances using it. Used to display the repo name only if there are
	// multiple repos in play.
	repos map[string]int
}

func NewList(spinner *spinner.Model, autoYes bool) *List {
	return &List{
		items:    []*session.Instance{},
		renderer: &InstanceRenderer{spinner: spinner},
		repos:    make(map[string]int),
		autoyes:  autoYes,
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

	// Render the list, grouping consecutive same-repo instances under a header. Headers
	// are emitted within the item loop (not as selectable rows), so selectedIdx stays a
	// flat index into l.items and navigation/reorder are unaffected. Items are not sorted:
	// headers annotate the runs in the user's existing (reorderable) order.
	showRepos := len(l.repos) > 1
	prevRepo := ""
	for i, item := range l.items {
		key := repoKey(item)
		newGroup := showRepos && key != prevRepo
		if i > 0 {
			// Looser spacing before a new repo group, tighter between items in a group.
			if newGroup {
				b.WriteString("\n\n")
			} else {
				b.WriteString("\n")
			}
		}
		if newGroup {
			b.WriteString(l.renderRepoHeader(key))
			b.WriteString("\n")
			prevRepo = key
		}
		b.WriteString(l.renderer.Render(item, i+1, i == l.selectedIdx))
	}
	return lipgloss.Place(l.width, l.height, lipgloss.Left, lipgloss.Top, b.String())
}

// Down selects the next item in the list.
func (l *List) Down() {
	if len(l.items) == 0 {
		return
	}
	if l.selectedIdx < len(l.items)-1 {
		l.selectedIdx++
	} else {
		l.selectedIdx = 0
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

	// Unregister the reponame.
	repoName, err := targetInstance.RepoName()
	if err != nil {
		log.ErrorLog.Printf("could not get repo name: %v", err)
	} else {
		l.rmRepo(repoName)
	}

	// Since there's items after this, the selectedIdx can stay the same.
	l.items = append(l.items[:l.selectedIdx], l.items[l.selectedIdx+1:]...)
}

func (l *List) Attach() (chan struct{}, error) {
	targetInstance := l.items[l.selectedIdx]
	return targetInstance.Attach()
}

// Up selects the prev item in the list.
func (l *List) Up() {
	if len(l.items) == 0 {
		return
	}
	if l.selectedIdx > 0 {
		l.selectedIdx--
	} else {
		l.selectedIdx = len(l.items) - 1
	}
}

func (l *List) addRepo(repo string) {
	if _, ok := l.repos[repo]; !ok {
		l.repos[repo] = 0
	}
	l.repos[repo]++
}

func (l *List) rmRepo(repo string) {
	if _, ok := l.repos[repo]; !ok {
		log.ErrorLog.Printf("repo %s not found", repo)
		return
	}
	l.repos[repo]--
	if l.repos[repo] == 0 {
		delete(l.repos, repo)
	}
}

// AddInstance adds a new instance to the list. It returns a finalizer function that should be called when the instance
// is started. If the instance was restored from storage or is paused, you can call the finalizer immediately.
// When creating a new one and entering the name, you want to call the finalizer once the name is done.
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
	// Keep the selection on the same logical item if we inserted at or before it.
	if insertAt <= l.selectedIdx && len(l.items) > 1 {
		l.selectedIdx++
	}
	// The finalizer registers the repo name once the instance is started.
	return func() {
		repoName, err := instance.RepoName()
		if err != nil {
			log.ErrorLog.Printf("could not get repo name: %v", err)
			return
		}

		l.addRepo(repoName)
	}
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

// SelectInstance finds and selects the given instance in the list.
func (l *List) SelectInstance(target *session.Instance) {
	for i, inst := range l.items {
		if inst == target {
			l.SetSelectedInstance(i)
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
	if repoKey(l.items[l.selectedIdx]) != repoKey(l.items[l.selectedIdx+1]) {
		return false
	}
	l.items[l.selectedIdx], l.items[l.selectedIdx+1] = l.items[l.selectedIdx+1], l.items[l.selectedIdx]
	l.selectedIdx++
	return true
}

// GetInstances returns all instances in the list
func (l *List) GetInstances() []*session.Instance {
	return l.items
}
