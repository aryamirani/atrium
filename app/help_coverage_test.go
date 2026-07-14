package app

import (
	"strings"
	"testing"

	"github.com/ZviBaratz/atrium/keys"

	"github.com/charmbracelet/x/ansi"
)

// The help cheatsheet is hand-written prose, decoupled from the binding maps —
// so a rebind that forgets the help screen drifts silently. This guard pins
// them together: every binding's displayed key must appear in the cheatsheet.
//
// Normalization: the cheatsheet writes chords with '-' ("shift-tab") while
// key.WithHelp uses '+' ("shift+tab"); fold one into the other before matching.
func TestHelpScreen_CoversEveryBinding(t *testing.T) {
	content := ansi.Strip(helpTypeGeneral{}.toContent())
	content = strings.ReplaceAll(content, "-", "+")
	// The cheatsheet compacts the scroll pair to "shift-↑↓"; expand it so both
	// bindings count as covered.
	content = strings.ReplaceAll(content, "shift+↑↓", "shift+↑ shift+↓")

	for name, binding := range keys.GlobalKeyBindings {
		k := strings.ReplaceAll(binding.Help().Key, "-", "+")
		if !strings.Contains(content, k) {
			t.Errorf("binding %v (%q) is missing from the help cheatsheet", name, binding.Help().Key)
		}
	}
}

// The inverse: keys retired by remaps must not linger in the help text. This
// list grows whenever a binding moves.
func TestHelpScreen_NoRetiredKeys(t *testing.T) {
	content := ansi.Strip(helpTypeGeneral{}.toContent())

	for _, stale := range []string{"checkout"} {
		if strings.Contains(strings.ToLower(content), stale) {
			t.Errorf("help cheatsheet still mentions retired %q", stale)
		}
	}
}

// The screensaver backtick is a deliberate easter egg. It surfaces to users by
// exactly two routes, so this guards both: a GlobalKeyBindings entry (which
// would feed it to the coverage guard above *and* to the hint bar — see
// ui.renderHintLine, the map's only other reader), and the hand-written prose.
//
// The structural half is the load-bearing one: the absence of a binding is what
// makes the omission automatic rather than a thing every future editor has to
// remember.
func TestHelpScreen_OmitsScreensaverKey(t *testing.T) {
	if _, ok := keys.GlobalKeyBindings[keys.KeyScreensaver]; ok {
		t.Error("KeyScreensaver must have no GlobalKeyBindings entry — that absence is " +
			"what keeps the easter egg out of both the help cheatsheet and the hint bar")
	}

	content := strings.ToLower(ansi.Strip(helpTypeGeneral{}.toContent()))
	if strings.Contains(content, "screensaver") {
		t.Error("help cheatsheet must not mention the screensaver")
	}
}
