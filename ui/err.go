package ui

import (
	"strings"

	"github.com/ZviBaratz/atrium/ui/theme"

	"github.com/mattn/go-runewidth"
)

// ErrBox is the single-row transient banner rendered beneath the hint bar when
// the bar isn't up to carry a toast. It carries either an error (red) or a
// neutral info notice (#287), graded by NoticeLevel so info never reads as an
// error.
type ErrBox struct {
	height, width int
	text          string
	level         NoticeLevel
}

// NewErrBox returns an empty ErrBox.
func NewErrBox() *ErrBox {
	return &ErrBox{}
}

// SetError sets an error-level notice to display.
func (e *ErrBox) SetError(err error) {
	if err == nil {
		e.Clear()
		return
	}
	e.text = err.Error()
	e.level = NoticeError
}

// SetNotice sets a notice to display at the given level (info renders neutral,
// error renders red).
func (e *ErrBox) SetNotice(text string, level NoticeLevel) {
	e.text = text
	e.level = level
}

// Clear removes the displayed notice.
func (e *ErrBox) Clear() {
	e.text = ""
	e.level = NoticeInfo
}

// HasError reports whether an error-level (as opposed to neutral info) notice is
// showing; an info notice riding the box reports false so it never looks like an
// error. The layout allots a row on HasContent, not this — HasError only grades
// the level for callers that distinguish the two.
func (e *ErrBox) HasError() bool {
	return e.text != "" && e.level == NoticeError
}

// HasContent reports whether any notice (info or error) is showing. The layout
// uses this to decide whether to allot the box a row.
func (e *ErrBox) HasContent() bool {
	return e.text != ""
}

// SetSize sets the box's render dimensions; long text is truncated to fit.
func (e *ErrBox) SetSize(width, height int) {
	e.width = width
	e.height = height
}

// Fits reports whether a toast can convey err without losing content: a single
// line that survives String()'s truncation threshold intact. Multi-line errors
// never fit (String flattens them with "//"); over-wide ones don't either,
// unless the box has no measured width yet (startup, tests), where the toast is
// the safe default. Callers route non-fitting errors to a persistent modal.
func (e *ErrBox) Fits(err error) bool {
	if err == nil {
		return true
	}
	msg := err.Error()
	if strings.Contains(msg, "\n") {
		return false
	}
	return e.width <= 0 || runewidth.StringWidth(msg) <= e.width-3
}

func (e *ErrBox) String() string {
	// No content means no row: returning "" keeps the caller from joining a
	// blank line beneath the hint bar (lipgloss.JoinVertical counts "" as one
	// line).
	if e.text == "" {
		return ""
	}
	text := e.text
	lines := strings.Split(text, "\n")
	text = strings.Join(lines, "//")
	if runewidth.StringWidth(text) > e.width-3 && e.width-3 >= 0 {
		text = runewidth.Truncate(text, e.width-3, "…")
	}
	style := theme.Current().FgStyle()
	if e.level == NoticeError {
		style = theme.Current().DangerStyle()
	}
	return centerInBox(e.width, e.height, style.Render(text))
}
