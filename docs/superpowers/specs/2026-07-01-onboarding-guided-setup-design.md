# First-run guided setup — design

**Date:** 2026-07-01
**Status:** Approved (design); pending implementation plan
**Scope:** One focused iteration. Successor to the two merged refactoring waves.

## Context

Atrium is a terminal command-center for orchestrating AI coding agents (claude,
codex, gemini, aider). A brand-new user installs it, launches it, and today lands
on a **passive** first-run experience: a one-time Welcome modal that is pure text
("Run multiple agents… press n / ?") and configures nothing. The user is left on
defaults and must *discover* the settings panel (`,`) or hand-edit `config.json`.

The single most common way onboarding fails: **the configured default agent isn't
installed**, so the first session fails with a cryptic error rather than any
guidance. Detection already exists (`config.DetectAgentProfiles()`, the `profiles
detect` CLI command, the `doctor` CLI command) but is **CLI-only** — a first-run
user never sees it.

This iteration makes first-run **guided and agent-focused**: detect installed
agents, let the user pick the default program in the Welcome modal, and warn when
the chosen/configured program isn't on PATH — including a light always-on check so
returning users whose agent breaks are protected too.

### Decisions made with the user
- **Core problem:** first-run guided setup (not the profiles/accounts-in-TUI gap,
  not a general config-health tool — those stay out of scope).
- **Setup scope:** agent-focused and lean — detect + pick the default program +
  warn if missing + nudge to the first session. No theme/accounts steps.
- **Form:** evolve the existing one-time Welcome modal into an *interactive* one
  (not a full-screen wizard, not a passive banner).
- **Health check:** also add a *light always-on* "default program not on PATH"
  warning for returning users, not just first-run.
- **Build approach:** a new dedicated `WelcomeOverlay` component that reuses the
  existing `ProfilePicker` and `DetectAgentProfiles` (rather than bending the
  settings panel into a welcome, or a disjoint two-step flow).

### Non-goals (explicitly out of scope)
- Profiles / Claude & GH account management in the TUI (separate future iteration).
- A general in-TUI config-health / validation tool beyond the single
  program-on-PATH warning.
- Theme / Nerd-Font / any other config step in the welcome.
- Changing the CLI `profiles detect` / `doctor` / `debug` commands.

## Reused building blocks (no change needed)
- `config.DetectAgentProfiles() []Profile` — probes claude/codex/gemini/aider via
  `config/detect.go`, which already handles the shell-function/alias nuance (not
  naive `LookPath`). Returns `Profile{Name, Program}` per found agent.
- `config.(*Config).MergeDetectedProfiles(detected) []string` — appends
  newly-found profiles; never touches existing profiles or the default program.
- `overlay.ProfilePicker` (`ui/overlay/profilePicker.go`) — interactive picker:
  `NewProfilePicker(profiles)`, `Focus/Blur`, `SetWidth`, `HandleKeyPress`,
  `GetSelectedProfile`, `HasMultiple`.
- The existing Welcome trigger: `maybeShowWelcome()` (fired on the first
  `WindowSizeMsg`) and the seen-bit `helpTypeWelcome.mask()` (bit 4), set today on
  the first successful session start.
- The existing overlay render path (`overlay.PlaceOverlay` in `View()`) and notice
  path (`errBox` / hint bar / `handleError`).

## Design

### Component 1 — `WelcomeOverlay` (`ui/overlay/welcome.go`, new)
An interactive first-run overlay composing: the welcome copy, an embedded
`ProfilePicker` over the detected agents, and a one-line health/status line.

- `NewWelcomeOverlay(detected []config.Profile, currentDefault string) *WelcomeOverlay`
  — builds the copy + a `ProfilePicker` seeded from `detected`, preselecting
  `currentDefault` when it is among them. When `detected` is empty, no picker is
  built; an install hint is shown instead.
- `SetSize(w, h)` / `Render() string` — same conventions as the other overlays
  (fixed box, content clipped to fit; reuses `theme` panel styling).
- `HandleKeyPress(msg tea.KeyMsg) (done bool, confirmed bool)` — delegates
  navigation to the picker; **Enter** → `(done=true, confirmed=true)`; **Esc** →
  `(done=true, confirmed=false)`; other keys consumed by the picker.
- `SelectedProgram() string` — the chosen profile's `Program` (empty in the
  no-agents case).
- `SetDetecting(bool)` / a "Detecting installed agents…" placeholder state shown
  until detection returns.

Layout (populated state):
```
╭─ Welcome to Atrium ──────────────────────────────╮
│  Run multiple coding agents in parallel — each    │
│  in its own git worktree and tmux session.        │
│                                                    │
│  Choose your default agent:                        │
│    ● claude   (claude)                             │
│    ○ codex    (codex)                              │
│    ○ aider    (aider)                              │
│                                                    │
│  ✓ 3 agents detected on your PATH                  │
│  ↑/↓ choose · enter confirm · esc skip             │
╰────────────────────────────────────────────────────╯
```
Empty-detection variant replaces the picker + check line with:
`⚠ No supported agent CLIs found on PATH — install claude, codex, gemini, or aider (or press , later).` and hint `enter continue · esc skip`.

### Component 2 — App wiring (`app/`)
- New `stateWelcome` in the `state` enum (`app/app.go`); `View()` renders the
  `welcomeOverlay` via `PlaceOverlay` in that state (same branch shape as the
  other overlay states).
- `maybeShowWelcome()` opens a `WelcomeOverlay` in the "Detecting…" state, sets
  `state = stateWelcome`, and returns a `detectAgentsCmd` (async).
- `detectAgentsCmd` (a `tea.Cmd`) runs `detectAgents()` under a bounded deadline
  (a `doctor.ProbeTimeout`-style context) and returns `agentsDetectedMsg{profiles}`.
  Its `Update` handler populates the overlay's picker (or the empty-state variant)
  if the app is still in `stateWelcome`.
- `handleWelcomeState(msg tea.KeyMsg)` (`app/app_keys.go`, mirroring
  `handleConfirmState`): delegates to `welcomeOverlay.HandleKeyPress`; on
  **confirm** → apply (see flow), on **skip** → close without setting the seen-bit.
- Package-var seam `detectAgents = config.DetectAgentProfiles` so app tests stub
  detection (matches the existing func-var seam idiom, e.g. `worktreeCleanup`).

### Component 3 — `config.ProgramInstalled(program string) bool` (`config/`, new)
Resolves the program string's **first token** the same way `config/detect.go`
already resolves agent commands (handling shell functions / aliases), returning
whether it is runnable. Used by the always-on check. Placed next to the existing
detection code so it shares that resolution logic rather than re-implementing a
naive `LookPath` (which would false-negative on function-body agents — the exact
case detect.go documents).

## Flow

### First run (interactive welcome)
1. First `WindowSizeMsg` → `maybeShowWelcome()` (only when the seen-bit is unset):
   open overlay in "Detecting…" state, `state = stateWelcome`, fire
   `detectAgentsCmd`.
2. `agentsDetectedMsg` arrives → populate the picker (or empty-state).
3. User navigates and presses **Enter (confirm)**:
   - If a program was picked: `cfg.MergeDetectedProfiles(detected)` (found agents
     become profiles) and set `cfg.DefaultProgram = SelectedProgram()`; `SaveConfig`
     (best-effort — on failure, log a warning and keep the in-memory default for
     the run); update `m.program` for the run.
   - Set the welcome seen-bit.
   - Close the overlay (`state = stateDefault`).
4. **Esc (skip)** → close without setting the seen-bit → the welcome re-shows next
   launch (until the user confirms a program or creates a first session — today's
   behavior, preserved).

### Seen-bit semantics (one new set-point)
Enter/acknowledge sets `helpTypeWelcome`'s bit; Esc/skip never does. The existing
"set the bit on the first successful session" is unchanged. No other change.

### Always-on health check (returning users)
Once per launch, **only when the welcome is not shown this launch**: if
`!config.ProgramInstalled(m.program)`, surface one non-blocking notice via the
existing path — `⚠ '<program>' not found on PATH — press , to change`. Guarded by
a one-shot flag so it never re-fires per tick. First-run is covered by the welcome
(standalone warning suppressed); returning-and-broken is covered here.

## Error handling & edge cases
- **No agents detected / detection times out / errors:** empty-state variant;
  Enter acknowledges (sets seen-bit), Esc skips. The always-on warning becomes the
  ongoing reminder on later launches.
- **Shell-function / aliased agent:** resolved via detect.go's logic → no false
  "missing" in either the picker or `ProgramInstalled`.
- **`SaveConfig` failure on confirm:** best-effort — log a warning, keep the
  in-memory `DefaultProgram`/`m.program` for the run, still close. Matches the
  existing best-effort welcome-seen persist.
- **Explicit `-p` program flag:** the welcome still sets the persisted
  `DefaultProgram`; the flag remains a per-run override.
- **Tiny terminal:** the overlay clips to its box like the other overlays;
  `maybeShowWelcome` already runs only once the window size is known.
- **Daemon / multiple TUIs:** unaffected — the welcome is TUI-only and uses the
  existing `SaveConfig`.

## Testing
- `ui/overlay/welcome_test.go`: picker navigation, `SelectedProgram`, the
  empty-state rendering, and `HandleKeyPress` confirm-vs-skip return values.
- `config` test: `ProgramInstalled` true for a present bin, false for a bogus one
  (the shell-function nuance is already covered by detect.go's tests).
- `app` tests (hermetic HOME; stub `detectAgents`; construct via the `assembleHome`
  helper): first `WindowSizeMsg` enters `stateWelcome`; `agentsDetectedMsg`
  populates it; confirm applies `MergeDetectedProfiles` + `DefaultProgram` +
  seen-bit; skip leaves the seen-bit unset; the always-on warning fires only for a
  returning user with an uninstalled program and is suppressed when the welcome
  shows.

## Success criteria
- A first-run user with a supported agent installed reaches a working first session
  without editing any file: the welcome detects it, they pick it, it persists.
- A first-run user with **no** supported agent installed is told so explicitly
  (rather than discovering it via a failed session).
- A returning user whose default program is no longer on PATH sees one clear,
  actionable warning at launch.
- All existing tests stay green; new behavior is covered by the tests above.
