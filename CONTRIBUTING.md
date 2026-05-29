# Contributing

Thank you for considering contributing to our project! This document outlines the process for contributing.

## Development Setup

1. Fork the repository
2. Clone your fork: `git clone https://github.com/YOUR-USERNAME/atrium.git`
3. Add the upstream repository: `git remote add upstream https://github.com/ZviBaratz/atrium.git`
4. Install dependencies: `go mod download`

## Code Standards

Tasks are standardized through the [`justfile`](justfile) — run `just --list` to
see everything. If `go` isn't on your `PATH`, prefix with `GO=/path/to/go`.

### Format & lint

```bash
just fmt        # format all Go code
just fmt-check  # what CI checks (non-mutating)
just lint       # golangci-lint
```

### Testing

```bash
just test       # full suite (sandboxes HOME — safe to run anywhere)
just test-race  # with the race detector
just cover      # with coverage
```

Please include tests for new features or bug fixes. Tests must not read or write
the real Atrium data directory — see `internal/testutil.SandboxHomeMain`.

## Commits

Use [Conventional Commits](https://www.conventionalcommits.org/), lowercase
(`feat: …`, `fix: …`). The release changelog is generated from commit prefixes.

## Questions?

Feel free to open an issue for any questions about contributing.

