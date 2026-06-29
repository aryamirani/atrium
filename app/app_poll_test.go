package app

import (
	"testing"

	"github.com/ZviBaratz/atrium/session"

	"github.com/stretchr/testify/require"
)

// TestPollTargets covers the metadata-poll throttle decision: a full sweep polls every
// active session, while a light tick polls only the selected session and any session
// with a queued prompt (so prompt delivery stays responsive). Throttling the rest is
// what keeps the shared tmux server from being hammered every 500ms.
func TestPollTargets(t *testing.T) {
	newInst := func(prompt string) *session.Instance {
		inst, err := session.NewInstance(session.InstanceOptions{
			Title: "s", Path: t.TempDir(), Program: "claude",
		})
		require.NoError(t, err)
		inst.Prompt = prompt
		return inst
	}

	plain := newInst("")
	selected := newInst("")
	queued := newInst("do the thing")
	active := []*session.Instance{plain, selected, queued}

	t.Run("full sweep returns every active session", func(t *testing.T) {
		require.Equal(t, active, pollTargets(active, selected, true))
	})

	t.Run("light tick polls only selected and queued-prompt sessions", func(t *testing.T) {
		got := pollTargets(active, selected, false)
		require.ElementsMatch(t, []*session.Instance{selected, queued}, got)
		require.NotContains(t, got, plain, "a stable non-selected session is skipped on light ticks")
	})

	t.Run("light tick with no selection still polls queued prompts", func(t *testing.T) {
		require.Equal(t, []*session.Instance{queued}, pollTargets(active, nil, false))
	})
}

// TestSweepMetadataNowCmdGuards pins the cheap exit: the one-shot detach sweep is skipped
// only when there is no active session to refresh. The sweep now covers the selected row
// too (polled face-value), so a lone selected session still yields a sweep.
func TestSweepMetadataNowCmdGuards(t *testing.T) {
	newInst := func() *session.Instance {
		inst, err := session.NewInstance(session.InstanceOptions{
			Title: "s", Path: t.TempDir(), Program: "claude",
		})
		require.NoError(t, err)
		return inst
	}
	selected, other := newInst(), newInst()

	require.Nil(t, sweepMetadataNowCmd(nil, selected),
		"no active sessions yields no sweep")
	require.NotNil(t, sweepMetadataNowCmd([]*session.Instance{selected}, selected),
		"the selected row alone still yields a sweep (it is refreshed face-value)")
	require.NotNil(t, sweepMetadataNowCmd([]*session.Instance{selected, other}, selected),
		"active rows present yield a sweep")
}
