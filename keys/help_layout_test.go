package keys

import (
	"strings"
	"testing"
)

// Direction 1 of the drift guard: every registered binding must be documented
// by the cheatsheet layout — in a row's key column (Keys) or, rarely, named
// inside a row's prose (Mentions, verified rendered below). A new action
// without a row fails here, structurally, before any rendering happens.
func TestHelpGroups_CoverEveryBinding(t *testing.T) {
	covered := map[KeyName]bool{}
	for _, g := range HelpGroups {
		for _, r := range g.Rows {
			for _, k := range r.Keys {
				covered[k] = true
			}
			for _, k := range r.Mentions {
				covered[k] = true
			}
		}
	}
	for name := range GlobalKeyBindings {
		if !covered[name] {
			t.Errorf("binding %v has no cheatsheet row (add it to a HelpGroup, "+
				"or a Mentions if it is taught inside another row's prose)", name)
		}
	}
}

// Direction 2, structurally: a row can only document keys the registry knows.
// Referencing a name with no binding renders an empty key label — and
// KeyScreensaver, whose absence from the registry is the easter egg's
// exclusion contract, fails here if someone tries to document it.
func TestHelpGroups_RefsResolve(t *testing.T) {
	for _, g := range HelpGroups {
		for _, r := range g.Rows {
			if len(r.Keys) == 0 {
				t.Errorf("group %q: row %q has no Keys — desc-only rows can't be "+
					"tied to the registry", g.Title, r.Desc)
			}
			for _, k := range append(append([]KeyName{}, r.Keys...), r.Mentions...) {
				if _, ok := GlobalKeyBindings[k]; !ok {
					t.Errorf("group %q: row %q references %v, which has no registry "+
						"binding", g.Title, r.Desc, k)
				}
			}
		}
	}
}

// A Mention is a key taught inside another row's prose (space lives in the
// multi-select row). The mention only counts as documentation if the prose
// actually names the key — delete the words and this fails, which is what
// keeps Mentions from becoming a silent exclusion list.
func TestHelpGroups_MentionsAreRendered(t *testing.T) {
	for _, g := range HelpGroups {
		for _, r := range g.Rows {
			for _, k := range r.Mentions {
				label := GlobalKeyBindings[k].Help().Key
				if !strings.Contains(r.Desc, label) {
					t.Errorf("group %q: row %q mentions %v but its desc does not "+
						"contain %q", g.Title, r.Desc, k, label)
				}
			}
		}
	}
}

// The reorder rows must name their units exactly (the #357 class): a session
// row, a repo group, an account cluster. Exact matches — a Contains("account")
// check would have passed the very bug this guards against.
func TestHelpGroups_ReorderRowDescs(t *testing.T) {
	want := map[KeyName]string{
		KeyMoveUp:        "reorder within a repo group",
		KeyMoveGroupUp:   "move a whole group up / down",
		KeyMoveAccountUp: "move an account cluster up / down",
	}
	for _, g := range HelpGroups {
		for _, r := range g.Rows {
			for _, k := range r.Keys {
				if desc, ok := want[k]; ok {
					if r.Desc != desc {
						t.Errorf("row for %v: desc %q, want %q", k, r.Desc, desc)
					}
					delete(want, k)
				}
			}
		}
	}
	for k := range want {
		t.Errorf("no cheatsheet row keyed by %v", k)
	}
}
