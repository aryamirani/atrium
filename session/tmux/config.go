package tmux

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"text/template"

	"github.com/ZviBaratz/atrium/config"
	"github.com/ZviBaratz/atrium/ui/theme"
)

// socketName is the dedicated tmux socket Atrium runs all of its sessions on.
// Using a private socket (tmux -L) gives Atrium a fresh server, which is what
// makes the -f config below take effect (tmux only reads -f when a server starts)
// and keeps Atrium sessions out of the user's default `tmux ls`. It derives from
// config.RuntimeName so legacy installs stay on the "claudesquad" socket and keep
// their live sessions reachable after the rebrand.
func socketName() string {
	return config.RuntimeName()
}

// managedConfigFileName is the managed config materialized under the config dir,
// named to match the active brand (atrium.conf / claudesquad.conf).
func managedConfigFileName() string {
	return config.RuntimeName() + ".conf"
}

//go:embed atrium.conf.tmpl
var embeddedTmuxConfigTemplate string

// configOverridePath, when non-empty and pointing at an existing file, is used as
// the tmux config instead of the managed one. Set via Init from config.json.
var configOverridePath string

// statusLineRows is the number of rows Atrium's managed tmux status line (the
// in-session context bar) occupies, recorded by Init. A detached session is
// clientless, so its status line is never drawn and would not shrink the window's
// pane the way an attached client's status line does; applyDetachedGeometry subtracts
// these rows from the detached window height so the captured pane matches the
// attached-client pane area (and the preview's content height). It is 0 when the
// context bar is disabled, or when a user override owns the config (we make no
// assumption about an override's status line).
var statusLineRows int

// renderManagedConfig renders the embedded template. ContextBar toggles the header
// strip; BarBg/BarFg fill that strip's full-width background from the active theme's
// dedicated header-bar token (a slate a clear step above BgElevated) so the header
// reads as a distinct band over the agent's near-black pane.
func renderManagedConfig(contextBar bool) ([]byte, error) {
	tmpl, err := template.New("atrium.conf").Parse(embeddedTmuxConfigTemplate)
	if err != nil {
		return nil, fmt.Errorf("failed to parse managed tmux config template: %w", err)
	}
	th := theme.Current()
	data := struct {
		ContextBar   bool
		BarBg, BarFg string
	}{
		ContextBar: contextBar,
		BarBg:      string(th.Palette.BarBg),
		BarFg:      string(th.Palette.Fg),
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("failed to render managed tmux config: %w", err)
	}
	return buf.Bytes(), nil
}

// Init records an optional user-supplied tmux config override and, when none is
// set, materializes the bundled config into the config dir. contextBar toggles the
// in-session status line (config session_context_bar). The managed file is
// overwritten on every launch so it stays in sync with the binary. Call once at
// startup; it is idempotent and safe to call from both the TUI and daemon
// processes.
func Init(overridePath string, contextBar bool) error {
	configOverridePath = overridePath
	// Record how many rows the managed status line occupies so the clientless detached
	// resize can reproduce an attached client's pane area. An override owns its own
	// config, so make no assumption about its status line.
	statusLineRows = 0
	if overridePath == "" && contextBar {
		statusLineRows = 1
	}
	if overridePath != "" {
		return nil
	}
	dir, err := config.GetConfigDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	rendered, err := renderManagedConfig(contextBar)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, managedConfigFileName()), rendered, 0o644)
}

// tmuxConfigPath returns the path to pass via tmux -f: the override when it is set
// and exists, otherwise the managed file when it exists. Returns "" when neither
// is available — the command helper then omits -f and relies on socket isolation
// alone. The existence check matters: `tmux -f <missing-file>` fails outright, so
// we must never pass a path that isn't on disk (e.g. if Init failed to write it).
func tmuxConfigPath() string {
	if configOverridePath != "" {
		if _, err := os.Stat(configOverridePath); err == nil {
			return configOverridePath
		}
	}
	dir, err := config.GetConfigDir()
	if err != nil {
		return ""
	}
	managed := filepath.Join(dir, managedConfigFileName())
	if _, err := os.Stat(managed); err != nil {
		return ""
	}
	return managed
}
