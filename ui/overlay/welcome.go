package overlay

import (
	"fmt"
	"strings"

	"github.com/ZviBaratz/atrium/config"
	"github.com/ZviBaratz/atrium/ui/theme"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// WelcomeOverlay is the interactive first-run modal: it greets the user, lets
// them pick a default agent from the ones detected on their PATH, and warns when
// none are found. It follows the same local-bordered-box idiom as the
// confirmation and rename overlays (fixed width, centered by PlaceOverlay).
type WelcomeOverlay struct {
	detecting bool
	detected  []config.Profile
	picker    *ProfilePicker
	confirmed bool
	width     int
}

// NewWelcomeOverlay creates the overlay in its "detecting" state; the caller
// fills it in with SetDetected once agent detection resolves.
func NewWelcomeOverlay() *WelcomeOverlay {
	return &WelcomeOverlay{detecting: true, width: 54}
}

// SetDetected leaves the detecting state and installs a picker over the detected
// agents. An empty slice renders the no-agents guidance instead of a picker.
func (w *WelcomeOverlay) SetDetected(detected []config.Profile) {
	w.detecting = false
	w.detected = detected
	if len(detected) > 0 {
		w.picker = NewProfilePicker(detected)
		w.picker.Focus()
		w.picker.SetWidth(w.width - 4)
	}
}

// SetWidth sets the modal's box width.
func (w *WelcomeOverlay) SetWidth(width int) {
	w.width = width
	if w.picker != nil {
		w.picker.SetWidth(width - 4)
	}
}

// HandleKeyPress returns true when the overlay should close. Enter confirms
// (Confirmed() == true); Esc and ctrl+c skip (ctrl+c mirrors the app's
// overlay-cancel idiom, so a first-run quit reflex is not swallowed). While
// detecting, only the skip keys close.
func (w *WelcomeOverlay) HandleKeyPress(msg tea.KeyMsg) bool {
	if msg.Type == tea.KeyEsc || msg.Type == tea.KeyCtrlC {
		return true
	}
	if w.detecting {
		return false
	}
	if msg.Type == tea.KeyEnter {
		w.confirmed = true
		return true
	}
	if w.picker != nil {
		w.picker.HandleKeyPress(msg)
	}
	return false
}

// Confirmed reports whether the overlay was closed by confirming (Enter).
func (w *WelcomeOverlay) Confirmed() bool { return w.confirmed }

// SelectedProfile is the chosen profile (Name + Program), or the zero Profile
// when there was no picker (empty detection). The caller persists its Name as
// the default program so resolution keeps flowing through the profile list.
func (w *WelcomeOverlay) SelectedProfile() config.Profile {
	if w.picker == nil {
		return config.Profile{}
	}
	return w.picker.GetSelectedProfile()
}

// Detected returns the profiles detection found (for the caller to merge on confirm).
func (w *WelcomeOverlay) Detected() []config.Profile { return w.detected }

// Render draws the bordered welcome modal.
func (w *WelcomeOverlay) Render() string {
	var b strings.Builder
	b.WriteString(theme.Current().OverlayTitleStyle().Render("Welcome to Atrium"))
	b.WriteString("\n\n")
	b.WriteString("Run multiple coding agents in parallel — each in its own\n")
	b.WriteString("git worktree and tmux session, managed from one place.\n\n")

	var hint string
	switch {
	case w.detecting:
		b.WriteString(overlayDimStyle().Render("Detecting installed agents…"))
		hint = "esc skip"
	case len(w.detected) == 0:
		b.WriteString("⚠ No supported agent CLIs found on PATH.\n")
		b.WriteString(overlayDimStyle().Render("Install claude, codex, gemini, or aider (or press , later)."))
		hint = "enter continue · esc skip"
	default:
		b.WriteString("Choose your default agent:\n\n")
		b.WriteString(w.picker.Render())
		b.WriteString("\n\n")
		b.WriteString(overlayDimStyle().Render(fmt.Sprintf("✓ %d agent(s) detected on your PATH", len(w.detected))))
		hint = "↑/↓ choose · enter confirm · esc skip"
	}

	b.WriteString("\n\n")
	b.WriteString(theme.Current().OverlayHintStyle().Render(hint))

	style := lipgloss.NewStyle().
		Border(theme.Current().Borders.Style).
		BorderForeground(theme.Current().Palette.Accent).
		Padding(1, 2).
		Width(w.width)
	return style.Render(b.String())
}
