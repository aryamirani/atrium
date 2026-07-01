package app

import (
	"fmt"
	"strings"

	"github.com/ZviBaratz/atrium/config"
	"github.com/ZviBaratz/atrium/log"
	"github.com/ZviBaratz/atrium/ui"

	tea "github.com/charmbracelet/bubbletea"
)

// detectAgents is the agent-detection seam. A package var (matching the app's
// checkForUpdate/cleanupTerminalForInstance idiom) so tests inject a fake PATH.
var detectAgents = config.DetectAgentProfiles

// agentsDetectedMsg carries the result of async agent detection back to Update.
type agentsDetectedMsg struct {
	profiles []config.Profile
}

// detectAgentsCmd probes for installed agent CLIs off the main loop (detection
// shells out per agent) and delivers the result as an agentsDetectedMsg.
func (m *home) detectAgentsCmd() tea.Cmd {
	return func() tea.Msg {
		return agentsDetectedMsg{profiles: detectAgents()}
	}
}

// handleWelcomeState routes a key to the welcome overlay. On confirm it merges
// the detected agents into the profiles, persists the picked default program,
// and retires the welcome; on skip it just closes (the seen-bit stays unset so
// the welcome re-shows until the user engages or creates a session).
func (m *home) handleWelcomeState(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	shouldClose := m.welcomeOverlay.HandleKeyPress(msg)
	if !shouldClose {
		return m, nil
	}
	confirmed := m.welcomeOverlay.Confirmed()
	detected := m.welcomeOverlay.Detected()
	program := m.welcomeOverlay.SelectedProgram()
	m.state = stateDefault
	m.welcomeOverlay = nil

	if !confirmed {
		return m, nil
	}
	// Confirm: adopt the detected agents as profiles and persist the pick.
	m.appConfig.MergeDetectedProfiles(detected)
	if program != "" {
		m.appConfig.DefaultProgram = program
		m.program = program
	}
	if err := config.SaveConfig(m.appConfig); err != nil {
		log.WarningLog.Printf("failed to persist welcome setup: %v", err)
	}
	if seen := m.appState.GetHelpScreensSeen(); seen&(helpTypeWelcome{}.mask()) == 0 {
		if err := m.appState.SetHelpScreensSeen(seen | helpTypeWelcome{}.mask()); err != nil {
			log.WarningLog.Printf("failed to persist welcome-seen state: %v", err)
		}
	}
	return m, nil
}

// maybeWarnMissingProgram surfaces a one-shot, non-blocking warning when the
// effective program is not installed. Used on launches where the welcome does
// not show (returning users). It never opens a modal.
func (m *home) maybeWarnMissingProgram() tea.Cmd {
	if m.pathWarned || config.ProgramInstalled(m.program) {
		return nil
	}
	m.pathWarned = true
	cmd := firstToken(m.program)
	text := fmt.Sprintf("%s not found on PATH — press , to change the default program", cmd)
	if m.menuVisible() && m.menu != nil {
		m.menu.SetNotice(text, ui.NoticeError)
	} else {
		m.errBox.SetError(fmt.Errorf("%s", text))
		m.recomputeLayout()
	}
	return m.scheduleNoticeHide()
}

// firstToken returns the first whitespace-separated token of s (the command
// name of a program string like "aider --model x"), or s when there is none.
func firstToken(s string) string {
	if fields := strings.Fields(s); len(fields) > 0 {
		return fields[0]
	}
	return s
}
