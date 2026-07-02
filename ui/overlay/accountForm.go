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

func (f *accountForm) Name() string            { return strings.TrimSpace(f.inputs[fldName].Value()) }
func (f *accountForm) ConfigDir() string       { return strings.TrimSpace(f.inputs[fldConfigDir].Value()) }
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
