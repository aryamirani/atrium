package ui

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMenu_NewInstanceHintShownOnlyWhileNaming(t *testing.T) {
	m := NewMenu()
	m.SetSize(200, 3)
	m.SetNewInstanceHint("myrepo")

	m.SetState(StateNewInstance)
	require.Contains(t, m.String(), "myrepo", "hint should show while naming a new session")

	// The hint must not leak into other states.
	m.SetState(StateDefault)
	require.NotContains(t, m.String(), "myrepo")
}
