package config

import (
	"fmt"
	"os"

	"github.com/spf13/viper"
)

// AppConfig holds the parsed configuration from Viper
type AppConfig struct {
	PushoverPushToken string `mapstructure:"PUSHOVER_PUSH_TOKEN"`
	PushoverPushUser  string `mapstructure:"PUSHOVER_PUSH_USER"`
	TelegramBotToken  string `mapstructure:"TELEGRAM_BOT_TOKEN"`
	TelegramChatID    string `mapstructure:"TELEGRAM_CHAT_ID"`
}

// Load uses Viper to read the ~/.camply configuration file
func Load() (*AppConfig, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("could not find user home directory: %w", err)
	}

	viper.SetConfigName(".camply")
	viper.SetConfigType("env")
	viper.AddConfigPath(homeDir)

	// Also allow environment variables to override the config file.
	viper.AutomaticEnv()

	// AutomaticEnv on its own does not reach Unmarshal: Viper only unmarshals
	// keys it already knows about, so a value supplied purely through the
	// environment — as it is under Kubernetes, where these arrive from a
	// Secret via envFrom — is silently dropped. Bind each key explicitly so
	// the environment alone is enough to configure notifications.
	for _, key := range []string{
		"PUSHOVER_PUSH_TOKEN",
		"PUSHOVER_PUSH_USER",
		"TELEGRAM_BOT_TOKEN",
		"TELEGRAM_CHAT_ID",
	} {
		if err := viper.BindEnv(key); err != nil {
			return nil, fmt.Errorf("unable to bind %s: %w", key, err)
		}
	}

	var config AppConfig

	// Read the config file if it exists, otherwise it will just use defaults/env vars
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
	}

	if err := viper.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("unable to decode into struct: %w", err)
	}

	return &config, nil
}
