// Package telegram contains the small Telegram Bot API client and update
// routing used by the attendance webhook.
package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const apiBaseURL = "https://api.telegram.org"

var ErrTokenNotConfigured = errors.New("telegram bot token is not configured")

// HTTPDoer is the subset of http.Client used by Client. Keeping the transport
// injectable makes API calls testable without contacting Telegram.
type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// Client is a thin JSON client for the Telegram Bot API.
type Client struct {
	token   string
	baseURL string
	http    HTTPDoer
}

// NewClient creates a Telegram client using the default HTTP transport.
func NewClient(token string) (*Client, error) {
	return NewClientWithTransport(token, http.DefaultClient)
}

// NewClientWithTransport creates a Telegram client with a substitutable HTTP
// transport. The token is kept private and is never included in returned
// errors.
func NewClientWithTransport(token string, transport HTTPDoer) (*Client, error) {
	if strings.TrimSpace(token) == "" {
		return nil, ErrTokenNotConfigured
	}
	if transport == nil {
		transport = http.DefaultClient
	}
	return &Client{token: token, baseURL: apiBaseURL, http: transport}, nil
}

type apiResponse struct {
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result"`
	ErrorCode   int             `json:"error_code"`
	Description string          `json:"description"`
}

type sendMessageRequest struct {
	ChatID int64  `json:"chat_id"`
	Text   string `json:"text"`
}

type answerCallbackQueryRequest struct {
	CallbackQueryID string `json:"callback_query_id"`
}

type setWebhookRequest struct {
	URL         string `json:"url"`
	SecretToken string `json:"secret_token"`
}

// SendMessage sends plain text to a Telegram chat. No parse mode is supplied,
// so user names containing formatting characters remain ordinary text.
func (c *Client) SendMessage(ctx context.Context, chatID int64, text string) error {
	return c.call(ctx, "sendMessage", sendMessageRequest{ChatID: chatID, Text: text}, nil)
}

// AnswerCallbackQuery acknowledges a callback query without sending a chat
// message. It is safe to call for callback updates that need no further action.
func (c *Client) AnswerCallbackQuery(ctx context.Context, callbackQueryID string) error {
	if callbackQueryID == "" {
		return errors.New("telegram callback query ID is required")
	}
	return c.call(ctx, "answerCallbackQuery", answerCallbackQueryRequest{CallbackQueryID: callbackQueryID}, nil)
}

// SetWebhook registers a webhook URL and Telegram's secret header value. This
// is deliberately a callable operation rather than something performed during
// API startup, so a restart cannot repoint a live bot unexpectedly.
func (c *Client) SetWebhook(ctx context.Context, webhookURL, secretToken string) error {
	if strings.TrimSpace(webhookURL) == "" {
		return errors.New("telegram webhook URL is required")
	}
	if strings.TrimSpace(secretToken) == "" {
		return errors.New("telegram webhook secret is required")
	}
	return c.call(ctx, "setWebhook", setWebhookRequest{URL: webhookURL, SecretToken: secretToken}, nil)
}

func (c *Client) call(ctx context.Context, method string, requestBody any, result any) error {
	body, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("telegram %s: encode request", method)
	}

	requestURL := strings.TrimRight(c.baseURL, "/") + "/bot" + c.token + "/" + method
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL, strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("telegram %s: create request", method)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		// Do not wrap err: net/http errors include the request URL, which would
		// expose the bot token.
		return fmt.Errorf("telegram %s: request failed", method)
	}
	defer func() { _ = resp.Body.Close() }()

	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("telegram %s: read response", method)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("telegram %s: API returned status %d", method, resp.StatusCode)
	}

	var response apiResponse
	if err := json.Unmarshal(responseBody, &response); err != nil {
		return fmt.Errorf("telegram %s: decode response", method)
	}
	if !response.OK {
		return fmt.Errorf("telegram %s: API request was not accepted", method)
	}
	if result != nil && len(response.Result) > 0 {
		if err := json.Unmarshal(response.Result, result); err != nil {
			return fmt.Errorf("telegram %s: decode result", method)
		}
	}
	return nil
}
