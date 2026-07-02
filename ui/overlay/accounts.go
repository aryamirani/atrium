package overlay

import (
	"strings"

	"github.com/ZviBaratz/atrium/config"
	"github.com/ZviBaratz/atrium/ui/theme"

	"github.com/charmbracelet/bubbles/textinput"
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

	form      *accountForm
	editIndex int // -1 = new (append); >=0 = replace at index

	previewInputs []textinput.Model // [remote, path]
	previewFocus  int
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
	switch o.mode {
	case modeEdit:
		return o.handleEditKey(msg)
	case modeConfirmDelete:
		return o.handleConfirmKey(msg)
	case modePreview:
		return o.handlePreviewKey(msg)
	default:
		return o.handleListKey(msg)
	}
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
	case "n":
		o.openForm(-1)
	case "e", "enter":
		if o.activeLen() > 0 {
			o.openForm(o.cursor)
		}
	case "d":
		if o.activeLen() > 0 {
			o.mode = modeConfirmDelete
		}
	case "t":
		o.previewInputs = []textinput.Model{newFieldInput("remote URL (optional)"), newFieldInput("path (optional)")}
		o.previewInputs[0].Focus()
		o.previewFocus = 0
		o.mode = modePreview
	}
	return false, false
}

func (o *AccountsOverlay) showToken() bool { return o.tab == tabGH }

func (o *AccountsOverlay) openForm(index int) {
	o.editIndex = index
	o.lastErr = ""
	if index < 0 {
		o.form = newAccountForm(o.showToken(), "", "", "", "", "")
	} else if o.tab == tabClaude {
		a := o.cfg.ClaudeAccounts[index]
		o.form = newAccountForm(false, a.Name, a.ConfigDir,
			strings.Join(a.RemoteMatches, ", "), strings.Join(a.PathMatches, ", "), "")
	} else {
		a := o.cfg.GHAccounts[index]
		o.form = newAccountForm(true, a.Name, a.ConfigDir,
			strings.Join(a.RemoteMatches, ", "), strings.Join(a.PathMatches, ", "),
			strings.Join(a.TokenEnv, ", "))
	}
	o.mode = modeEdit
}

func (o *AccountsOverlay) handleEditKey(msg tea.KeyMsg) (closed bool, dirty bool) {
	if !o.form.HandleKeyPress(msg) {
		return false, false
	}
	if o.form.Canceled() {
		o.form = nil
		o.mode = modeList
		return false, false
	}
	// submitted → validate then commit
	if err := o.validate(); err != "" {
		o.lastErr = err
		o.form.submitted = false // stay in edit
		return false, false
	}
	o.commit()
	o.form = nil
	o.mode = modeList
	o.lastErr = ""
	return false, true
}

// validate rejects an empty or duplicate (within the active tab) name.
func (o *AccountsOverlay) validate() string {
	name := o.form.Name()
	if name == "" {
		return "name is required"
	}
	for i, r := range o.rows() {
		if i != o.editIndex && r.name == name {
			return "an account named '" + name + "' already exists"
		}
	}
	return ""
}

func (o *AccountsOverlay) commit() {
	if o.tab == tabClaude {
		a := config.ClaudeAccount{
			Name: o.form.Name(), ConfigDir: o.form.ConfigDir(),
			RemoteMatches: o.form.RemoteMatches(), PathMatches: o.form.PathMatches(),
		}
		if o.editIndex < 0 {
			o.cfg.ClaudeAccounts = append(o.cfg.ClaudeAccounts, a)
		} else {
			o.cfg.ClaudeAccounts[o.editIndex] = a
		}
		return
	}
	a := config.GHAccount{
		Name: o.form.Name(), ConfigDir: o.form.ConfigDir(),
		RemoteMatches: o.form.RemoteMatches(), PathMatches: o.form.PathMatches(),
		TokenEnv: o.form.TokenEnv(),
	}
	if o.editIndex < 0 {
		o.cfg.GHAccounts = append(o.cfg.GHAccounts, a)
	} else {
		o.cfg.GHAccounts[o.editIndex] = a
	}
}

func (o *AccountsOverlay) handleConfirmKey(msg tea.KeyMsg) (closed bool, dirty bool) {
	switch msg.String() {
	case "y", "enter":
		if o.tab == tabClaude {
			o.cfg.ClaudeAccounts = append(o.cfg.ClaudeAccounts[:o.cursor], o.cfg.ClaudeAccounts[o.cursor+1:]...)
		} else {
			o.cfg.GHAccounts = append(o.cfg.GHAccounts[:o.cursor], o.cfg.GHAccounts[o.cursor+1:]...)
		}
		o.clampCursor()
		o.mode = modeList
		return false, true
	case "n", "esc", "ctrl+c":
		o.mode = modeList
	}
	return false, false
}

func (o *AccountsOverlay) handlePreviewKey(msg tea.KeyMsg) (closed bool, dirty bool) {
	switch msg.String() {
	case "esc", "ctrl+c":
		o.previewInputs = nil
		o.mode = modeList
	case "tab", "shift+tab":
		o.previewFocus = (o.previewFocus + 1) % 2
		for i := range o.previewInputs {
			if i == o.previewFocus {
				o.previewInputs[i].Focus()
			} else {
				o.previewInputs[i].Blur()
			}
		}
	default:
		o.previewInputs[o.previewFocus], _ = o.previewInputs[o.previewFocus].Update(msg)
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
	var body string
	switch o.mode {
	case modeEdit:
		body = o.renderEdit()
	case modePreview:
		body = o.renderPreview()
	default:
		body = o.renderList()
	}
	title := t.OverlayTitleStyle().Render("Accounts")
	return style.Render(title + "\n\n" + body)
}

func (o *AccountsOverlay) renderEdit() string {
	t := theme.Current()
	kind := "Claude"
	if o.tab == tabGH {
		kind = "GitHub"
	}
	verb := "New"
	if o.editIndex >= 0 {
		verb = "Edit"
	}
	var b strings.Builder
	b.WriteString(t.AccentStyle().Render(verb+" "+kind+" account") + "\n\n")
	b.WriteString(o.form.Render(o.inner()) + "\n")
	if o.lastErr != "" {
		b.WriteString(t.DangerStyle().Render(o.lastErr) + "\n")
	}
	b.WriteString(t.OverlayHintStyle().Render("tab/⇧tab field · ⌃o browse dir · ↵ save · esc cancel"))
	return b.String()
}

func (o *AccountsOverlay) renderPreview() string {
	t := theme.Current()
	remote := strings.TrimSpace(o.previewInputs[0].Value())
	path := strings.TrimSpace(o.previewInputs[1].Value())

	name, cdir, _ := o.cfg.ResolveClaudeAccount(remote, path)
	claude := "inherit ambient env"
	if name != "" && cdir != "" {
		claude = name + " (" + cdir + ")"
	} else if name != "" && name != "default" {
		claude = name + " (inherit ambient env)"
	}

	ghDir, ghTok := o.cfg.ResolveGHAccount(remote, path)
	gh := "inherit ambient env"
	if ghDir != "" {
		gh = ghDir
		if len(ghTok) > 0 {
			gh += " [" + strings.Join(ghTok, ", ") + "]"
		}
	}

	var b strings.Builder
	b.WriteString(t.AccentStyle().Render("Test routing") + "\n\n")
	b.WriteString(t.DimStyle().Render("Remote URL") + "\n" + o.previewInputs[0].View() + "\n")
	b.WriteString(t.DimStyle().Render("Path") + "\n" + o.previewInputs[1].View() + "\n\n")
	b.WriteString(t.DimStyle().Render("Claude → ") + claude + "\n")
	b.WriteString(t.DimStyle().Render("GitHub → ") + gh + "\n\n")
	b.WriteString(t.OverlayHintStyle().Render("tab switch field · esc back"))
	return b.String()
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
	if o.mode == modeConfirmDelete {
		b.WriteString(theme.Current().DangerStyle().Render("Delete '" + o.rows()[o.cursor].name + "'?  y / n"))
	} else {
		b.WriteString(t.OverlayHintStyle().Render("↑/↓ move · tab switch · n new · e edit · d delete") + "\n")
		b.WriteString(t.OverlayHintStyle().Render("t test routing · esc close"))
	}
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
