package ui

import (
	"claude-squad/session"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/stretchr/testify/require"
)

func newGroupList(t *testing.T, paths ...string) *List {
	t.Helper()
	s := spinner.New()
	l := NewList(&s, false)
	for _, p := range paths {
		inst, err := session.NewInstance(session.InstanceOptions{Title: "x", Path: p, Program: "echo"})
		require.NoError(t, err)
		l.AddInstance(inst)
	}
	l.SetSize(80, 40)
	return l
}

func TestRepoKey_FallsBackToPathBaseWhenUnstarted(t *testing.T) {
	inst, err := session.NewInstance(session.InstanceOptions{Title: "x", Path: "/tmp/repoA", Program: "echo"})
	require.NoError(t, err)
	require.Equal(t, "repoA", repoKey(inst))
}

func TestList_RendersRepoHeadersWhenMultipleRepos(t *testing.T) {
	l := newGroupList(t, "/tmp/repoA", "/tmp/repoA", "/tmp/repoB")
	// Simulate two distinct started repos so grouping activates (repos map is normally
	// populated by the start finalizer).
	l.repos["repoA"] = 2
	l.repos["repoB"] = 1

	out := l.String()
	// Headers are uppercased as section dividers.
	require.Contains(t, out, "REPOA")
	require.Contains(t, out, "REPOB")
}

func TestList_NoHeadersForSingleRepo(t *testing.T) {
	l := newGroupList(t, "/tmp/repoA", "/tmp/repoA")
	l.repos["repoA"] = 2 // single distinct repo → grouping inactive

	out := l.String()
	// With a single repo no header is emitted, so the uppercased header token must
	// not appear.
	require.NotContains(t, out, repoHeaderStyle.Render("REPOA"))
	_ = strings.TrimSpace(out)
}
