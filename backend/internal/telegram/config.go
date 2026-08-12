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

// Enabled reports whether the environment contains any Telegram setting. An
// empty configuration keeps Telegram disabled; a partial configuration is an
// attempted enablement and is rejected by Validate rather than silently
// running with a webhook that cannot authenticate or build links.
func (c Config) Enabled() bool {
	return strings.TrimSpace(c.BotToken) != "" ||
		strings.TrimSpace(c.WebhookSecret) != "" ||
		strings.TrimSpace(c.BotUsername) != "" ||
		strings.TrimSpace(c.WebhookPath) != ""
}

// Validate checks the complete configuration required when Telegram is
// enabled. Keeping this gate in one place prevents the runtime and dashboard
// from disagreeing about whether the bot is usable.
func (c Config) Validate() error {
	if !c.Enabled() {
		return nil
	}
	for _, required := range []struct {
		name  string
		value string
	}{
		{name: botTokenEnv, value: c.BotToken},
		{name: webhookSecretEnv, value: c.WebhookSecret},
		{name: botUsernameEnv, value: c.BotUsername},
		{name: webhookPathEnv, value: c.WebhookPath},
	} {
		if strings.TrimSpace(required.value) == "" {
			return errors.New(required.name + " must be set when Telegram is enabled")
		}
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
