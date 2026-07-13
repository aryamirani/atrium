package tmux

import (
	"errors"
	"os/exec"
	"testing"
)

func TestAvailable(t *testing.T) {
	orig := lookPath
	t.Cleanup(func() { lookPath = orig })

	t.Run("missing tmux returns ErrNotInstalled", func(t *testing.T) {
		lookPath = func(string) (string, error) { return "", exec.ErrNotFound }
		if err := Available(); !errors.Is(err, ErrNotInstalled) {
			t.Fatalf("Available() = %v, want ErrNotInstalled", err)
		}
	})

	t.Run("present tmux returns nil", func(t *testing.T) {
		lookPath = func(string) (string, error) { return "/usr/bin/tmux", nil }
		if err := Available(); err != nil {
			t.Fatalf("Available() = %v, want nil", err)
		}
	})
}
