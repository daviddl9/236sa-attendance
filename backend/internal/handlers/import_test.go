package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/davidlivingston/go-nextjs-starter/backend/internal/middleware"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/models"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/services/agent"
)

// TestPreviewImport_DisabledWhenParserNil exercises the short-circuit
// path: when ANTHROPIC_API_KEY is unset on the server (parser == nil),
// the handler must return 503 *before* touching the DB or the request
// body. This keeps the rest of the app working when the key is missing.
func TestPreviewImport_DisabledWhenParserNil(t *testing.T) {
	h := &ImportHandler{db: nil, parser: nil}

	req := httptest.NewRequest(http.MethodPost, "/api/admin/users/import-document/preview", nil)
	rec := httptest.NewRecorder()

	h.PreviewImport(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	if !strings.Contains(rec.Body.String(), "ANTHROPIC_API_KEY") {
		t.Errorf("response should mention ANTHROPIC_API_KEY, got %q", rec.Body.String())
	}
}

// TestPreviewImport_RequiresAuthenticatedAdmin: when the request has no
// user in context, return 401 even though parser is configured.
func TestPreviewImport_RequiresAuthenticatedAdmin(t *testing.T) {
	h := &ImportHandler{db: nil, parser: &noopParser{}}

	body, contentType := buildMultipartBody(t, "roster.xlsx", []byte("ignored"))
	req := httptest.NewRequest(http.MethodPost, "/api/admin/users/import-document/preview", body)
	req.Header.Set("content-type", contentType)
	rec := httptest.NewRecorder()

	h.PreviewImport(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestImportStatusFromError(t *testing.T) {
	tests := []struct {
		err  error
		want int
	}{
		{agent.ErrAPIKeyNotConfigured, http.StatusServiceUnavailable},
		{agent.ErrInvalidAPIKey, http.StatusServiceUnavailable},
		{agent.ErrDocumentTooLarge, http.StatusRequestEntityTooLarge},
		{agent.ErrAPITimeout, http.StatusGatewayTimeout},
		{agent.ErrNoRowsDetected, http.StatusUnprocessableEntity},
		{errors.New("agent: unsupported media type \"x\""), http.StatusUnsupportedMediaType},
		{errors.New("unrelated boom"), http.StatusInternalServerError},
	}
	for _, tt := range tests {
		got := importStatusFromError(tt.err)
		if got != tt.want {
			t.Errorf("importStatusFromError(%v) = %d, want %d", tt.err, got, tt.want)
		}
	}
}

func TestImportMessageFromError(t *testing.T) {
	tests := []struct {
		err      error
		wantPart string
	}{
		{agent.ErrAPIKeyNotConfigured, "ANTHROPIC_API_KEY"},
		{agent.ErrInvalidAPIKey, "rejected"},
		{agent.ErrAPITimeout, "timed out"},
		{agent.ErrNoRowsDetected, "No personnel rows"},
		{agent.ErrDocumentTooLarge, "limit"},
	}
	for _, tt := range tests {
		got := importMessageFromError(tt.err)
		if !strings.Contains(got, tt.wantPart) {
			t.Errorf("importMessageFromError(%v) = %q, want substring %q", tt.err, got, tt.wantPart)
		}
	}
}

// TestWriteJSON ensures the helper sets content-type and writes valid JSON.
func TestWriteJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	writeJSON(rec, http.StatusTeapot, map[string]int{"x": 1})
	if rec.Code != http.StatusTeapot {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusTeapot)
	}
	if ct := rec.Header().Get("content-type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("content-type = %q, want json", ct)
	}
	var out map[string]int
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	if out["x"] != 1 {
		t.Errorf("body decoded wrong: %+v", out)
	}
}

// --- CommitImport unit tests (stateless paths only) ---

// TestCommitImport_RequiresAuthentication checks that a request with no
// user in context is rejected before touching the DB.
func TestCommitImport_RequiresAuthentication(t *testing.T) {
	h := &ImportHandler{db: nil, parser: &noopParser{}}

	body, _ := buildJSONBody(t, CommitRequest{
		ImportID:  "some-id",
		MergeMode: "fill_gaps",
		Rows:      []CommitRow{{FullName: "TAN AH KOW", NRICLast5: "1234A"}},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/admin/users/import-document/commit", body)
	req.Header.Set("content-type", "application/json")
	rec := httptest.NewRecorder()

	h.CommitImport(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

// TestCommitImport_RejectsBadMergeMode ensures 400 on an unknown merge mode.
func TestCommitImport_RejectsBadMergeMode(t *testing.T) {
	h := &ImportHandler{db: nil, parser: &noopParser{}}

	body, _ := buildJSONBody(t, CommitRequest{
		ImportID:  "some-id",
		MergeMode: "destroy_all",
		Rows:      []CommitRow{{FullName: "TAN AH KOW", NRICLast5: "1234A"}},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/admin/users/import-document/commit", body)
	req.Header.Set("content-type", "application/json")
	req = req.WithContext(ctxWithUser(req.Context()))
	rec := httptest.NewRecorder()

	h.CommitImport(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

// TestCommitImport_RejectsEmptyRows ensures 400 when rows slice is empty.
func TestCommitImport_RejectsEmptyRows(t *testing.T) {
	h := &ImportHandler{db: nil, parser: &noopParser{}}

	body, _ := buildJSONBody(t, CommitRequest{
		ImportID:  "some-id",
		MergeMode: "fill_gaps",
		Rows:      []CommitRow{},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/admin/users/import-document/commit", body)
	req.Header.Set("content-type", "application/json")
	req = req.WithContext(ctxWithUser(req.Context()))
	rec := httptest.NewRecorder()

	h.CommitImport(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

// TestCommitImport_RejectsMissingImportID ensures 400 when importId is empty.
func TestCommitImport_RejectsMissingImportID(t *testing.T) {
	h := &ImportHandler{db: nil, parser: &noopParser{}}

	body, _ := buildJSONBody(t, CommitRequest{
		ImportID:  "",
		MergeMode: "fill_gaps",
		Rows:      []CommitRow{{FullName: "TAN AH KOW", NRICLast5: "1234A"}},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/admin/users/import-document/commit", body)
	req.Header.Set("content-type", "application/json")
	req = req.WithContext(ctxWithUser(req.Context()))
	rec := httptest.NewRecorder()

	h.CommitImport(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

// TestPrepareCommitRows_SkipClassification verifies that invalid rows are
// classified with the correct skipReason before any DB access occurs.
// This test passes only rows that short-circuit in the validation switch
// so no DB connection is needed.
func TestPrepareCommitRows_SkipClassification(t *testing.T) {
	h := &ImportHandler{db: nil, parser: nil}

	// Only rows that hit the skip path (validation failures) — the DB lookup
	// never runs for these, so a nil DB is safe.
	rows := []CommitRow{
		{FullName: "LEE BENG", NRICLast5: "ABCDE"}, // invalid: not 4 digits + letter
		{FullName: "", NRICLast5: "5678B"},          // invalid: missing name
		{FullName: "ONG TECK", NRICLast5: "12345"},  // invalid: digits only, no letter
	}

	result, err := h.prepareCommitRows(context.Background(), rows)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("len = %d, want 3", len(result))
	}

	tests := []struct{ want string }{
		{"invalid_nric_format"},
		{"missing_full_name"},
		{"invalid_nric_format"},
	}
	for i, tt := range tests {
		if result[i].skipReason != tt.want {
			t.Errorf("row %d: skipReason = %q, want %q", i, result[i].skipReason, tt.want)
		}
	}
}

// --- helpers ---

type noopParser struct{}

func (noopParser) ParseDocument(_ context.Context, _ string, _ []byte, _ string) ([]agent.ParsedPersonnelRow, error) {
	return nil, agent.ErrNoRowsDetected
}

func buildJSONBody(t *testing.T, v any) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(v); err != nil {
		t.Fatalf("encode JSON body: %v", err)
	}
	return &buf, "application/json"
}

// ctxWithUser injects a minimal fake user into a context so that
// middleware.GetUserFromContext returns (user, true).
func ctxWithUser(ctx context.Context) context.Context {
	user := &models.User{ID: "test-admin", IsSuperadmin: true}
	return context.WithValue(ctx, middleware.UserKey, user)
}

func buildMultipartBody(t *testing.T, filename string, contents []byte) (*bytes.Buffer, string) {
	t.Helper()
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	fw, err := w.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := fw.Write(contents); err != nil {
		t.Fatalf("write contents: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	return body, fmt.Sprintf("multipart/form-data; boundary=%s", w.Boundary())
}
