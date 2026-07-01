# First-run guided setup — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn Atrium's passive first-launch Welcome modal into an interactive, agent-focused setup that detects installed agent CLIs, lets the user pick and persist the default program, and warns returning users when their default program isn't on PATH.

**Architecture:** A new `overlay.WelcomeOverlay` (reusing the existing `overlay.ProfilePicker` and `config.DetectAgentProfiles`) is shown in a new `stateWelcome` on first launch; detection runs async (`detectAgentsCmd` → `agentsDetectedMsg`, mirroring `startProjectScan`). Confirming persists `DefaultProgram` + merges detected profiles + sets the welcome seen-bit. A separate, one-shot startup check surfaces a non-blocking notice when the effective program isn't installed, for launches where the welcome doesn't show.

**Tech Stack:** Go, Bubble Tea (`github.com/charmbracelet/bubbletea`), lipgloss. Existing packages: `config`, `ui/overlay`, `app`.

## Global Constraints

- Toolchain is mise-managed; use absolute paths (shims error):
  - `GO=/home/zvi/.local/share/mise/installs/go/1.26.4/bin/go`
  - `JUST=/home/zvi/.local/share/mise/installs/just/1.25.2/just`
  - `GOFMT=/home/zvi/.local/share/mise/installs/go/1.26.4/bin/gofmt`
- Full suite (source of truth, hermetic): `GO=$GO $JUST test`. Vet: `GO=$GO $JUST vet`. Format check: run `$GOFMT -l <files>` (the `just fmt-check` recipe calls bare `gofmt`, not on PATH).
- Every commit runs `GO=$GO $JUST build` and `GO=$GO $JUST test` and must be green. `golangci-lint` is **not installed locally** (CI-verified) — note "lint CI-verified" in each commit body.
- Tests MUST stay hermetic: never touch the real data dir. The `app` package's `TestMain` already sandboxes `$HOME` via `testutil.SandboxHomeMain`; `config` tests set `HOME` to a temp dir per test. Any new test reaching config/state does the same.
- Conventional Commits, lowercase (`feat:`/`refactor:`/`test:`). End each commit message with:
  `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`
- Work on branch `zvi/onboarding-guided-setup` in worktree `/home/zvi/Projects/atrium-onboarding` (based on `origin/main` `1d429bb`). The design spec is `docs/superpowers/specs/2026-07-01-onboarding-guided-setup-design.md`.
- Concrete types over new interfaces. Package-var func seams for test injection are the established idiom (e.g. `checkForUpdate`, `cleanupTerminalForInstance`).

## File Structure

- `config/agents.go` — **modify**: add `ProgramInstalled(program string) bool` (reuses the existing `detectAgentCommand` var, so the claude shell-function nuance is preserved). Add `"strings"` import.
- `config/agents_program_test.go` — **create**: `ProgramInstalled` unit tests.
- `ui/overlay/welcome.go` — **create**: `WelcomeOverlay` (composes copy + reused `ProfilePicker` + health line; local bordered style like `confirmationOverlay`/`renameOverlay`).
- `ui/overlay/welcome_test.go` — **create**: `WelcomeOverlay` unit tests.
- `app/app.go` — **modify**: add `stateWelcome` to the `state` enum; add `welcomeOverlay *overlay.WelcomeOverlay` and `pathWarned bool` fields; add the `stateWelcome` branch in `View()`.
- `app/help.go` — **modify**: `maybeShowWelcome()` returns `tea.Cmd`, builds the interactive `WelcomeOverlay` on first run (or defers to `maybeWarnMissingProgram` otherwise).
- `app/app_welcome.go` — **create**: `detectAgents` seam, `detectAgentsCmd`, `agentsDetectedMsg`, `handleWelcomeState`, `maybeWarnMissingProgram`.
- `app/app_update.go` — **modify**: `WindowSizeMsg` case returns the welcome cmd; add the `agentsDetectedMsg` case; dispatch `stateWelcome` keys to `handleWelcomeState`.
- `app/app_layout.go` — **modify**: add `stateWelcome` to the overlay-geometry `switch`.
- `app/app_welcome_test.go` — **create**: app-level welcome flow + always-on warning tests.
- `app/overhaul_test.go` — **modify**: update `TestWelcomeShowsOnce` for the new interactive welcome (behavior change; test edit is expected here).

---

### Task 1: `config.ProgramInstalled`

**Files:**
- Modify: `config/agents.go`
- Test: `config/agents_program_test.go` (create)

**Interfaces:**
- Consumes: existing `detectAgentCommand func(bin string) (string, error)` (package var, `config/agents.go:15`).
- Produces: `func ProgramInstalled(program string) bool` — reports whether the program string's first whitespace-token resolves to something runnable (claude via the shell-function-aware probe, others via `LookPath`). Empty program → false.

- [ ] **Step 1: Write the failing test**

Create `config/agents_program_test.go`:
```go
package config

import "testing"

func TestProgramInstalled(t *testing.T) {
	// Present binary: "sh" is on PATH in any POSIX test environment. detectAgentCommand
	// resolves non-"claude" tokens with a plain exec.LookPath, so this needs no stubbing.
	if !ProgramInstalled("sh") {
		t.Errorf("ProgramInstalled(\"sh\") = false, want true")
	}
	// A program string with args: only the first token is checked.
	if !ProgramInstalled("sh -c 'echo hi'") {
		t.Errorf("ProgramInstalled with args = false, want true")
	}
	// Bogus binary must be reported missing.
	if ProgramInstalled("definitely-not-a-real-binary-xyzzy") {
		t.Errorf("ProgramInstalled(bogus) = true, want false")
	}
	// Empty program is not installed.
	if ProgramInstalled("") {
		t.Errorf("ProgramInstalled(\"\") = true, want false")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GO=/home/zvi/.local/share/mise/installs/go/1.26.4/bin/go; $GO test ./config/ -run TestProgramInstalled -v`
Expected: FAIL — `undefined: ProgramInstalled`.

- [ ] **Step 3: Add the implementation**

In `config/agents.go`, change the import block from:
```go
import "os/exec"
```
to:
```go
import (
	"os/exec"
	"strings"
)
```

Then append this function to `config/agents.go`:
```go
// ProgramInstalled reports whether program's command — its first
// whitespace-separated token — resolves to something runnable. It reuses
// detectAgentCommand so the resolution matches agent detection exactly: the
// "claude" token goes through the shell-profile-aware probe (an aliased or
// shell-function claude is not falsely reported missing), every other token is
// a plain PATH lookup. An empty program (no token) is never installed.
func ProgramInstalled(program string) bool {
	fields := strings.Fields(program)
	if len(fields) == 0 {
		return false
	}
	_, err := detectAgentCommand(fields[0])
	return err == nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `$GO test ./config/ -run TestProgramInstalled -v`
Expected: PASS.

- [ ] **Step 5: Format, vet, full test, commit**

```bash
GO=/home/zvi/.local/share/mise/installs/go/1.26.4/bin/go
JUST=/home/zvi/.local/share/mise/installs/just/1.25.2/just
GOFMT=/home/zvi/.local/share/mise/installs/go/1.26.4/bin/gofmt
$GOFMT -l config/agents.go config/agents_program_test.go   # expect empty
GO=$GO $JUST vet && GO=$GO $JUST test
git add config/agents.go config/agents_program_test.go
git commit -m "$(printf 'feat(config): add ProgramInstalled PATH check\n\nReports whether a program string'\''s first token resolves to a runnable\ncommand, reusing detectAgentCommand so the claude shell-function/alias\nnuance is preserved (no false "missing"). Used by the first-run setup and\nthe always-on default-program warning. lint CI-verified.\n\nCo-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>')"
```

---

### Task 2: `overlay.WelcomeOverlay`

**Files:**
- Create: `ui/overlay/welcome.go`
- Test: `ui/overlay/welcome_test.go`

**Interfaces:**
- Consumes: `config.Profile{Name, Program string}`; `NewProfilePicker([]config.Profile) *ProfilePicker`, `(*ProfilePicker).HandleKeyPress(tea.KeyMsg) bool`, `.GetSelectedProfile() config.Profile`, `.SetWidth(int)`, `.Focus()`, `.Render() string`; shared `overlayDimStyle()`; `theme.Current().Borders.Style`, `.Palette.Accent`, `.OverlayTitleStyle()`, `.OverlayHintStyle()`.
- Produces:
  - `func NewWelcomeOverlay() *WelcomeOverlay` — starts in the "detecting" state.
  - `func (w *WelcomeOverlay) SetDetected(detected []config.Profile)` — leaves detecting; builds the picker over `detected` (none when empty).
  - `func (w *WelcomeOverlay) SetWidth(width int)`
  - `func (w *WelcomeOverlay) HandleKeyPress(msg tea.KeyMsg) bool` — returns true when the overlay should close. Enter → close with `Confirmed()==true`; Esc → close with `Confirmed()==false`; nav keys → forwarded to the picker (not closing).
  - `func (w *WelcomeOverlay) Confirmed() bool`
  - `func (w *WelcomeOverlay) SelectedProgram() string` — the picked profile's `Program`, or `""` when there was no picker (empty detection).
  - `func (w *WelcomeOverlay) Detected() []config.Profile`
  - `func (w *WelcomeOverlay) Render() string`

- [ ] **Step 1: Write the failing test**

Create `ui/overlay/welcome_test.go`:
```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `$GO test ./ui/overlay/ -run TestWelcomeOverlay -v`
Expected: FAIL — `undefined: NewWelcomeOverlay`.

- [ ] **Step 3: Write the implementation**

Create `ui/overlay/welcome.go`:
```go
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
// (Confirmed() == true), Esc skips; while detecting, only Esc (skip) closes.
func (w *WelcomeOverlay) HandleKeyPress(msg tea.KeyMsg) bool {
	if msg.Type == tea.KeyEsc {
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

// SelectedProgram is the chosen profile's Program, or "" when there was no
// picker (empty detection).
func (w *WelcomeOverlay) SelectedProgram() string {
	if w.picker == nil {
		return ""
	}
	return w.picker.GetSelectedProfile().Program
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `$GO test ./ui/overlay/ -run TestWelcomeOverlay -v`
Expected: PASS (all three subtests).

- [ ] **Step 5: Format, vet, full test, commit**

```bash
$GOFMT -l ui/overlay/welcome.go ui/overlay/welcome_test.go   # expect empty
GO=$GO $JUST vet && GO=$GO $JUST test
git add ui/overlay/welcome.go ui/overlay/welcome_test.go
git commit -m "$(printf 'feat(ui): add interactive WelcomeOverlay\n\nA first-run modal that greets the user and, once agent detection resolves\nvia SetDetected, lets them pick a default agent from an embedded\nProfilePicker (reused). Empty detection renders install guidance. Follows\nthe confirmation/rename overlay idiom: fixed-width local bordered box,\nHandleKeyPress returns close?, flags read after. lint CI-verified.\n\nCo-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>')"
```

---

### Task 3: Interactive first-run welcome (app wiring)

**Files:**
- Modify: `app/app.go` (state enum; `welcomeOverlay`/`pathWarned` fields; `View()` branch)
- Modify: `app/help.go` (`maybeShowWelcome` → returns `tea.Cmd`)
- Create: `app/app_welcome.go` (`detectAgents` seam, `detectAgentsCmd`, `agentsDetectedMsg`, `handleWelcomeState`)
- Modify: `app/app_update.go` (`WindowSizeMsg` return; `agentsDetectedMsg` case; `stateWelcome` dispatch)
- Modify: `app/app_layout.go` (overlay-geometry switch)
- Modify: `app/overhaul_test.go` (`TestWelcomeShowsOnce`)
- Test: `app/app_welcome_test.go` (create)

**Interfaces:**
- Consumes: `overlay.NewWelcomeOverlay()`, `SetDetected`, `HandleKeyPress`, `Confirmed`, `SelectedProgram`, `Detected` (Task 2); `config.DetectAgentProfiles`, `(*config.Config).MergeDetectedProfiles`, `config.SaveConfig`, `config.Config.DefaultProgram` (Task 1 file/config); `helpTypeWelcome{}.mask()`; `home.appState.GetHelpScreensSeen/SetHelpScreensSeen`.
- Produces: `var detectAgents = config.DetectAgentProfiles`; `agentsDetectedMsg{profiles []config.Profile}`; `func (m *home) detectAgentsCmd() tea.Cmd`; `func (m *home) handleWelcomeState(msg tea.KeyMsg) (tea.Model, tea.Cmd)`; the `stateWelcome` const; `home.welcomeOverlay *overlay.WelcomeOverlay`; `home.pathWarned bool`.

- [ ] **Step 1: Write the failing test**

Create `app/app_welcome_test.go`:
```go
package app

import (
	"context"
	"testing"

	"github.com/ZviBaratz/atrium/config"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"
)

// stubDetect swaps the package detection seam for the test's duration.
func stubDetect(t *testing.T, profiles []config.Profile) {
	t.Helper()
	orig := detectAgents
	detectAgents = func() []config.Profile { return profiles }
	t.Cleanup(func() { detectAgents = orig })
}

func TestWelcome_FirstRunConfirmPersistsProgram(t *testing.T) {
	stubDetect(t, []config.Profile{
		{Name: "claude", Program: "claude"},
		{Name: "codex", Program: "codex"},
	})
	h := newCreateFormHome(t)

	// First WindowSizeMsg opens the welcome and fires the detect cmd.
	model, cmd := h.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	h = model.(*home)
	require.Equal(t, stateWelcome, h.state, "first launch enters stateWelcome")
	require.NotNil(t, h.welcomeOverlay)
	require.NotNil(t, cmd, "detection cmd should be returned")

	// Run the detect cmd and feed its message back.
	msg := cmd()
	detected, ok := msg.(agentsDetectedMsg)
	require.True(t, ok, "cmd should yield agentsDetectedMsg")
	model, _ = h.Update(detected)
	h = model.(*home)

	// Move to codex and confirm.
	h.Update(tea.KeyMsg{Type: tea.KeyDown})
	model, _ = h.Update(tea.KeyMsg{Type: tea.KeyEnter})
	h = model.(*home)

	require.Equal(t, stateDefault, h.state, "confirm closes the welcome")
	require.Equal(t, "codex", h.appConfig.DefaultProgram, "confirm persists the picked program")
	require.Equal(t, "codex", h.program, "confirm applies the program to the run")
	require.NotZero(t, h.appState.GetHelpScreensSeen()&(helpTypeWelcome{}.mask()), "confirm sets the welcome seen-bit")
}

func TestWelcome_SkipLeavesSeenBitUnset(t *testing.T) {
	stubDetect(t, []config.Profile{{Name: "claude", Program: "claude"}})
	h := newCreateFormHome(t)

	model, cmd := h.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	h = model.(*home)
	model, _ = h.Update(cmd().(agentsDetectedMsg))
	h = model.(*home)

	model, _ = h.Update(tea.KeyMsg{Type: tea.KeyEsc})
	h = model.(*home)

	require.Equal(t, stateDefault, h.state, "esc closes the welcome")
	require.Zero(t, h.appState.GetHelpScreensSeen()&(helpTypeWelcome{}.mask()), "skip must not set the seen-bit")
}

var _ = context.Background // keep import if unused after edits
```

- [ ] **Step 2: Run test to verify it fails**

Run: `$GO test ./app/ -run TestWelcome_ -v`
Expected: FAIL — `undefined: stateWelcome` / `undefined: detectAgents` / `undefined: agentsDetectedMsg`.

- [ ] **Step 3: Add `stateWelcome` and the home fields (`app/app.go`)**

In the `const (...)` state block (`app/app.go:98`), add a new state after `stateVisual`:
```go
	// stateWelcome is the interactive first-launch setup modal: pick a default
	// agent from the ones detected on PATH, then start the first session.
	stateWelcome
)
```

In the `home` struct's `-- UI Components --` block (near the other overlay pointers, `app/app.go:255`), add:
```go
	// welcomeOverlay is the interactive first-run setup modal (stateWelcome).
	welcomeOverlay *overlay.WelcomeOverlay
	// pathWarned guards the one-shot startup warning that the effective program
	// is not installed, so it fires at most once per launch.
	pathWarned bool
```

- [ ] **Step 4: Create the welcome wiring (`app/app_welcome.go`)**

Create `app/app_welcome.go`:
```go
package app

import (
	"fmt"
	"strings"

	"github.com/ZviBaratz/atrium/config"
	"github.com/ZviBaratz/atrium/log"
	"github.com/ZviBaratz/atrium/ui"
	"github.com/ZviBaratz/atrium/ui/overlay"

	tea "github.com/charmbracelet/bubbletea"
)

// detectAgents is the agent-detection seam. A package var (matching the app's
// checkForUpdate/cleanupTerminalForInstance idiom) so tests inject a fake PATH.
var detectAgents = config.DetectAgentProfiles

// agentsDetectedMsg carries the result of async agent detection back to Update.
type agentsDetectedMsg struct {
	profiles []config.Profile
}

// detectAgentsCmd probes for installed agent CLIs off the main loop (detection
// shells out per agent) and delivers the result as an agentsDetectedMsg.
func (m *home) detectAgentsCmd() tea.Cmd {
	return func() tea.Msg {
		return agentsDetectedMsg{profiles: detectAgents()}
	}
}

// handleWelcomeState routes a key to the welcome overlay. On confirm it merges
// the detected agents into the profiles, persists the picked default program,
// and retires the welcome; on skip it just closes (the seen-bit stays unset so
// the welcome re-shows until the user engages or creates a session).
func (m *home) handleWelcomeState(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	shouldClose := m.welcomeOverlay.HandleKeyPress(msg)
	if !shouldClose {
		return m, nil
	}
	confirmed := m.welcomeOverlay.Confirmed()
	detected := m.welcomeOverlay.Detected()
	program := m.welcomeOverlay.SelectedProgram()
	m.state = stateDefault
	m.welcomeOverlay = nil

	if !confirmed {
		return m, nil
	}
	// Confirm: adopt the detected agents as profiles and persist the pick.
	m.appConfig.MergeDetectedProfiles(detected)
	if program != "" {
		m.appConfig.DefaultProgram = program
		m.program = program
	}
	if err := config.SaveConfig(m.appConfig); err != nil {
		log.WarningLog.Printf("failed to persist welcome setup: %v", err)
	}
	if seen := m.appState.GetHelpScreensSeen(); seen&(helpTypeWelcome{}.mask()) == 0 {
		if err := m.appState.SetHelpScreensSeen(seen | helpTypeWelcome{}.mask()); err != nil {
			log.WarningLog.Printf("failed to persist welcome-seen state: %v", err)
		}
	}
	return m, nil
}

// maybeWarnMissingProgram surfaces a one-shot, non-blocking warning when the
// effective program is not installed. Used on launches where the welcome does
// not show (returning users). It never opens a modal.
func (m *home) maybeWarnMissingProgram() tea.Cmd {
	if m.pathWarned || config.ProgramInstalled(m.program) {
		return nil
	}
	m.pathWarned = true
	cmd := firstToken(m.program)
	text := fmt.Sprintf("%s not found on PATH — press , to change the default program", cmd)
	if m.menuVisible() && m.menu != nil {
		m.menu.SetNotice(text, ui.NoticeError)
	} else {
		m.errBox.SetError(fmt.Errorf("%s", text))
		m.recomputeLayout()
	}
	return m.scheduleNoticeHide()
}

// firstToken returns the first whitespace-separated token of s (the command
// name of a program string like "aider --model x"), or s when there is none.
func firstToken(s string) string {
	if fields := strings.Fields(s); len(fields) > 0 {
		return fields[0]
	}
	return s
}

// ensure the overlay import is used even before View wiring lands.
var _ = overlay.NewWelcomeOverlay
```

- [ ] **Step 5: Rewrite `maybeShowWelcome` (`app/help.go`)**

Replace the existing `maybeShowWelcome` (`app/help.go:166-173`) with:
```go
// maybeShowWelcome opens the interactive first-launch setup on first run (until
// the welcome seen-bit is set), returning the async agent-detection command. On
// later launches it instead runs the always-on missing-program check. Guarded by
// welcomeChecked so it acts once per process.
func (m *home) maybeShowWelcome() tea.Cmd {
	if m.welcomeChecked {
		return nil
	}
	m.welcomeChecked = true

	if m.appState.GetHelpScreensSeen()&(helpTypeWelcome{}.mask()) != 0 {
		// Welcome already retired — protect returning users whose default
		// program is no longer installed.
		return m.maybeWarnMissingProgram()
	}

	m.welcomeOverlay = overlay.NewWelcomeOverlay()
	m.state = stateWelcome
	m.recomputeLayout()
	return m.detectAgentsCmd()
}
```
(The `helpTypeWelcome` type, its `toContent`/`hint`/`mask`, and `showHelpScreen` stay as-is — `showHelpScreen` is still used by the general cheatsheet. Only `maybeShowWelcome` changes.)

- [ ] **Step 6: Wire Update — `WindowSizeMsg`, `agentsDetectedMsg`, dispatch (`app/app_update.go`)**

Change the `WindowSizeMsg` case tail (`app/app_update.go:234-236`) from:
```go
		m.updateHandleWindowSizeEvent(msg)
		// First launch ever: show the one-time welcome once the size is known.
		m.maybeShowWelcome()
		return m, nil
```
to:
```go
		m.updateHandleWindowSizeEvent(msg)
		// First launch ever: show the interactive welcome once the size is known
		// (its async detection cmd is returned); returning users get the
		// always-on missing-program check instead.
		return m, m.maybeShowWelcome()
```

Add an `agentsDetectedMsg` case to the `Update` type switch (place it near the other async-result cases, e.g. after `projectScanDoneMsg`):
```go
	case agentsDetectedMsg:
		if m.state == stateWelcome && m.welcomeOverlay != nil {
			m.welcomeOverlay.SetDetected(msg.profiles)
			m.welcomeOverlay.SetWidth(54)
		}
		return m, nil
```

In `handleKeyPress` (`app/app_update.go`), add the `stateWelcome` dispatch alongside the other state handlers (e.g. right after the `stateHelp` branch):
```go
	if m.state == stateWelcome {
		return m.handleWelcomeState(msg)
	}
```

- [ ] **Step 7: Add the `View()` branch (`app/app.go`)**

In `View()` (`app/app.go:377`), add a branch to the overlay if-chain (e.g. after the `stateConfirm` branch):
```go
	} else if m.state == stateWelcome {
		if m.welcomeOverlay == nil {
			log.ErrorLog.Printf("welcome overlay is nil")
		}
		return overlay.PlaceOverlay(0, 0, m.welcomeOverlay.Render(), mainView, true)
```

- [ ] **Step 8: Add `stateWelcome` to the layout switch (`app/app_layout.go`)**

In the overlay-geometry `switch m.state` (`app/app_layout.go:87`), add `stateWelcome` to the modal case list so a resize while the welcome is up reserves geometry like the other modals:
```go
	case statePrompt, stateRename, stateConfirm, stateHelp, stateInfo, stateSettings, stateWelcome:
```

- [ ] **Step 9: Update the existing welcome test (`app/overhaul_test.go`)**

Replace `TestWelcomeShowsOnce` (`app/overhaul_test.go:16-30`) — it asserted the old static-modal behavior (`stateHelp` + `textOverlay`). The welcome is now interactive:
```go
func TestWelcomeShowsOnce(t *testing.T) {
	stubDetect(t, []config.Profile{{Name: "claude", Program: "claude"}})
	h := newCreateFormHome(t)

	cmd := h.maybeShowWelcome()
	require.Equal(t, stateWelcome, h.state, "welcome should appear on first check")
	require.NotNil(t, h.welcomeOverlay)
	require.True(t, h.welcomeChecked)
	require.NotNil(t, cmd, "detection cmd should be returned")

	// Simulate dismissing it, then check again: it must not reappear this process.
	h.state = stateDefault
	h.welcomeOverlay = nil
	cmd = h.maybeShowWelcome()
	require.Equal(t, stateDefault, h.state, "welcome must not reappear")
	require.Nil(t, h.welcomeOverlay)
	require.Nil(t, cmd)
}
```
Add `"github.com/ZviBaratz/atrium/config"` to `overhaul_test.go`'s imports if not already present. (`stubDetect` is defined in `app_welcome_test.go`, same package.)

- [ ] **Step 10: Remove the temporary import guard**

Delete the `var _ = overlay.NewWelcomeOverlay` line added in Step 4 — `overlay` is now used by `maybeShowWelcome` and `View`. (If `go build` reports `overlay` unused before this, the guard covered the gap; it is redundant once Steps 5/7 land.)

- [ ] **Step 11: Run tests**

Run: `$GO test ./app/ -run 'TestWelcome' -v`
Expected: PASS (`TestWelcome_FirstRunConfirmPersistsProgram`, `TestWelcome_SkipLeavesSeenBitUnset`, `TestWelcomeShowsOnce`).

- [ ] **Step 12: Format, vet, full test, build, commit**

```bash
$GOFMT -l app/app.go app/help.go app/app_welcome.go app/app_update.go app/app_layout.go app/app_welcome_test.go app/overhaul_test.go   # expect empty
GO=$GO $JUST vet && GO=$GO $JUST build && GO=$GO $JUST test
git add app/app.go app/help.go app/app_welcome.go app/app_update.go app/app_layout.go app/app_welcome_test.go app/overhaul_test.go
git commit -m "$(printf 'feat(app): interactive first-run welcome with agent detection\n\nReplace the passive first-launch Welcome modal with an interactive\nWelcomeOverlay in a new stateWelcome. On first launch maybeShowWelcome\nopens it and fires async agent detection (detectAgentsCmd ->\nagentsDetectedMsg); confirming merges the detected agents into profiles,\npersists the picked default program, and sets the welcome seen-bit. Skip\nleaves the seen-bit unset so the welcome re-shows until the user engages\nor creates a session. Detection is behind the detectAgents seam for\nhermetic tests. lint CI-verified.\n\nCo-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>')"
```

---

### Task 4: Always-on missing-program warning

**Files:**
- Modify: `app/app_welcome.go` (already has `maybeWarnMissingProgram` from Task 3; this task adds its test coverage and confirms the wiring)
- Test: `app/app_welcome_test.go` (extend)

**Interfaces:**
- Consumes: `config.ProgramInstalled` (Task 1); `home.menu.SetNotice`, `home.errBox.SetError`, `home.scheduleNoticeHide`, `home.menuVisible` (existing); the `maybeShowWelcome` returning-user branch (Task 3).
- Produces: no new symbols — verifies `maybeWarnMissingProgram` behavior.

> Note: `maybeWarnMissingProgram` and its wiring in `maybeShowWelcome`'s returning-user branch already landed in Task 3 (they were needed for `maybeShowWelcome` to compile). This task adds the guard tests that lock its behavior. Keep it a separate task so a reviewer can gate the warning behavior independently of the welcome UI.

- [ ] **Step 1: Write the failing tests**

Append to `app/app_welcome_test.go`:
```go
// markWelcomeSeen flips the welcome seen-bit so maybeShowWelcome takes the
// returning-user branch instead of showing the welcome.
func markWelcomeSeen(t *testing.T, h *home) {
	t.Helper()
	seen := h.appState.GetHelpScreensSeen()
	require.NoError(t, h.appState.SetHelpScreensSeen(seen|(helpTypeWelcome{}.mask())))
}

func TestWarn_ReturningUserMissingProgram(t *testing.T) {
	h := newCreateFormHome(t)
	markWelcomeSeen(t, h)
	h.program = "definitely-not-a-real-binary-xyzzy"

	// Size the app so the menu/errBox are laid out, then trigger the startup path.
	h.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	require.Equal(t, stateDefault, h.state, "no welcome for a returning user")
	require.True(t, h.pathWarned, "a missing program must trigger the one-shot warning")
}

func TestWarn_ReturningUserInstalledProgram(t *testing.T) {
	h := newCreateFormHome(t)
	markWelcomeSeen(t, h)
	h.program = "sh" // present on PATH

	h.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	require.False(t, h.pathWarned, "an installed program must not warn")
}

func TestWarn_SuppressedWhenWelcomeShows(t *testing.T) {
	stubDetect(t, []config.Profile{{Name: "claude", Program: "claude"}})
	h := newCreateFormHome(t)
	h.program = "definitely-not-a-real-binary-xyzzy" // would warn, but welcome shows

	h.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	require.Equal(t, stateWelcome, h.state, "first run shows the welcome")
	require.False(t, h.pathWarned, "the standalone warning is suppressed while the welcome shows")
}
```

- [ ] **Step 2: Run tests to verify they fail (or pass)**

Run: `$GO test ./app/ -run TestWarn_ -v`
Expected: These exercise `maybeWarnMissingProgram` wired in Task 3. If Task 3 landed correctly they PASS; if `pathWarned`/wiring is wrong they FAIL, pointing at the gap. Fix wiring in `app/app_welcome.go` / `maybeShowWelcome` until green. (This is the guard: the behavior is only "done" when these pass.)

- [ ] **Step 3: If failing, correct the wiring**

Confirm `maybeShowWelcome`'s returning-user branch returns `m.maybeWarnMissingProgram()` (Task 3, Step 5) and that `maybeWarnMissingProgram` sets `m.pathWarned = true` before returning (Task 3, Step 4). No new code should be needed; adjust only if a test fails.

- [ ] **Step 4: Run tests to verify they pass**

Run: `$GO test ./app/ -run TestWarn_ -v`
Expected: PASS (all three).

- [ ] **Step 5: Format, vet, full test, race, commit**

```bash
$GOFMT -l app/app_welcome_test.go   # expect empty
GO=$GO $JUST vet && GO=$GO $JUST test
GO=$GO $JUST test-race    # scheduleNoticeHide spawns a timer goroutine; confirm race-clean
git add app/app_welcome_test.go
git commit -m "$(printf 'test(app): cover the always-on missing-program warning\n\nGuard maybeWarnMissingProgram: a returning user (welcome seen-bit set)\nwith an uninstalled default program gets the one-shot warning; an\ninstalled program does not; and the standalone warning is suppressed on a\nlaunch where the welcome shows. lint CI-verified.\n\nCo-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>')"
```

---

## Final verification (after all tasks)

```bash
GO=/home/zvi/.local/share/mise/installs/go/1.26.4/bin/go
JUST=/home/zvi/.local/share/mise/installs/just/1.25.2/just
GO=$GO $JUST build
GO=$GO $JUST test
GO=$GO $JUST vet
GO=$GO $JUST test-race   # app (timer goroutine) + session/tmux
/home/zvi/.local/share/mise/installs/go/1.26.4/bin/gofmt -l app/ ui/overlay/ config/   # expect empty
```
Then push the branch and open a PR off `main` (the maintainer opens/merges; note lint is CI-verified).

## Self-Review

**Spec coverage:**
- Interactive Welcome modal (WelcomeOverlay, reused ProfilePicker + DetectAgentProfiles) → Task 2 + Task 3. ✓
- Async detection ("Detecting…" → populate) → Task 3 (`detectAgentsCmd`/`agentsDetectedMsg`; overlay detecting state in Task 2). ✓
- Confirm persists DefaultProgram + MergeDetectedProfiles + seen-bit; skip leaves seen-bit unset → Task 3 (`handleWelcomeState`) + tests. ✓
- `detectAgents` package-var seam → Task 3. ✓
- Empty-detection variant → Task 2 (render + test) + Task 3 (confirm with empty `SelectedProgram` is a no-op on config). ✓
- `config.ProgramInstalled` reusing detect.go resolution → Task 1. ✓
- Always-on one-shot warning for returning users, suppressed when welcome shows → Task 3 (`maybeWarnMissingProgram` + `maybeShowWelcome` branch) + Task 4 (tests). ✓
- Seen-bit "one new set-point" (confirm) with first-session set-point unchanged → Task 3 (only `handleWelcomeState` adds a set; `app_msgs.go` untouched). ✓
- Best-effort SaveConfig on confirm → Task 3 (`handleWelcomeState` logs + continues). ✓
- Hermetic tests → all tests use `newCreateFormHome`/sandboxed HOME + `stubDetect`. ✓

**Deliberate simplification vs spec:** the spec mentioned *preselecting* the current default in the picker. `ProfilePicker` has no select-by-value API, and adding one widens scope into a shared component; the picker instead defaults to the first detected agent (registry order → claude first), which is the right default for a fresh install. Noted here rather than silently dropped.

**Placeholder scan:** no TBD/TODO; every code step shows complete code; commands have expected output. ✓

**Type consistency:** `SetDetected([]config.Profile)` (single arg) is used identically in Task 2, Task 3 Step 6, and tests. `agentsDetectedMsg{profiles}`, `detectAgents` (`func() []config.Profile`), `handleWelcomeState(tea.KeyMsg) (tea.Model, tea.Cmd)`, `maybeShowWelcome() tea.Cmd`, `WelcomeOverlay` method set — all consistent across tasks. `SetDetected` takes no `currentDefault` (preselection dropped), matching Task 2's final signature. ✓
