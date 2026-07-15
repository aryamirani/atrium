package doctor

import (
	"fmt"
	"strings"
)

func (s Status) label() string {
	switch s {
	case StatusOK:
		return "ok"
	case StatusDrifted:
		return "⚠ drifted"
	case StatusNotInstalled:
		return "not installed"
	default:
		return "unknown"
	}
}

// label renders one gate's outcome, carrying both values on a flip so the row is
// actionable without cross-referencing the registry.
func (r GateResult) label() string {
	switch r.State {
	case GateMatchesPin:
		return fmt.Sprintf("ok (%t)", r.Actual)
	case GateFlipped:
		return fmt.Sprintf("⚠ flipped (pinned %t, resolved %t)", r.Pinned, r.Actual)
	default:
		return "unknown (no resolved value on disk)"
	}
}

// RenderGates formats feature-gate results as an aligned, newline-terminated table
// under a "Feature gates:" header, parallel to Render and RenderDeps. Returns ""
// when no adapter pins a gate, so the section simply does not exist rather than
// rendering an empty header.
//
// Matching rows are printed, not suppressed: doctor is a diagnostic, and "the gate
// was checked and matches" and "the check silently found nothing to say" must not
// look identical. The remediation hint is section-level and appears only on a real
// flip — it is one instruction regardless of how many rows tripped.
func RenderGates(results []GateResult) string {
	if len(results) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Feature gates:\n")
	for _, r := range results {
		fmt.Fprintf(&b, "  %-12s %-22s %-10s %s\n", r.Name, r.Gate, r.Account, r.label())
	}
	if GatesFlipped(results) {
		b.WriteString("    → heuristics were verified on the other branch of this gate, and a flip\n")
		b.WriteString("      changes the UI with no version change. Re-verify session/agent/registry.go.\n")
	}
	return b.String()
}

// Render formats drift results as an aligned, newline-terminated table.
func Render(results []Result) string {
	var b strings.Builder
	b.WriteString("Agent heuristics:\n")
	for _, r := range results {
		installed := r.Installed
		if installed == "" {
			installed = "-"
		}
		verified := r.Verified
		if verified == "" {
			verified = "-"
		}
		fmt.Fprintf(&b, "  %-12s installed %-10s verified %-10s %s\n",
			r.Name, installed, verified, r.Status.label())
	}
	return b.String()
}
