# Atrium — project guide

Atrium is a terminal command center for orchestrating multiple AI coding agents
(Claude Code, Codex, Gemini, Aider, …). Each session runs in its own tmux session
on a dedicated socket, inside an isolated git worktree, so many agents work in
parallel without conflicts. It is a Go TUI built on Bubble Tea, with a Cobra CLI
entrypoint in `main.go`.

Module path: `github.com/ZviBaratz/atrium`. Binary: `atrium` (alias `atr`).

## Commands (use `just`)

All development tasks go through the `justfile` — discover them with `just --list`.

| Task | Command |
|------|---------|
| Build (stamps version from git) | `just build` → `./bin/atrium` |
| Run | `just run -- <args>` |
| Test (hermetic — safe anywhere) | `just test` |
| Test with race detector | `just test-race` |
| Coverage | `just cover` |
| Lint | `just lint` (golangci-lint) |
| Format | `just fmt` / check with `just fmt-check` |
| Vet | `just vet` |
| Local release snapshot | `just snapshot` (GoReleaser) |
| Tag a release | `just release <X.Y.Z>` |

**Toolchain note:** if `go` is not on `PATH`, pass it explicitly:
`GO=/path/to/go just test`. CI uses `go-version-file: go.mod`.

## Verifying your work

Always confirm a change with `just build` **and** `just test` before claiming it
works. `just test` is the source of truth for correctness; `just build` proves the
binary still compiles and version-stamps.

One known-flaky test, `TestSessionDeathStopsProbing` (package `session/tmux`),
drives a real tmux server and can fail in constrained/nested environments — it is
unrelated to most changes. Investigate only if your change touches tmux session
lifecycle; otherwise skip it with `-skip TestSessionDeathStopsProbing`.

## Conventions

- **Commits:** Conventional Commits, lowercase (`feat: …`, `fix: …`).
- **Versioning:** the git tag is the single source of truth. `main.go`'s `version`
  defaults to `dev` and is injected via `-ldflags` at build/release time — never
  hand-edit a version string.
- **License:** AGPL-3.0 (mandatory — Atrium is a derivative of
  [claude-squad](https://github.com/smtg-ai/claude-squad); see `NOTICE`).

## Identity & live-state safety (important)

There are three identity layers. The first two are pure renames; the third is
state-bearing and must never be migrated in place:

- **Module / imports:** `github.com/ZviBaratz/atrium/...`.
- **Brand:** binary name, URLs, docs.
- **Runtime identifiers (live state):** the data dir and the tmux socket. Resolved
  by one function, `config.RuntimeName()`, which returns `atrium` for fresh
  installs and the legacy `claudesquad` when only `~/.claude-squad` exists. From it
  derive the data dir (`~/.atrium` vs `~/.claude-squad`), the tmux socket, the
  session-name prefix (`TmuxPrefix()`), and the managed conf filename.

`config.GetConfigDir()` implements **prefer-new, fall back to legacy, never move**:
it picks `~/.atrium` if present, else an existing `~/.claude-squad` (untouched),
else defaults to `~/.atrium`. This matters because the data dir contains the
`worktrees/` tree and a `state.json` of **absolute** paths, and git records each
worktree's absolute path in the project repo's `.git/worktrees/<name>/gitdir` —
moving the dir would orphan live sessions. When adding anything that names the
data dir or the tmux socket, derive it from `config.RuntimeName()`; do not
hardcode either name.

## Tests must stay hermetic

Tests must never read or write the user's real data dir. Packages that resolve the
config dir, save state, or touch tmux set `HOME` to a temp dir (see
`config/config_test.go` and `app/app_test.go` `TestMain`). Any new test that can
reach `config`/`state`/`tmux` writes must do the same.
