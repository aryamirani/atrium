package app

import (
	"testing"

	"github.com/ZviBaratz/atrium/session"
	"github.com/ZviBaratz/atrium/session/git"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Pressing c (create PR) on a direct (non-git) session has no branch to open a PR
// for; it must explain itself with an error rather than opening a confirmation it
// could never satisfy.
func TestCreate_DirectSessionExplains(t *testing.T) {
	h := newCreateFormHome(t)
	inst, err := session.NewInstance(session.InstanceOptions{
		Title:   "direct",
		Path:    t.TempDir(),
		Program: "echo",
		Direct:  true,
	})
	require.NoError(t, err)
	h.list.AddInstance(inst)

	pressKey(h, 'c')

	assert.Equal(t, stateDefault, h.state, "no confirmation overlay for a direct session")
	require.True(t, h.menu.HasNotice(), "the guard must explain itself")
	assert.Contains(t, h.menu.String(), "direct")
}

// Pressing c on a session that is still starting must defer with a notice rather
// than reaching for a worktree that does not exist yet.
func TestCreate_LoadingSessionDefers(t *testing.T) {
	h := newCreateFormHome(t)
	inst := newBranchInstance(t, "booting", "zvi/feat")
	inst.SetStatus(session.Loading)
	h.list.AddInstance(inst)

	pressKey(h, 'c')

	assert.Equal(t, stateDefault, h.state, "no confirmation overlay while loading")
	require.True(t, h.menu.HasNotice(), "the loading guard must explain itself")
	assert.Contains(t, h.menu.String(), "starting")
}

// Pressing c on a branch that isn't pushed yet must explain why (push first)
// rather than opening a confirmation gh would reject for lack of a remote head.
func TestCreate_UnpushedExplains(t *testing.T) {
	h := newCreateFormHome(t)
	inst := newBranchInstance(t, "feat", "zvi/feat")
	inst.SetStatus(session.Running)
	inst.SetPRStatus(&git.PRStatus{Pushed: false, HasPR: false})
	h.list.AddInstance(inst)

	pressKey(h, 'c')

	assert.Equal(t, stateDefault, h.state, "an unpushed branch must not open the confirmation")
	require.True(t, h.menu.HasNotice())
	assert.Contains(t, h.menu.String(), "push")
}

// Pressing c when the branch already has a PR must explain (merge it instead)
// rather than opening a duplicate-PR confirmation.
func TestCreate_AlreadyHasPRExplains(t *testing.T) {
	h := newCreateFormHome(t)
	inst := newBranchInstance(t, "feat", "zvi/feat")
	inst.SetStatus(session.Running)
	inst.SetPRStatus(&git.PRStatus{Pushed: true, HasPR: true, State: "OPEN"})
	h.list.AddInstance(inst)

	pressKey(h, 'c')

	assert.Equal(t, stateDefault, h.state, "an existing PR must not open the create confirmation")
	require.True(t, h.menu.HasNotice())
	assert.Contains(t, h.menu.String(), "already")
}

// A pushed session with no PR opens the create confirmation. With the default
// draft setting, the confirmation must name it as a draft. (The action itself is
// not run here — it would shell out to gh.)
func TestCreate_CreatablePROpensConfirmation(t *testing.T) {
	h := newCreateFormHome(t)
	inst := newBranchInstance(t, "feat", "zvi/feat")
	inst.SetStatus(session.Running)
	inst.SetPRStatus(&git.PRStatus{Pushed: true, HasPR: false})
	h.list.AddInstance(inst)

	pressKey(h, 'c')

	assert.Equal(t, stateConfirm, h.state, "a pushed, PR-less branch must open the confirmation")
	require.NotNil(t, h.pendingConfirmAction, "the confirmed create action must be staged")
	require.NotNil(t, h.confirmationOverlay)
	assert.Contains(t, h.confirmationOverlay.Render(), "draft", "default config opens drafts; the confirm must say so")
}
