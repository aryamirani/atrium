package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetGroupMode(t *testing.T) {
	require.Equal(t, GroupModeRepo, (*Config)(nil).GetGroupMode(), "nil config defaults to repo")
	require.Equal(t, GroupModeRepo, (&Config{}).GetGroupMode(), "empty value defaults to repo")
	require.Equal(t, GroupModeRepo, (&Config{GroupMode: "bogus"}).GetGroupMode(), "unknown normalizes to repo")
	require.Equal(t, GroupModeAccount, (&Config{GroupMode: GroupModeAccount}).GetGroupMode())
}
