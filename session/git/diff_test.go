package git

import "testing"

func TestParseNumstat(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantAdded   int
		wantRemoved int
		wantFiles   int
	}{
		{
			name:        "empty output",
			input:       "",
			wantAdded:   0,
			wantRemoved: 0,
			wantFiles:   0,
		},
		{
			name:        "single file",
			input:       "3\t1\tfoo.go\n",
			wantAdded:   3,
			wantRemoved: 1,
			wantFiles:   1,
		},
		{
			name:        "multiple files sum correctly",
			input:       "3\t1\tfoo.go\n10\t2\tbar/baz.go\n",
			wantAdded:   13,
			wantRemoved: 3,
			wantFiles:   2,
		},
		{
			name:        "binary files count but skip line totals",
			input:       "5\t0\tfoo.go\n-\t-\timage.png\n2\t2\tbar.go\n",
			wantAdded:   7,
			wantRemoved: 2,
			wantFiles:   3,
		},
		{
			name:        "path with tabs is preserved via SplitN",
			input:       "4\t4\tpath\twith\ttabs.go\n",
			wantAdded:   4,
			wantRemoved: 4,
			wantFiles:   1,
		},
		{
			name:        "trailing newlines do not add garbage",
			input:       "1\t0\ta.go\n\n\n",
			wantAdded:   1,
			wantRemoved: 0,
			wantFiles:   1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotAdded, gotRemoved, gotFiles := parseNumstat(tt.input)
			if gotAdded != tt.wantAdded || gotRemoved != tt.wantRemoved || gotFiles != tt.wantFiles {
				t.Errorf("parseNumstat(%q) = (%d, %d, %d), want (%d, %d, %d)",
					tt.input, gotAdded, gotRemoved, gotFiles, tt.wantAdded, tt.wantRemoved, tt.wantFiles)
			}
		})
	}
}

func TestParseLeftRightCount(t *testing.T) {
	// `git rev-list --left-right --count <baseRef>...HEAD` prints "<behind>\t<ahead>":
	// left side = commits in baseRef not in HEAD (base moved on), right side = commits
	// in HEAD not in baseRef (session progress).
	tests := []struct {
		name       string
		input      string
		wantBehind int
		wantAhead  int
		wantOK     bool
	}{
		{name: "ahead and behind", input: "3\t2\n", wantBehind: 3, wantAhead: 2, wantOK: true},
		{name: "no divergence", input: "0\t0\n", wantBehind: 0, wantAhead: 0, wantOK: true},
		{name: "ahead only", input: "0\t5\n", wantBehind: 0, wantAhead: 5, wantOK: true},
		{name: "no trailing newline", input: "1\t4", wantBehind: 1, wantAhead: 4, wantOK: true},
		{name: "empty output is not ok", input: "", wantOK: false},
		{name: "malformed output is not ok", input: "garbage\n", wantOK: false},
		{name: "single field is not ok", input: "3\n", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			behind, ahead, ok := parseLeftRightCount(tt.input)
			if ok != tt.wantOK || (ok && (behind != tt.wantBehind || ahead != tt.wantAhead)) {
				t.Errorf("parseLeftRightCount(%q) = (%d, %d, %v), want (%d, %d, %v)",
					tt.input, behind, ahead, ok, tt.wantBehind, tt.wantAhead, tt.wantOK)
			}
		})
	}
}

func TestCountDiffFiles(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int
	}{
		{name: "empty diff", input: "", want: 0},
		{
			name:  "single file",
			input: "diff --git a/foo.go b/foo.go\n@@ -1 +1 @@\n-old\n+new\n",
			want:  1,
		},
		{
			name: "two files",
			input: "diff --git a/foo.go b/foo.go\n+x\n" +
				"diff --git a/bar.go b/bar.go\n+y\n",
			want: 2,
		},
		{
			name:  "added-line content starting with diff word is not miscounted",
			input: "diff --git a/foo.go b/foo.go\n+diff --git this is code\n",
			want:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := countDiffFiles(tt.input); got != tt.want {
				t.Errorf("countDiffFiles(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}
