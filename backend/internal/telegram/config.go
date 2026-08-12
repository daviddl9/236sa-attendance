package telegram

import (
	"errors"
	"os"
	"strings"
)

const (
	botTokenEnv      = "TELEGRAM_BOT_TOKEN"
	webhookSecretEnv = "TELEGRAM_WEBHOOK_SECRET"
	botUsernameEnv   = "TELEGRAM_BOT_USERNAME"
	webhookPathEnv   = "TELEGRAM_WEBHOOK_PATH"
)

// Config contains Telegram settings loaded from environment variables.
type Config struct {
	BotToken      string
	WebhookSecret string
	BotUsername   string
	WebhookPath   string
}

// LoadConfig reads Telegram settings without applying defaults that could
// accidentally enable the bot in an environment that has not configured it.
func LoadConfig() Config {
	return Config{
		BotToken:      os.Getenv(botTokenEnv),
		WebhookSecret: os.Getenv(webhookSecretEnv),
		BotUsername:   os.Getenv(botUsernameEnv),
		WebhookPath:   os.Getenv(webhookPathEnv),
	}
}

func (c Config) Enabled() bool {
	return strings.TrimSpace(c.BotToken) != ""
}

// Validate checks configuration that is required only when the bot is enabled.
// Telegram stays optional, but an enabled bot must have an independently
// configured webhook path.
func (c Config) Validate() error {
	if !c.Enabled() {
		return nil
	}
	if strings.TrimSpace(c.WebhookPath) == "" {
		return errors.New("TELEGRAM_WEBHOOK_PATH must be set when Telegram is enabled")
	}
	return nil
}

// WebhookPathSegment returns the configured path segment. It is deliberately
// independent from the secret used for header authentication.
func (c Config) WebhookPathSegment() string {
	return strings.TrimSpace(c.WebhookPath)
}

// WebhookURL returns the URL to pass to SetWebhook. Registration is explicit;
// this helper only composes the URL.
func (c Config) WebhookURL(baseURL string) string {
	return strings.TrimRight(baseURL, "/") + "/api/telegram/webhook/" + c.WebhookPathSegment()
}
