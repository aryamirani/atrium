package doctor

import (
	"strings"
	"testing"

	"github.com/ZviBaratz/atrium/session/agent"
)

func TestRenderGatesEmpty(t *testing.T) {
	if out := RenderGates(nil); out != "" {
		t.Errorf("RenderGates(nil) = %q, want \"\" so the section does not render at all", out)
	}
}

// TestRenderGatesMatching pins that a matching gate still prints a row. Doctor is
// a diagnostic: "checked, and it matches" must not look like "the check found
// nothing to say".
func TestRenderGatesMatching(t *testing.T) {
	out := RenderGates([]GateResult{
		{Name: "Claude Code", Account: defaultAccount, Gate: "tengu_copper_thistle", Pinned: false, Actual: false, State: GateMatchesPin},
	})

	for _, want := range []string{"Feature gates:", "Claude Code", "tengu_copper_thistle", "default", "ok (false)"} {
		if !strings.Contains(out, want) {
			t.Errorf("RenderGates() output missing %q\n--- got ---\n%s", want, out)
		}
	}
	if strings.Contains(out, "→") {
		t.Errorf("RenderGates() rendered a remediation hint with nothing flipped\n--- got ---\n%s", out)
	}
}

func TestRenderGatesFlipped(t *testing.T) {
	out := RenderGates([]GateResult{
		{Name: "Claude Code", Account: "personal", Gate: "tengu_copper_thistle", Pinned: false, Actual: true, State: GateFlipped},
		{Name: "Claude Code", Account: "work", Gate: "tengu_copper_thistle", Pinned: false, State: GateUnknown},
	})

	for _, want := range []string{
		"personal", "⚠ flipped (pinned false, resolved true)",
		"work", "unknown (no resolved value on disk)",
		"→ heuristics were verified on the other branch", "session/agent/registry.go",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("RenderGates() output missing %q\n--- got ---\n%s", want, out)
		}
	}
	if n := strings.Count(out, "→"); n != 1 {
		t.Errorf("RenderGates() rendered %d hints, want 1 section-level hint\n--- got ---\n%s", n, out)
	}
}

func TestRender(t *testing.T) {
	out := Render([]Result{
		{Key: agent.KeyClaude, Name: "Claude Code", Installed: "2.1.179", Verified: "2.1.170", Status: StatusDrifted},
		{Key: agent.KeyGemini, Name: "Gemini CLI", Installed: "0.27.4", Verified: "0.27", Status: StatusOK},
		{Key: agent.KeyCodex, Name: "Codex", Status: StatusNotInstalled},
		{Key: agent.KeyAider, Name: "Aider", Installed: "0.64.1", Status: StatusUnknown},
	})

	for _, want := range []string{
		"Claude Code", "2.1.179", "2.1.170", "drifted",
		"Gemini CLI", "ok",
		"Codex", "not installed",
		"Aider", "unknown",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("Render() output missing %q\n--- got ---\n%s", want, out)
		}
	}
}
