package git

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// withOrigin gives repoPath a bare origin and pushes its current branch, returning
// the bare repo path. It leaves the working clone's local branch in place so callers
// can advance origin independently to simulate a stale local base.
func withOrigin(t *testing.T, repoPath, branch string) string {
	t.Helper()
	bare := filepath.Join(t.TempDir(), "origin.git")
	mustRunGit(t, "", "init", "--bare", bare)
	mustRunGit(t, repoPath, "remote", "add", "origin", bare)
	mustRunGit(t, repoPath, "push", "origin", branch)
	mustRunGit(t, repoPath, "fetch", "origin")
	return bare
}

// advanceOrigin makes a throwaway clone of bare, adds a commit on branch, and pushes
// it — advancing origin/<branch> ahead of the original repo's local branch. Returns
// the new origin tip SHA.
func advanceOrigin(t *testing.T, bare, branch, content string) string {
	t.Helper()
	clone := filepath.Join(t.TempDir(), "clone")
	mustRunGit(t, "", "clone", bare, clone)
	mustRunGit(t, clone, "config", "user.name", "Test User")
	mustRunGit(t, clone, "config", "user.email", "test@example.com")
	mustRunGit(t, clone, "checkout", branch)
	if err := os.WriteFile(filepath.Join(clone, "advance.txt"), []byte(content), 0644); err != nil {
		t.Fatalf("write advance.txt: %v", err)
	}
	mustRunGit(t, clone, "add", "advance.txt")
	mustRunGit(t, clone, "commit", "-m", "advance origin")
	mustRunGit(t, clone, "push", "origin", branch)
	return revParse(t, clone, "HEAD")
}

func currentBranch(t *testing.T, repoPath string) string {
	t.Helper()
	return CurrentBranchName(context.Background(), repoPath)
}

// newWorktreeFreshening builds a session worktree off baseRef with the freshen flags
// set directly, bypassing config (tests stay independent of the machine's config.json).
func newWorktreeFreshening(t *testing.T, repoPath, sessionName, baseRef string, ff bool) (*Worktree, string) {
	t.Helper()
	wt, branch, err := NewWorktreeFromBase(context.Background(), repoPath, sessionName, baseRef)
	if err != nil {
		t.Fatalf("NewWorktreeFromBase error = %v", err)
	}
	wt.updateBaseOnCreate = true
	wt.fastForwardLocalBase = ff
	return wt, branch
}

// When local base is behind origin, a freshened session starts off the origin tip,
// records origin/<ref> as its base (so diff counts stay honest), and reports 0/0.
func TestUpdateBase_LocalBehind_BranchesOffOrigin(t *testing.T) {
	repoPath := newTestRepo(t)
	branch := currentBranch(t, repoPath)
	bare := withOrigin(t, repoPath, branch)
	originTip := advanceOrigin(t, bare, branch, "from origin\n")

	localTip := revParse(t, repoPath, branch)
	if localTip == originTip {
		t.Fatal("setup failed: local should be behind origin")
	}

	wt, sessionBranch := newWorktreeFreshening(t, repoPath, "behindsess", branch, false)
	if err := wt.Setup(); err != nil {
		t.Fatalf("Setup() error = %v", err)
	}

	if got := revParse(t, repoPath, sessionBranch); got != originTip {
		t.Fatalf("session branch tip = %q, want origin tip %q", got, originTip)
	}
	if got := wt.GetBaseRef(); got != "origin/"+branch {
		t.Fatalf("baseRef = %q, want %q (keeps ahead/behind honest)", got, "origin/"+branch)
	}
	ahead, behind, unpushed, ok := wt.revListCounts(wt.GetWorktreePath())
	if !ok || ahead != 0 || behind != 0 || unpushed != 0 {
		t.Fatalf("fresh session counts = (%d ahead, %d behind, %d unpushed, ok=%v), want 0/0/0", ahead, behind, unpushed, ok)
	}

	// The user's local base branch must be untouched (non-invasive default).
	if got := revParse(t, repoPath, branch); got != localTip {
		t.Fatalf("local %s moved to %q; non-invasive default must leave it at %q", branch, got, localTip)
	}
}

// When the local base has commits origin lacks (ahead/diverged), freshening must keep
// the local tip — never silently drop unpushed work — and leave baseRef unchanged.
func TestUpdateBase_LocalAhead_KeepsLocal(t *testing.T) {
	repoPath := newTestRepo(t)
	branch := currentBranch(t, repoPath)
	withOrigin(t, repoPath, branch)

	// Add a local-only commit so local is ahead of origin/<branch>.
	if err := os.WriteFile(filepath.Join(repoPath, "local.txt"), []byte("local only\n"), 0644); err != nil {
		t.Fatalf("write local.txt: %v", err)
	}
	mustRunGit(t, repoPath, "add", "local.txt")
	mustRunGit(t, repoPath, "commit", "-m", "local only")
	localTip := revParse(t, repoPath, branch)

	wt, sessionBranch := newWorktreeFreshening(t, repoPath, "aheadsess", branch, false)
	if err := wt.Setup(); err != nil {
		t.Fatalf("Setup() error = %v", err)
	}

	if got := revParse(t, repoPath, sessionBranch); got != localTip {
		t.Fatalf("session branch tip = %q, want local tip %q (must not drop unpushed commits)", got, localTip)
	}
	if got := wt.GetBaseRef(); got != branch {
		t.Fatalf("baseRef = %q, want %q (unchanged when local is preferred)", got, branch)
	}
}

// A repo with no origin remote freshens silently and branches off local, exactly as
// before the feature. This is the guard that keeps the rest of the git suite green.
func TestUpdateBase_NoRemote_UsesLocal(t *testing.T) {
	repoPath := newTestRepo(t)
	branch := currentBranch(t, repoPath)
	localTip := revParse(t, repoPath, branch)

	wt, sessionBranch := newWorktreeFreshening(t, repoPath, "noremotesess", branch, false)
	if err := wt.Setup(); err != nil {
		t.Fatalf("Setup() error = %v", err)
	}
	if got := revParse(t, repoPath, sessionBranch); got != localTip {
		t.Fatalf("session branch tip = %q, want local tip %q", got, localTip)
	}
	if got := wt.GetBaseRef(); got != branch {
		t.Fatalf("baseRef = %q, want %q (unchanged with no remote)", got, branch)
	}
}

// A broken remote (fetch fails) must not break creation: the session is still created
// from the local base.
func TestUpdateBase_FetchFailure_FallsBackToLocal(t *testing.T) {
	repoPath := newTestRepo(t)
	branch := currentBranch(t, repoPath)
	localTip := revParse(t, repoPath, branch)
	// Point origin at a nonexistent path so fetch fails fast.
	mustRunGit(t, repoPath, "remote", "add", "origin", filepath.Join(t.TempDir(), "does-not-exist.git"))

	wt, sessionBranch := newWorktreeFreshening(t, repoPath, "brokensess", branch, false)
	if err := wt.Setup(); err != nil {
		t.Fatalf("Setup() error = %v (fetch failure must not break creation)", err)
	}
	if got := revParse(t, repoPath, sessionBranch); got != localTip {
		t.Fatalf("session branch tip = %q, want local tip %q", got, localTip)
	}
}

// Opt-in fast-forward advances a base branch that is NOT checked out anywhere via a
// pure ref move.
func TestUpdateBase_FastForward_NotCheckedOut_MovesRef(t *testing.T) {
	repoPath := newTestRepo(t)
	// Create a side base branch but stay on the default branch, so "base" is not
	// checked out in any worktree — the pure-ref-move path.
	mustRunGit(t, repoPath, "branch", "base")
	bare := withOrigin(t, repoPath, "base")
	originTip := advanceOrigin(t, bare, "base", "ff target\n")
	mustRunGit(t, repoPath, "fetch", "origin")

	if revParse(t, repoPath, "base") == originTip {
		t.Fatal("setup failed: local base should be behind origin/base")
	}

	wt, _ := newWorktreeFreshening(t, repoPath, "ffsess", "base", true)
	if err := wt.Setup(); err != nil {
		t.Fatalf("Setup() error = %v", err)
	}
	if got := revParse(t, repoPath, "base"); got != originTip {
		t.Fatalf("local base = %q, want fast-forwarded to origin tip %q", got, originTip)
	}
}

// Opt-in fast-forward must refuse to touch a checked-out base branch with a dirty
// working tree, leaving both the ref and the tree as they were.
func TestUpdateBase_FastForward_DirtyCheckout_Skips(t *testing.T) {
	repoPath := newTestRepo(t)
	branch := currentBranch(t, repoPath)
	bare := withOrigin(t, repoPath, branch)
	advanceOrigin(t, bare, branch, "ff target\n")
	mustRunGit(t, repoPath, "fetch", "origin")
	localTip := revParse(t, repoPath, branch)

	// Dirty the base repo's checked-out tree.
	if err := os.WriteFile(filepath.Join(repoPath, "README.md"), []byte("dirty\n"), 0644); err != nil {
		t.Fatalf("dirty README: %v", err)
	}

	wt, _ := newWorktreeFreshening(t, repoPath, "dirtysess", branch, true)
	if err := wt.Setup(); err != nil {
		t.Fatalf("Setup() error = %v", err)
	}
	if got := revParse(t, repoPath, branch); got != localTip {
		t.Fatalf("local %s = %q, want untouched %q (dirty tree must be left alone)", branch, got, localTip)
	}
}

// With updateBaseOnCreate off, creation reproduces the historical local-preferred
// behavior: no fetch, branch off local, baseRef unchanged — even when origin is ahead.
func TestUpdateBase_Disabled_UsesLocal(t *testing.T) {
	repoPath := newTestRepo(t)
	branch := currentBranch(t, repoPath)
	bare := withOrigin(t, repoPath, branch)
	advanceOrigin(t, bare, branch, "from origin\n")
	mustRunGit(t, repoPath, "fetch", "origin")
	localTip := revParse(t, repoPath, branch)

	wt, sessionBranch, err := NewWorktreeFromBase(context.Background(), repoPath, "offsess", branch)
	if err != nil {
		t.Fatalf("NewWorktreeFromBase error = %v", err)
	}
	wt.updateBaseOnCreate = false // explicit, independent of config
	if err := wt.Setup(); err != nil {
		t.Fatalf("Setup() error = %v", err)
	}
	if got := revParse(t, repoPath, sessionBranch); got != localTip {
		t.Fatalf("session branch tip = %q, want local tip %q (toggle off = old behavior)", got, localTip)
	}
	if got := wt.GetBaseRef(); got != branch {
		t.Fatalf("baseRef = %q, want %q (unchanged when disabled)", got, branch)
	}
}

// Re-entry guard: a worktree whose persisted baseRef already carries an "origin/"
// prefix resolves a start point without error.
func TestUpdateBase_OriginPrefixedBaseRef_Resolves(t *testing.T) {
	repoPath := newTestRepo(t)
	branch := currentBranch(t, repoPath)
	withOrigin(t, repoPath, branch)

	wt, sessionBranch := newWorktreeFreshening(t, repoPath, "reentry", "origin/"+branch, false)
	if err := wt.Setup(); err != nil {
		t.Fatalf("Setup() error = %v (origin/-prefixed baseRef must resolve)", err)
	}
	if revParse(t, repoPath, sessionBranch) == "" {
		t.Fatal("session branch was not created")
	}
}
