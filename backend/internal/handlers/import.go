package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/davidlivingston/go-nextjs-starter/backend/internal/database"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/middleware"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/models"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/services/agent"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

const (
	importUploadMemoryLimit = 12 << 20 // 12 MiB room for the multipart envelope
)

// MatchStatus labels a parsed row against the user table.
type MatchStatus string

const (
	MatchStatusNew      MatchStatus = "new"
	MatchStatusExisting MatchStatus = "existing_match"
	// MatchStatusInvalid means the row failed the feature-001 NRIC
	// format check or is missing required fields; never importable.
	MatchStatusInvalid MatchStatus = "invalid"
)

// PreviewedRow is the row shape returned to the admin for confirmation.
type PreviewedRow struct {
	FullName      string            `json:"fullName"`
	Rank          string            `json:"rank,omitempty"`
	Battery       string            `json:"battery,omitempty"`
	NRICLast5     string            `json:"nricLast5"`
	Extras        map[string]string `json:"extras,omitempty"`
	SourceSnippet string            `json:"sourceSnippet,omitempty"`
	Confidence    float64           `json:"confidence"`
	MatchStatus   MatchStatus       `json:"matchStatus"`
	ExistingID    *string           `json:"existingId,omitempty"`
	ValidationMsg string            `json:"validationMsg,omitempty"`
}

// PreviewResponse is the JSON body returned by POST .../preview.
type PreviewResponse struct {
	ImportID  string         `json:"importId"`
	Filename  string         `json:"filename"`
	RowCounts models.RowCounts `json:"rowCounts"`
	Rows      []PreviewedRow `json:"rows"`
}

// ImportHandler holds the dependencies for the admin document-import
// flow. Parser is an interface so tests can inject a fake without
// hitting the real Anthropic API.
type ImportHandler struct {
	db     *database.DB
	parser agent.Parser
}

// NewImportHandler builds an ImportHandler. parser may be nil — when
// nil, the routes return a clear "key not configured" error rather
// than panicking, so the rest of the app keeps working without an
// Anthropic API key set.
func NewImportHandler(db *database.DB, parser agent.Parser) *ImportHandler {
	return &ImportHandler{db: db, parser: parser}
}

// PreviewImport handles POST /api/admin/users/import-document/preview.
// It validates the multipart upload, persists a pending import row,
// invokes the agent parser, computes match status against the user
// table, and returns the structured preview. **No row in the `user`
// table is written here.**
func (h *ImportHandler) PreviewImport(w http.ResponseWriter, r *http.Request) {
	if h.parser == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "ANTHROPIC_API_KEY is not configured on the server")
		return
	}

	user, ok := middleware.GetUserFromContext(r.Context())
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "Not authenticated")
		return
	}

	if err := r.ParseMultipartForm(importUploadMemoryLimit); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Failed to parse upload")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "No file uploaded — expected multipart field 'file'")
		return
	}
	defer file.Close()

	contents, err := io.ReadAll(file)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "Failed to read uploaded file")
		return
	}
	if len(contents) == 0 {
		writeJSONError(w, http.StatusBadRequest, "Uploaded file is empty")
		return
	}
	if len(contents) > agent.DefaultMaxDocumentBytes {
		writeJSONError(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("File exceeds %d MB limit", agent.DefaultMaxDocumentBytes/(1024*1024)))
		return
	}

	ctx := r.Context()
	importID := generateID()
	if err := insertPendingImport(ctx, h.db, importID, user.ID, header.Filename); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "Failed to record import job")
		return
	}

	rows, err := h.parser.ParseDocument(ctx, header.Filename, contents, header.Header.Get("Content-Type"))
	if err != nil {
		_ = markImportFailed(ctx, h.db, importID, err)
		writeJSONError(w, importStatusFromError(err), importMessageFromError(err))
		return
	}

	previewed, counts := h.labelRows(ctx, rows)
	if err := markImportPreviewed(ctx, h.db, importID, counts); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "Failed to persist preview state")
		return
	}

	writeJSON(w, http.StatusOK, PreviewResponse{
		ImportID:  importID,
		Filename:  header.Filename,
		RowCounts: counts,
		Rows:      previewed,
	})
}

// labelRows runs the per-row lookup against the user table. The lookup
// is done one row at a time intentionally: rosters are small (< ~200
// rows) and per-row queries keep this code easy to reason about.
func (h *ImportHandler) labelRows(ctx context.Context, parsed []agent.ParsedPersonnelRow) ([]PreviewedRow, models.RowCounts) {
	out := make([]PreviewedRow, 0, len(parsed))
	var counts models.RowCounts

	for _, row := range parsed {
		p := PreviewedRow{
			FullName:      row.FullName,
			Rank:          row.Rank,
			Battery:       row.Battery,
			NRICLast5:     row.NRICLast5,
			Extras:        row.Extras,
			SourceSnippet: row.SourceSnippet,
			Confidence:    row.Confidence,
		}

		if p.FullName == "" || !agent.IsValidNRICLast5(p.NRICLast5) {
			p.MatchStatus = MatchStatusInvalid
			if p.FullName == "" {
				p.ValidationMsg = "missing full name"
			} else {
				p.ValidationMsg = nricLast5FormatMessage
			}
			counts.Skipped++
			out = append(out, p)
			continue
		}

		existingID, err := findExistingUserID(ctx, h.db, p.FullName, p.NRICLast5)
		switch {
		case err == nil && existingID != "":
			p.MatchStatus = MatchStatusExisting
			id := existingID
			p.ExistingID = &id
			counts.Updated++
		default:
			p.MatchStatus = MatchStatusNew
			counts.Created++
		}
		out = append(out, p)
	}

	return out, counts
}

// findExistingUserID returns the user.id (or "") for the user matching
// the given (full_name, nric_last5). The query is case-insensitive on
// full_name and exact on NRIC.
func findExistingUserID(ctx context.Context, db *database.DB, fullName, nricLast5 string) (string, error) {
	var id string
	err := db.Pool.QueryRow(ctx, `
		SELECT id FROM "user"
		WHERE upper("full_name") = upper($1) AND "nric_last5" = $2
		LIMIT 1
	`, fullName, nricLast5).Scan(&id)
	if err != nil {
		return "", err
	}
	return id, nil
}

// --- Commit types ---

// CommitRow is the minimal per-row shape the admin sends back for commit.
// The server re-derives existingID — the client's existingId is ignored.
type CommitRow struct {
	FullName  string `json:"fullName"`
	Rank      string `json:"rank,omitempty"`
	Battery   string `json:"battery,omitempty"`
	NRICLast5 string `json:"nricLast5"`
}

// CommitRequest is the body for POST .../commit.
type CommitRequest struct {
	ImportID  string           `json:"importId"`
	MergeMode models.MergeMode `json:"mergeMode"`
	Rows      []CommitRow      `json:"rows"`
}

// CommitResponse is returned on a successful commit.
type CommitResponse struct {
	ImportID  string           `json:"importId"`
	RowCounts models.RowCounts `json:"rowCounts"`
}

// preparedRow is an intermediate shape computed before opening the
// transaction: validation result, existing user lookup, and bcrypt hash
// for new rows are all done outside the transaction to keep it short.
type preparedRow struct {
	CommitRow
	existingID string // "" means this will be a new user
	skipReason string // non-empty means this row is skipped
	pwdHash    string // only set for new (existingID=="") valid rows
}

// CommitImport handles POST /api/admin/users/import-document/commit.
// It validates the import is in 'previewed' state, then processes every
// row in a single pgx transaction — creating new users or updating
// existing ones according to the chosen merge mode — and writes an audit
// trail to personnel_import_changes. NRIC Last 5 and password columns
// are never written for existing users regardless of merge mode.
func (h *ImportHandler) CommitImport(w http.ResponseWriter, r *http.Request) {
	_, ok := middleware.GetUserFromContext(r.Context())
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "Not authenticated")
		return
	}

	var req CommitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if req.ImportID == "" {
		writeJSONError(w, http.StatusBadRequest, "importId is required")
		return
	}
	if req.MergeMode != models.MergeFillGaps && req.MergeMode != models.MergeOverride {
		writeJSONError(w, http.StatusBadRequest, "mergeMode must be 'fill_gaps' or 'override'")
		return
	}
	if len(req.Rows) == 0 {
		writeJSONError(w, http.StatusBadRequest, "rows must not be empty")
		return
	}

	ctx := r.Context()

	var currentStatus string
	if err := h.db.Pool.QueryRow(ctx,
		`SELECT status FROM personnel_imports WHERE id = $1`, req.ImportID,
	).Scan(&currentStatus); err != nil {
		writeJSONError(w, http.StatusNotFound, "Import record not found")
		return
	}
	if currentStatus != string(models.ImportStatusPreviewed) {
		writeJSONError(w, http.StatusConflict, fmt.Sprintf("import is '%s', expected 'previewed'", currentStatus))
		return
	}

	prepared, err := h.prepareCommitRows(ctx, req.Rows)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "Failed to prepare rows: "+err.Error())
		return
	}

	counts, err := h.executeCommitTx(ctx, req.ImportID, req.MergeMode, prepared)
	if err != nil {
		_ = markImportFailed(ctx, h.db, req.ImportID, err)
		writeJSONError(w, http.StatusInternalServerError, "Import transaction failed: "+err.Error())
		return
	}

	countsJSON, _ := json.Marshal(counts)
	now := time.Now().UTC()
	_, _ = h.db.Pool.Exec(ctx, `
		UPDATE personnel_imports
		SET status = $1, merge_mode = $2, row_counts = $3, finished_at = $4
		WHERE id = $5
	`, string(models.ImportStatusCommitted), string(req.MergeMode), countsJSON, now, req.ImportID)

	writeJSON(w, http.StatusOK, CommitResponse{ImportID: req.ImportID, RowCounts: counts})
}

// prepareCommitRows validates each row, looks up existing users, and
// pre-hashes passwords for new rows — all outside the transaction so
// the transaction is as short as possible.
func (h *ImportHandler) prepareCommitRows(ctx context.Context, rows []CommitRow) ([]preparedRow, error) {
	out := make([]preparedRow, 0, len(rows))
	for _, row := range rows {
		p := preparedRow{CommitRow: row}
		switch {
		case row.FullName == "":
			p.skipReason = "missing_full_name"
		case !agent.IsValidNRICLast5(row.NRICLast5):
			p.skipReason = "invalid_nric_format"
		default:
			p.existingID, _ = findExistingUserID(ctx, h.db, row.FullName, row.NRICLast5)
			if p.existingID == "" {
				hash, err := bcrypt.GenerateFromPassword([]byte(row.NRICLast5), 4)
				if err != nil {
					return nil, fmt.Errorf("hashing password for %q: %w", row.FullName, err)
				}
				p.pwdHash = string(hash)
			}
		}
		out = append(out, p)
	}
	return out, nil
}

// executeCommitTx opens a single transaction and writes all rows. On
// any error the transaction is rolled back and the error is returned.
func (h *ImportHandler) executeCommitTx(ctx context.Context, importID string, mode models.MergeMode, rows []preparedRow) (models.RowCounts, error) {
	tx, err := h.db.Pool.Begin(ctx)
	if err != nil {
		return models.RowCounts{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	now := time.Now().UTC()
	var counts models.RowCounts

	for _, p := range rows {
		if p.skipReason != "" {
			if err := txInsertSkipChange(ctx, tx, importID, p.skipReason, now); err != nil {
				return models.RowCounts{}, err
			}
			counts.Skipped++
			continue
		}
		if p.existingID != "" {
			if err := txUpdateExistingUser(ctx, tx, importID, p.existingID, p.CommitRow, mode, now); err != nil {
				return models.RowCounts{}, err
			}
			counts.Updated++
		} else {
			created, err := txInsertNewUser(ctx, tx, importID, p.CommitRow, p.pwdHash, now)
			if err != nil {
				return models.RowCounts{}, err
			}
			if created {
				counts.Created++
			} else {
				counts.Skipped++ // duplicate constraint
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return models.RowCounts{}, err
	}
	return counts, nil
}

// txInsertNewUser inserts a new user row and a 'created' change record.
// Returns (false, nil) when the insert hits a unique constraint so the
// caller can count it as skipped rather than failing the whole import.
func txInsertNewUser(ctx context.Context, tx pgx.Tx, importID string, row CommitRow, pwdHash string, now time.Time) (bool, error) {
	userID := generateID()
	_, err := tx.Exec(ctx, `
		INSERT INTO "user"
			(id, "full_name", rank, battery, "nric_last5", password, extras, "createdAt", "updatedAt")
		VALUES ($1, $2, $3, $4, $5, $6, '{}'::jsonb, $7, $8)
	`, userID, row.FullName, row.Rank, row.Battery, row.NRICLast5, pwdHash, now, now)
	if err != nil {
		if strings.Contains(err.Error(), "unique") || strings.Contains(err.Error(), "duplicate") {
			return false, txInsertSkipChange(ctx, tx, importID, "duplicate_entry", now)
		}
		return false, err
	}
	changeID := generateID()
	_, err = tx.Exec(ctx, `
		INSERT INTO personnel_import_changes (id, import_id, user_id, action, "createdAt")
		VALUES ($1, $2, $3, $4, $5)
	`, changeID, importID, userID, string(models.ImportActionCreated), now)
	return true, err
}

// txUpdateExistingUser updates rank/battery on an existing user
// according to the merge mode, then records field-level change rows.
// NRIC Last 5 and password are never touched.
func txUpdateExistingUser(ctx context.Context, tx pgx.Tx, importID, userID string, row CommitRow, mode models.MergeMode, now time.Time) error {
	var curRank, curBattery string
	_ = tx.QueryRow(ctx,
		`SELECT COALESCE(rank, ''), COALESCE(battery, '') FROM "user" WHERE id = $1`, userID,
	).Scan(&curRank, &curBattery)

	newRank, newBattery := row.Rank, row.Battery
	if mode == models.MergeFillGaps {
		if curRank != "" {
			newRank = curRank
		}
		if curBattery != "" {
			newBattery = curBattery
		}
	}

	_, err := tx.Exec(ctx,
		`UPDATE "user" SET rank = $1, battery = $2, "updatedAt" = $3 WHERE id = $4`,
		newRank, newBattery, now, userID,
	)
	if err != nil {
		return err
	}

	if newRank != curRank {
		if err := txInsertChangeRow(ctx, tx, importID, userID, "rank", curRank, newRank, now); err != nil {
			return err
		}
	}
	if newBattery != curBattery {
		if err := txInsertChangeRow(ctx, tx, importID, userID, "battery", curBattery, newBattery, now); err != nil {
			return err
		}
	}
	if newRank == curRank && newBattery == curBattery {
		changeID := generateID()
		_, err = tx.Exec(ctx, `
			INSERT INTO personnel_import_changes (id, import_id, user_id, action, reason, "createdAt")
			VALUES ($1, $2, $3, $4, $5, $6)
		`, changeID, importID, userID, string(models.ImportActionUpdated), "no_change", now)
		return err
	}
	return nil
}

func txInsertChangeRow(ctx context.Context, tx pgx.Tx, importID, userID, field, before, after string, now time.Time) error {
	changeID := generateID()
	_, err := tx.Exec(ctx, `
		INSERT INTO personnel_import_changes
			(id, import_id, user_id, action, field_name, before_value, after_value, "createdAt")
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, changeID, importID, userID, string(models.ImportActionUpdated), field, before, after, now)
	return err
}

func txInsertSkipChange(ctx context.Context, tx pgx.Tx, importID, reason string, now time.Time) error {
	changeID := generateID()
	_, err := tx.Exec(ctx, `
		INSERT INTO personnel_import_changes (id, import_id, action, reason, "createdAt")
		VALUES ($1, $2, $3, $4, $5)
	`, changeID, importID, string(models.ImportActionSkipped), reason, now)
	return err
}

func insertPendingImport(ctx context.Context, db *database.DB, importID, adminUserID, filename string) error {
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO personnel_imports (id, admin_user_id, document_filename, status, started_at)
		VALUES ($1, $2, $3, $4, NOW())
	`, importID, adminUserID, filename, string(models.ImportStatusPending))
	return err
}

func markImportPreviewed(ctx context.Context, db *database.DB, importID string, counts models.RowCounts) error {
	countsJSON, err := json.Marshal(counts)
	if err != nil {
		return err
	}
	_, err = db.Pool.Exec(ctx, `
		UPDATE personnel_imports
		SET status = $1, row_counts = $2
		WHERE id = $3
	`, string(models.ImportStatusPreviewed), countsJSON, importID)
	return err
}

func markImportFailed(ctx context.Context, db *database.DB, importID string, cause error) error {
	msg := cause.Error()
	now := time.Now().UTC()
	_, err := db.Pool.Exec(ctx, `
		UPDATE personnel_imports
		SET status = $1, error_message = $2, finished_at = $3
		WHERE id = $4
	`, string(models.ImportStatusFailed), msg, now, importID)
	return err
}

// importStatusFromError maps an agent error to an HTTP status code.
func importStatusFromError(err error) int {
	switch {
	case errors.Is(err, agent.ErrInvalidAPIKey), errors.Is(err, agent.ErrAPIKeyNotConfigured):
		return http.StatusServiceUnavailable
	case errors.Is(err, agent.ErrDocumentTooLarge):
		return http.StatusRequestEntityTooLarge
	case errors.Is(err, agent.ErrAPITimeout):
		return http.StatusGatewayTimeout
	case errors.Is(err, agent.ErrNoRowsDetected):
		return http.StatusUnprocessableEntity
	default:
		if strings.Contains(err.Error(), "unsupported media type") {
			return http.StatusUnsupportedMediaType
		}
		return http.StatusInternalServerError
	}
}

func importMessageFromError(err error) string {
	switch {
	case errors.Is(err, agent.ErrAPIKeyNotConfigured):
		return "ANTHROPIC_API_KEY is not configured on the server"
	case errors.Is(err, agent.ErrInvalidAPIKey):
		return "The Anthropic API key was rejected"
	case errors.Is(err, agent.ErrDocumentTooLarge):
		return fmt.Sprintf("Document exceeds the %d MB limit", agent.DefaultMaxDocumentBytes/(1024*1024))
	case errors.Is(err, agent.ErrAPITimeout):
		return "The Anthropic API call timed out"
	case errors.Is(err, agent.ErrNoRowsDetected):
		return "No personnel rows were detected in the document"
	default:
		return err.Error()
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
