// Package agent encapsulates all calls to the Anthropic API used by the
// admin document-import flow (feature 002). External callers depend only
// on the Parser interface defined here, so the implementation can be
// swapped for a Claude Agent SDK sidecar without touching the handler
// layer.
package agent

import (
	"context"
	"errors"
)

// ParsedPersonnelRow is one personnel record extracted from a source
// document by the agent. Fields are intentionally not validated against
// the domain's rank/battery enums at this layer — that belongs to the
// commit-time handler so the preview can surface near-misses for the
// admin to correct.
type ParsedPersonnelRow struct {
	FullName  string            `json:"fullName"`
	Rank      string            `json:"rank,omitempty"`
	Battery   string            `json:"battery,omitempty"`
	NRICLast5 string            `json:"nricLast5"`
	Extras    map[string]string `json:"extras,omitempty"`

	// SourceSnippet is a verbatim quote from the document that the
	// agent used to derive this row, surfaced in the preview for
	// low-confidence rows so the admin can verify or correct them.
	SourceSnippet string `json:"sourceSnippet,omitempty"`

	// Confidence is the agent's self-reported confidence in [0, 1].
	// Rows below the confidence threshold are flagged in the preview.
	Confidence float64 `json:"confidence"`
}

// Parser turns a single uploaded document into structured personnel
// rows. Implementations call the Anthropic API behind the scenes; the
// caller stays ignorant of HTTP, prompts, and tool schemas.
type Parser interface {
	ParseDocument(ctx context.Context, filename string, contents []byte, mediaType string) ([]ParsedPersonnelRow, error)
}

// Typed errors returned by Parser implementations. Handlers and tests
// match on these via errors.Is so we never have to sniff error strings.
var (
	// ErrAPIKeyNotConfigured means ANTHROPIC_API_KEY is unset on the
	// server. The admin route returns a clear "key not configured"
	// response without retrying.
	ErrAPIKeyNotConfigured = errors.New("agent: ANTHROPIC_API_KEY not configured")

	// ErrInvalidAPIKey means Anthropic rejected the key (401). The key
	// value itself must never appear in logs or error messages.
	ErrInvalidAPIKey = errors.New("agent: anthropic rejected the API key")

	// ErrAPITimeout means the Anthropic call exceeded the configured
	// timeout. No retries are attempted in v1.
	ErrAPITimeout = errors.New("agent: anthropic API call timed out")

	// ErrDocumentTooLarge means the uploaded document exceeded the
	// per-import size cap (default 10 MB) and was rejected before any
	// API call.
	ErrDocumentTooLarge = errors.New("agent: document exceeds size limit")

	// ErrNoRowsDetected means the agent finished parsing but returned
	// zero personnel rows. The handler surfaces this to the admin
	// rather than committing an empty import.
	ErrNoRowsDetected = errors.New("agent: no personnel rows detected in document")
)
