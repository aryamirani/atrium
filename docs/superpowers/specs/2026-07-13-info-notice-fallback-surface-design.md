# Fallback surface for info/blocked-action notices — design

**Issue:** [#287](https://github.com/ZviBaratz/atrium/issues/287) — `fix(ux): info/blocked-action notices are silently dropped when the hint bar is hidden`
**Labels:** enhancement, P3, area:app, area:ux
**Date:** 2026-07-13

## Problem

`handleInfoNotice` (`app/app_feedback.go`) writes transient acknowledgments to the
hint bar's reserved row via `m.menu.SetNotice`. When the user runs with the hint
bar turned off (`hint_bar: false` in `config.json`), `menuVisible()` is false, so
the function returns `nil` early and the notice is **silently dropped**:

```go
func (m *home) handleInfoNotice(text string) tea.Cmd {
    if !m.menuVisible() || m.menu == nil {
        return nil // notice discarded — no reserved row to ride
    }
    m.menu.SetNotice(text, ui.NoticeInfo)
    return m.scheduleNoticeHide()
}
```

Every `handleInfoNotice` caller is affected: pure acknowledgments ("branch
copied", "pushed changes"), state guards routed as info ("no paused sessions to
resume", "session is paused — press r to resume before sending"), and the #269
quick-send toasts ("queued for X" / "delivered to X"). With the hint bar off, a
user gets no feedback that the action registered.

`handleError` already solved the same problem for errors: when the menu isn't
visible it falls back to the `errBox` transient row (`app/app_feedback.go:45-50`).
Info notices cannot reuse that surface as-is because `errBox` only knows how to
render an *error* — `String()` always applies `DangerStyle()` (red), and the
layout/view gate the row on `HasError()`.

This was explicitly listed as an out-of-scope follow-up in #269 ("Routing
blocked-action notices when the hint bar is off").

## Goal

Give transient info/blocked-action notices a fallback surface when the hint bar
is hidden, so a confirmation/guard message is shown rather than dropped. Reuse the
`errBox` transient row (the surface `handleError` already falls back to), rendered
neutrally so info never reads as an error. Keep it non-intrusive and
auto-clearing via the existing `scheduleNoticeHide` / `hideErrMsg` machinery.

## Non-goals

- The persistent row indicators (the `↦` queued/delivered glyph) stay as-is. They
  are additive; with the hint bar off, queued/delivered will now show both the
  transient toast and the persistent glyph — mirroring how they already behave
  with the hint bar on.
- No new configuration knob. The fallback is unconditional, matching `handleError`.
- No new overlay/toast component (considered and rejected: a second transient
  surface competing for the bottom row is more moving parts than reusing `errBox`).

## Design

### 1. `ui/err.go` — teach `ErrBox` a severity

`ErrBox` becomes a generic single-row transient banner carrying `(text, level)`,
reusing the `NoticeLevel` enum that already lives in the same `ui` package
(`menu.go`: `NoticeInfo`, `NoticeError`).

Field change: replace `err error` with `text string` + `level NoticeLevel`.

| Method | Behavior |
|--------|----------|
| `SetError(err error)` | `text = err.Error()`, `level = NoticeError`. Signature kept, so `handleError`'s `Fits`-routing and every existing caller compile unchanged. |
| `SetNotice(text string, level NoticeLevel)` | new — sets both fields. |
| `HasError() bool` | `text != "" && level == NoticeError`. An info notice riding the box reports `HasError() == false`, preserving the "info must never look like an error" invariant. |
| `HasContent() bool` | new — `text != ""`. Layout and view gate the reserved row on this. |
| `Clear()` | resets `text = ""`, `level = NoticeInfo` (zero value). |
| `Fits(err error) bool` | unchanged — still takes an error; used only by `handleError`. |
| `String() string` | `FgStyle()` (neutral) for `NoticeInfo`, `DangerStyle()` (red) for `NoticeError`; keeps the existing newline-flatten (`//`) + width truncate. Returns `""` when `text == ""`. |

### 2. Layout & view — gate the row on content, not error-ness

- `app/app_layout.go` (~line 41): `if m.errBox.HasError()` → `if m.errBox.HasContent()`.
- `app/app.go` View (~line 462): `if m.errBox.HasError()` → `if m.errBox.HasContent()`.

An info notice claims and releases the reserved row identically to an error.

### 3. `app/app_feedback.go` — one shared fallback helper

Extract the menu-or-errBox fallback (currently duplicated in `handleError` and
`warnMissingProgram`) into a single helper:

```go
// flashNotice shows a transient toast on the hint bar's reserved row when the
// bar is visible, else on the errBox's fallback row, styled by level. The toast
// auto-hides after errToastDuration via scheduleNoticeHide.
func (m *home) flashNotice(text string, level ui.NoticeLevel) tea.Cmd {
    if m.menuVisible() && m.menu != nil {
        m.menu.SetNotice(text, level)
    } else {
        m.errBox.SetNotice(text, level)
        m.recomputeLayout()
    }
    return m.scheduleNoticeHide()
}
```

Call sites:
- `handleInfoNotice` → `return m.flashNotice(text, ui.NoticeInfo)` (the early
  `return nil` is removed).
- `handleError` → keeps its `Fits`→`showInfo` pre-check (multi-line / over-wide
  errors still route to the persistent modal from `stateDefault`) and its
  `log.ErrorLog` line, then `return m.flashNotice(err.Error(), ui.NoticeError)`.
- `warnMissingProgram` (`app/app_welcome.go`) → collapses its hand-rolled
  duplicate branch into `return m.flashNotice(text, ui.NoticeError)`.

`hideErrMsg`'s handler (`app/app_update.go:52`) already clears both surfaces
(`menu.ClearNotice()` + `errBox.Clear()`) and recomputes layout — no change.

## Behavioral notes

- With the hint bar **on** (default), nothing changes: notices ride the menu row,
  the frame height never shifts.
- With the hint bar **off**, an info notice now transiently claims a row (frame
  grows by one line for ~5 s, then releases) — exactly how errors already behave
  in that mode. The "feedback never moves the layout" invariant only ever held
  with the hint bar on, which is unchanged.
- Errors continue to take visual priority: `HasError()` remains true only for
  error-level content, and an error set after an info notice overwrites it.

## Testing

**`ui/err_test.go`:**
- `SetNotice` sets content; `HasContent()` true, `HasError()` false for
  `NoticeInfo`; `HasError()` true for `NoticeError`.
- `String()` for info vs error differ (neutral vs danger styling) and both
  contain the text.
- Existing `Fits`, `HasError` (via `SetError`), and `String` flatten/truncate
  tests continue to pass unchanged.

**`app/notice_test.go`:**
- Rewrite `TestHandleInfoNotice_HintBarOffDropsIt` →
  `TestHandleInfoNotice_HintBarOffFallsBackToErrRow`: with `hint_bar` off, the
  returned cmd is non-nil, `errBox.HasContent()` is true, `errBox.HasError()` is
  false, `menu.HasNotice()` is false.
- Keep `TestHandleInfoNotice_MenuCarriesIt` (hint bar on → menu carries it,
  errBox empty).

**New `warnMissingProgram` coverage:** with the hint bar off, the missing-program
warning lands on the errBox row (locks the refactor of its duplicate branch).

**Verification:** `just build` and `just test` both pass.

## Files touched

- `ui/err.go` — severity-aware `ErrBox`.
- `ui/err_test.go` — new/updated unit tests.
- `app/app_feedback.go` — `flashNotice` helper; `handleInfoNotice`, `handleError`
  routed through it.
- `app/app_welcome.go` — `warnMissingProgram` routed through it.
- `app/app_layout.go` — `HasContent()` gate.
- `app/app.go` — `HasContent()` gate in View.
- `app/notice_test.go` — updated hint-bar-off info test.
