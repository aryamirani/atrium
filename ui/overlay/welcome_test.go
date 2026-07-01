package overlay

import (
	"strings"
	"testing"

	"github.com/ZviBaratz/atrium/config"

	tea "github.com/charmbracelet/bubbletea"
)

func detectedFixture() []config.Profile {
	return []config.Profile{
		{Name: "claude", Program: "claude"},
		{Name: "codex", Program: "codex"},
		{Name: "aider", Program: "aider"},
	}
}

func TestWelcomeOverlay_DetectingThenPick(t *testing.T) {
	w := NewWelcomeOverlay()
	w.SetWidth(54)

	// Before detection resolves, Enter/nav must not close or confirm.
	if w.HandleKeyPress(tea.KeyMsg{Type: tea.KeyDown}) {
		t.Fatal("nav during detecting should not close")
	}
	if !strings.Contains(w.Render(), "Detecting") {
		t.Errorf("detecting state should render a Detecting… line, got:\n%s", w.Render())
	}

	w.SetDetected(detectedFixture())

	// First profile (registry order → claude) is selected by default.
	if got := w.SelectedProgram(); got != "claude" {
		t.Errorf("default selection = %q, want \"claude\"", got)
	}
	// Down moves selection to codex.
	w.HandleKeyPress(tea.KeyMsg{Type: tea.KeyDown})
	if got := w.SelectedProgram(); got != "codex" {
		t.Errorf("after Down, selection = %q, want \"codex\"", got)
	}
	// Enter confirms and closes.
	if !w.HandleKeyPress(tea.KeyMsg{Type: tea.KeyEnter}) {
		t.Fatal("Enter should close the overlay")
	}
	if !w.Confirmed() {
		t.Error("Enter should mark the overlay confirmed")
	}
	if len(w.Detected()) != 3 {
		t.Errorf("Detected() = %d profiles, want 3", len(w.Detected()))
	}
}

func TestWelcomeOverlay_SkipDoesNotConfirm(t *testing.T) {
	w := NewWelcomeOverlay()
	w.SetDetected(detectedFixture())
	if !w.HandleKeyPress(tea.KeyMsg{Type: tea.KeyEsc}) {
		t.Fatal("Esc should close the overlay")
	}
	if w.Confirmed() {
		t.Error("Esc must not confirm")
	}
}

func TestWelcomeOverlay_EmptyDetection(t *testing.T) {
	w := NewWelcomeOverlay()
	w.SetWidth(54)
	w.SetDetected(nil)

	if got := w.SelectedProgram(); got != "" {
		t.Errorf("empty detection SelectedProgram = %q, want \"\"", got)
	}
	out := w.Render()
	if !strings.Contains(out, "No supported agent") {
		t.Errorf("empty-detection render should warn about no agents, got:\n%s", out)
	}
	// Enter/Esc both close; Enter acknowledges (Confirmed true) but has no program.
	if !w.HandleKeyPress(tea.KeyMsg{Type: tea.KeyEnter}) {
		t.Fatal("Enter should close even with no agents")
	}
}
