package telegram

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"
)

const (
	botTokenEnv      = "TELEGRAM_BOT_TOKEN"
	webhookSecretEnv = "TELEGRAM_WEBHOOK_SECRET"
	botUsernameEnv   = "TELEGRAM_BOT_USERNAME"
)

// Config contains Telegram settings loaded from environment variables.
type Config struct {
	BotToken      string
	WebhookSecret string
	BotUsername   string
}

// LoadConfig reads Telegram settings without applying defaults that could
// accidentally enable the bot in an environment that has not configured it.
func LoadConfig() Config {
	return Config{
		BotToken:      os.Getenv(botTokenEnv),
		WebhookSecret: os.Getenv(webhookSecretEnv),
		BotUsername:   os.Getenv(botUsernameEnv),
	}
}

func (c Config) Enabled() bool {
	return strings.TrimSpace(c.BotToken) != ""
}

// WebhookPathSegment derives a stable, opaque path segment from the configured
// secret. It is separate from the header value, so a URL logged by a proxy
// does not disclose the value Telegram must echo in its header.
func (c Config) WebhookPathSegment() string {
	digest := sha256.Sum256([]byte(c.WebhookSecret))
	return hex.EncodeToString(digest[:8])
}

// WebhookURL returns the URL to pass to SetWebhook. Registration is explicit;
// this helper only composes the URL.
func (c Config) WebhookURL(baseURL string) string {
	return strings.TrimRight(baseURL, "/") + "/api/telegram/webhook/" + c.WebhookPathSegment()
}
