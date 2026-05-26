package overlay

import (
	"claude-squad/config"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
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
)

// focusStop identifies a focusable component in the overlay. The overlay holds an
// ordered slice of the stops that are actually present, so adding or removing a
// component is a matter of editing that slice rather than juggling hardcoded indices.
type focusStop int

const (
	stopDirectory focusStop = iota
	stopProfile
	stopTextarea
	stopBranch
	stopEnter
)

// TextInputOverlay represents a text input overlay with state management.
type TextInputOverlay struct {
	textarea        textarea.Model
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

func (t *TextInputOverlay) SetSize(width, height int) {
	t.textarea.SetHeight(height)
	t.width = width
	t.height = height
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
		if t.isDirectoryPicker() || t.isProfilePicker() {
			// Enter on directory/profile picker = advance to the next stop
			t.setFocusIndex(t.FocusIndex + 1)
			return false, false
		}
		// Send enter to textarea
		if t.isTextarea() {
			t.textarea, _ = t.textarea.Update(msg)
		}
		return false, false
	default:
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

// GetSelectedPath returns the selected target directory from the directory picker.
// Returns empty string if no directory picker is present.
func (t *TextInputOverlay) GetSelectedPath() string {
	if t.directoryPicker == nil {
		return ""
	}
	return t.directoryPicker.GetSelectedPath()
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

	// Set textarea width to fit within the overlay
	t.textarea.SetWidth(innerWidth)

	// Build a horizontal divider line
	divider := tiDividerStyle.Render(strings.Repeat("─", innerWidth))

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

	// Render enter button with appropriate style
	enterButton := " Enter "
	if t.isEnterButton() {
		enterButton = tiFocusedButtonStyle.Render(enterButton)
	} else {
		enterButton = tiButtonStyle.Render(enterButton)
	}
	content += enterButton

	return tiStyle.Render(content)
}
