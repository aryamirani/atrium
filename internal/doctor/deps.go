package doctor

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// DepKind distinguishes a hard requirement from an optional dependency.
type DepKind int

const (
	// DepRequired marks a dependency Atrium cannot run without (tmux, git). A
	// missing required dep makes `atrium doctor` exit nonzero.
	DepRequired DepKind = iota
	// DepOptional marks a dependency only some flows need (gh, for push/PR). Its
	// absence or unauthenticated state is reported but never fails doctor.
	DepOptional
)

// DepState is the outcome of one core-dependency probe.
type DepState int

const (
	// DepOK means the binary is on PATH (and, for gh, authenticated).
	DepOK DepState = iota
	// DepMissing means the binary is not on PATH.
	DepMissing
	// DepPresentUnauth means the binary is present but its auth check failed
	// (gh only). Reported, never blocking — auth is a push/PR-time concern.
	DepPresentUnauth
	// DepPresentUnknown means the binary is present but its version output was
	// unparseable. Still usable, so never blocking.
	DepPresentUnknown
)

// DepResult is the report for one core dependency.
type DepResult struct {
	Name    string // human/binary name shown in the table ("tmux", "git", "gh")
	Bin     string // binary probed on PATH
	Kind    DepKind
	State   DepState
	Version string // parsed version, "" when missing/unparseable
	Hint    string // remediation hint; empty for DepOK and for a present-but-unknown dep (nothing to fix)
}

// depSpec is a static core-dependency probe definition.
type depSpec struct {
	name       string
	bin        string
	versionArg string // tmux prints its version to -V; git/gh use --version
	kind       DepKind
}

// coreDeps is the fixed set of hard dependencies doctor probes, in display order.
var coreDeps = []depSpec{
	{"tmux", "tmux", "-V", DepRequired},
	{"git", "git", "--version", DepRequired},
	{"gh", "gh", "--version", DepOptional},
}

// depRunner probes a core-dep binary with its version flag. The method is
// unexported so only in-package fakes (tests) implement it, mirroring Runner.
type depRunner interface {
	probe(ctx context.Context, bin, versionArg string) (string, error)
}

// execDepRunner is the production depRunner: it shells out to `<bin> <versionArg>`.
type execDepRunner struct{}

func (execDepRunner) probe(ctx context.Context, bin, versionArg string) (string, error) {
	path, err := exec.LookPath(bin)
	if err != nil {
		return "", ErrNotInstalled // reuse the sentinel from check.go
	}
	out, err := exec.CommandContext(ctx, path, versionArg).Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// authChecker reports whether gh is authenticated. Injected so tests never shell
// to a real gh and so deps.go need not import session/git (which would pull a
// package edge and its network-timeout constants). Return nil when authenticated.
type authChecker func(ctx context.Context) error

// CheckDeps probes the core dependencies against the real environment. goos
// selects install hints (pass runtime.GOOS); auth runs the gh auth sub-check when
// gh is present (nil disables it, leaving gh at DepOK on mere presence).
func CheckDeps(ctx context.Context, goos string, auth authChecker) []DepResult {
	return checkDeps(ctx, coreDeps, execDepRunner{}, goos, auth)
}

// checkDeps is the testable core of CheckDeps: a fake depRunner, an explicit goos,
// and a stubbable auth checker keep it hermetic. It never errors — every spec
// yields one DepResult whose State carries the outcome.
func checkDeps(ctx context.Context, specs []depSpec, r depRunner, goos string, auth authChecker) []DepResult {
	results := make([]DepResult, 0, len(specs))
	for _, s := range specs {
		res := DepResult{Name: s.name, Bin: s.bin, Kind: s.kind}
		out, err := r.probe(ctx, s.bin, s.versionArg)
		switch {
		case errors.Is(err, ErrNotInstalled):
			res.State = DepMissing
		case err != nil:
			// Present but the version probe failed (odd output, timeout). Treat as
			// present-unknown: the binary resolved on PATH, so it is not missing.
			res.State = DepPresentUnknown
		default:
			if v, ok := parseVersion(out); ok {
				res.Version = v
				res.State = DepOK
			} else {
				res.State = DepPresentUnknown
			}
			// gh's presence isn't enough — run the auth sub-check when supplied.
			if res.State == DepOK && s.bin == "gh" && auth != nil {
				if auth(ctx) != nil {
					res.State = DepPresentUnauth
				}
			}
		}
		if res.State != DepOK {
			res.Hint = installHint(goos, s, res.State)
		}
		results = append(results, res)
	}
	return results
}

// installHint returns the remediation string for a dependency in a non-OK state.
// It is state-aware so it never tells the user to reinstall a binary that is
// already present: an unauthenticated gh only needs `gh auth login`, and a
// present-but-unparseable-version binary has nothing to install (no hint). Only a
// genuinely missing binary gets an OS-appropriate install command. goos is
// injected (runtime.GOOS at the callsite) so hint selection is testable on any
// host, mirroring internal/actions.chooseOpener; gh's Linux install points at the
// docs rather than a wrong `apt install gh`.
func installHint(goos string, s depSpec, state DepState) string {
	switch state {
	case DepPresentUnauth:
		// gh is installed; only its auth sub-check failed.
		return "run: gh auth login"
	case DepPresentUnknown:
		// On PATH but the version was unreadable — nothing to install or fix.
		return ""
	}
	if s.bin == "gh" {
		switch goos {
		case "darwin":
			return "install: brew install gh, then run: gh auth login"
		default:
			return "install: https://github.com/cli/cli#installation, then run: gh auth login"
		}
	}
	switch goos {
	case "darwin":
		return "install: brew install " + s.bin
	case "linux":
		return "install: sudo apt install " + s.bin
	default:
		return fmt.Sprintf("install: see the %s project's install docs", s.bin)
	}
}

// MissingRequired reports whether any required dependency is missing, so the
// doctor command can exit nonzero for scripts/CI. An optional dep (gh) never
// trips it, including when merely unauthenticated.
func MissingRequired(results []DepResult) bool {
	for _, r := range results {
		if r.Kind == DepRequired && r.State == DepMissing {
			return true
		}
	}
	return false
}

func (s DepState) label() string {
	switch s {
	case DepOK:
		return "ok"
	case DepMissing:
		return "not installed"
	case DepPresentUnauth:
		return "⚠ not authenticated"
	default:
		return "⚠ unknown version"
	}
}

// RenderDeps formats core-dependency results as an aligned, newline-terminated
// table under a "Core dependencies:" header, parallel to Render's agent table. A
// hint line is appended only for a dependency that needs action.
func RenderDeps(results []DepResult) string {
	var b strings.Builder
	b.WriteString("Core dependencies:\n")
	for _, r := range results {
		version := r.Version
		if version == "" {
			version = "-"
		}
		fmt.Fprintf(&b, "  %-6s %-10s %s\n", r.Name, version, r.State.label())
		if r.Hint != "" {
			fmt.Fprintf(&b, "         → %s\n", r.Hint)
		}
	}
	return b.String()
}
