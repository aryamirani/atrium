package git

import (
	"context"
	"testing"
)

// TestCleanupSucceedsAfterRebindFromCancelledContext reproduces the #282 orphan
// mechanism and proves SetBaseContext reverses it. When the lifecycle context is
// cancelled mid-shutdown (SIGTERM/SIGHUP), Cleanup's ctx-gated `git branch -D`
// never runs, so the session branch survives — the "branch exists on title reuse"
// symptom. After rebinding to a context.WithoutCancel context, Cleanup can
// actually delete the branch.
func TestCleanupSucceedsAfterRebindFromCancelledContext(t *testing.T) {
	repoPath := newTestRepo(t)

	ctx, cancel := context.WithCancel(context.Background())
	wt, branch, err := NewWorktree(ctx, repoPath, "orphan-sess")
	if err != nil {
		t.Fatalf("NewWorktree error = %v", err)
	}
	if err := wt.Setup(); err != nil {
		t.Fatalf("Setup() error = %v", err)
	}
	if !LocalBranchExists(context.Background(), repoPath, branch) {
		t.Fatalf("branch %q should exist after Setup()", branch)
	}

	// Signal shutdown cancels the lifecycle context while the session is still
	// mid-teardown. The ctx-gated git ops fail, so the branch is left behind — the
	// exact orphan the fix must prevent.
	cancel()
	_ = wt.Cleanup()
	if !LocalBranchExists(context.Background(), repoPath, branch) {
		t.Fatal("precondition for #282: Cleanup() under a cancelled ctx should have left the branch behind")
	}

	// Rebinding to a survivable context lets a second teardown actually complete —
	// mirroring app.Run handing the quiescent orphan a WithoutCancel context.
	wt.SetBaseContext(context.WithoutCancel(ctx))
	if err := wt.Cleanup(); err != nil {
		t.Fatalf("Cleanup() after rebind error = %v", err)
	}
	if LocalBranchExists(context.Background(), repoPath, branch) {
		t.Fatalf("branch %q should be gone after rebind + Cleanup()", branch)
	}
}
