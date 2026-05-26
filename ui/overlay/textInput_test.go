package overlay

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

func tab(o *TextInputOverlay)      { o.HandleKeyPress(tea.KeyMsg{Type: tea.KeyTab}) }
func shiftTab(o *TextInputOverlay) { o.HandleKeyPress(tea.KeyMsg{Type: tea.KeyShiftTab}) }

func TestTextInputOverlay_SimpleFocusCycle(t *testing.T) {
	o := NewTextInputOverlay("Title", "")
	// Stops: [textarea, enter]; focus starts on the textarea.
	assert.True(t, o.isTextarea())
	tab(o)
	assert.True(t, o.isEnterButton())
	tab(o)
	assert.True(t, o.isTextarea())
}

func TestTextInputOverlay_BranchPickerFocusOrder(t *testing.T) {
	// No profiles → stops: [directory, textarea, branch, enter]. Focus starts on textarea.
	o := NewTextInputOverlayWithBranchPicker("Enter prompt", "", nil, []string{"/repo/a", "/repo/b"})
	assert.True(t, o.isTextarea(), "focus should start on the textarea")

	tab(o)
	assert.True(t, o.isBranchPicker())
	tab(o)
	assert.True(t, o.isEnterButton())
	tab(o)
	assert.True(t, o.isDirectoryPicker(), "directory is the first stop, reached after wrap")
	tab(o)
	assert.True(t, o.isTextarea())

	// Shift+Tab from the textarea lands on the directory picker.
	shiftTab(o)
	assert.True(t, o.isDirectoryPicker())

	// The directory picker exposes the default (first) candidate.
	assert.Equal(t, "/repo/a", o.GetSelectedPath())
}

func TestTextInputOverlay_GetSelectedPathNilWithoutPicker(t *testing.T) {
	o := NewTextInputOverlay("Title", "")
	assert.Equal(t, "", o.GetSelectedPath())
}

func TestTextInputOverlay_InvalidateBumpsVersion(t *testing.T) {
	o := NewTextInputOverlayWithBranchPicker("Enter prompt", "", nil, []string{"/repo/a"})
	before := o.BranchFilterVersion()
	after := o.InvalidateBranchSearch()
	assert.Greater(t, after, before)
}
