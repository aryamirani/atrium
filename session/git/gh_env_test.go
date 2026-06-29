package git

import (
	"context"
	"os"
	"slices"
	"testing"
)

// TestGHEnvAndContext exercises the pure context/env helpers: a dir tagged onto a
// context round-trips and produces a GH_CONFIG_DIR-bearing env that is a superset
// of os.Environ; an untagged context yields "" and a nil env (inherit unchanged).
func TestGHEnvAndContext(t *testing.T) {
	const dir = "/home/tester/.config/gh-quantivly"

	// Tagged context.
	ctx := withGHConfigDir(context.Background(), dir)
	if got := ghConfigDirFromContext(ctx); got != dir {
		t.Fatalf("ghConfigDirFromContext = %q, want %q", got, dir)
	}
	env := ghEnv(ctx)
	if !slices.Contains(env, "GH_CONFIG_DIR="+dir) {
		t.Fatalf("ghEnv missing GH_CONFIG_DIR=%s; got %v", dir, env)
	}
	if len(env) != len(os.Environ())+1 {
		t.Fatalf("ghEnv should be os.Environ plus one entry: len=%d, want %d", len(env), len(os.Environ())+1)
	}

	// Empty dir is a no-op: context unchanged, "" read back, nil env (inherit).
	if withGHConfigDir(context.Background(), "") != context.Background() {
		t.Fatal("withGHConfigDir(ctx, \"\") must return ctx unchanged")
	}
	if got := ghConfigDirFromContext(context.Background()); got != "" {
		t.Fatalf("ghConfigDirFromContext on bare ctx = %q, want \"\"", got)
	}
	if env := ghEnv(context.Background()); env != nil {
		t.Fatalf("ghEnv on bare ctx = %v, want nil (inherit)", env)
	}
}

// TestWorktreeGHContext: ghContext tags the context with the worktree's dir, and
// is a no-op when the field is empty.
func TestWorktreeGHContext(t *testing.T) {
	const dir = "/home/tester/.config/gh-quantivly"
	withDir := &Worktree{ghConfigDir: dir}
	if got := ghConfigDirFromContext(withDir.ghContext(context.Background())); got != dir {
		t.Fatalf("ghContext did not thread dir: got %q, want %q", got, dir)
	}
	noDir := &Worktree{}
	if got := ghConfigDirFromContext(noDir.ghContext(context.Background())); got != "" {
		t.Fatalf("ghContext with empty field should not tag: got %q", got)
	}
}

// captureGHCLI swaps the gh-availability gate for one that records the context it
// received and returns nil, so we can assert the account dir was threaded into it.
func captureGHCLI(into *context.Context) func() {
	orig := checkGHCLI
	checkGHCLI = func(ctx context.Context) error { *into = ctx; return nil }
	return func() { checkGHCLI = orig }
}

// TestMergePR_ThreadsGHConfigDir asserts MergePR tags both the gh gate and the
// merge seam with the worktree's GH_CONFIG_DIR.
func TestMergePR_ThreadsGHConfigDir(t *testing.T) {
	const dir = "/home/tester/.config/gh-quantivly"
	var gateCtx, seamCtx context.Context
	defer captureGHCLI(&gateCtx)()
	orig := runGHMerge
	runGHMerge = func(ctx context.Context, _, _ string) error { seamCtx = ctx; return nil }
	defer func() { runGHMerge = orig }()

	wt := &Worktree{repoPath: "/base/repo", branchName: "feat", ghConfigDir: dir}
	if err := wt.MergePR(); err != nil {
		t.Fatalf("MergePR: %v", err)
	}
	if got := ghConfigDirFromContext(gateCtx); got != dir {
		t.Errorf("gate ctx dir = %q, want %q", got, dir)
	}
	if got := ghConfigDirFromContext(seamCtx); got != dir {
		t.Errorf("merge seam ctx dir = %q, want %q", got, dir)
	}
}

// TestCreatePR_ThreadsGHConfigDir asserts CreatePR tags both the gate and the
// create seam.
func TestCreatePR_ThreadsGHConfigDir(t *testing.T) {
	const dir = "/home/tester/.config/gh-quantivly"
	var gateCtx, seamCtx context.Context
	defer captureGHCLI(&gateCtx)()
	orig := runGHCreate
	runGHCreate = func(ctx context.Context, _ string, _ []string) ([]byte, error) {
		seamCtx = ctx
		return []byte("https://example/1\n"), nil
	}
	defer func() { runGHCreate = orig }()

	wt := &Worktree{worktreePath: "/wt", branchName: "feat", ghConfigDir: dir}
	if _, err := wt.CreatePR(false); err != nil {
		t.Fatalf("CreatePR: %v", err)
	}
	if got := ghConfigDirFromContext(gateCtx); got != dir {
		t.Errorf("gate ctx dir = %q, want %q", got, dir)
	}
	if got := ghConfigDirFromContext(seamCtx); got != dir {
		t.Errorf("create seam ctx dir = %q, want %q", got, dir)
	}
}

// TestOpenPRURL_ThreadsGHConfigDir asserts OpenPRURL tags both the gate and the
// web seam.
func TestOpenPRURL_ThreadsGHConfigDir(t *testing.T) {
	const dir = "/home/tester/.config/gh-quantivly"
	var gateCtx, seamCtx context.Context
	defer captureGHCLI(&gateCtx)()
	defer stubGHPRWeb(func(ctx context.Context, _, _ string) error { seamCtx = ctx; return nil })()

	wt := &Worktree{repoPath: "/base/repo", branchName: "feat", ghConfigDir: dir}
	if err := wt.OpenPRURL(); err != nil {
		t.Fatalf("OpenPRURL: %v", err)
	}
	if got := ghConfigDirFromContext(gateCtx); got != dir {
		t.Errorf("gate ctx dir = %q, want %q", got, dir)
	}
	if got := ghConfigDirFromContext(seamCtx); got != dir {
		t.Errorf("web seam ctx dir = %q, want %q", got, dir)
	}
}

// TestPRStatus_ThreadsGHConfigDir asserts the PR poll's gh pr view seam receives
// the worktree's GH_CONFIG_DIR (so the badge reflects the right account).
func TestPRStatus_ThreadsGHConfigDir(t *testing.T) {
	const dir = "/home/tester/.config/gh-quantivly"
	wt := pushedWorktree(t)
	wt.ghConfigDir = dir

	var seamCtx context.Context
	defer stubGHPRView(func(ctx context.Context, _, _ string) ([]byte, error) {
		seamCtx = ctx
		return []byte(`{"number":1,"url":"https://example/1","state":"OPEN","isDraft":false}`), nil
	})()

	_ = wt.PRStatus(context.Background(), false)
	if got := ghConfigDirFromContext(seamCtx); got != dir {
		t.Errorf("pr view seam ctx dir = %q, want %q", got, dir)
	}
}
