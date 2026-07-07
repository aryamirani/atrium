package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetNotifications(t *testing.T) {
	require.Equal(t, NotificationsOff, (*Config)(nil).GetNotifications(), "nil config defaults to off")
	require.Equal(t, NotificationsOff, (&Config{}).GetNotifications(), "empty value (old config) defaults to off")
	require.Equal(t, NotificationsOff, (&Config{Notifications: "bogus"}).GetNotifications(), "unknown normalizes to off")
	require.Equal(t, NotificationsBell, (&Config{Notifications: NotificationsBell}).GetNotifications())
	require.Equal(t, NotificationsDesktop, (&Config{Notifications: NotificationsDesktop}).GetNotifications())
}

func TestGetNotifyCommand(t *testing.T) {
	require.Equal(t, "", (*Config)(nil).GetNotifyCommand(), "nil config yields empty command")
	require.Equal(t, "", (&Config{}).GetNotifyCommand(), "unset command is empty")
	require.Equal(t, "notify-send x", (&Config{NotifyCommand: "notify-send x"}).GetNotifyCommand())
}
