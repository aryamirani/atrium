package tmux

import (
	"context"
	"fmt"
	"io/fs"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ZviBaratz/atrium/internal/testutil"
)

// TestManagedConfigParsesUnderRealTmux feeds the rendered managed config to a real
// tmux via `source-file` and asserts a clean parse. This is the regression guard for
// the `\E`/`\7` clipboard-override bug (`atrium.conf:NN: invalid octal escape`): the
// pre-existing string-only render test never asked tmux to parse the file, so the bad
// escape shipped.
//
// It must use `source-file`, NOT `new-session -d -f <conf>`. A detached new-session
// returns success and defers any config parse error until a client attaches, so a
// new-session-based check would false-pass the broken config. We build the tmux
// commands directly (rather than via tmuxCommand) precisely because we need a
// throwaway socket and an explicit `-f`-less probe server that we control.
func TestManagedConfigParsesUnderRealTmux(t *testing.T) {
	testutil.RequireTmux(t)
	for _, contextBar := range []bool{true, false} {
		t.Run(fmt.Sprintf("contextBar=%v", contextBar), func(t *testing.T) {
			rendered, err := renderManagedConfig(contextBar)
			if err != nil {
				t.Fatalf("renderManagedConfig(%v): %v", contextBar, err)
			}
			path := filepath.Join(t.TempDir(), "atrium.conf")
			if err := os.WriteFile(path, rendered, 0o644); err != nil {
				t.Fatalf("write rendered config: %v", err)
			}

			// tmux puts the socket under $TMUX_TMPDIR (default /tmp — it ignores
			// TMPDIR), and never unlinks the file when the server dies — so without
			// this the probe socket outlives kill-server and piles up in the shared
			// /tmp/tmux-<uid> next to Atrium's live socket. A temp root of our own
			// lets the cleanup below take the socket with it.
			//
			// The root has to stay short: tmux binds the socket at
			// $TMUX_TMPDIR/tmux-<uid>/<sock>, and that path has to fit sockaddr_un's
			// sun_path (104 bytes on darwin, 108 on linux) or the server dies with
			// "File name too long". So neither t.TempDir() (names the dir after this
			// long test) nor $TMPDIR (darwin's per-user one is ~56 chars on its own)
			// works as the base. /tmp is where tmux would have put the socket anyway,
			// so this keeps the filesystem it already used and only makes the dir
			// unique and removable.
			tmuxTmp, err := os.MkdirTemp("/tmp", "atr")
			if err != nil {
				t.Fatalf("tmux tmpdir: %v", err)
			}
			t.Cleanup(func() { _ = os.RemoveAll(tmuxTmp) })
			t.Setenv("TMUX_TMPDIR", tmuxTmp)

			// No '/' in the socket name: tmux reads -L as a path under
			// $TMUX_TMPDIR/tmux-<uid>, and a slash (t.Name carries the subtest path)
			// would point at a missing dir.
			sock := fmt.Sprintf("cfgparse-%d", rand.Int31())
			ctx := context.Background()
			// Clean probe server (no -f) kept alive by a session so source-file has a
			// target. Never the live socket.
			if out, err := exec.CommandContext(ctx, "tmux", "-L", sock, "new-session", "-d", "sleep 60").CombinedOutput(); err != nil {
				t.Fatalf("start probe tmux server: %v: %s", err, out)
			}
			defer func() { _ = exec.CommandContext(ctx, "tmux", "-L", sock, "kill-server").Run() }()

			// Prove TMUX_TMPDIR took effect rather than being silently ignored: the
			// live server's socket must sit somewhere under the temp root. Searching
			// by name keeps this off tmux's socket-dir layout, which the -L comment
			// above deliberately leaves to tmux.
			if !containsFile(t, tmuxTmp, sock) {
				t.Fatalf("probe socket %q not found under TMUX_TMPDIR %q: the socket is leaking into the shared socket dir", sock, tmuxTmp)
			}

			out, err := exec.CommandContext(ctx, "tmux", "-L", sock, "source-file", path).CombinedOutput()
			if msg := strings.TrimSpace(string(out)); err != nil || msg != "" {
				t.Fatalf("tmux rejected the rendered managed config (contextBar=%v): err=%v msg=%q\n---\n%s",
					contextBar, err, msg, rendered)
			}
		})
	}
}

// containsFile reports whether any entry named name exists anywhere under root.
func containsFile(t *testing.T, root, name string) bool {
	t.Helper()
	found := false
	if err := filepath.WalkDir(root, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && d.Name() == name {
			found = true
			return fs.SkipAll
		}
		return nil
	}); err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return found
}
