# Account configuration in the TUI — design

**Date:** 2026-07-02
**Status:** Approved (design); pending implementation plan
**Scope:** One focused iteration. Successor to the merged onboarding feature (#260),
which deferred exactly this: *"Profiles / Claude & GH account management in the TUI
(separate future iteration)."*

## Context

Atrium already has a complete **account backend**: `config.ClaudeAccount` and
`config.GHAccount` models (`config/types.go`), routing (`ResolveClaudeAccount` /
`ResolveGHAccount` / `matchRouteIndex` in `config/accounts.go`), per-session env
injection (`CLAUDE_CONFIG_DIR`, `GH_CONFIG_DIR`, and the gh token under each
`TokenEnv` name — `session/tmux/tmux.go`), and per-session persistence
(`session/storage.go`). But there is **no UI for any of it**: accounts can only be
created or edited by hand-editing `config.json`.

This iteration adds an in-TUI **Accounts manager** so users manage Claude and GitHub
accounts without touching `config.json`. It is **UI-over-existing-model** — it adds no
new backend concepts and requires **no `config/` changes**. The one place it goes
beyond raw CRUD is making **GitHub routing legible**: GH has no picker and its
injection is silent, so this overlay is the only surface where a user can reason about
which account a given repo resolves to.

### Decisions made with the user
- **Which accounts:** **both** Claude and GitHub, in one overlay with two tabs (their
  field sets differ — GH adds `TokenEnv`).
- **Depth:** CRUD over the existing model **plus** routing legibility (badges, warnings,
  a routing preview). Git commit identity is out.
- **How accounts are added:** **manual entry**; the existing `DirectoryPicker` assists
  the Config-dir field. No auto-detection of existing logins.
- **Entry point:** a new top-level **`@`** key (verified free), mirroring `,`=settings.
- **Build approach:** a dedicated `AccountsOverlay` templated on `SettingsOverlay`
  (self-capped modal, in-place `*config.Config` mutation, best-effort `SaveConfig`),
  with a per-account edit sub-form (`RenameOverlay.applyFocus` generalized to N fields —
  not the create-form `focusRing`, which is coupled to concrete create-form stops).

### Non-goals (explicitly out of scope)
- **Git commit identity** (`user.name`/`user.email`) — unmodeled today; needs new model
  fields *and* new per-session injection. Deferred to a follow-up.
- **Auto-detection / enumeration** of existing Claude/GH logins (manual entry only).
- **A GH override picker in new-session** — gh deliberately routes from the repo, not
  from the picked Claude login (an intentional existing design).
- **Auth pre-flight** beyond a cheap, non-blocking "config dir exists" hint.
- **Reordering accounts** — see Known limitations.

## Reused building blocks (no change needed unless noted)
- The account model + helpers: `config.ClaudeAccount` / `config.GHAccount` with
  `ResolvedConfigDir()` (expands `~`) and `IsCatchAll()` (`config/types.go`).
- Routing: `(*Config).ResolveClaudeAccount(remote, path)` and `ResolveGHAccount(remote,
  path)` and `matchRouteIndex` (`config/accounts.go`) — reused live for the badge and
  the routing preview.
- `overlay.SettingsOverlay` (`ui/overlay/settings.go`) — the schema-driven modal
  template: self-capped `boxWidth`, `SetSize(w,h)`, `HandleKeyPress → (closed,
  changedKey)`, inline `textinput` editor, windowed rows, the `carry_files` comma-split
  idiom (`:247-254`), and the stay-in-edit-on-error validation pattern (`:429-433`).
- `overlay.RenameOverlay` (`ui/overlay/renameOverlay.go`) — `applyFocus()` multi-field
  focus, generalized here to N fields over a `[]textinput.Model`.
- `overlay.DirectoryPicker` (`ui/overlay/directoryPicker.go`) — reused as a transient
  Config-dir browse sub-mode (**one small enhancement**, below).
- `overlay.PlaceOverlay` centering/fade; `ui/overlay/styles.go` helpers +
  `theme.Current().{AccentStyle,DimStyle,DangerStyle,OverlayHintStyle,OverlayTitleStyle}()`.
- Persistence: `config.SaveConfig(*Config)` (`config/persist.go`), best-effort + logged,
  exactly as `handleWelcomeState` / `applySettingChange` use it.
- The new-overlay wiring pattern set by `stateSettings` (keys → state/field/View →
  update dispatch/construct → handler → layout → help).

## Design

### Component 1 — `AccountsOverlay` (`ui/overlay/accounts.go`, new)
A keyboard-only modal with two **tabs** (Claude / GitHub) and a small **mode machine**.
It holds the **same `*config.Config` pointer** as `m.appConfig` and mutates its
`ClaudeAccounts`/`GHAccounts` slices in place; the app persists on change.
`NewAccountsOverlay(cfg)` seeds a default 80×24 so `Render()` works before the first
`SetSize` and in app tests where `windowWidth==0` (mirrors `NewSettingsOverlay`).

`HandleKeyPress(msg tea.KeyMsg) (closed bool, dirty bool)` — a boolean variant of
Settings' `(closed, changedKey)`; accounts need no per-field live-apply hook, only a
"persist now" signal. `dirty` is true only on an edit commit or a delete.

Modes (the list `cursor` is **always clamped** to the active tab's length on tab switch
and after delete — the tabs differ in length; a per-tab cursor may be kept to preserve
position):
- `modeList` — one row per account: `name · abbreviated ConfigDir · badge`. Keys:
  `↑/↓` move, `tab`/`←/→` switch tab, `n` new, `enter`/`e` edit, `d` delete, `t` test
  (routing preview), `esc`/`ctrl+c` close. `e`/`d`/`enter` **no-op on an empty tab**
  (guards an index-out-of-range panic). Empty tab → dim placeholder
  "No Claude accounts — press n to add".
- `modeEdit` — the `accountForm` sub-form (Component 2). `enter` validates + commits.
- `modeConfirmDelete` — in-overlay danger-styled `Delete 'NAME'? y/n` (NOT the app's
  `stateConfirm`, which would make the overlay vanish; mirrors Settings' internal
  `editing` sub-mode). Deletes by index; clamps `cursor` after removal.
- `modePreview` — two optional inputs (Remote URL, Path) that live-resolve via the
  routers and show the winner. **Render rules** (avoid blank-name / synthetic-default
  footguns): Claude `name==""` → "(no Claude accounts — inherits ambient)";
  `isDefault && dir==""` → "inherit ambient"; a real account → `name (ConfigDir)`. GH
  `dir==""` → "inherit ambient", else `ConfigDir [tokenEnv…]`. Optionally seed the Path
  field from the selected session's repo path.

**`esc`/`ctrl+c` precedence (layered, mirrors Settings' inline editor `:437-440`):** in
the picker sub-mode → close the picker (→ form); in `modeEdit` with no picker → discard
the form (→ list); in `modePreview`/`modeConfirmDelete` → return to list; in `modeList`
→ close the overlay.

**Badge:** `IsCatchAll()==false` → "routed" (accent), else "default" (dim); track a
seen-catch-all flag so any *second* rule-less account renders "catch-all (unreachable)"
(only the first rule-less account is the fallback in `matchRouteIndex`). Per tab, when
no catch-all exists, show a dim note "unmatched repos inherit the ambient account".

Layout (Claude tab, list mode):
```
╭─ Accounts ───────────────────────────────────────╮
│  ‹Claude›  GitHub                                 │
│                                                    │
│  › work       ~/.claude-work            routed     │
│    personal   ~/.claude                 default    │
│                                                    │
│  ↑/↓ move · tab switch · n new · e edit · d delete │
│  t test routing · esc close                        │
╰────────────────────────────────────────────────────╯
```

### Component 2 — `accountForm` (`ui/overlay/accountForm.go`, new)
A `[]textinput.Model` navigated by a `focus int` (`applyFocus` generalized to N fields).
Fields: Name, Config dir, Remote match, Path match, and Token env (GH tab only —
structurally omitted from the slice on the Claude tab, so nav/render/commit key off
`len(inputs)`). List fields carry a "comma-separated, e.g. github.com/acme" placeholder.

- **List parse/render:** render as `strings.Join(list, ", ")`; parse with the
  `carry_files` idiom (split on `,`, `TrimSpace`, **drop blanks**), collapsing empty →
  `nil` (fields are `omitempty`). A `" "` token must be dropped, not stored (it would
  `strings.Contains`-match any path with a space).
- **Config dir:** a typed field (so `~/.claude-work` can be typed, preserving `~`); a
  cached `os.Stat` on the resolved dir (keyed by the resolved-path string; stat only on
  change) shows a dim `(exists)` / danger `(not found)` hint when non-empty. Never blocks.
- **Commit (in the overlay, after submit):** build a **fresh** account with fresh slices
  and replace the whole element (`editIndex<0` → append; else replace at `editIndex`) so
  `esc` truly discards and the live config is never mutated mid-edit. Validate first
  (stay-in-edit-on-error): reject empty Name and duplicate Name within the active tab.
  ConfigDir may be empty (a valid "inject nothing" account) — rendered "(inherit ambient
  env)", never a silent blank.

### Component 3 — `DirectoryPicker` label (`ui/overlay/directoryPicker.go`, small change)
`ctrl+o` on the Config-dir field opens `DirectoryPicker` as a **transient sub-mode**
(`enter` writes `GetSelectedPath()` back and closes the sub-mode; `tab` =
`CompletePrefix()`; `esc` closes the sub-mode → form). `SetSelectionState` is **not**
called (its git-repo hints are irrelevant and stay suppressed while
`validityChecked==false`).

`Render()` currently hardcodes the label at two sites with different formatting —
`"Project: "` (`:432`) and `"Project"` (`:444`). Add a `label` field defaulting to
`"Project"` + a `SetLabel(string)` setter; render `dp.label + ": "` and `dp.label`
respectively (this preserves the existing `directoryPicker_test.go:111` `"Project:"`
assertion). The accounts form calls `SetLabel("Config dir")`; the create-form call site
is unchanged. (`GetSelectedPath` returns an absolute path — functionally equivalent
since `ResolvedConfigDir` expands `~`; optionally collapse home→`~` on write-back.)

### Component 4 — App wiring (`app/`)
Mirror the `stateSettings` wiring exactly (all anchors verified):
- `keys/keys.go` — `KeyAccounts` const after `KeySettings` (`:86`); `"@": KeyAccounts`
  in `GlobalKeyStringsMap` (`:166`); binding in `GlobalKeyBindings` (`:346`).
- `app/app.go` — `stateAccounts` after `stateWelcome` (`:130`); `accountsOverlay
  *overlay.AccountsOverlay` field; a `View()` branch (the `:405-415` chain).
- `app/app_update.go` — early return `if m.state == stateAccounts { return
  m.handleAccountsState(msg) }` placed after the `stateSettings` block (`:385-387`,
  before the global `esc`/`q`/`ctrl+c` handling); `case keys.KeyAccounts:` in
  `switch name` that sets the state, builds the overlay, calls `m.recomputeLayout()` and
  `return m, tea.WindowSize()` (the round-trip drives the overlay's first `SetSize`).
- **New `app/app_accounts.go`** — `handleAccountsState` mirroring `handleSettingsState`:
  `closed, dirty := overlay.HandleKeyPress(msg)`; if `dirty`, best-effort
  `config.SaveConfig(m.appConfig)` (log on failure); if `closed`, nil the overlay,
  `state = stateDefault`, `recomputeLayout()`, `tea.WindowSize()`.
- `app/app_layout.go` — `m.accountsOverlay.SetSize(msg.Width, msg.Height)` in the
  window-size handler (sibling to settings, `:61-65`); add `stateAccounts` to the
  `menuVisible()` false-list (`:97`). The overlay self-caps width — no new helper.
- `app/help.go` — `helpRow("@", "accounts (Claude / GitHub)")` in the "Other" section.

## Flow

- **Open:** `@` → `stateAccounts`, overlay built over `m.appConfig`, Claude tab, list mode.
- **Add:** `n` → empty form for the active tab → fill fields (Config dir via `ctrl+o` or
  typing) → `enter` validates + appends + `SaveConfig` → back to list.
- **Edit:** `enter`/`e` on a row → form seeded from that account (by index) → `enter`
  commits a fresh replacement → `SaveConfig`.
- **Delete:** `d` → `Delete 'NAME'? y/n` → `y` removes by index, clamps cursor,
  `SaveConfig`.
- **Preview:** `t` → type a Remote URL and/or Path → live resolution of the winning
  Claude & GH account (read-only) → `esc` back to list.
- **Close:** `esc`/`ctrl+c` from list → nil overlay, back to `stateDefault`.

No live-apply is needed beyond `SaveConfig`: every consumer reads `m.appConfig` live at
its point of use (new sessions re-resolve at creation; the create overlay reads
`ClaudeAccounts` when opened; the list badge re-resolves each poll). Running sessions
keep their already-injected env by design (they read persisted per-session state and
never re-resolve — `session/instance.go` `FromInstanceData`/`restart`).

## Error handling & edge cases
- **Existing sessions are immune** to account edits/deletes (persisted per-session env,
  no re-resolution) — no dangling reference, no crash on reattach/restart.
- **Stale create-form draft:** on account commit/delete, set `m.stashedDraft = nil`
  (keep the disk draft) so a reopened create form rebuilds from live config and can't
  pin a just-deleted account.
- **Cursor safety:** clamp to the active tab's length on tab switch and after delete;
  edit/delete/enter no-op on an empty tab (prevents an out-of-range panic across the
  differently-sized tabs).
- **Blank/whitespace route tokens** trimmed + dropped on parse.
- **Empty ConfigDir** allowed and rendered "(inherit ambient env)"; for a GH account
  with `TokenEnv` set and ConfigDir empty, the token resolves from the ambient gh
  account — surfaced by the preview/badge rather than hidden.
- **Second rule-less account** marked "catch-all (unreachable)".
- **`SaveConfig` failure:** best-effort — logged; in-memory config is ahead of disk for
  the run (same property as Settings). Acceptable.
- **Tiny terminal:** the overlay self-caps and windows its rows like Settings.
- **Daemon / multiple TUIs:** unaffected — the manager is TUI-only, mutually exclusive
  with the daemon via the existing flock; the daemon reads state, not config accounts.

## Testing
Hermetic (`$HOME` sandboxed via the existing `TestMain`).
- **Overlay** (`package overlay`, no terminal; mirror `accountPicker_test.go`):
  nav/mode/tab transitions incl. `←/→` **and the empty/shorter-tab delete → no panic**;
  add/edit/delete mutate config only after save; comma-split round-trip incl. a `" "`
  token dropped; validation (empty/dup Name rejected → stays in edit; 2nd catch-all
  flagged; empty ConfigDir renders "(inherit ambient env)"); cancel-discards (mutate a
  rule list, `esc`, `m.appConfig` and `LoadConfig()` byte-identical); preview for a
  match, a no-match-with-catch-all, and **0-accounts/rule-only → both routers "inherit
  ambient", no blank name**; exists-hint cache (TempDir vs bogus path).
- **DirectoryPicker:** `SetLabel("Config dir")` renders "Config dir:"; the default still
  renders "Project:".
- **Config round-trip** (`config/config_test.go`): SaveConfig/LoadConfig a Config with
  both account kinds incl. all rule slices + TokenEnv survives; pure catch-all reloads
  `IsCatchAll()==true`; empty ConfigDir keeps `"config_dir":""`.
- **App** (mirror `newSettingsTestHome`): `@` opens `stateAccounts` + builds overlay;
  `menuVisible()==false`; a save reaches disk; `esc` tears down to `stateDefault`.
  Existing-session immunity (delete a referenced account → `ToInstanceData` →
  `FromInstanceData` keeps pinned env, no panic). Stash-staleness (create form stashed →
  edit accounts → reopen reflects the new set). Add a `{stateAccounts}` row to
  `TestStateMachine_BackgroundMessagesNeverPanic`.
- **Seams:** construct over `*config.Config` like `NewSettingsOverlay(cfg)`; expose
  tab/index selection + a working-copy/`lastErr` accessor so tests drive `HandleKeyPress`
  and assert without rendering; set ConfigDir via the textinput directly to bypass the
  real-FS picker.

## Success criteria
- A user can add, edit, and delete both Claude and GitHub accounts from inside the TUI
  (`@`) — including route rules and GH `TokenEnv` — and the changes persist to
  `config.json` with no hand-editing.
- The overlay makes routing legible: catch-all badges, an "unreachable" warning for a
  dead second catch-all, "(inherit ambient env)" for empty dirs, and a routing preview
  that shows which Claude & GH account a given remote/path resolves to.
- No running or persisted session is disturbed by account edits.
- All existing tests stay green; the new behavior is covered by the tests above;
  `just build`, `just test`, `just vet` pass.

## Known limitations / future work
- **No reordering.** Routing is first-match-in-config-order and `n` appends, so a new
  specific rule can land after a broader existing one and be shadowed. The routing
  preview + the "unreachable" badge make this visible; the workaround is delete+re-add in
  order. Reordering (move up/down) is the natural fast-follow.
- Git commit identity, login auto-detection, and a GH new-session picker remain future
  iterations.
