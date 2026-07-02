package app

import (
	"github.com/ZviBaratz/atrium/config"
	"github.com/ZviBaratz/atrium/log"

	tea "github.com/charmbracelet/bubbletea"
)

// handleAccountsState routes a key to the accounts overlay, persists on change, and
// reclaims the menu row when the panel closes. Persisting is all that's needed: new
// sessions and the create overlay read m.appConfig live; running sessions keep their
// already-injected env (they never re-resolve accounts).
func (m *home) handleAccountsState(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	closed, dirty := m.accountsOverlay.HandleKeyPress(msg)
	if dirty {
		if err := config.SaveConfig(m.appConfig); err != nil {
			log.WarningLog.Printf("failed to persist accounts: %v", err)
		}
		// A stashed create-form draft cached its account list at build time; drop it so
		// the next open rebuilds from live config and can't pin a just-deleted account.
		m.stashedDraft = nil
	}
	if closed {
		m.accountsOverlay = nil
		m.state = stateDefault
		m.recomputeLayout()
		return m, tea.WindowSize()
	}
	return m, nil
}
