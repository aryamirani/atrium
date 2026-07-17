package ui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/ZviBaratz/atrium/ui/theme"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

// Diff row model for diff-tab comments (#383). parseDiffRows turns a raw unified
// diff into one diffRow per rendered row — in the exact order the pane paints them,
// including the faint rule inserted before each "diff --git" header — so a line
// cursor can index rendered rows directly and still recover each code line's
// file:line for an anchored comment. git discards nothing here that a comment needs:
// the file comes from the nearest "diff --git … b/<path>" and the line number is
// tracked from the "@@ -old,+new @@" hunk headers.

// diffRowKind classifies one rendered row of the diff pane.
type diffRowKind int

const (
	rowBlank      diffRowKind = iota // an empty line in the diff
	rowRule                          // the faint separator inserted before a file header
	rowFileHeader                    // the "diff --git" line (rendered as the bold path)
	rowHunk                          // an "@@ … @@" hunk header
	rowMeta                          // index/---/+++/mode/rename/binary metadata
	rowContext                       // an unchanged context line
	rowAdd                           // an added ("+") line
	rowDel                           // a removed ("-") line
)

// diffRow is one rendered row of the diff pane. For code rows (add/del/context)
// file and lineNo locate the row in its file: lineNo is the new-file line for
// additions and context, and the old-file line for deletions. Non-code rows carry
// lineNo 0 and (except headers) no file.
type diffRow struct {
	kind   diffRowKind
	text   string // the raw line, unstyled and unfitted
	file   string // owning file path (post-change b/<path>), for code and header rows
	lineNo int    // 1-based file line for code rows; 0 otherwise
}

// annotatable reports whether a comment can anchor to this row — i.e. it is a real
// code line (added, removed, or context), not chrome or metadata.
func (r diffRow) annotatable() bool {
	return r.kind == rowAdd || r.kind == rowDel || r.kind == rowContext
}

// IsCommenting reports whether the pane is frozen in comment mode.
func (d *DiffPane) IsCommenting() bool { return d.commenting }

// EnterComment freezes the pane on its current rows and places the line cursor on
// the first annotatable (code) row. It returns false — staying live — when the diff
// has no code line to comment on (only chrome), so the caller can decline the mode.
func (d *DiffPane) EnterComment() bool {
	first := d.nextAnnotatable(-1, +1)
	if first < 0 {
		return false
	}
	d.commenting = true
	d.cursor = first
	d.refreshCommentView()
	return true
}

// ExitComment leaves comment mode; the next live SetDiff repaints without the cursor.
func (d *DiffPane) ExitComment() {
	if !d.commenting {
		return
	}
	d.commenting = false
	// Repaint the frozen snapshot once without the cursor so the highlight clears
	// immediately, even before the next poll delivers fresh content.
	d.diff = renderDiffRows(d.rows, d.width, -1)
	d.viewport.SetContent(lipgloss.JoinVertical(lipgloss.Left, d.stats, d.diff))
}

// CursorDown moves the line cursor to the next annotatable row, clamping at the end.
func (d *DiffPane) CursorDown() { d.moveCursor(+1) }

// CursorUp moves the line cursor to the previous annotatable row, clamping at the start.
func (d *DiffPane) CursorUp() { d.moveCursor(-1) }

func (d *DiffPane) moveCursor(dir int) {
	if !d.commenting {
		return
	}
	if next := d.nextAnnotatable(d.cursor, dir); next >= 0 {
		d.cursor = next
		d.refreshCommentView()
	}
}

// nextAnnotatable returns the index of the first annotatable row strictly past
// `from` in direction dir (+1/-1), or -1 if there is none (so ends clamp).
func (d *DiffPane) nextAnnotatable(from, dir int) int {
	for i := from + dir; i >= 0 && i < len(d.rows); i += dir {
		if d.rows[i].annotatable() {
			return i
		}
	}
	return -1
}

// CommentAnchor returns the row the cursor is on, valid only in comment mode on an
// annotatable row.
func (d *DiffPane) CommentAnchor() (diffRow, bool) {
	if !d.commenting || d.cursor < 0 || d.cursor >= len(d.rows) || !d.rows[d.cursor].annotatable() {
		return diffRow{}, false
	}
	return d.rows[d.cursor], true
}

// CommentLocation returns the "file:line" the cursor sits on, for the composer
// title. Valid only in comment mode on an annotatable row.
func (d *DiffPane) CommentLocation() (string, bool) {
	a, ok := d.CommentAnchor()
	if !ok {
		return "", false
	}
	return fmt.Sprintf("%s:%d", a.file, a.lineNo), true
}

// CommentMessage builds the queued-prompt text for the cursor's line and the given
// note, or false when there is no valid anchor.
func (d *DiffPane) CommentMessage(note string) (string, bool) {
	a, ok := d.CommentAnchor()
	if !ok {
		return "", false
	}
	return composeDiffComment(a, note), true
}

// refreshCommentView re-renders the frozen rows with the cursor highlighted and
// scrolls the viewport so the cursor row stays visible.
func (d *DiffPane) refreshCommentView() {
	d.diff = renderDiffRows(d.rows, d.width, d.cursor)
	d.viewport.SetContent(lipgloss.JoinVertical(lipgloss.Left, d.stats, d.diff))
	// The diff block sits below the stats block in the viewport; the cursor row's
	// absolute Y is that offset plus its row index.
	cursorY := lipgloss.Height(d.stats) + d.cursor
	if cursorY < d.viewport.YOffset {
		d.viewport.SetYOffset(cursorY)
	} else if cursorY >= d.viewport.YOffset+d.viewport.Height {
		d.viewport.SetYOffset(cursorY - d.viewport.Height + 1)
	}
}

// diffCursorStyle highlights the line the comment cursor sits on — a filled bar in
// the bar background so the target line reads unmistakably against the +/- colors.
func diffCursorStyle() lipgloss.Style {
	t := theme.Current()
	return lipgloss.NewStyle().Background(t.Palette.BarBg).Foreground(t.Palette.Fg).Bold(true)
}

// renderDiffRows styles parsed rows into the pane's patch body. It mirrors
// colorizeDiff's per-kind styling, and when cursor >= 0 paints that row as a
// full-width highlight bar (the comment-mode line cursor). width <= 0 skips the fit.
func renderDiffRows(rows []diffRow, width, cursor int) string {
	fit := func(line string) string {
		line = strings.ReplaceAll(line, "\t", "    ")
		if width > 0 && runewidth.StringWidth(line) > width {
			line = runewidth.Truncate(line, width-1, "…")
		}
		return line
	}
	ruleLen := 24
	if width > 0 {
		ruleLen = width
	}
	rule := theme.Current().FaintStyle().Render(strings.Repeat("─", ruleLen))

	lines := make([]string, len(rows))
	for i, r := range rows {
		if i == cursor {
			bar := fit(r.text)
			if width > 0 {
				bar = diffCursorStyle().Width(width).Render(bar)
			} else {
				bar = diffCursorStyle().Render(bar)
			}
			lines[i] = bar
			continue
		}
		switch r.kind {
		case rowBlank:
			lines[i] = ""
		case rowRule:
			lines[i] = rule
		case rowFileHeader:
			if r.file != "" {
				lines[i] = theme.Current().FgStyle().Bold(true).Render(fit(r.file))
			} else {
				lines[i] = metaStyle().Render(fit(r.text))
			}
		case rowHunk:
			lines[i] = hunkStyle().Render(fit(r.text))
		case rowAdd:
			lines[i] = additionStyle().Render(fit(r.text))
		case rowDel:
			lines[i] = deletionStyle().Render(fit(r.text))
		case rowMeta:
			lines[i] = metaStyle().Render(fit(r.text))
		default: // rowContext
			lines[i] = fit(r.text)
		}
	}
	return strings.Join(lines, "\n")
}

// parseDiffRows turns a raw unified diff into the sequence of rows the pane paints.
// It mirrors colorizeDiff's classification exactly (so a renderer can style straight
// off these rows without re-parsing) and additionally tracks each code row's file
// and line number. A "diff --git" header emits two rows — the faint rule then the
// header — matching the pane's section separator.
func parseDiffRows(content string) []diffRow {
	if content == "" {
		return nil
	}
	var rows []diffRow
	var file string
	var oldLine, newLine int // next file line for the '-' and '+' sides within a hunk

	for _, line := range strings.Split(content, "\n") {
		switch {
		case line == "":
			rows = append(rows, diffRow{kind: rowBlank})
		case strings.HasPrefix(line, "diff --git"):
			file = diffFilePath(line)
			rows = append(rows,
				diffRow{kind: rowRule},
				diffRow{kind: rowFileHeader, text: line, file: file})
		case strings.HasPrefix(line, "@@"):
			if o, n, ok := parseHunkHeader(line); ok {
				oldLine, newLine = o, n
			}
			rows = append(rows, diffRow{kind: rowHunk, text: line})
		case line[0] == '+' && (len(line) == 1 || line[1] != '+'):
			rows = append(rows, diffRow{kind: rowAdd, text: line, file: file, lineNo: newLine})
			newLine++
		case line[0] == '-' && (len(line) == 1 || line[1] != '-'):
			rows = append(rows, diffRow{kind: rowDel, text: line, file: file, lineNo: oldLine})
			oldLine++
		case isDiffMeta(line):
			rows = append(rows, diffRow{kind: rowMeta, text: line})
		default:
			// A context line (leading space), or anything colorizeDiff passes through
			// uncolored; either way it advances both sides.
			rows = append(rows, diffRow{kind: rowContext, text: line, file: file, lineNo: newLine})
			oldLine++
			newLine++
		}
	}
	return rows
}

// parseHunkHeader reads the starting line numbers from a "@@ -old,count +new,count @@"
// header (the counts are optional — git omits them when they are 1). It returns the
// 1-based old and new starting lines, or ok=false when the header is malformed.
func parseHunkHeader(line string) (oldStart, newStart int, ok bool) {
	fields := strings.Fields(line)
	// fields[0] is "@@"; the old range starts with '-', the new range with '+'.
	for _, f := range fields[1:] {
		switch {
		case strings.HasPrefix(f, "-") && oldStart == 0:
			oldStart = leadingInt(f[1:])
		case strings.HasPrefix(f, "+") && newStart == 0:
			newStart = leadingInt(f[1:])
		}
	}
	return oldStart, newStart, oldStart > 0 && newStart > 0
}

// leadingInt parses the integer prefix of s up to an optional "," (the hunk range's
// start line), returning 0 when there is no leading digit.
func leadingInt(s string) int {
	if i := strings.IndexByte(s, ','); i >= 0 {
		s = s[:i]
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}

// composeDiffComment builds the queued-prompt text a diff comment delivers to the
// agent: a "Re: file:line" reference, the exact diff line quoted (its +/- marker
// kept so the agent sees whether it is an added, removed, or context line), then the
// user's note trimmed of surrounding whitespace.
func composeDiffComment(anchor diffRow, note string) string {
	return fmt.Sprintf("Re: %s:%d\n\n    %s\n\n%s", anchor.file, anchor.lineNo, anchor.text, strings.TrimSpace(note))
}
