package config

import (
	"os/exec"
	"testing"
)

func TestProgramInstalled(t *testing.T) {
	// Stub detectAgentCommand to use real exec.LookPath for non-"claude" tokens,
	// so the test verifies actual PATH resolution: "sh" exists, bogus binary doesn't.
	orig := detectAgentCommand
	detectAgentCommand = func(bin string) (string, error) {
		if bin == defaultProgram {
			return GetClaudeCommand()
		}
		if _, err := exec.LookPath(bin); err != nil {
			return "", err
		}
		return bin, nil
	}
	t.Cleanup(func() { detectAgentCommand = orig })

	// Present binary: "sh" is on PATH in any POSIX test environment.
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
