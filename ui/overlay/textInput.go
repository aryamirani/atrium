package overlay

import (
	"claude-squad/config"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	tiStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1, 2)

	tiTitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("62")).
			Bold(true).
			MarginBottom(1)

	tiButtonStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("7"))

	tiFocusedButtonStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("62")).
				Foreground(lipgloss.Color("0"))

	tiDividerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))

	tiLabelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("62")).
			Bold(true)

	tiHintStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Italic(true)
)

// pickerVisibleRows is the fixed number of list rows the directory and branch pickers
// always render (padding with blanks). Keeping it constant means the overlay's total
// height never changes as focus moves between sections — otherwise the vertically
// centered overlay (see app View / PlaceOverlay) would visibly jump on every Tab.
const pickerVisibleRows = 3

// promptVisibleRows is the fixed height of the prompt textarea, so the prompt section is
// also constant-height.
const promptVisibleRows = 4

// createFormHelp is the single footer line describing how to navigate the create form,
// replacing the per-field hints that were ambiguous about what Tab does.
const createFormHelp = "Tab/⇧Tab move · ↑↓ select · type to filter · Enter create"

// renderPickerRows renders a list of pre-formatted labels windowed around the cursor,
// always emitting exactly pickerVisibleRows lines (padding with blanks) so the caller's
// height is constant. When labels is empty, placeholder (if set) occupies the first row.
func renderPickerRows(labels []string, cursor int, focused bool, placeholder string, selStyle, dimStyle lipgloss.Style) string {
	lines := make([]string, 0, pickerVisibleRows)
	if len(labels) == 0 {
		if placeholder != "" {
			lines = append(lines, dimStyle.Render("  "+placeholder))
		}
	} else {
		start := 0
		if cursor >= pickerVisibleRows {
			start = cursor - pickerVisibleRows + 1
		}
		end := start + pickerVisibleRows
		if end > len(labels) {
			end = len(labels)
		}
		for i := start; i < end; i++ {
			switch {
			case i == cursor && focused:
				lines = append(lines, selStyle.Render("> "+labels[i]))
			case i == cursor:
				lines = append(lines, "  "+labels[i])
			default:
				lines = append(lines, dimStyle.Render("  "+labels[i]))
			}
		}
	}
	for len(lines) < pickerVisibleRows {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

// focusStop identifies a focusable component in the overlay. The overlay holds an
// ordered slice of the stops that are actually present, so adding or removing a
// component is a matter of editing that slice rather than juggling hardcoded indices.
type focusStop int

const (
	stopTitle focusStop = iota
	stopDirectory
	stopProfile
	stopTextarea
	stopBranch
	stopEnter
)

// TextInputOverlay represents a text input overlay with state management.
type TextInputOverlay struct {
	textarea        textarea.Model
	titleInput      textinput.Model
	Title           string
	FocusIndex      int // index into stops
	Submitted       bool
	Canceled        bool
	OnSubmit        func()
	width           int
	height          int
	directoryPicker *DirectoryPicker
	profilePicker   *ProfilePicker
	branchPicker    *BranchPicker
	stops           []focusStop // ordered focusable stops actually present
	isCreateForm    bool        // true for the new-session form (has a title field)
}

// NewTextInputOverlay creates a new text input overlay with the given title and initial value.
func NewTextInputOverlay(title string, initialValue string) *TextInputOverlay {
	ti := newTextarea(initialValue)
	overlay := &TextInputOverlay{
		textarea: ti,
		Title:    title,
		stops:    []focusStop{stopTextarea, stopEnter},
	}
	overlay.focusStop(stopTextarea)
	return overlay
}

// NewTextInputOverlayWithBranchPicker creates a text input overlay that includes a
// directory picker (for choosing the target repo) and a branch picker. Branch results
// are populated asynchronously via SetBranchResults. dirCandidates is the ordered list
// of candidate repo paths, with the default/contextual target first.
func NewTextInputOverlayWithBranchPicker(title string, initialValue string, profiles []config.Profile, dirCandidates []string) *TextInputOverlay {
	ti := newTextarea(initialValue)
	bp := NewBranchPicker()
	dp := NewDirectoryPicker(dirCandidates)

	var pp *ProfilePicker
	if len(profiles) > 0 {
		pp = NewProfilePicker(profiles)
	}

	// Build the ordered list of stops. Directory precedes branch because branches are
	// scoped to the chosen directory. Focus starts on the textarea so the user can type
	// the prompt immediately.
	stops := []focusStop{stopDirectory}
	if pp != nil && pp.HasMultiple() {
		stops = append(stops, stopProfile)
	}
	stops = append(stops, stopTextarea, stopBranch, stopEnter)

	overlay := &TextInputOverlay{
		textarea:        ti,
		Title:           title,
		directoryPicker: dp,
		profilePicker:   pp,
		branchPicker:    bp,
		stops:           stops,
	}
	overlay.focusStop(stopTextarea)
	return overlay
}

// NewSessionCreateOverlay creates the unified new-session form: a title field, a project
// (directory) picker, an optional profile picker (only when more than one profile
// exists), a branch picker, and a prompt textarea. Focus starts on the title so the user
// can name the session immediately, and every section renders at a constant height so the
// centered overlay does not jump as focus moves. dirCandidates is the ordered list of
// candidate repo paths with the default/contextual target first.
func NewSessionCreateOverlay(profiles []config.Profile, dirCandidates []string) *TextInputOverlay {
	ti := newTextarea("")
	bp := NewBranchPicker()
	dp := NewDirectoryPicker(dirCandidates)

	var pp *ProfilePicker
	if len(profiles) > 0 {
		pp = NewProfilePicker(profiles)
	}

	// Title first (where focus starts), then project, optional profile, branch, and finally
	// the prompt — branches are scoped to the chosen project, and the prompt comes last.
	stops := []focusStop{stopTitle, stopDirectory}
	if pp != nil && pp.HasMultiple() {
		stops = append(stops, stopProfile)
	}
	stops = append(stops, stopBranch, stopTextarea, stopEnter)

	overlay := &TextInputOverlay{
		textarea:        ti,
		titleInput:      newTitleInput(),
		Title:           "New session",
		directoryPicker: dp,
		profilePicker:   pp,
		branchPicker:    bp,
		stops:           stops,
		isCreateForm:    true,
	}
	overlay.focusStop(stopTitle)
	return overlay
}

func newTextarea(initialValue string) textarea.Model {
	ti := textarea.New()
	ti.SetValue(initialValue)
	ti.Focus()
	ti.ShowLineNumbers = false
	ti.Prompt = ""
	ti.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ti.CharLimit = 0
	ti.MaxHeight = 0
	return ti
}

// newTitleInput builds the single-line session-title field, capped at 32 characters to
// match the inline-naming limit enforced in the quick `n` flow.
func newTitleInput() textinput.Model {
	in := textinput.New()
	in.Prompt = ""
	in.CharLimit = 32
	return in
}

func (t *TextInputOverlay) SetSize(width, height int) {
	t.width = width
	t.height = height
	// The create form keeps every section at a constant height so the centered overlay
	// does not jump; the prompt textarea therefore uses a fixed height rather than scaling
	// with the window. The plain prompt overlay keeps its original full-height behavior.
	if t.isCreateForm {
		t.textarea.SetHeight(promptVisibleRows)
	} else {
		t.textarea.SetHeight(height)
	}
	t.titleInput.Width = width - 6
	if t.directoryPicker != nil {
		t.directoryPicker.SetWidth(width - 6)
	}
	if t.branchPicker != nil {
		t.branchPicker.SetWidth(width - 6)
	}
	if t.profilePicker != nil {
		t.profilePicker.SetWidth(width - 6)
	}
}

// Init initializes the text input overlay model
func (t *TextInputOverlay) Init() tea.Cmd {
	return textarea.Blink
}

// View renders the model's view
func (t *TextInputOverlay) View() string {
	return t.Render()
}

// currentStop returns the focusStop the FocusIndex currently points at.
func (t *TextInputOverlay) currentStop() focusStop {
	if t.FocusIndex < 0 || t.FocusIndex >= len(t.stops) {
		return stopEnter
	}
	return t.stops[t.FocusIndex]
}

func (t *TextInputOverlay) isTitle() bool           { return t.currentStop() == stopTitle }
func (t *TextInputOverlay) isDirectoryPicker() bool { return t.currentStop() == stopDirectory }
func (t *TextInputOverlay) isProfilePicker() bool   { return t.currentStop() == stopProfile }
func (t *TextInputOverlay) isTextarea() bool        { return t.currentStop() == stopTextarea }
func (t *TextInputOverlay) isBranchPicker() bool    { return t.currentStop() == stopBranch }
func (t *TextInputOverlay) isEnterButton() bool     { return t.currentStop() == stopEnter }

// indexOfStop returns the FocusIndex of a stop kind, or -1 if absent.
func (t *TextInputOverlay) indexOfStop(kind focusStop) int {
	for i, s := range t.stops {
		if s == kind {
			return i
		}
	}
	return -1
}

// focusStop moves focus to the given stop kind (if present) and syncs focus state.
func (t *TextInputOverlay) focusStop(kind focusStop) {
	if i := t.indexOfStop(kind); i >= 0 {
		t.setFocusIndex(i)
	}
}

// setFocusIndex sets the focus index and syncs focus state.
func (t *TextInputOverlay) setFocusIndex(i int) {
	t.FocusIndex = i
	t.updateFocusState()
}

// updateFocusState syncs each component's focus/blur state to the current stop.
func (t *TextInputOverlay) updateFocusState() {
	if t.isTitle() {
		t.titleInput.Focus()
	} else {
		t.titleInput.Blur()
	}
	if t.isTextarea() {
		t.textarea.Focus()
	} else {
		t.textarea.Blur()
	}
	if t.directoryPicker != nil {
		if t.isDirectoryPicker() {
			t.directoryPicker.Focus()
		} else {
			t.directoryPicker.Blur()
		}
	}
	if t.branchPicker != nil {
		if t.isBranchPicker() {
			t.branchPicker.Focus()
		} else {
			t.branchPicker.Blur()
		}
	}
	if t.profilePicker != nil {
		if t.isProfilePicker() {
			t.profilePicker.Focus()
		} else {
			t.profilePicker.Blur()
		}
	}
}

// HandleKeyPress processes a key press and updates the state accordingly.
// Returns (shouldClose, branchFilterChanged).
func (t *TextInputOverlay) HandleKeyPress(msg tea.KeyMsg) (bool, bool) {
	numStops := len(t.stops)
	switch msg.Type {
	case tea.KeyTab:
		t.setFocusIndex((t.FocusIndex + 1) % numStops)
		return false, false
	case tea.KeyShiftTab:
		t.setFocusIndex((t.FocusIndex - 1 + numStops) % numStops)
		return false, false
	case tea.KeyEsc:
		t.Canceled = true
		return true, false
	case tea.KeyEnter:
		if t.isEnterButton() {
			t.Submitted = true
			if t.OnSubmit != nil {
				t.OnSubmit()
			}
			return true, false
		}
		if t.isBranchPicker() {
			// Enter on branch picker = advance to enter button
			t.setFocusIndex(numStops - 1)
			return false, false
		}
		if t.isTitle() || t.isDirectoryPicker() || t.isProfilePicker() {
			// Enter on the title/directory/profile = advance to the next stop
			t.setFocusIndex(t.FocusIndex + 1)
			return false, false
		}
		// Send enter to textarea
		if t.isTextarea() {
			t.textarea, _ = t.textarea.Update(msg)
		}
		return false, false
	default:
		if t.isTitle() {
			t.titleInput, _ = t.titleInput.Update(msg)
			return false, false
		}
		if t.isTextarea() {
			t.textarea, _ = t.textarea.Update(msg)
			return false, false
		}
		if t.isDirectoryPicker() {
			t.directoryPicker.HandleKeyPress(msg)
			return false, false
		}
		if t.isProfilePicker() {
			if msg.Type == tea.KeyLeft || msg.Type == tea.KeyRight {
				t.profilePicker.HandleKeyPress(msg)
			}
			return false, false
		}
		if t.isBranchPicker() {
			_, filterChanged := t.branchPicker.HandleKeyPress(msg)
			return false, filterChanged
		}
		return false, false
	}
}

// GetValue returns the current value of the text input.
func (t *TextInputOverlay) GetValue() string {
	return t.textarea.Value()
}

// GetTitle returns the trimmed session title from the title field (create form only).
func (t *TextInputOverlay) GetTitle() string {
	return strings.TrimSpace(t.titleInput.Value())
}

// IsCreateForm reports whether this overlay is the new-session creation form (as opposed
// to the plain prompt overlay used to send a prompt to a running session).
func (t *TextInputOverlay) IsCreateForm() bool {
	return t.isCreateForm
}

// GetSelectedPath returns the selected target directory from the directory picker.
// Returns empty string if no directory picker is present.
func (t *TextInputOverlay) GetSelectedPath() string {
	if t.directoryPicker == nil {
		return ""
	}
	return t.directoryPicker.GetSelectedPath()
}

// SetTargetValidity marks whether the currently selected target directory is a valid
// git repository, so the picker can surface an inline indicator while the user chooses.
// No-op when there is no directory picker.
func (t *TextInputOverlay) SetTargetValidity(valid bool) {
	if t.directoryPicker == nil {
		return
	}
	t.directoryPicker.SetSelectionValidity(valid)
}

// GetSelectedBranch returns the selected branch name from the branch picker.
// Returns empty string if no branch picker is present or "New branch" is selected.
func (t *TextInputOverlay) GetSelectedBranch() string {
	if t.branchPicker == nil {
		return ""
	}
	return t.branchPicker.GetSelectedBranch()
}

// GetSelectedProgram returns the program string from the selected profile.
// Returns empty string if no profile picker is present.
func (t *TextInputOverlay) GetSelectedProgram() string {
	if t.profilePicker == nil {
		return ""
	}
	return t.profilePicker.GetSelectedProfile().Program
}

// BranchFilterVersion returns the current filter version from the branch picker.
// Returns 0 if no branch picker is present.
func (t *TextInputOverlay) BranchFilterVersion() uint64 {
	if t.branchPicker == nil {
		return 0
	}
	return t.branchPicker.GetFilterVersion()
}

// BranchFilter returns the current filter text from the branch picker.
func (t *TextInputOverlay) BranchFilter() string {
	if t.branchPicker == nil {
		return ""
	}
	return t.branchPicker.GetFilter()
}

// InvalidateBranchSearch bumps the branch filter version and clears stale results,
// returning the new version. Used when the target directory changes so in-flight
// searches for the previous repo are rejected. Returns 0 if no branch picker.
func (t *TextInputOverlay) InvalidateBranchSearch() uint64 {
	if t.branchPicker == nil {
		return 0
	}
	return t.branchPicker.Invalidate()
}

// SetBranchResults updates the branch picker with search results.
// version must match the picker's current filterVersion to be accepted.
func (t *TextInputOverlay) SetBranchResults(branches []string, version uint64) {
	if t.branchPicker == nil {
		return
	}
	t.branchPicker.SetResults(branches, version)
}

// IsSubmitted returns whether the form was submitted.
func (t *TextInputOverlay) IsSubmitted() bool {
	return t.Submitted
}

// IsCanceled returns whether the form was canceled.
func (t *TextInputOverlay) IsCanceled() bool {
	return t.Canceled
}

// SetOnSubmit sets a callback function for form submission.
func (t *TextInputOverlay) SetOnSubmit(onSubmit func()) {
	t.OnSubmit = onSubmit
}

// Render renders the text input overlay.
func (t *TextInputOverlay) Render() string {
	// Inner content width (accounting for padding and borders)
	innerWidth := t.width - 6
	if innerWidth < 1 {
		innerWidth = 1
	}

	// Set component widths to fit within the overlay
	t.textarea.SetWidth(innerWidth)
	t.titleInput.Width = innerWidth

	// Build a horizontal divider line
	divider := tiDividerStyle.Render(strings.Repeat("─", innerWidth))

	if t.isCreateForm {
		return t.renderCreateForm(divider)
	}

	// Build the view
	var content string

	// Render directory picker if present, at the very top.
	if t.directoryPicker != nil {
		content += t.directoryPicker.Render() + "\n\n"
		content += divider + "\n\n"
	}

	// Render profile picker if present, above the prompt
	if t.profilePicker != nil {
		content += t.profilePicker.Render() + "\n\n"
		content += divider + "\n\n"
	}

	content += tiTitleStyle.Render(t.Title) + "\n"
	content += t.textarea.View() + "\n\n"

	// Render branch picker if present, with dividers
	if t.branchPicker != nil {
		content += divider + "\n\n"
		content += t.branchPicker.Render() + "\n\n"
	}

	content += divider + "\n\n"
	content += t.renderEnterButton()

	return tiStyle.Render(content)
}

// renderCreateForm renders the unified new-session form. Every section is constant-height
// (see pickerVisibleRows / promptVisibleRows), so the vertically centered overlay never
// jumps as focus moves between fields.
func (t *TextInputOverlay) renderCreateForm(divider string) string {
	var b strings.Builder

	// Each section sits directly above a divider with one blank line after it, keeping the
	// form compact while still visually separating fields.
	section := func(content string) {
		b.WriteString(content + "\n")
		b.WriteString(divider + "\n")
	}

	b.WriteString(tiTitleStyle.Render(t.Title) + "\n")
	section(tiLabelStyle.Render("Title") + "  " + t.titleInput.View())
	if t.directoryPicker != nil {
		section(t.directoryPicker.Render())
	}
	if t.profilePicker != nil {
		section(t.profilePicker.Render())
	}
	if t.branchPicker != nil {
		section(t.branchPicker.Render())
	}
	section(tiLabelStyle.Render("Prompt") + "\n" + t.textarea.View())

	b.WriteString(tiHintStyle.Render(createFormHelp) + "\n")
	b.WriteString(t.renderEnterButton())

	return tiStyle.Render(b.String())
}

// renderEnterButton renders the submit button, highlighted when it holds focus.
func (t *TextInputOverlay) renderEnterButton() string {
	enterButton := " Enter "
	if t.isEnterButton() {
		return tiFocusedButtonStyle.Render(enterButton)
	}
	return tiButtonStyle.Render(enterButton)
}
