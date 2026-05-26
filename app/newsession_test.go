package app

import (
	"claude-squad/config"
	"claude-squad/session"
	"claude-squad/ui"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestHomeWithInstances(t *testing.T, paths ...string) *home {
	t.Helper()
	s := spinner.New()
	l := ui.NewList(&s, false)
	for i, p := range paths {
		inst, err := session.NewInstance(session.InstanceOptions{
			Title:   string(rune('a' + i)),
			Path:    p,
			Program: "echo",
		})
		require.NoError(t, err)
		l.AddInstance(inst)
	}
	return &home{ctx: context.Background(), list: l, appState: config.DefaultState()}
}

func TestDefaultNewSessionPath_CwdFallback(t *testing.T) {
	h := newTestHomeWithInstances(t) // no instances → nothing highlighted
	cwd, err := os.Getwd()
	require.NoError(t, err)
	assert.Equal(t, cwd, h.defaultNewSessionPath())
}

func TestDefaultNewSessionPath_FromHighlightedInstance(t *testing.T) {
	h := newTestHomeWithInstances(t, "/tmp/repoA", "/tmp/repoB")
	h.list.SetSelectedInstance(1) // highlight repoB
	assert.Equal(t, "/tmp/repoB", h.defaultNewSessionPath())
}

func TestCandidateRepoPaths_CurrentFirstThenDeduped(t *testing.T) {
	h := newTestHomeWithInstances(t, "/tmp/repoA", "/tmp/repoB", "/tmp/repoA")
	h.newSessionPath = "/tmp/repoB" // current target

	got := h.candidateRepoPaths()

	// Current target comes first; duplicates are dropped; cwd is appended.
	require.GreaterOrEqual(t, len(got), 3)
	assert.Equal(t, "/tmp/repoB", got[0])
	assert.Contains(t, got, "/tmp/repoA")

	cwd, err := os.Getwd()
	require.NoError(t, err)
	assert.Contains(t, got, cwd)

	// No duplicates overall.
	seen := map[string]int{}
	for _, p := range got {
		seen[p]++
		assert.Equal(t, 1, seen[p], "path %q duplicated", p)
	}
}

func TestCandidateRepoPaths_DropsStaleRecentPaths(t *testing.T) {
	h := newTestHomeWithInstances(t)
	existing := t.TempDir()
	missing := filepath.Join(t.TempDir(), "deleted-repo")
	require.NoError(t, h.appState.AddRecentPath(missing))
	require.NoError(t, h.appState.AddRecentPath(existing))

	got := h.candidateRepoPaths()

	assert.Contains(t, got, existing, "existing recent path should be offered")
	assert.NotContains(t, got, missing, "missing recent path should be pruned")
}
