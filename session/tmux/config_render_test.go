package tmux

import (
	"regexp"
	"strings"
	"testing"
)

var wsRun = regexp.MustCompile(`[ \t]+`)

// collapseWS squeezes runs of spaces/tabs to a single space so assertions don't
// depend on the template's column alignment.
func collapseWS(s string) string { return wsRun.ReplaceAllString(s, " ") }

// The managed config is rendered from a template gated on the context bar. With the
// bar on, the status line is enabled and references the @atrium_* options Atrium
// pushes; with it off, the file collapses to the chrome-free `status off` that
// shipped before the feature. The terminal-title fix is unconditional in both.
func TestRenderManagedConfig(t *testing.T) {
	on, err := renderManagedConfig(true)
	if err != nil {
		t.Fatalf("renderManagedConfig(true) error: %v", err)
	}
	onStr := collapseWS(string(on))
	for _, want := range []string{"status on", "@atrium_left", "@atrium_right", "set-titles on"} {
		if !strings.Contains(onStr, want) {
			t.Errorf("context-bar config missing %q\n---\n%s", want, onStr)
		}
	}
	if strings.Contains(onStr, "status off") {
		t.Errorf("context-bar config should not disable the status line\n---\n%s", onStr)
	}

	off, err := renderManagedConfig(false)
	if err != nil {
		t.Fatalf("renderManagedConfig(false) error: %v", err)
	}
	offStr := collapseWS(string(off))
	if !strings.Contains(offStr, "status off") {
		t.Errorf("disabled config should set status off\n---\n%s", offStr)
	}
	if strings.Contains(offStr, "@atrium_left") {
		t.Errorf("disabled config should not reference the context bar options\n---\n%s", offStr)
	}
	// The terminal-title fix is independent of the bar toggle.
	if !strings.Contains(offStr, "set-titles on") {
		t.Errorf("set-titles should be on regardless of the bar\n---\n%s", offStr)
	}
}
