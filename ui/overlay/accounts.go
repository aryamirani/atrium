package overlay

import (
	"strings"

	"github.com/ZviBaratz/atrium/config"
	"github.com/ZviBaratz/atrium/ui/theme"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type accountsTab int

const (
	tabClaude accountsTab = iota
	tabGH
)

type accountsMode int

const (
	modeList accountsMode = iota
	modeEdit
	modeConfirmDelete
	modePreview
)

// AccountsOverlay is the in-TUI manager for Claude and GitHub accounts. It holds the
// same *config.Config the app holds and mutates ClaudeAccounts/GHAccounts in place;
// the app persists (SaveConfig) whenever HandleKeyPress reports dirty.
type AccountsOverlay struct {
	cfg    *config.Config
	tab    accountsTab
	mode   accountsMode
	cursor int

	width, height int

	lastErr string
	// form/editIndex (Task 5) and preview inputs (Task 6) are added later.
}

func NewAccountsOverlay(cfg *config.Config) *AccountsOverlay {
	return &AccountsOverlay{cfg: cfg, width: 80, height: 24} // floor so Render works pre-SetSize
}

func (o *AccountsOverlay) SetSize(w, h int) { o.width, o.height = w, h }

// test-only accessors
func (o *AccountsOverlay) selectTab(t accountsTab) { o.tab = t; o.clampCursor() }
func (o *AccountsOverlay) cursorIndex() int        { return o.cursor }

type acctRow struct {
	name, dir string
	catchAll  bool
}

func (o *AccountsOverlay) rows() []acctRow {
	var rows []acctRow
	if o.tab == tabClaude {
		for _, a := range o.cfg.ClaudeAccounts {
			rows = append(rows, acctRow{a.Name, a.ConfigDir, a.IsCatchAll()})
		}
		return rows
	}
	for _, a := range o.cfg.GHAccounts {
		rows = append(rows, acctRow{a.Name, a.ConfigDir, a.IsCatchAll()})
	}
	return rows
}

func (o *AccountsOverlay) activeLen() int { return len(o.rows()) }

func (o *AccountsOverlay) clampCursor() {
	n := o.activeLen()
	if n == 0 {
		o.cursor = 0
		return
	}
	if o.cursor >= n {
		o.cursor = n - 1
	}
	if o.cursor < 0 {
		o.cursor = 0
	}
}

func (o *AccountsOverlay) HandleKeyPress(msg tea.KeyMsg) (closed bool, dirty bool) {
	// Task 5 adds modeEdit/modeConfirmDelete; Task 6 adds modePreview.
	return o.handleListKey(msg)
}

func (o *AccountsOverlay) handleListKey(msg tea.KeyMsg) (closed bool, dirty bool) {
	switch msg.String() {
	case "esc", "ctrl+c":
		return true, false
	case "up", "k":
		if o.cursor > 0 {
			o.cursor--
		}
	case "down", "j":
		if o.cursor < o.activeLen()-1 {
			o.cursor++
		}
	case "tab", "left", "right":
		if o.tab == tabClaude {
			o.tab = tabGH
		} else {
			o.tab = tabClaude
		}
		o.clampCursor()
		o.lastErr = ""
	}
	return false, false
}

func (o *AccountsOverlay) boxWidth() int {
	w := o.width - 2
	if w > 64 {
		w = 64
	}
	if w < 20 {
		w = 20
	}
	return w
}

func (o *AccountsOverlay) inner() int { return o.boxWidth() - 4 } // Padding(1,2) → 4 cols

func (o *AccountsOverlay) Render() string {
	t := theme.Current()
	style := lipgloss.NewStyle().
		Border(t.Borders.Style).
		BorderForeground(t.Palette.Accent).
		Padding(1, 2).
		Width(o.boxWidth())
	title := t.OverlayTitleStyle().Render("Accounts")
	return style.Render(title + "\n\n" + o.renderList())
}

func (o *AccountsOverlay) renderTabs() string {
	t := theme.Current()
	if o.tab == tabClaude {
		return t.AccentStyle().Render("‹Claude›") + "  " + t.DimStyle().Render("GitHub")
	}
	return t.DimStyle().Render("Claude") + "  " + t.AccentStyle().Render("‹GitHub›")
}

func (o *AccountsOverlay) renderList() string {
	t := theme.Current()
	var b strings.Builder
	b.WriteString(o.renderTabs() + "\n\n")

	rows := o.rows()
	if len(rows) == 0 {
		kind := "Claude"
		if o.tab == tabGH {
			kind = "GitHub"
		}
		b.WriteString(t.DimStyle().Render("No "+kind+" accounts — press n to add") + "\n")
	} else {
		seenCatchAll := false
		for i, r := range rows {
			marker := "  "
			if i == o.cursor {
				marker = t.AccentStyle().Render("› ")
			}
			name := r.name
			if name == "" {
				name = t.DangerStyle().Render("(unnamed)")
			}
			dir := r.dir
			if dir == "" {
				dir = t.DimStyle().Render("(inherit ambient env)")
			} else {
				dir = truncTail(dir, 26)
			}
			b.WriteString(marker + padRight(name, 12) + " " + padRight(dir, 28) + " " + o.badge(r.catchAll, &seenCatchAll) + "\n")
		}
		if !o.hasCatchAll() {
			b.WriteString(t.DimStyle().Render("unmatched repos inherit the ambient account") + "\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(t.OverlayHintStyle().Render("↑/↓ move · tab switch · n new · e edit · d delete") + "\n")
	b.WriteString(t.OverlayHintStyle().Render("t test routing · esc close"))
	return b.String()
}

func (o *AccountsOverlay) badge(catchAll bool, seen *bool) string {
	t := theme.Current()
	if !catchAll {
		return t.AccentStyle().Render("routed")
	}
	if *seen {
		return t.DangerStyle().Render("catch-all (unreachable)")
	}
	*seen = true
	return t.DimStyle().Render("default")
}

func (o *AccountsOverlay) hasCatchAll() bool {
	for _, r := range o.rows() {
		if r.catchAll {
			return true
		}
	}
	return false
}

func padRight(s string, n int) string {
	if w := lipgloss.Width(s); w < n {
		return s + strings.Repeat(" ", n-w)
	}
	return s
}

func truncTail(s string, max int) string {
	r := []rune(s)
	if max <= 1 || len(r) <= max {
		return s
	}
	return "…" + string(r[len(r)-max+1:])
}
