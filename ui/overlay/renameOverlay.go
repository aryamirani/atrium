package overlay

import (
	"github.com/ZviBaratz/atrium/ui/theme"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// RenameOverlay is a minimal single-line dialog for editing a session's cosmetic display
// label. It mirrors ConfirmationOverlay's lightweight shape rather than the heavier
// create-form TextInputOverlay, since renaming only needs one short field.
type RenameOverlay struct {
	input     textinput.Model
	submitted bool
	canceled  bool
	width     int
}

// NewRenameOverlay creates a rename dialog pre-filled with the instance's current label,
// reusing the shared single-line title input (32-char cap, matching the new-session form).
func NewRenameOverlay(currentLabel string) *RenameOverlay {
	in := newTitleInput()
	in.SetValue(currentLabel)
	in.Focus()
	in.CursorEnd()
	return &RenameOverlay{
		input: in,
		width: 50,
	}
}

// HandleKeyPress processes a key press and returns true if the overlay should be closed.
// enter submits, esc/ctrl+c cancel, everything else edits the field.
func (r *RenameOverlay) HandleKeyPress(msg tea.KeyMsg) bool {
	switch msg.String() {
	case "enter":
		r.submitted = true
		return true
	case "esc", "ctrl+c":
		r.canceled = true
		return true
	default:
		r.input, _ = r.input.Update(msg)
		return false
	}
}

// Value returns the trimmed label the user entered.
func (r *RenameOverlay) Value() string {
	return strings.TrimSpace(r.input.Value())
}

// IsSubmitted reports whether the user accepted the new label.
func (r *RenameOverlay) IsSubmitted() bool { return r.submitted }

// IsCanceled reports whether the user dismissed the dialog without renaming.
func (r *RenameOverlay) IsCanceled() bool { return r.canceled }

// SetWidth sets the width of the overlay.
func (r *RenameOverlay) SetWidth(width int) { r.width = width }

// Render renders the rename overlay as a bordered box.
func (r *RenameOverlay) Render() string {
	style := lipgloss.NewStyle().
		Border(theme.Current().Borders.Style).
		BorderForeground(theme.Current().Palette.Accent).
		Padding(1, 2).
		Width(r.width)

	hint := lipgloss.NewStyle().Faint(true).Render("enter to save · esc to cancel")
	content := "Rename session\n\n" + r.input.View() + "\n\n" + hint
	return style.Render(content)
}
