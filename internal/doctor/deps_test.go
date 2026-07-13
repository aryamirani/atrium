package doctor

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// fakeDepRunner returns canned version output (or error) per binary.
type fakeDepRunner struct {
	out map[string]string
	err map[string]error
}

func (f fakeDepRunner) probe(_ context.Context, bin, _ string) (string, error) {
	if e, ok := f.err[bin]; ok {
		return "", e
	}
	if o, ok := f.out[bin]; ok {
		return o, nil
	}
	return "", fmt.Errorf("%w: %s", ErrNotInstalled, bin)
}

func stateFor(results []DepResult, name string) DepResult {
	for _, r := range results {
		if r.Name == name {
			return r
		}
	}
	return DepResult{}
}

func TestCheckDepsClassifies(t *testing.T) {
	okAuth := authChecker(func(context.Context) error { return nil })
	failAuth := authChecker(func(context.Context) error { return errors.New("not logged in") })

	t.Run("all present and parseable", func(t *testing.T) {
		r := fakeDepRunner{out: map[string]string{
			"tmux": "tmux 3.6\n",
			"git":  "git version 2.53.0\n",
			"gh":   "gh version 2.46.0 (2025-12-13)\n",
		}}
		got := checkDeps(context.Background(), coreDeps, r, "linux", okAuth)
		for _, name := range []string{"tmux", "git", "gh"} {
			d := stateFor(got, name)
			if d.State != DepOK {
				t.Errorf("%s: State = %v, want DepOK", name, d.State)
			}
			if d.Hint != "" {
				t.Errorf("%s: Hint = %q, want empty for DepOK", name, d.Hint)
			}
		}
		if v := stateFor(got, "tmux").Version; v != "3.6" {
			t.Errorf("tmux version = %q, want 3.6", v)
		}
	})

	t.Run("tmux missing", func(t *testing.T) {
		r := fakeDepRunner{
			out: map[string]string{"git": "git version 2.53.0\n", "gh": "gh version 2.46.0\n"},
			err: map[string]error{"tmux": ErrNotInstalled},
		}
		got := checkDeps(context.Background(), coreDeps, r, "darwin", okAuth)
		d := stateFor(got, "tmux")
		if d.State != DepMissing {
			t.Fatalf("tmux State = %v, want DepMissing", d.State)
		}
		if !strings.Contains(d.Hint, "brew install tmux") {
			t.Errorf("tmux Hint = %q, want a brew hint", d.Hint)
		}
		if !MissingRequired(got) {
			t.Error("MissingRequired = false, want true when tmux missing")
		}
	})

	t.Run("gh present but unauthenticated", func(t *testing.T) {
		r := fakeDepRunner{out: map[string]string{
			"tmux": "tmux 3.6\n", "git": "git version 2.53.0\n", "gh": "gh version 2.46.0\n",
		}}
		got := checkDeps(context.Background(), coreDeps, r, "linux", failAuth)
		d := stateFor(got, "gh")
		if d.State != DepPresentUnauth {
			t.Fatalf("gh State = %v, want DepPresentUnauth", d.State)
		}
		if !strings.Contains(d.Hint, "gh auth login") {
			t.Errorf("gh Hint = %q, want an auth hint", d.Hint)
		}
		// gh is optional, so an unauthenticated gh must not fail doctor.
		if MissingRequired(got) {
			t.Error("MissingRequired = true, want false when only gh is unauthenticated")
		}
	})

	t.Run("tmux present but unparseable version", func(t *testing.T) {
		r := fakeDepRunner{out: map[string]string{
			"tmux": "tmux next\n", "git": "git version 2.53.0\n", "gh": "gh version 2.46.0\n",
		}}
		got := checkDeps(context.Background(), coreDeps, r, "linux", okAuth)
		d := stateFor(got, "tmux")
		if d.State != DepPresentUnknown {
			t.Fatalf("tmux State = %v, want DepPresentUnknown", d.State)
		}
		// Present-but-unknown is not missing, so it must not fail doctor.
		if MissingRequired(got) {
			t.Error("MissingRequired = true, want false when tmux is present with an odd version")
		}
	})

	t.Run("nil auth leaves present gh at ok", func(t *testing.T) {
		r := fakeDepRunner{out: map[string]string{
			"tmux": "tmux 3.6\n", "git": "git version 2.53.0\n", "gh": "gh version 2.46.0\n",
		}}
		got := checkDeps(context.Background(), coreDeps, r, "linux", nil)
		if d := stateFor(got, "gh"); d.State != DepOK {
			t.Fatalf("gh State = %v, want DepOK when auth checker is nil", d.State)
		}
	})
}

func TestInstallHint(t *testing.T) {
	tmux := depSpec{name: "tmux", bin: "tmux"}
	gh := depSpec{name: "gh", bin: "gh"}
	cases := []struct {
		goos, want string
		spec       depSpec
	}{
		{"darwin", "brew install tmux", tmux},
		{"linux", "sudo apt install tmux", tmux},
		{"windows", "install docs", tmux},
		{"darwin", "brew install gh", gh},
		{"linux", "github.com/cli/cli", gh},
	}
	for _, c := range cases {
		if got := installHint(c.goos, c.spec); !strings.Contains(got, c.want) {
			t.Errorf("installHint(%q, %q) = %q, want substring %q", c.goos, c.spec.bin, got, c.want)
		}
	}
}

func TestRenderDeps(t *testing.T) {
	out := RenderDeps([]DepResult{
		{Name: "tmux", Bin: "tmux", Kind: DepRequired, State: DepMissing, Hint: "install: brew install tmux"},
		{Name: "git", Bin: "git", Kind: DepRequired, State: DepOK, Version: "2.53.0"},
		{Name: "gh", Bin: "gh", Kind: DepOptional, State: DepPresentUnauth, Hint: "run: gh auth login"},
	})

	for _, want := range []string{
		"Core dependencies:",
		"tmux", "not installed", "brew install tmux",
		"git", "2.53.0", "ok",
		"gh", "not authenticated", "gh auth login",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("RenderDeps() missing %q\n--- got ---\n%s", want, out)
		}
	}
	// An OK dep must not emit a hint line.
	if strings.Contains(out, "→ install: brew install git") {
		t.Errorf("RenderDeps() emitted a hint for an OK dep\n%s", out)
	}
}
