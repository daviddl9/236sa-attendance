package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"
)

const (
	anthropicBaseURL      = "https://api.anthropic.com"
	anthropicAPIVersion   = "2023-06-01"
	defaultModel          = "claude-sonnet-4-6"
	defaultMaxTokens      = 8192
	defaultRequestTimeout = 90 * time.Second
)

// Client is the narrow interface the parser depends on. Implementations
// talk to Anthropic over HTTPS (production) or return canned responses
// (tests).
type Client interface {
	CreateMessage(ctx context.Context, req MessageRequest) (*MessageResponse, error)
}

// MessageRequest is the subset of the Messages API request body we use.
type MessageRequest struct {
	Model      string         `json:"model"`
	MaxTokens  int            `json:"max_tokens"`
	System     string         `json:"system,omitempty"`
	Messages   []Message      `json:"messages"`
	Tools      []Tool         `json:"tools,omitempty"`
	ToolChoice *ToolChoice    `json:"tool_choice,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

type Message struct {
	Role    string         `json:"role"`
	Content []ContentBlock `json:"content"`
}

// ContentBlock is a discriminated union (text / document / tool_use /
// tool_result). Fields not relevant to the chosen Type are omitted.
type ContentBlock struct {
	Type string `json:"type"`

	// Text block
	Text string `json:"text,omitempty"`

	// Document block
	Source *DocumentSource `json:"source,omitempty"`
	Title  string          `json:"title,omitempty"`

	// Tool use block (only ever populated on responses)
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
}

type DocumentSource struct {
	Type      string `json:"type"`       // "base64"
	MediaType string `json:"media_type"` // e.g. "application/pdf"
	Data      string `json:"data"`       // base64-encoded contents
}

type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"input_schema"`
}

type ToolChoice struct {
	Type string `json:"type"`           // "tool" | "auto" | "any"
	Name string `json:"name,omitempty"` // required when Type == "tool"
}

type MessageResponse struct {
	ID         string         `json:"id"`
	Type       string         `json:"type"`
	Role       string         `json:"role"`
	Content    []ContentBlock `json:"content"`
	Model      string         `json:"model"`
	StopReason string         `json:"stop_reason"`
}

// HTTPClient is the production Client backed by Anthropic's REST API.
type HTTPClient struct {
	apiKey  string
	baseURL string
	http    *http.Client
}

// NewHTTPClient reads ANTHROPIC_API_KEY from the process environment.
// Returns ErrAPIKeyNotConfigured when unset so callers can decide
// whether to disable the feature or error.
func NewHTTPClient() (*HTTPClient, error) {
	key := os.Getenv("ANTHROPIC_API_KEY")
	if key == "" {
		return nil, ErrAPIKeyNotConfigured
	}
	return &HTTPClient{
		apiKey:  key,
		baseURL: anthropicBaseURL,
		http:    &http.Client{Timeout: defaultRequestTimeout},
	}, nil
}

func (c *HTTPClient) CreateMessage(ctx context.Context, req MessageRequest) (*MessageResponse, error) {
	if req.Model == "" {
		req.Model = defaultModel
	}
	if req.MaxTokens == 0 {
		req.MaxTokens = defaultMaxTokens
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("agent: marshal request: %w", err)
	}

	endpoint, err := url.JoinPath(c.baseURL, "/v1/messages")
	if err != nil {
		return nil, fmt.Errorf("agent: build endpoint: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("agent: build request: %w", err)
	}
	httpReq.Header.Set("x-api-key", c.apiKey)
	httpReq.Header.Set("anthropic-version", anthropicAPIVersion)
	httpReq.Header.Set("content-type", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, ErrAPITimeout
		}
		return nil, fmt.Errorf("agent: send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("agent: read response: %w", err)
	}

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, ErrInvalidAPIKey
	}
	if resp.StatusCode == http.StatusRequestTimeout || resp.StatusCode == http.StatusGatewayTimeout {
		return nil, ErrAPITimeout
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("agent: anthropic API %d: %s", resp.StatusCode, summarize(respBody))
	}

	var out MessageResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, fmt.Errorf("agent: decode response: %w", err)
	}
	return &out, nil
}

// summarize returns a short, log-safe extract of an error body. It
// never includes the API key (the request body is never logged at all)
// but defensively truncates long responses.
func summarize(body []byte) string {
	const limit = 240
	if len(body) <= limit {
		return string(body)
	}
	return string(body[:limit]) + "…"
}
