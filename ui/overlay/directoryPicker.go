package overlay

import (
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// DirectoryPicker is an embeddable component for choosing the target repository
// directory of a new session. Unlike the branch picker it is fully synchronous —
// the candidate list is local. The filter text doubles as free-text path entry:
// typing a fragment filters the candidates, while typing a path (one starting with
// "/", "~" or ".") offers that path as a selectable entry. Validation that the
// chosen path is a git repo happens at the call site, on selection/submit.
type DirectoryPicker struct {
	candidates []string // absolute candidate repo paths, deduped; candidates[0] is the default
	filter     string
	cursor     int
	focused    bool
	width      int

	// validityChecked/selectionValid let the call site surface an inline "(not a git
	// repo)" hint as the selection changes, instead of only rejecting it at submit.
	validityChecked bool
	selectionValid  bool
}

// NewDirectoryPicker creates a directory picker over the given candidate paths.
// The caller should pass the default/contextual target first; the list is deduped
// while preserving order and the cursor starts on the first entry.
func NewDirectoryPicker(candidates []string) *DirectoryPicker {
	seen := make(map[string]bool, len(candidates))
	deduped := make([]string, 0, len(candidates))
	for _, c := range candidates {
		if c == "" || seen[c] {
			continue
		}
		seen[c] = true
		deduped = append(deduped, c)
	}
	return &DirectoryPicker{candidates: deduped}
}

// SetWidth sets the width of the directory picker.
func (dp *DirectoryPicker) SetWidth(w int) {
	dp.width = w
}

// Focus gives the directory picker focus.
func (dp *DirectoryPicker) Focus() {
	dp.focused = true
}

// Blur removes focus from the directory picker.
func (dp *DirectoryPicker) Blur() {
	dp.focused = false
}

// IsFocused returns whether the directory picker is focused.
func (dp *DirectoryPicker) IsFocused() bool {
	return dp.focused
}

// SetSelectionValidity records whether the currently selected path is a valid git
// repository, so Render can show an inline indicator while the user is choosing.
func (dp *DirectoryPicker) SetSelectionValidity(valid bool) {
	dp.validityChecked = true
	dp.selectionValid = valid
}

// HandleKeyPress processes a key event. Returns (consumed, selectionChanged).
func (dp *DirectoryPicker) HandleKeyPress(msg tea.KeyMsg) (consumed bool, selectionChanged bool) {
	switch msg.Type {
	case tea.KeyUp:
		if dp.cursor > 0 {
			dp.cursor--
			return true, true
		}
		return true, false
	case tea.KeyDown:
		if dp.cursor < len(dp.visibleItems())-1 {
			dp.cursor++
			return true, true
		}
		return true, false
	case tea.KeyBackspace:
		if len(dp.filter) > 0 {
			runes := []rune(dp.filter)
			dp.filter = string(runes[:len(runes)-1])
			dp.cursor = 0
			return true, true
		}
		return true, false
	case tea.KeyRunes:
		dp.filter += string(msg.Runes)
		dp.cursor = 0
		return true, true
	case tea.KeySpace:
		dp.filter += " "
		dp.cursor = 0
		return true, true
	}
	return false, false
}

// looksLikePath reports whether the filter should be treated as a path to enter
// rather than a fragment to match against the candidates.
func looksLikePath(s string) bool {
	return strings.HasPrefix(s, "/") || strings.HasPrefix(s, "~") || strings.HasPrefix(s, ".")
}

// expandPath expands a leading "~" and resolves the path to absolute.
func expandPath(s string) string {
	if s == "~" || strings.HasPrefix(s, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			s = filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(s, "~"), "/"))
		}
	}
	if abs, err := filepath.Abs(s); err == nil {
		return abs
	}
	return s
}

// visibleItems returns the candidates matching the current filter, plus the typed
// path itself when the filter looks like a path that isn't already a candidate.
func (dp *DirectoryPicker) visibleItems() []string {
	var items []string
	lower := strings.ToLower(dp.filter)
	for _, c := range dp.candidates {
		if dp.filter == "" || strings.Contains(strings.ToLower(c), lower) {
			items = append(items, c)
		}
	}
	if dp.filter != "" && looksLikePath(dp.filter) {
		typed := expandPath(dp.filter)
		found := false
		for _, it := range items {
			if it == typed {
				found = true
				break
			}
		}
		if !found {
			items = append(items, typed)
		}
	}
	return items
}

// GetSelectedPath returns the currently selected (absolute) path, or empty string
// if there is no selection.
func (dp *DirectoryPicker) GetSelectedPath() string {
	items := dp.visibleItems()
	if dp.cursor < 0 || dp.cursor >= len(items) {
		return ""
	}
	return items[dp.cursor]
}

var (
	dpLabelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("62")).
			Bold(true)

	dpFilterStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("7"))

	dpSelectedStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("62")).
			Foreground(lipgloss.Color("0"))

	dpDimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))

	dpHintStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Italic(true)

	dpInvalidStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("203"))
)

// Render renders the directory picker. When unfocused it collapses to a single line
// showing the chosen project and how to change it, so the target repo is always
// visible without crowding the overlay. When focused it expands to the filterable list.
func (dp *DirectoryPicker) Render() string {
	var s strings.Builder

	if !dp.focused {
		s.WriteString(dpLabelStyle.Render("Project: "))
		if sel := dp.GetSelectedPath(); sel != "" {
			s.WriteString(dp.displayPath(sel))
		} else {
			s.WriteString(dpDimStyle.Render("(none)"))
		}
		s.WriteString(dpHintStyle.Render("  (Tab to change)"))
		return s.String()
	}

	s.WriteString(dpLabelStyle.Render("Project"))
	cursor := dp.filter + "█"
	s.WriteString(dpFilterStyle.Render(" (filter/path: " + cursor + ")"))
	if dp.validityChecked && !dp.selectionValid {
		s.WriteString(dpInvalidStyle.Render("  (not a git repo)"))
	}
	s.WriteString("\n\n")

	items := dp.visibleItems()
	if len(items) == 0 {
		s.WriteString(dpDimStyle.Render("  No matches — type a path (/, ~, .) to use another project"))
		return s.String()
	}

	// Show max 5 visible items, windowed around the cursor.
	maxVisible := 5
	start := 0
	if dp.cursor >= maxVisible {
		start = dp.cursor - maxVisible + 1
	}
	end := start + maxVisible
	if end > len(items) {
		end = len(items)
	}

	for i := start; i < end; i++ {
		prefix := "  "
		label := dp.displayPath(items[i])
		if i == dp.cursor && dp.focused {
			prefix = "> "
			s.WriteString(dpSelectedStyle.Render(prefix + label))
		} else if i == dp.cursor {
			s.WriteString(prefix + label)
		} else {
			s.WriteString(dpDimStyle.Render(prefix + label))
		}
		if i < end-1 {
			s.WriteString("\n")
		}
	}

	return s.String()
}

// displayPath shortens a path for display by collapsing the home directory to "~".
func (dp *DirectoryPicker) displayPath(path string) string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if path == home {
			return "~"
		}
		if strings.HasPrefix(path, home+string(filepath.Separator)) {
			return "~" + path[len(home):]
		}
	}
	return path
}
