package overlay

import (
	"strings"
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

func TestTextInputOverlay_GetSelectedPathNilWithoutPicker(t *testing.T) {
	o := NewTextInputOverlay("Title", "")
	assert.Equal(t, "", o.GetSelectedPath())
}

func TestQuickSendOverlay_EnterSubmits(t *testing.T) {
	o := NewQuickSendOverlay("Send to foo")
	assert.True(t, o.isTextarea(), "focus should start on the textarea")
	o.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("yes")})

	shouldClose, _ := o.HandleKeyPress(tea.KeyMsg{Type: tea.KeyEnter})
	assert.True(t, shouldClose, "Enter should close the quick-send overlay")
	assert.True(t, o.IsSubmitted(), "Enter should submit in quick-send mode")
	assert.False(t, o.IsCanceled())
	assert.Equal(t, "yes", o.GetValue())
}

func TestQuickSendOverlay_AltEnterInsertsNewline(t *testing.T) {
	o := NewQuickSendOverlay("Send to foo")
	o.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("line one")})

	shouldClose, _ := o.HandleKeyPress(tea.KeyMsg{Type: tea.KeyEnter, Alt: true})
	assert.False(t, shouldClose, "Alt+Enter must not submit")
	assert.False(t, o.IsSubmitted(), "Alt+Enter must not submit")

	o.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("line two")})
	assert.Equal(t, "line one\nline two", o.GetValue(), "Alt+Enter should insert a newline")
}

func TestQuickSendOverlay_EscCancels(t *testing.T) {
	o := NewQuickSendOverlay("Send to foo")
	shouldClose, _ := o.HandleKeyPress(tea.KeyMsg{Type: tea.KeyEsc})
	assert.True(t, shouldClose)
	assert.True(t, o.IsCanceled())
	assert.False(t, o.IsSubmitted())
}

func TestTextInputOverlay_InvalidateBumpsVersion(t *testing.T) {
	o := NewSessionCreateOverlay(nil, []string{"/repo/a"})
	before := o.BranchFilterVersion()
	after := o.InvalidateBranchSearch()
	assert.Greater(t, after, before)
}

func TestSessionCreateOverlay_FocusStartsOnTitleAndCycles(t *testing.T) {
	// No profiles → stops: [title, textarea, directory, branch, enter]; focus starts on title.
	o := NewSessionCreateOverlay(nil, []string{"/repo/a", "/repo/b"})
	assert.True(t, o.IsCreateForm())
	assert.True(t, o.isTitle(), "focus should start on the title")

	tab(o)
	assert.True(t, o.isTextarea(), "prompt comes right after the title")
	tab(o)
	assert.True(t, o.isDirectoryPicker())
	tab(o)
	assert.True(t, o.isBranchPicker())
	tab(o)
	assert.True(t, o.isEnterButton())
	tab(o)
	assert.True(t, o.isTitle(), "Tab wraps back to the title")

	shiftTab(o)
	assert.True(t, o.isEnterButton())
}

func TestSessionCreateOverlay_CtrlSSubmitsFromAnyField(t *testing.T) {
	o := NewSessionCreateOverlay(nil, []string{"/repo/a"})
	// Focus starts on the title, not the submit button.
	assert.True(t, o.isTitle())

	shouldClose, _ := o.HandleKeyPress(tea.KeyMsg{Type: tea.KeyCtrlS})
	assert.True(t, shouldClose, "Ctrl+S should close the form")
	assert.True(t, o.IsSubmitted(), "Ctrl+S should submit from a non-button field")
	assert.False(t, o.IsCanceled())
}

func TestSessionCreateOverlay_GetTitle(t *testing.T) {
	o := NewSessionCreateOverlay(nil, []string{"/repo/a"})
	// Focus starts on the title, so runes land there.
	o.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("my-feature")})
	assert.Equal(t, "my-feature", o.GetTitle())
	// The default candidate is exposed as the chosen project.
	assert.Equal(t, "/repo/a", o.GetSelectedPath())
}

// The whole form must render the same number of lines no matter which field holds focus,
// so the vertically centered overlay does not jump as the user Tabs between fields.
func TestSessionCreateOverlay_RenderHeightConstantAcrossFocus(t *testing.T) {
	o := NewSessionCreateOverlay(nil, []string{"/repo/a", "/repo/b"})
	o.SetSize(80, 40)

	o.focusStop(stopDirectory)
	dirFocused := strings.Count(o.Render(), "\n")
	o.focusStop(stopTextarea)
	promptFocused := strings.Count(o.Render(), "\n")
	o.focusStop(stopBranch)
	branchFocused := strings.Count(o.Render(), "\n")

	assert.Equal(t, dirFocused, promptFocused, "overlay height must not change between fields")
	assert.Equal(t, dirFocused, branchFocused, "overlay height must not change between fields")
}

// The form must shrink to fit short terminals (it has a fixed-height default that overflows
// otherwise), and must still render at a constant height regardless of which field is focused.
func TestSessionCreateOverlay_FitsShortTerminal(t *testing.T) {
	o := NewSessionCreateOverlay(nil, []string{"/repo/a"})
	o.SetBranchResults([]string{"main", "develop", "feature/x"}, o.BranchFilterVersion())

	// The form collapses its picker/prompt rows to fit; at a comfortable-but-short 24 rows it
	// must already shrink from its 28-line default, and never exceed the terminal height.
	for _, h := range []int{24, 30, 50} {
		o.SetSize(80, h)
		for _, stop := range []focusStop{stopTitle, stopTextarea, stopDirectory, stopBranch, stopEnter} {
			o.focusStop(stop)
			got := strings.Count(o.Render(), "\n") + 1
			assert.LessOrEqual(t, got, h, "h=%d focus=%d: overlay rendered %d lines, must fit", h, stop, got)
		}
	}
}
