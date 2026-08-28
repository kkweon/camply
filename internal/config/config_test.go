package config

import (
	"testing"

	"github.com/spf13/viper"
)

// Under Kubernetes the notification credentials arrive purely from the
// environment (a Secret mounted with envFrom) and no ~/.camply file exists.
// viper.AutomaticEnv does not feed Unmarshal, so without an explicit BindEnv
// every field came back empty and camply silently refused to notify.
func TestLoadReadsCredentialsFromEnvironmentAlone(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	// An empty HOME means there is no .camply file to read.
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PUSHOVER_PUSH_TOKEN", "push-token")
	t.Setenv("PUSHOVER_PUSH_USER", "push-user")
	t.Setenv("TELEGRAM_BOT_TOKEN", "bot-token")
	t.Setenv("TELEGRAM_CHAT_ID", "chat-id")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned an error: %v", err)
	}

	for _, tc := range []struct {
		name string
		got  string
		want string
	}{
		{"PushoverPushToken", cfg.PushoverPushToken, "push-token"},
		{"PushoverPushUser", cfg.PushoverPushUser, "push-user"},
		{"TelegramBotToken", cfg.TelegramBotToken, "bot-token"},
		{"TelegramChatID", cfg.TelegramChatID, "chat-id"},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %q, want %q", tc.name, tc.got, tc.want)
		}
	}
}
