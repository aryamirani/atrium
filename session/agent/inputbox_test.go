package agent

import "testing"

// A realistic empty claude composer: rounded box with the "❯" prompt and the live footer
// below it. No gate, no blocking prompt — keystrokes typed here land in the box.
const emptyBox = "" +
	"  Some earlier transcript line\n" +
	"\n" +
	"╭──────────────────────────────────────────────╮\n" +
	"│ ❯                                              │\n" +
	"╰──────────────────────────────────────────────╯\n" +
	"  ? for shortcuts\n"

// The same composer with a typed (but unsubmitted) prompt, wrapped across two interior rows.
const typedBox = "" +
	"╭──────────────────────────────────────────────╮\n" +
	"│ ❯ refactor the parser and add a regression    │\n" +
	"│   test for the nested case                     │\n" +
	"╰──────────────────────────────────────────────╯\n" +
	"  ? for shortcuts\n"

// A startup frame before the box has painted: a banner, no "❯" composer.
const preBoxFrame = "" +
	"  ✻ Welcome to Claude Code\n" +
	"\n" +
	"  Booting…\n"

func TestInputBoxText(t *testing.T) {
	t.Run("empty box is found with no text", func(t *testing.T) {
		text, ok := inputBoxText(emptyBox)
		if !ok {
			t.Fatal("an on-screen composer must be detected as present")
		}
		if text != "" {
			t.Fatalf("an empty composer must read back as empty, got %q", text)
		}
	})

	t.Run("typed text is read back across wrapped rows", func(t *testing.T) {
		text, ok := inputBoxText(typedBox)
		if !ok {
			t.Fatal("a composer with text must be detected as present")
		}
		// The two interior rows are joined; the exact spacing is normalized, so assert on
		// the squashed-whitespace content the delivery check actually compares.
		want := "refactor the parser and add a regression test for the nested case"
		if text != want {
			t.Fatalf("readback = %q, want %q", text, want)
		}
	})

	t.Run("no composer on a pre-box startup frame", func(t *testing.T) {
		if _, ok := inputBoxText(preBoxFrame); ok {
			t.Fatal("a frame without an input box must not be detected as a composer")
		}
	})

	t.Run("a quoted '>' in transcript far above the bottom is not the box", func(t *testing.T) {
		// A ">" at the very top, then many non-empty lines, then no real box: the bottom
		// WindowPrompt budget must keep the stray quote from being read as the composer.
		content := "  > not the box, this is a quoted shell line\n"
		for i := 0; i < WindowPrompt+5; i++ {
			content += "  plain transcript line\n"
		}
		if _, ok := inputBoxText(content); ok {
			t.Fatal("a '>' outside the bottom window must not count as the input box")
		}
	})
}

func TestAdapterInputBoxVisible(t *testing.T) {
	claude := Resolve("claude")
	if !claude.InputBoxVisible(emptyBox) {
		t.Error("the empty composer must be reported visible")
	}
	if !claude.InputBoxVisible(typedBox) {
		t.Error("the composer with typed text must be reported visible")
	}
	if claude.InputBoxVisible(preBoxFrame) {
		t.Error("a pre-box startup frame must not be reported visible")
	}
}
