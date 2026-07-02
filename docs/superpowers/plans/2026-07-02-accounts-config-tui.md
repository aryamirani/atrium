# Accounts-config-in-TUI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an in-TUI "Accounts" manager overlay (opened with `@`) to create, edit, and delete Claude and GitHub accounts that today are hand-edited in `config.json`.

**Architecture:** A keyboard-only modal (`AccountsOverlay`) templated on `SettingsOverlay`: two tabs (Claude / GitHub), a mode machine (list / edit / confirm-delete / preview), holding the same `*config.Config` the app holds and mutating its `ClaudeAccounts`/`GHAccounts` slices in place. The app persists with `config.SaveConfig` on change. A per-account edit sub-form (`accountForm`) generalizes `RenameOverlay`'s focus idiom to N `textinput.Model` fields and reuses the existing `DirectoryPicker` for the Config-dir field. UI-over-existing-model: **no `config/` changes**.

**Tech Stack:** Go, Bubble Tea (`github.com/charmbracelet/bubbletea`), Bubbles (`textinput`), Lipgloss, testify.

## Global Constraints

- Module path: `github.com/ZviBaratz/atrium`.
- Toolchain (mise; absolute paths — shims error). Export once per shell:
  `export GO=/home/zvi/.local/share/mise/installs/go/1.26.4/bin/go`
  `export JUST=/home/zvi/.local/share/mise/installs/just/1.25.2/just`
- Targeted test run: `$GO test ./ui/overlay/ -run TestName -v`. Full gate: `GO=$GO $JUST build && GO=$GO $JUST test && GO=$GO $JUST vet` (golangci-lint is CI-only).
- **Tests must stay hermetic**: never touch the real data dir. `package overlay` tests construct over an in-memory `*config.Config` and never call `SaveConfig`. `package app` tests rely on the existing `TestMain` HOME sandbox and mirror `newSettingsTestHome` (`app/settings_test.go:17`).
- Commits: Conventional Commits, lowercase. End every commit message with a trailer line: `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>` (omitted from the short samples below for brevity — include it).
- No `config/` package changes: the model (`config/types.go`), routers (`config/accounts.go`), and persistence (`config/persist.go`) already exist and are reused.
- License header/AGPL: match surrounding files (new files need no special header; follow the package convention).

## File Structure

- **Create** `ui/overlay/accountForm.go` — `accountForm`: N-field edit sub-form (Name, Config dir, Remote match, Path match, [Token env]); comma-list parse/render; `ctrl+o` directory-picker sub-mode; config-dir exists hint. (Tasks 2–3)
- **Create** `ui/overlay/accounts.go` — `AccountsOverlay`: tabs, mode machine, list render + badges, edit/commit/validation, delete-confirm, routing preview, self-capped box. (Tasks 4–6)
- **Create** `app/app_accounts.go` — `handleAccountsState`: routes keys to the overlay, best-effort `SaveConfig` on change, invalidates the stashed create-form draft, tears down on close. (Task 7)
- **Modify** `ui/overlay/directoryPicker.go` — add a configurable `label` (default `"Project"`) so the Config-dir picker isn't mislabeled. (Task 1)
- **Modify** `keys/keys.go`, `app/app.go`, `app/app_update.go`, `app/app_layout.go`, `app/help.go` — the `stateAccounts` wiring, mirroring `stateSettings`. (Task 7)
- **Test** `ui/overlay/directoryPicker_test.go`, `ui/overlay/accountForm_test.go` (new), `ui/overlay/accounts_test.go` (new), `config/config_test.go`, `app/accounts_test.go` (new), `app/statemachine_test.go`.

Order is dependency-driven: the picker label (1) and the form (2–3) are leaves; the overlay (4–6) composes them; the app wiring (7) mounts the overlay.

---

### Task 1: DirectoryPicker gets a configurable label

**Files:**
- Modify: `ui/overlay/directoryPicker.go` (struct field, `NewDirectoryPicker`, `Render` at :432 and :444, new `SetLabel`)
- Test: `ui/overlay/directoryPicker_test.go`

**Interfaces:**
- Produces: `func (dp *DirectoryPicker) SetLabel(label string)`; `DirectoryPicker.Render()` now prints the configured label instead of the literal `"Project"`.

- [ ] **Step 1: Write the failing test**

Add to `ui/overlay/directoryPicker_test.go`:
```go
func TestDirectoryPicker_SetLabel(t *testing.T) {
	dp := NewDirectoryPicker(nil)
	assert.Contains(t, dp.Render(), "Project:", "defaults to the Project label")

	dp.SetLabel("Config dir")
	out := dp.Render()
	assert.Contains(t, out, "Config dir:", "label is configurable")
	assert.NotContains(t, out, "Project", "old label is gone once overridden")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `$GO test ./ui/overlay/ -run TestDirectoryPicker_SetLabel -v`
Expected: FAIL — `dp.SetLabel undefined`.

- [ ] **Step 3: Add the label field + setter and use it in Render**

In the struct (`ui/overlay/directoryPicker.go`, after `visibleRows int`):
```go
	// label names the field in the header; defaults to "Project" (session creation),
	// overridable via SetLabel (e.g. "Config dir" for account editing).
	label string
```
In `NewDirectoryPicker`:
```go
func NewDirectoryPicker(candidates []string) *DirectoryPicker {
	return &DirectoryPicker{candidates: dedupePaths(candidates), visibleRows: defaultPickerRows, label: "Project"}
}
```
Add near the other setters:
```go
// SetLabel overrides the field label shown in the header (default "Project").
func (dp *DirectoryPicker) SetLabel(label string) { dp.label = label }
```
In `Render`, replace the two literals:
- line ~432: `s.WriteString(dpLabelStyle().Render("Project: "))` → `s.WriteString(dpLabelStyle().Render(dp.label + ": "))`
- line ~444: `s.WriteString(dpLabelStyle().Render("Project"))` → `s.WriteString(dpLabelStyle().Render(dp.label))`

- [ ] **Step 4: Run tests to verify they pass**

Run: `$GO test ./ui/overlay/ -run 'TestDirectoryPicker' -v`
Expected: PASS (new test + all existing `TestDirectoryPicker*`, incl. the `"Project:"` assertion at :111, still green because the default is unchanged).

- [ ] **Step 5: Commit**

```bash
git add ui/overlay/directoryPicker.go ui/overlay/directoryPicker_test.go
git commit -m "feat(overlay): make DirectoryPicker label configurable"
```

---

### Task 2: accountForm — fields, focus, comma-list parsing

**Files:**
- Create: `ui/overlay/accountForm.go`
- Test: `ui/overlay/accountForm_test.go`

**Interfaces:**
- Produces:
  - `func newAccountForm(showToken bool, name, configDir, remote, path, token string) *accountForm`
  - `func (f *accountForm) HandleKeyPress(msg tea.KeyMsg) (done bool)` — sets `submitted`/`canceled`
  - `func (f *accountForm) Name() string`, `ConfigDir() string`, `RemoteMatches() []string`, `PathMatches() []string`, `TokenEnv() []string`
  - `func (f *accountForm) Submitted() bool`, `Canceled() bool`, `Render(inner int) string`
  - `func parseList(s string) []string`
  - field-index consts `fldName, fldConfigDir, fldRemote, fldPath, fldToken`

- [ ] **Step 1: Write the failing tests**

Create `ui/overlay/accountForm_test.go`:
```go
package overlay

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

func TestParseList_TrimsAndDropsBlanks(t *testing.T) {
	assert.Equal(t, []string{"a", "b"}, parseList("a ,, b,"))
	assert.Nil(t, parseList("   "), "whitespace-only → nil, never a blank token")
	assert.Nil(t, parseList(""), "empty → nil")
	assert.Equal(t, []string{"github.com/acme"}, parseList(" github.com/acme "))
}

func TestAccountForm_SeedAndParse(t *testing.T) {
	f := newAccountForm(false, "work", "~/.claude-work", "github.com/acme, gh.com/x", "~/work/", "")
	assert.Equal(t, "work", f.Name())
	assert.Equal(t, "~/.claude-work", f.ConfigDir())
	assert.Equal(t, []string{"github.com/acme", "gh.com/x"}, f.RemoteMatches())
	assert.Equal(t, []string{"~/work/"}, f.PathMatches())
	assert.Nil(t, f.TokenEnv(), "Claude form has no token field")
	assert.Len(t, f.inputs, 4)
}

func TestAccountForm_GHHasTokenField(t *testing.T) {
	f := newAccountForm(true, "gh", "~/.config/gh-work", "", "", "GH_TOKEN, GITHUB_TOKEN")
	assert.Len(t, f.inputs, 5)
	assert.Equal(t, []string{"GH_TOKEN", "GITHUB_TOKEN"}, f.TokenEnv())
}

func TestAccountForm_NavAndSubmitCancel(t *testing.T) {
	f := newAccountForm(false, "", "", "", "", "")
	assert.Equal(t, fldName, f.focus)
	f.HandleKeyPress(tea.KeyMsg{Type: tea.KeyTab})
	assert.Equal(t, fldConfigDir, f.focus, "tab advances focus")
	f.HandleKeyPress(tea.KeyMsg{Type: tea.KeyShiftTab})
	assert.Equal(t, fldName, f.focus, "shift+tab retreats focus")

	assert.True(t, f.HandleKeyPress(tea.KeyMsg{Type: tea.KeyEnter}))
	assert.True(t, f.Submitted())

	g := newAccountForm(false, "", "", "", "", "")
	assert.True(t, g.HandleKeyPress(tea.KeyMsg{Type: tea.KeyEsc}))
	assert.True(t, g.Canceled())
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `$GO test ./ui/overlay/ -run 'TestParseList|TestAccountForm' -v`
Expected: FAIL — `undefined: newAccountForm`, `parseList`.

- [ ] **Step 3: Implement `accountForm.go`**

Create `ui/overlay/accountForm.go`:
```go
package overlay

import (
	"strings"

	"github.com/ZviBaratz/atrium/ui/theme"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

const (
	fldName = iota
	fldConfigDir
	fldRemote
	fldPath
	fldToken
)

// accountForm is the add/edit sub-form for one Claude or GitHub account. It works
// purely in strings; the owning AccountsOverlay validates and builds the typed
// config.ClaudeAccount / config.GHAccount on submit. showToken adds the GH-only
// Token env field (index fldToken); on the Claude tab that field is absent from
// inputs entirely, so nav/render/commit key off len(inputs).
type accountForm struct {
	inputs    []textinput.Model
	focus     int
	showToken bool

	picker *DirectoryPicker // non-nil only while browsing the config dir (Task 3)

	// exists-hint cache (Task 3): recompute os.Stat only when the resolved dir changes.
	statPath string
	statOK   bool
	statDone bool

	submitted bool
	canceled  bool
}

func newFieldInput(placeholder string) textinput.Model {
	ti := textinput.New()
	ti.Prompt = ""
	ti.Placeholder = placeholder
	ti.CharLimit = 256
	return ti
}

func newAccountForm(showToken bool, name, configDir, remote, path, token string) *accountForm {
	inputs := []textinput.Model{
		newFieldInput("e.g. work"),
		newFieldInput("~/.claude-work  (empty = inherit ambient env)"),
		newFieldInput("comma-separated, e.g. github.com/acme"),
		newFieldInput("comma-separated, e.g. ~/work/"),
	}
	inputs[fldName].SetValue(name)
	inputs[fldConfigDir].SetValue(configDir)
	inputs[fldRemote].SetValue(remote)
	inputs[fldPath].SetValue(path)
	if showToken {
		tok := newFieldInput("comma-separated, e.g. GH_TOKEN, GITHUB_TOKEN")
		tok.SetValue(token)
		inputs = append(inputs, tok)
	}
	f := &accountForm{inputs: inputs, showToken: showToken}
	f.applyFocus()
	return f
}

// applyFocus focuses exactly one input and blurs the rest.
func (f *accountForm) applyFocus() {
	for i := range f.inputs {
		if i == f.focus {
			f.inputs[i].Focus()
			f.inputs[i].CursorEnd()
		} else {
			f.inputs[i].Blur()
		}
	}
}

// HandleKeyPress edits the focused field; returns true when the form is done
// (submitted or canceled). The picker branch is added in Task 3.
func (f *accountForm) HandleKeyPress(msg tea.KeyMsg) (done bool) {
	switch msg.String() {
	case "enter":
		f.submitted = true
		return true
	case "esc", "ctrl+c":
		f.canceled = true
		return true
	case "tab":
		f.focus = (f.focus + 1) % len(f.inputs)
		f.applyFocus()
		return false
	case "shift+tab":
		f.focus = (f.focus - 1 + len(f.inputs)) % len(f.inputs)
		f.applyFocus()
		return false
	default:
		f.inputs[f.focus], _ = f.inputs[f.focus].Update(msg)
		return false
	}
}

func (f *accountForm) Name() string      { return strings.TrimSpace(f.inputs[fldName].Value()) }
func (f *accountForm) ConfigDir() string { return strings.TrimSpace(f.inputs[fldConfigDir].Value()) }
func (f *accountForm) RemoteMatches() []string { return parseList(f.inputs[fldRemote].Value()) }
func (f *accountForm) PathMatches() []string   { return parseList(f.inputs[fldPath].Value()) }

func (f *accountForm) TokenEnv() []string {
	if !f.showToken {
		return nil
	}
	return parseList(f.inputs[fldToken].Value())
}

func (f *accountForm) Submitted() bool { return f.submitted }
func (f *accountForm) Canceled() bool  { return f.canceled }

// parseList splits a comma-separated field, trims each token, and drops empties
// (a stray " " token would otherwise substring-match any path with a space).
// Returns nil (not []string{}) so the omitempty config fields stay dormant.
func parseList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Render draws the field list. The picker sub-view + exists hint arrive in Task 3.
func (f *accountForm) Render(inner int) string {
	t := theme.Current()
	labels := []string{"Name", "Config dir", "Remote match", "Path match", "Token env"}
	var b strings.Builder
	for i := range f.inputs {
		label := t.DimStyle().Render(labels[i])
		if i == f.focus {
			label = t.AccentStyle().Render(labels[i])
		}
		b.WriteString(label + "\n" + f.inputs[i].View() + "\n")
	}
	return b.String()
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `$GO test ./ui/overlay/ -run 'TestParseList|TestAccountForm' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add ui/overlay/accountForm.go ui/overlay/accountForm_test.go
git commit -m "feat(overlay): add accountForm edit sub-form (fields + comma-list parsing)"
```

---

### Task 3: accountForm — directory-picker sub-mode + config-dir exists hint

**Files:**
- Modify: `ui/overlay/accountForm.go` (`HandleKeyPress`, `Render`, add `configDirHint`)
- Test: `ui/overlay/accountForm_test.go`

**Interfaces:**
- Consumes: `DirectoryPicker.SetLabel` (Task 1); `NewDirectoryPicker`, `Focus`, `SetWidth`, `SetVisibleRows`, `HandleKeyPress`, `GetSelectedPath`, `CompletePrefix`.
- Produces: `ctrl+o` on the Config-dir field opens the picker; `func (f *accountForm) configDirHint() string`.

- [ ] **Step 1: Write the failing tests**

Add to `ui/overlay/accountForm_test.go`:
```go
func TestAccountForm_CtrlOOpensPickerOnConfigDirOnly(t *testing.T) {
	f := newAccountForm(false, "", "", "", "", "")
	f.HandleKeyPress(tea.KeyMsg{Type: tea.KeyCtrlO}) // focus is Name
	assert.Nil(t, f.picker, "ctrl+o does nothing unless the config-dir field is focused")

	f.focus = fldConfigDir
	f.applyFocus()
	f.HandleKeyPress(tea.KeyMsg{Type: tea.KeyCtrlO})
	assert.NotNil(t, f.picker, "ctrl+o on config dir opens the picker")

	// esc closes the picker (returns to the form), does NOT finish the form.
	done := f.HandleKeyPress(tea.KeyMsg{Type: tea.KeyEsc})
	assert.False(t, done)
	assert.Nil(t, f.picker)
	assert.False(t, f.Canceled(), "esc in the picker must not cancel the whole form")
}

func TestAccountForm_PickerEnterWritesBack(t *testing.T) {
	dir := t.TempDir()
	f := newAccountForm(false, "", dir, "", "", "")
	f.focus = fldConfigDir
	f.applyFocus()
	f.HandleKeyPress(tea.KeyMsg{Type: tea.KeyCtrlO})
	require.NotNil(t, f.picker)
	f.HandleKeyPress(tea.KeyMsg{Type: tea.KeyEnter}) // accept current selection
	assert.Nil(t, f.picker)
	assert.Equal(t, dir, f.ConfigDir(), "the picked path is written into the config-dir field")
}

func TestAccountForm_ConfigDirExistsHint(t *testing.T) {
	dir := t.TempDir()
	f := newAccountForm(false, "", dir, "", "", "")
	assert.Contains(t, f.configDirHint(), "exists")

	g := newAccountForm(false, "", "/no/such/path/xyzzy", "", "", "")
	assert.Contains(t, g.configDirHint(), "not found")

	h := newAccountForm(false, "", "", "", "", "")
	assert.Equal(t, "", h.configDirHint(), "empty config dir shows no hint")
}
```
Add `"github.com/stretchr/testify/require"` to the test imports.

- [ ] **Step 2: Run tests to verify they fail**

Run: `$GO test ./ui/overlay/ -run TestAccountForm -v`
Expected: FAIL — `f.configDirHint undefined`; picker stays nil.

- [ ] **Step 3: Implement the picker sub-mode + hint**

In `accountForm.go`, add imports `"os"` and `"github.com/ZviBaratz/atrium/config"`. Replace `HandleKeyPress` with the picker-aware version:
```go
func (f *accountForm) HandleKeyPress(msg tea.KeyMsg) (done bool) {
	if f.picker != nil {
		switch msg.String() {
		case "enter":
			f.inputs[fldConfigDir].SetValue(f.picker.GetSelectedPath())
			f.picker = nil
		case "esc", "ctrl+c":
			f.picker = nil
		case "tab":
			f.picker.CompletePrefix()
		default:
			f.picker.HandleKeyPress(msg)
		}
		return false
	}
	switch msg.String() {
	case "enter":
		f.submitted = true
		return true
	case "esc", "ctrl+c":
		f.canceled = true
		return true
	case "ctrl+o":
		if f.focus == fldConfigDir {
			f.openPicker()
		}
		return false
	case "tab":
		f.focus = (f.focus + 1) % len(f.inputs)
		f.applyFocus()
		return false
	case "shift+tab":
		f.focus = (f.focus - 1 + len(f.inputs)) % len(f.inputs)
		f.applyFocus()
		return false
	default:
		f.inputs[f.focus], _ = f.inputs[f.focus].Update(msg)
		return false
	}
}

func (f *accountForm) openPicker() {
	cur := config.ClaudeAccount{ConfigDir: f.ConfigDir()}.ResolvedConfigDir()
	p := NewDirectoryPicker([]string{cur, "~"})
	p.SetLabel("Config dir")
	p.SetVisibleRows(5)
	p.Focus()
	f.picker = p
}

// configDirHint reports whether the resolved config dir exists, cached by path so
// the os.Stat runs only when the value changes (not on every render/keystroke).
func (f *accountForm) configDirHint() string {
	v := f.ConfigDir()
	if v == "" {
		return ""
	}
	resolved := config.ClaudeAccount{ConfigDir: v}.ResolvedConfigDir()
	if !f.statDone || resolved != f.statPath {
		info, err := os.Stat(resolved)
		f.statOK = err == nil && info.IsDir()
		f.statPath = resolved
		f.statDone = true
	}
	if f.statOK {
		return theme.Current().DimStyle().Render("  (exists)")
	}
	return theme.Current().DangerStyle().Render("  (not found)")
}
```
Update `Render` to show the picker when open and the hint on the config-dir line:
```go
func (f *accountForm) Render(inner int) string {
	t := theme.Current()
	if f.picker != nil {
		f.picker.SetWidth(inner)
		return t.DimStyle().Render("Browse config dir") + "\n\n" + f.picker.Render()
	}
	labels := []string{"Name", "Config dir", "Remote match", "Path match", "Token env"}
	var b strings.Builder
	for i := range f.inputs {
		label := t.DimStyle().Render(labels[i])
		if i == f.focus {
			label = t.AccentStyle().Render(labels[i])
		}
		hint := ""
		if i == fldConfigDir {
			hint = f.configDirHint()
		}
		b.WriteString(label + hint + "\n" + f.inputs[i].View() + "\n")
	}
	return b.String()
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `$GO test ./ui/overlay/ -run 'TestAccountForm|TestParseList' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add ui/overlay/accountForm.go ui/overlay/accountForm_test.go
git commit -m "feat(overlay): accountForm config-dir picker sub-mode + exists hint"
```

---

### Task 4: AccountsOverlay — tabs, list mode, badges, sizing

**Files:**
- Create: `ui/overlay/accounts.go`
- Test: `ui/overlay/accounts_test.go`

**Interfaces:**
- Produces:
  - `func NewAccountsOverlay(cfg *config.Config) *AccountsOverlay`
  - `func (o *AccountsOverlay) SetSize(w, h int)`
  - `func (o *AccountsOverlay) HandleKeyPress(msg tea.KeyMsg) (closed bool, dirty bool)`
  - `func (o *AccountsOverlay) Render() string`
  - test-only helpers `func (o *AccountsOverlay) selectTab(t accountsTab)`, `func (o *AccountsOverlay) cursorIndex() int`
  - consts `tabClaude, tabGH accountsTab`; `modeList, modeEdit, modeConfirmDelete, modePreview accountsMode`
- Consumes (Task 5–6 extend): the `modeList` keys `n`/`e`/`d`/`t` are added in later tasks; here only nav / tab-switch / close.

- [ ] **Step 1: Write the failing tests**

Create `ui/overlay/accounts_test.go`:
```go
package overlay

import (
	"testing"

	"github.com/ZviBaratz/atrium/config"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func twoTabCfg() *config.Config {
	return &config.Config{
		ClaudeAccounts: []config.ClaudeAccount{
			{Name: "work", ConfigDir: "~/.claude-work", RemoteMatches: []string{"github.com/acme"}},
			{Name: "personal", ConfigDir: "~/.claude"},
		},
		GHAccounts: []config.GHAccount{
			{Name: "gh-work", ConfigDir: "~/.config/gh-work", RemoteMatches: []string{"github.com/acme"}},
		},
	}
}

func TestAccountsOverlay_NavAndTabSwitchClampsCursor(t *testing.T) {
	o := NewAccountsOverlay(twoTabCfg())
	o.SetSize(80, 24)
	require.Equal(t, tabClaude, o.tab)

	o.HandleKeyPress(tea.KeyMsg{Type: tea.KeyDown})
	assert.Equal(t, 1, o.cursorIndex())

	// Claude tab has 2 rows, cursor=1; GitHub tab has 1 row → cursor must clamp to 0.
	o.HandleKeyPress(tea.KeyMsg{Type: tea.KeyTab})
	assert.Equal(t, tabGH, o.tab)
	assert.Equal(t, 0, o.cursorIndex(), "cursor clamped into the shorter tab (no panic later)")
}

func TestAccountsOverlay_EmptyTabIsSafe(t *testing.T) {
	o := NewAccountsOverlay(&config.Config{})
	o.SetSize(80, 24)
	// No accounts on either tab; nav/tab/render must not panic.
	o.HandleKeyPress(tea.KeyMsg{Type: tea.KeyDown})
	o.HandleKeyPress(tea.KeyMsg{Type: tea.KeyTab})
	assert.Equal(t, 0, o.cursorIndex())
	assert.Contains(t, o.Render(), "No GitHub accounts")
}

func TestAccountsOverlay_EscCloses(t *testing.T) {
	o := NewAccountsOverlay(twoTabCfg())
	o.SetSize(80, 24)
	closed, dirty := o.HandleKeyPress(tea.KeyMsg{Type: tea.KeyEsc})
	assert.True(t, closed)
	assert.False(t, dirty)
}

func TestAccountsOverlay_BadgesMarkCatchAllAndUnreachable(t *testing.T) {
	cfg := &config.Config{ClaudeAccounts: []config.ClaudeAccount{
		{Name: "a"},                                            // first rule-less → default
		{Name: "b"},                                            // second rule-less → unreachable
		{Name: "c", RemoteMatches: []string{"github.com/x"}},   // routed
	}}
	o := NewAccountsOverlay(cfg)
	o.SetSize(80, 24)
	out := o.Render()
	assert.Contains(t, out, "default")
	assert.Contains(t, out, "unreachable")
	assert.Contains(t, out, "routed")
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `$GO test ./ui/overlay/ -run TestAccountsOverlay -v`
Expected: FAIL — `undefined: NewAccountsOverlay`.

- [ ] **Step 3: Implement `accounts.go` (list mode only)**

Create `ui/overlay/accounts.go`:
```go
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
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `$GO test ./ui/overlay/ -run TestAccountsOverlay -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add ui/overlay/accounts.go ui/overlay/accounts_test.go
git commit -m "feat(overlay): AccountsOverlay list mode with tabs and routing badges"
```

---

### Task 5: AccountsOverlay — add/edit/commit + delete-confirm

**Files:**
- Modify: `ui/overlay/accounts.go` (struct fields `form`/`editIndex`; `handleListKey` n/e/d; new `handleEditKey`, `handleConfirmKey`, `commit`, edit/confirm render; dispatch in `HandleKeyPress`)
- Test: `ui/overlay/accounts_test.go`; `config/config_test.go`

**Interfaces:**
- Consumes: `newAccountForm` (Task 2–3); `config.ClaudeAccount`, `config.GHAccount`.
- Produces: `n`/`e`/`enter`/`d` in list mode; committed accounts land in `cfg.ClaudeAccounts`/`cfg.GHAccounts`.

- [ ] **Step 1: Write the failing tests (overlay)**

Add to `ui/overlay/accounts_test.go`:
```go
// typeInto sends each rune of s to the overlay as individual key messages.
func typeInto(o *AccountsOverlay, s string) {
	for _, r := range s {
		o.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
}

func TestAccountsOverlay_AddAppendsOnCommit(t *testing.T) {
	cfg := &config.Config{}
	o := NewAccountsOverlay(cfg)
	o.SetSize(80, 24)

	o.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}}) // new
	require.Equal(t, modeEdit, o.mode)
	typeInto(o, "work")                                  // Name
	o.HandleKeyPress(tea.KeyMsg{Type: tea.KeyTab})       // → Config dir
	typeInto(o, "~/.claude-work")
	_, dirty := o.HandleKeyPress(tea.KeyMsg{Type: tea.KeyEnter}) // commit

	assert.True(t, dirty)
	assert.Equal(t, modeList, o.mode)
	require.Len(t, cfg.ClaudeAccounts, 1)
	assert.Equal(t, "work", cfg.ClaudeAccounts[0].Name)
	assert.Equal(t, "~/.claude-work", cfg.ClaudeAccounts[0].ConfigDir)
}

func TestAccountsOverlay_ValidationRejectsEmptyAndDuplicateName(t *testing.T) {
	cfg := &config.Config{ClaudeAccounts: []config.ClaudeAccount{{Name: "work"}}}
	o := NewAccountsOverlay(cfg)
	o.SetSize(80, 24)

	o.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	_, dirty := o.HandleKeyPress(tea.KeyMsg{Type: tea.KeyEnter}) // empty name
	assert.False(t, dirty)
	assert.Equal(t, modeEdit, o.mode, "stays in edit on validation error")
	assert.NotEmpty(t, o.lastErr)
	assert.Len(t, cfg.ClaudeAccounts, 1, "config not mutated")

	typeInto(o, "work") // duplicate of the existing account
	_, dirty = o.HandleKeyPress(tea.KeyMsg{Type: tea.KeyEnter})
	assert.False(t, dirty)
	assert.Equal(t, modeEdit, o.mode)
	assert.Len(t, cfg.ClaudeAccounts, 1)
}

func TestAccountsOverlay_CancelDiscardsEdits(t *testing.T) {
	cfg := &config.Config{ClaudeAccounts: []config.ClaudeAccount{
		{Name: "work", RemoteMatches: []string{"github.com/acme"}},
	}}
	o := NewAccountsOverlay(cfg)
	o.SetSize(80, 24)

	o.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}}) // edit row 0
	require.Equal(t, modeEdit, o.mode)
	typeInto(o, "-extra")                                // mutate the Name field
	o.HandleKeyPress(tea.KeyMsg{Type: tea.KeyEsc})       // cancel

	assert.Equal(t, modeList, o.mode)
	assert.Equal(t, "work", cfg.ClaudeAccounts[0].Name, "esc discards edits")
	assert.Equal(t, []string{"github.com/acme"}, cfg.ClaudeAccounts[0].RemoteMatches)
}

func TestAccountsOverlay_DeleteWithConfirm(t *testing.T) {
	cfg := &config.Config{ClaudeAccounts: []config.ClaudeAccount{{Name: "a"}, {Name: "b"}}}
	o := NewAccountsOverlay(cfg)
	o.SetSize(80, 24)
	o.cursor = 1

	o.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	require.Equal(t, modeConfirmDelete, o.mode)
	_, dirty := o.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	assert.True(t, dirty)
	require.Len(t, cfg.ClaudeAccounts, 1)
	assert.Equal(t, "a", cfg.ClaudeAccounts[0].Name)
	assert.Equal(t, 0, o.cursor, "cursor clamped after delete")
}

func TestAccountsOverlay_GHCommitIncludesTokenEnv(t *testing.T) {
	cfg := &config.Config{}
	o := NewAccountsOverlay(cfg)
	o.SetSize(80, 24)
	o.selectTab(tabGH)

	o.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	typeInto(o, "gh-work")
	// jump to the Token env field (index fldToken) via tab presses
	for i := 0; i < fldToken; i++ {
		o.HandleKeyPress(tea.KeyMsg{Type: tea.KeyTab})
	}
	typeInto(o, "GH_TOKEN")
	o.HandleKeyPress(tea.KeyMsg{Type: tea.KeyEnter})

	require.Len(t, cfg.GHAccounts, 1)
	assert.Equal(t, []string{"GH_TOKEN"}, cfg.GHAccounts[0].TokenEnv)
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `$GO test ./ui/overlay/ -run TestAccountsOverlay -v`
Expected: FAIL — `n` does nothing yet (mode stays `modeList`).

- [ ] **Step 3: Implement edit + delete**

In `accounts.go`, add fields to the struct:
```go
	form      *accountForm
	editIndex int // -1 = new (append); >=0 = replace at index
```
Replace `HandleKeyPress` with the full dispatch:
```go
func (o *AccountsOverlay) HandleKeyPress(msg tea.KeyMsg) (closed bool, dirty bool) {
	switch o.mode {
	case modeEdit:
		return o.handleEditKey(msg)
	case modeConfirmDelete:
		return o.handleConfirmKey(msg)
	default:
		return o.handleListKey(msg)
	}
}
```
Extend `handleListKey` with the `n`/`e`/`enter`/`d` cases (add before the closing `return`):
```go
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
```
Add the helpers:
```go
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
```
Update `Render` to branch on mode:
```go
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
```
Extend `renderList`'s footer so the confirm prompt replaces the hints when `o.mode == modeConfirmDelete` (wrap the existing footer):
```go
	b.WriteString("\n")
	if o.mode == modeConfirmDelete {
		b.WriteString(theme.Current().DangerStyle().Render("Delete '" + o.rows()[o.cursor].name + "'?  y / n"))
	} else {
		b.WriteString(t.OverlayHintStyle().Render("↑/↓ move · tab switch · n new · e edit · d delete") + "\n")
		b.WriteString(t.OverlayHintStyle().Render("t test routing · esc close"))
	}
	return b.String()
```

- [ ] **Step 4: Run to verify they pass**

Run: `$GO test ./ui/overlay/ -run TestAccountsOverlay -v`
Expected: PASS.

- [ ] **Step 5: Write + run the config round-trip test**

Add to `config/config_test.go`:
```go
func TestConfig_AccountsRoundTrip(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ClaudeAccounts = []ClaudeAccount{
		{Name: "work", ConfigDir: "~/.claude-work", RemoteMatches: []string{"github.com/acme"}, PathMatches: []string{"~/work/"}},
		{Name: "personal", ConfigDir: ""}, // catch-all, empty dir
	}
	cfg.GHAccounts = []GHAccount{
		{Name: "gh-work", ConfigDir: "~/.config/gh-work", RemoteMatches: []string{"github.com/acme"}, TokenEnv: []string{"GH_TOKEN", "GITHUB_TOKEN"}},
	}
	require.NoError(t, SaveConfig(cfg))

	got := LoadConfig()
	require.Len(t, got.ClaudeAccounts, 2)
	assert.Equal(t, []string{"github.com/acme"}, got.ClaudeAccounts[0].RemoteMatches)
	assert.Equal(t, "", got.ClaudeAccounts[1].ConfigDir)
	assert.True(t, got.ClaudeAccounts[1].IsCatchAll())
	require.Len(t, got.GHAccounts, 1)
	assert.Equal(t, []string{"GH_TOKEN", "GITHUB_TOKEN"}, got.GHAccounts[0].TokenEnv)
}
```
Run: `GO=$GO $JUST test` (or `$GO test ./config/ -run TestConfig_AccountsRoundTrip -v`). Expected: PASS. (HOME is sandboxed by the config package's `TestMain`.)

- [ ] **Step 6: Commit**

```bash
git add ui/overlay/accounts.go ui/overlay/accounts_test.go config/config_test.go
git commit -m "feat(overlay): AccountsOverlay add/edit/delete with validation"
```

---

### Task 6: AccountsOverlay — routing preview

**Files:**
- Modify: `ui/overlay/accounts.go` (preview inputs; `t` in list mode; `handlePreviewKey`; `renderPreview`; dispatch + Render branch)
- Test: `ui/overlay/accounts_test.go`

**Interfaces:**
- Consumes: `(*config.Config).ResolveClaudeAccount(remote, path)`, `ResolveGHAccount(remote, path)`.
- Produces: `t` opens the routing preview; read-only live resolution.

- [ ] **Step 1: Write the failing tests**

Add to `ui/overlay/accounts_test.go`:
```go
func TestAccountsOverlay_PreviewResolves(t *testing.T) {
	cfg := &config.Config{ClaudeAccounts: []config.ClaudeAccount{
		{Name: "work", ConfigDir: "~/.claude-work", RemoteMatches: []string{"github.com/acme"}},
		{Name: "personal", ConfigDir: "~/.claude"}, // catch-all
	}}
	o := NewAccountsOverlay(cfg)
	o.SetSize(80, 24)

	o.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	require.Equal(t, modePreview, o.mode)
	typeInto(o, "github.com/acme/widgets")
	assert.Contains(t, o.renderPreview(), "work", "remote matches the work account")

	o.HandleKeyPress(tea.KeyMsg{Type: tea.KeyEsc})
	assert.Equal(t, modeList, o.mode)
}

func TestAccountsOverlay_PreviewEmptyAndRuleOnlyInheritAmbient(t *testing.T) {
	// 0 accounts
	o := NewAccountsOverlay(&config.Config{})
	o.SetSize(80, 24)
	o.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	typeInto(o, "github.com/acme")
	out := o.renderPreview()
	assert.Contains(t, out, "inherit")
	assert.NotContains(t, out, "Claude → \n", "no blank name")

	// rule-only (no catch-all), unmatched input
	cfg := &config.Config{ClaudeAccounts: []config.ClaudeAccount{
		{Name: "work", RemoteMatches: []string{"github.com/acme"}},
	}}
	o2 := NewAccountsOverlay(cfg)
	o2.SetSize(80, 24)
	o2.HandleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	typeInto(o2, "github.com/other")
	assert.Contains(t, o2.renderPreview(), "inherit", "no-match with no catch-all inherits ambient")
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `$GO test ./ui/overlay/ -run TestAccountsOverlay_Preview -v`
Expected: FAIL — `t` does nothing; `renderPreview` undefined.

- [ ] **Step 3: Implement the preview**

Add fields to the struct:
```go
	previewInputs []textinput.Model // [remote, path]
	previewFocus  int
```
Add `"github.com/charmbracelet/bubbles/textinput"` to the imports.
In `HandleKeyPress`, add a `case modePreview: return o.handlePreviewKey(msg)` branch.
In `handleListKey`, add:
```go
	case "t":
		o.previewInputs = []textinput.Model{newFieldInput("remote URL (optional)"), newFieldInput("path (optional)")}
		o.previewInputs[0].Focus()
		o.previewFocus = 0
		o.mode = modePreview
```
Add:
```go
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
```
Add the Render branch: `case modePreview: body = o.renderPreview()`.

- [ ] **Step 4: Run to verify they pass**

Run: `$GO test ./ui/overlay/ -run TestAccountsOverlay -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add ui/overlay/accounts.go ui/overlay/accounts_test.go
git commit -m "feat(overlay): AccountsOverlay routing preview"
```

---

### Task 7: App wiring + integration tests

**Files:**
- Modify: `keys/keys.go`, `app/app.go`, `app/app_update.go`, `app/app_layout.go`, `app/help.go`, `app/statemachine_test.go`
- Create: `app/app_accounts.go`
- Test: `app/accounts_test.go`

**Interfaces:**
- Consumes: `overlay.NewAccountsOverlay`, `AccountsOverlay.{HandleKeyPress,SetSize,Render}` (Tasks 4–6).
- Produces: `keys.KeyAccounts`, `stateAccounts`, `m.accountsOverlay`, `(*home).handleAccountsState`.

- [ ] **Step 1: Wire keys, state, view, update, layout, help (no test yet)**

`keys/keys.go`: add `KeyAccounts` to the `KeyName` iota right after `KeySettings` (`:86`):
```go
	KeyAccounts // Open the accounts panel to manage Claude/GitHub accounts
```
Add to `GlobalKeyStringsMap` (next to `",": KeySettings`, `:166`): `"@": KeyAccounts,`
Add to `GlobalKeyBindings` (next to `KeySettings`, `:346`):
```go
	KeyAccounts: key.NewBinding(
		key.WithKeys("@"),
		key.WithHelp("@", "accounts"),
	),
```

`app/app.go`: add `stateAccounts` after `stateWelcome` (`:130`):
```go
	// stateAccounts is the Claude/GitHub account manager modal.
	stateAccounts
```
Add the field near `settingsOverlay` (`:261`): `accountsOverlay *overlay.AccountsOverlay`
Add a `View()` branch after the `stateWelcome` branch (`:414`):
```go
	} else if m.state == stateAccounts {
		if m.accountsOverlay == nil {
			log.ErrorLog.Printf("accounts overlay is nil")
		}
		return overlay.PlaceOverlay(0, 0, m.accountsOverlay.Render(), mainView, true)
	}
```

`app/app_update.go`: add the early return after the `stateSettings` block (`:387`):
```go
	if m.state == stateAccounts {
		return m.handleAccountsState(msg)
	}
```
Add to `switch name` next to `case keys.KeySettings` (`:449`):
```go
	case keys.KeyAccounts:
		m.state = stateAccounts
		m.accountsOverlay = overlay.NewAccountsOverlay(m.appConfig)
		m.recomputeLayout()
		return m, tea.WindowSize()
```

`app/app_layout.go`: add the sizing call next to the settings one (`:61-65`):
```go
	if m.accountsOverlay != nil {
		m.accountsOverlay.SetSize(msg.Width, msg.Height)
	}
```
Add `stateAccounts` to the `menuVisible()` false-list (`:97`):
```go
	case statePrompt, stateRename, stateConfirm, stateHelp, stateInfo, stateSettings, stateWelcome, stateAccounts:
```

`app/help.go`: add after the settings row (`:105`): `helpRow("@", "accounts (Claude / GitHub)"),`

- [ ] **Step 2: Create the handler**

Create `app/app_accounts.go`:
```go
package app

import (
	"github.com/ZviBaratz/atrium/config"
	"github.com/ZviBaratz/atrium/log"

	tea "github.com/charmbracelet/bubbletea"
)

// handleAccountsState routes a key to the accounts overlay, persists on change, and
// reclaims the menu row when the panel closes. Persisting is all that's needed: new
// sessions and the create overlay read m.appConfig live; running sessions keep their
// already-injected env (they never re-resolve accounts).
func (m *home) handleAccountsState(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	closed, dirty := m.accountsOverlay.HandleKeyPress(msg)
	if dirty {
		if err := config.SaveConfig(m.appConfig); err != nil {
			log.WarningLog.Printf("failed to persist accounts: %v", err)
		}
		// A stashed create-form draft cached its account list at build time; drop it so
		// the next open rebuilds from live config and can't pin a just-deleted account.
		m.stashedDraft = nil
	}
	if closed {
		m.accountsOverlay = nil
		m.state = stateDefault
		m.recomputeLayout()
		return m, tea.WindowSize()
	}
	return m, nil
}
```
(`m.stashedDraft *overlay.TextInputOverlay` is confirmed at `app/app.go:248`; it is already niled elsewhere via `m.stashedDraft = nil`, so this line matches the existing idiom.)

- [ ] **Step 3: Build to verify wiring compiles**

Run: `GO=$GO $JUST build`
Expected: builds cleanly (`./bin/atrium`).

- [ ] **Step 4: Write the app-level tests**

Create `app/accounts_test.go`:
```go
package app

import (
	"testing"

	"github.com/ZviBaratz/atrium/config"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAccountsPanel_OpenAddPersistClose(t *testing.T) {
	resetSettingsTestState(t) // restores config.json on cleanup
	h := newSettingsTestHome()

	_, _ = h.handleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("@")})
	require.Equal(t, stateAccounts, h.state)
	require.NotNil(t, h.accountsOverlay)
	assert.False(t, h.menuVisible(), "the modal renders its own hints")

	// n → type a name → tab → config dir → enter commits + persists.
	_, _ = h.handleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	for _, r := range "work" {
		_, _ = h.handleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	_, _ = h.handleKeyPress(tea.KeyMsg{Type: tea.KeyTab})
	for _, r := range "~/.claude-work" {
		_, _ = h.handleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	_, _ = h.handleKeyPress(tea.KeyMsg{Type: tea.KeyEnter})

	require.Len(t, h.appConfig.ClaudeAccounts, 1)
	assert.Equal(t, "work", h.appConfig.ClaudeAccounts[0].Name)
	assert.Len(t, config.LoadConfig().ClaudeAccounts, 1, "the change reached disk immediately")

	_, _ = h.handleKeyPress(tea.KeyMsg{Type: tea.KeyEsc})
	assert.Equal(t, stateDefault, h.state)
	assert.Nil(t, h.accountsOverlay)
}
```

- [ ] **Step 5: Add the statemachine panic-guard row**

In `app/statemachine_test.go`, add a row to the `states` table in `TestStateMachine_BackgroundMessagesNeverPanic` (the table has fields `name string; st state; wire func(h *home, inst *session.Instance)`), next to the `"settings"` row:
```go
		{"accounts", stateAccounts, func(h *home, _ *session.Instance) {
			h.accountsOverlay = overlay.NewAccountsOverlay(h.appConfig)
		}},
```
This asserts `WindowSizeMsg`/tick messages into the open overlay don't panic (the harness sizes it via the `WindowSizeMsg` it already feeds).

- [ ] **Step 6: Run the full gate**

Run: `GO=$GO $JUST build && GO=$GO $JUST test && GO=$GO $JUST vet`
Expected: all green.

- [ ] **Step 7: Commit**

```bash
git add keys/keys.go app/app.go app/app_update.go app/app_layout.go app/help.go app/app_accounts.go app/accounts_test.go app/statemachine_test.go
git commit -m "feat(app): wire the accounts manager overlay behind @"
```

---

## Self-Review

**Spec coverage:** CRUD both kinds (Tasks 2–5); route-rule editing (Task 2 parse); GH TokenEnv (Tasks 2,5); `@` entry (Task 7); tabs + modes (Tasks 4–6); catch-all/unreachable badges + no-catch-all note (Task 4); empty-dir "(inherit ambient env)" (Tasks 4,6); routing preview (Task 6); directory picker + label fix (Tasks 1,3); persistence/no-live-apply + stash invalidation (Task 7); existing-session immunity (architectural — no re-resolution touched; guarded by the config round-trip + the fact that no session code changes); hermetic tests throughout. **Gap check:** the spec's "existing-session immunity" has no dedicated app test here because no session code is modified — if a reviewer wants it explicit, add a `session/storage` round-trip test asserting `InstanceData` account fields survive marshal/unmarshal.

**Placeholder scan:** none — every code step shows complete code; the only conditional is the `stashedDraft` field-name check in Task 7 Step 2 (verify-and-match, not a placeholder).

**Type consistency:** `newAccountForm(showToken bool, name, configDir, remote, path, token string)` used identically in Tasks 2/3 tests and Task 5 `openForm`; `HandleKeyPress` returns `(closed, dirty)` on the overlay and `(done bool)` on the form consistently; `parseList`, `acctRow`, `badge`, `clampCursor`, `boxWidth`/`inner` all defined in Task 4 and reused in 5–6.

## Execution Handoff

Plan complete. Recommended: **subagent-driven-development** (fresh subagent per task, review between tasks) given the 7 well-bounded tasks. Alternatively inline **executing-plans**.
