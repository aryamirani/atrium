# Info/Blocked-Action Notice Fallback Surface — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Surface transient info/blocked-action notices on the errBox fallback row when the hint bar is hidden, instead of silently dropping them (#287).

**Architecture:** Teach `ui.ErrBox` to carry a `NoticeLevel` (info→neutral, error→red) so info can ride the same transient bottom row errors already fall back to. Gate the row on "has content" rather than "has error". Route `handleInfoNotice`, `handleError`, and `warnMissingProgram` through one shared `flashNotice(text, level)` helper.

**Tech Stack:** Go, Bubble Tea, lipgloss; `just` task runner; testify + Go stdlib testing.

## Global Constraints

- Module path: `github.com/ZviBaratz/atrium`. Commands run via `just` (build/test); if `go` is not on PATH pass `GO=/path/to/go just ...`.
- Conventional Commits, lowercase (`feat:`, `fix:`, `refactor:`, `test:`, `docs:`).
- `NoticeLevel` (`NoticeInfo`, `NoticeError`) is already defined in package `ui` (`ui/menu.go`) — reuse it; do not redefine.
- Tests must stay hermetic: never touch the user's real data dir. The `app` package's `TestMain` already sets `HOME` to a temp dir — new `app` tests need no extra setup. `ui` tests construct components directly and touch no config.
- Verify every change with `just build` **and** `just test` before claiming completion.
- The persistent `↦` queued/delivered glyph is out of scope — do not touch it.

---

### Task 1: `ErrBox` carries a severity level

Make `ErrBox` a generic single-row transient banner holding `(text, level)` instead of an `error`, styled neutral for info and red for error, while keeping `SetError`/`Fits`/`HasError`/`Clear`/`String` behavior for existing callers.

**Files:**
- Modify: `ui/err.go`
- Test: `ui/err_test.go`

**Interfaces:**
- Consumes: `NoticeLevel` (`NoticeInfo`, `NoticeError`) and `theme.Current().FgStyle()` / `theme.Current().DangerStyle()`, both already in package `ui` scope.
- Produces:
  - `func (e *ErrBox) SetError(err error)` — unchanged signature; sets `text = err.Error()`, `level = NoticeError`.
  - `func (e *ErrBox) SetNotice(text string, level NoticeLevel)` — new.
  - `func (e *ErrBox) HasError() bool` — `text != "" && level == NoticeError`.
  - `func (e *ErrBox) HasContent() bool` — new; `text != ""`.
  - `func (e *ErrBox) Clear()` — resets `text=""`, `level=NoticeInfo`.
  - `func (e *ErrBox) Fits(err error) bool` — unchanged.
  - `func (e *ErrBox) String() string` — neutral `FgStyle` for info, `DangerStyle` for error.

- [ ] **Step 1: Write the failing tests**

Add these to `ui/err_test.go`:

```go
func TestErrBox_SetNotice_Info(t *testing.T) {
	e := NewErrBox()
	e.SetSize(80, 1)
	e.SetNotice("branch copied", NoticeInfo)

	if !e.HasContent() {
		t.Fatal("HasContent should be true after SetNotice")
	}
	if e.HasError() {
		t.Fatal("an info notice must not report HasError")
	}
	if got := e.String(); !strings.Contains(got, "branch copied") {
		t.Errorf("String() = %q, expected to contain notice text", got)
	}
}

func TestErrBox_SetNotice_ErrorLevelReportsHasError(t *testing.T) {
	e := NewErrBox()
	e.SetNotice("boom", NoticeError)
	if !e.HasError() {
		t.Fatal("an error-level notice must report HasError")
	}
}

func TestErrBox_InfoAndErrorStyleDiffer(t *testing.T) {
	info := NewErrBox()
	info.SetSize(80, 1)
	info.SetNotice("same text", NoticeInfo)

	err := NewErrBox()
	err.SetSize(80, 1)
	err.SetError(errors.New("same text"))

	if info.String() == err.String() {
		t.Error("info (neutral) and error (danger) must render with different styling")
	}
}

func TestErrBox_ClearResetsContent(t *testing.T) {
	e := NewErrBox()
	e.SetNotice("branch copied", NoticeInfo)
	e.Clear()
	if e.HasContent() {
		t.Fatal("HasContent should be false after Clear")
	}
	if got := e.String(); got != "" {
		t.Errorf("String() after Clear = %q, want empty", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GO=$(mise which go 2>/dev/null || echo go) just test 2>&1 | tail -20` (or `just test` if `go` is on PATH)
Expected: FAIL — `e.SetNotice undefined`, `e.HasContent undefined`.

- [ ] **Step 3: Rewrite `ui/err.go`**

Replace the whole file with:

```go
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

// HasError reports whether an error-level notice is showing. handleError's
// Fits-routing and error-only callers use this; an info notice riding the box
// reports false so it never looks like an error.
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
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `just test 2>&1 | tail -20` (adjust `GO=` prefix if needed)
Expected: PASS — the four new tests plus the pre-existing `TestErrBox_Fits`, `TestErrBox_HasError`, `TestErrBox_String_*` all green.

- [ ] **Step 5: Commit**

```bash
git add ui/err.go ui/err_test.go
git commit -m "refactor(ui): ErrBox carries a NoticeLevel so info can ride its row (#287)"
```

---

### Task 2: Gate the reserved row on `HasContent`

The layout and view currently allot the errBox row only when `HasError()`. An info notice sets content but reports `HasError()==false`, so switch both gates to `HasContent()`.

**Files:**
- Modify: `app/app_layout.go` (~line 41)
- Modify: `app/app.go` (View, ~line 462)

**Interfaces:**
- Consumes: `ErrBox.HasContent()` from Task 1.

- [ ] **Step 1: Update the layout gate**

In `app/app_layout.go`, inside `updateHandleWindowSizeEvent`, change:

```go
	errHeight := 0
	if m.errBox.HasError() {
		errHeight = 1
	}
```

to:

```go
	errHeight := 0
	if m.errBox.HasContent() {
		errHeight = 1
	}
```

- [ ] **Step 2: Update the View gate**

In `app/app.go`'s `View()`, change:

```go
	if m.errBox.HasError() {
		parts = append(parts, m.errBox.String())
	}
```

to:

```go
	if m.errBox.HasContent() {
		parts = append(parts, m.errBox.String())
	}
```

- [ ] **Step 3: Verify it still builds and tests pass**

Run: `just build && just test 2>&1 | tail -20`
Expected: build succeeds; all tests pass (no behavior change yet — nothing routes info to the errBox until Task 3).

- [ ] **Step 4: Commit**

```bash
git add app/app_layout.go app/app.go
git commit -m "refactor(app): gate the errBox row on HasContent, not HasError (#287)"
```

---

### Task 3: Shared `flashNotice` helper + route info through it

Extract the menu-or-errBox fallback into one helper and route `handleInfoNotice` and `handleError` through it, so an info notice falls back to the errBox row when the hint bar is off.

**Files:**
- Modify: `app/app_feedback.go` (`handleError` ~line 40, `handleInfoNotice` ~line 80)
- Test: `app/notice_test.go`

**Interfaces:**
- Consumes: `ErrBox.SetNotice` (Task 1); `NoticeInfo`/`NoticeError` from package `ui`; existing `m.menuVisible()`, `m.menu.SetNotice`, `m.recomputeLayout()`, `m.scheduleNoticeHide()`.
- Produces: `func (m *home) flashNotice(text string, level ui.NoticeLevel) tea.Cmd` — used here and by Task 4.

- [ ] **Step 1: Rewrite the failing test**

In `app/notice_test.go`, replace `TestHandleInfoNotice_HintBarOffDropsIt` (the whole func, lines ~54-66) with:

```go
// Info acknowledgments used to be dropped with the hint bar off (#287). They now
// fall back to the errBox row — shown, not silently discarded — but styled
// neutrally so they never read as an error.
func TestHandleInfoNotice_HintBarOffFallsBackToErrRow(t *testing.T) {
	h := newCreateFormHome(t)
	off := false
	h.appConfig.HintBar = &off

	cmd := h.handleInfoNotice("branch copied")

	require.NotNil(t, cmd, "a fallen-back info notice still schedules its own hide")
	assert.True(t, h.errBox.HasContent(), "the notice must claim the errBox row")
	assert.False(t, h.errBox.HasError(), "info must never look like an error")
	assert.False(t, h.menu.HasNotice(), "the hidden hint bar carries nothing")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `just test 2>&1 | tail -20`
Expected: FAIL — `handleInfoNotice` returns `nil` with the hint bar off, so `cmd` is nil and `errBox.HasContent()` is false.

- [ ] **Step 3: Add `flashNotice` and route both callers through it**

In `app/app_feedback.go`, add the helper (place it just above `handleInfoNotice`):

```go
// flashNotice shows a transient toast on the hint bar's reserved row when the
// bar is visible, else on the errBox's fallback row, styled by level. The toast
// auto-hides after errToastDuration via scheduleNoticeHide. It is the single
// chokepoint for menu-or-errBox fallback shared by handleError,
// handleInfoNotice, and warnMissingProgram.
func (m *home) flashNotice(text string, level ui.NoticeLevel) tea.Cmd {
	if m.menuVisible() && m.menu != nil {
		m.menu.SetNotice(text, level)
	} else {
		m.errBox.SetNotice(text, level)
		m.recomputeLayout() // give the notice its row; panes shrink by one
	}
	return m.scheduleNoticeHide()
}
```

Rewrite `handleError` to keep its Fits-routing and logging but delegate the fallback:

```go
func (m *home) handleError(err error) tea.Cmd {
	if m.state == stateDefault && !m.errBox.Fits(err) {
		return m.showInfo(err.Error()) // showInfo logs the message itself
	}
	log.ErrorLog.Printf("%v", err)
	return m.flashNotice(err.Error(), ui.NoticeError)
}
```

Rewrite `handleInfoNotice`:

```go
// handleInfoNotice flashes a neutral acknowledgment ("branch copied"). When the
// hint bar is up it rides the bar's reserved row; when the bar is off it falls
// back to the errBox row (#287) rather than being dropped.
func (m *home) handleInfoNotice(text string) tea.Cmd {
	return m.flashNotice(text, ui.NoticeInfo)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `just test 2>&1 | tail -20`
Expected: PASS — `TestHandleInfoNotice_HintBarOffFallsBackToErrRow`, `TestHandleInfoNotice_MenuCarriesIt`, `TestHandleError_*`, and `TestHideNotice_StaleGenerationIgnored` all green.

- [ ] **Step 5: Commit**

```bash
git add app/app_feedback.go app/notice_test.go
git commit -m "fix(ux): fall back info notices to the errBox row when the hint bar is off (#287)"
```

---

### Task 4: Route `warnMissingProgram` through `flashNotice`

Collapse `warnMissingProgram`'s hand-rolled duplicate of the menu-or-errBox branch into the shared helper, and lock its hint-bar-off behavior with a test.

**Files:**
- Modify: `app/app_welcome.go` (`warnMissingProgram` ~lines 112-118)
- Test: `app/notice_test.go` (or `app/newsession_test.go` if a program-warning helper already lives there — prefer `notice_test.go` for cohesion)

**Interfaces:**
- Consumes: `flashNotice` (Task 3).

- [ ] **Step 1: Write the failing test**

Add to `app/notice_test.go`:

```go
// With the hint bar off, the missing-program warning must land on the errBox row
// rather than vanish — it goes through the same flashNotice fallback (#287).
func TestWarnMissingProgram_HintBarOffFallsBackToErrRow(t *testing.T) {
	h := newCreateFormHome(t)
	off := false
	h.appConfig.HintBar = &off

	cmd := h.warnMissingProgram("definitely-not-a-real-program")

	require.NotNil(t, cmd, "the warning schedules its own hide")
	assert.True(t, h.errBox.HasContent(), "the warning must claim the errBox row")
	assert.True(t, h.errBox.HasError(), "a missing-program warning is error-level")
	assert.False(t, h.menu.HasNotice())
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `just test 2>&1 | tail -20`
Expected: The test compiles and passes on content/HasError already (the old code path also sets the errBox), BUT it is written to pin behavior across the refactor. If it fails to compile because `warnMissingProgram`'s signature differs, adjust the call to match `app/app_welcome.go:101`. Confirm it passes against the current code before refactoring — this is the safety net.

> Note: unlike Tasks 1 and 3 this test is green against the pre-refactor code (both branches set the errBox). It exists to guarantee the Step-3 refactor is behavior-preserving. Run it before and after Step 3.

- [ ] **Step 3: Refactor `warnMissingProgram`**

In `app/app_welcome.go`, replace:

```go
	if m.menuVisible() && m.menu != nil {
		m.menu.SetNotice(text, ui.NoticeError)
	} else {
		m.errBox.SetError(fmt.Errorf("%s", text))
		m.recomputeLayout()
	}
	return m.scheduleNoticeHide()
```

with:

```go
	return m.flashNotice(text, ui.NoticeError)
```

Then remove the now-unused `fmt` import from `app/app_welcome.go` **only if** no other use of `fmt` remains in the file (check with `grep -n 'fmt\.' app/app_welcome.go` — the `fmt.Sprintf` at line ~108 for building `text` still uses it, so the import stays; do not remove it).

- [ ] **Step 4: Run tests to verify they pass**

Run: `just test 2>&1 | tail -20`
Expected: PASS — `TestWarnMissingProgram_HintBarOffFallsBackToErrRow` still green, behavior preserved.

- [ ] **Step 5: Commit**

```bash
git add app/app_welcome.go app/notice_test.go
git commit -m "refactor(app): route warnMissingProgram through flashNotice (#287)"
```

---

### Task 5: Full build + test sweep and final verification

Confirm the whole change compiles, the suite is green, and the fix behaves as designed.

**Files:** none (verification only).

- [ ] **Step 1: Full build**

Run: `just build`
Expected: `./bin/atrium` builds with no errors.

- [ ] **Step 2: Full test suite**

Run: `just test 2>&1 | tail -30`
Expected: all packages PASS (`ok  github.com/ZviBaratz/atrium/ui`, `.../app`, etc.). No `FAIL`.

- [ ] **Step 3: Grep for stale `HasError` layout gates**

Run: `grep -rn "errBox.HasError" app/`
Expected: only the two gates that were intentionally changed are gone; any remaining `HasError` uses are semantic error checks (there should be none left in `app/` except inside tests). If a production gate for the *row* still uses `HasError`, switch it to `HasContent`.

- [ ] **Step 4: Manual smoke (optional but recommended)**

Run the app with the hint bar off and trigger an info notice (e.g. copy a branch name, or press a guarded key like resume-with-no-paused-sessions), confirm a neutral one-line toast appears on the bottom row for ~5s and then clears without shifting other content permanently. Use the isolated-preview idiom (temp `HOME`) so real sessions are untouched.

- [ ] **Step 5: Final commit (if any manual tweaks were made; otherwise skip)**

```bash
git add -A
git commit -m "chore: final verification pass for notice fallback (#287)"
```

---

## Self-Review

**Spec coverage:**
- ErrBox severity (spec §1) → Task 1. ✓
- Layout/view `HasContent` gate (spec §2) → Task 2. ✓
- `flashNotice` helper + `handleInfoNotice`/`handleError` routing (spec §3) → Task 3. ✓
- `warnMissingProgram` refactor (spec §3) → Task 4. ✓
- `ui/err_test.go` coverage (spec Testing) → Task 1. ✓
- `notice_test.go` rewrite (spec Testing) → Task 3. ✓
- `warnMissingProgram` hint-bar-off test (spec Testing) → Task 4. ✓
- `just build` + `just test` (spec Verification) → Task 5. ✓
- Persistent `↦` glyph untouched (spec Non-goals) → no task touches it. ✓

**Type consistency:** `SetNotice(text string, level NoticeLevel)`, `HasContent()`, `HasError()`, `flashNotice(text string, level ui.NoticeLevel) tea.Cmd` are named identically everywhere they appear (Tasks 1, 3, 4). `NoticeLevel`/`NoticeInfo`/`NoticeError` reused from `ui/menu.go`, not redefined. ✓

**Placeholder scan:** No TBD/TODO; every code step shows complete code; test bodies are concrete. ✓
