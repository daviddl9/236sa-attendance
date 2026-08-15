package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/davidlivingston/go-nextjs-starter/backend/internal/database"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/middleware"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/models"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/services/deeplink"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/services/groups"
	"github.com/go-chi/chi/v5"
	"github.com/xuri/excelize/v2"
)

// GroupHandler exposes reusable participant-group operations.
type GroupHandler struct {
	db      *database.DB
	service *groups.Service
}

// NewGroupHandler constructs a group handler.
func NewGroupHandler(db *database.DB) *GroupHandler {
	return &GroupHandler{
		db:      db,
		service: groups.NewService(db),
	}
}

type CreateGroupRequest struct {
	Name           string   `json:"name"`
	ParticipantIDs []string `json:"participantIds"`
}

type GroupPreviewRequest struct {
	Name string `json:"name,omitempty"`
}

// GroupResponse is a group with its member IDs (populated on get).
type GroupResponse struct {
	models.ParticipantGroup
	MemberIDs []string `json:"memberIds,omitempty"`
}

// CreateGroup creates a named reusable group from a list of roster user IDs.
// POST /api/groups
func (h *GroupHandler) CreateGroup(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.GetUserFromContext(r.Context())
	if !ok {
		http.Error(w, "Not authenticated", http.StatusUnauthorized)
		return
	}
	var req CreateGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	group, err := h.service.Create(r.Context(), req.Name, req.ParticipantIDs, user.ID)
	if err != nil {
		writeGroupError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(GroupResponse{ParticipantGroup: group})
}

// ListGroups lists all reusable groups with member counts.
// GET /api/groups
func (h *GroupHandler) ListGroups(w http.ResponseWriter, r *http.Request) {
	groups, err := h.service.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"groups": groups})
}

// GetGroup returns a group with its member IDs.
// GET /api/groups/{id}
func (h *GroupHandler) GetGroup(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	group, memberIDs, err := h.service.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, groups.ErrNotFound) {
			http.Error(w, "Group not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to to load group", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(GroupResponse{ParticipantGroup: group, MemberIDs: memberIDs})
}

// DeleteGroup removes a group.
// DELETE /api/groups/{id}
func (h *GroupHandler) DeleteGroup(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.service.Delete(r.Context(), id); err != nil {
		if errors.Is(err, groups.ErrNotFound) {
			http.Error(w, "Group not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to delete group", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// PreviewGroupFromExcel parses an uploaded participant Excel and matches roster
// users, so the caller can review before creating the group.
// POST /api/groups/preview  (multipart: file + optional password)
func (h *GroupHandler) PreviewGroupFromExcel(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}
	password := r.FormValue("password")

	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "No file uploaded", http.StatusBadRequest)
		return
	}
	defer file.Close()

	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(file); err != nil {
		http.Error(w, "Failed to read file", http.StatusInternalServerError)
		return
	}

	opts := excelize.Options{}
	if password != "" {
		opts.Password = password
	}
	xlFile, err := excelize.OpenReader(buf, opts)
	if err != nil {
		http.Error(w, "Failed to open Excel file (wrong password?)", http.StatusBadRequest)
		return
	}
	defer func() { _ = xlFile.Close() }()

	sheetName := findDataSheet(xlFile)
	if sheetName == "" {
		http.Error(w, "No valid sheet found", http.StatusBadRequest)
		return
	}

	rows, err := xlFile.GetRows(sheetName)
	if err != nil || len(rows) < 2 {
		http.Error(w, "File must have a header row and at least one data row", http.StatusBadRequest)
		return
	}

	nameIdx, nricIdx := -1, -1
	header := rows[0]
	for i, cell := range header {
		lower := strings.ToLower(strings.TrimSpace(cell))
		switch {
		case nameIdx == -1 && (lower == "full name" || lower == "name" || lower == "fullname"):
			nameIdx = i
		case nricIdx == -1 && strings.Contains(lower, "nric"):
			nricIdx = i
		}
	}
	if nameIdx == -1 {
		http.Error(w, "Could not find 'Full Name' column in the Excel file", http.StatusBadRequest)
		return
	}

	ctx := context.Background()
	var matched []participantMatch
	var unmatched []unmatchedRow

	for i, row := range rows[1:] {
		rowNum := i + 2
		fullName := strings.TrimSpace(cellValue(row, nameIdx))
		if fullName == "" {
			continue
		}
		nricLast5 := ""
		if nricIdx >= 0 {
			nricLast5 = strings.TrimSpace(cellValue(row, nricIdx))
		}

		var uid string
		var rank, battery *string

		if nricLast5 != "" {
			normalized, ok := normalizeNRICLast5(nricLast5)
			if ok {
				_ = h.db.Pool.QueryRow(ctx, `
					SELECT id, rank, battery FROM "user"
					WHERE upper("full_name") = upper($1) AND upper("nric_last5") = $2 AND verified = true
					LIMIT 1
				`, fullName, normalized).Scan(&uid, &rank, &battery)
			}
		}

		if uid == "" {
			var count int
			_ = h.db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM "user" WHERE upper("full_name") = upper($1) AND verified = true`, fullName).Scan(&count)
			if count == 1 {
				_ = h.db.Pool.QueryRow(ctx, `SELECT id, rank, battery FROM "user" WHERE upper("full_name") = upper($1) AND verified = true LIMIT 1`, fullName).Scan(&uid, &rank, &battery)
			} else if count > 1 {
				unmatched = append(unmatched, unmatchedRow{RowNum: rowNum, FullName: fullName, NRICLast5: nricLast5, Reason: "ambiguous: multiple users with this name"})
				continue
			}
		}

		if uid == "" {
			unmatched = append(unmatched, unmatchedRow{RowNum: rowNum, FullName: fullName, NRICLast5: nricLast5, Reason: "not found in system"})
			continue
		}

		matched = append(matched, participantMatch{UserID: uid, FullName: fullName, Rank: rank, Battery: battery})
	}

	if matched == nil {
		matched = []participantMatch{}
	}
	if unmatched == nil {
		unmatched = []unmatchedRow{}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(CustomSessionPreviewResponse{Matched: matched, Unmatched: unmatched})
}

// CreateSessionFromGroup creates a custom_list session whose participants come
// from a saved group.
// POST /api/groups/{id}/sessions
func (h *GroupHandler) CreateSessionFromGroup(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.GetUserFromContext(r.Context())
	if !ok {
		http.Error(w, "Not authenticated", http.StatusUnauthorized)
		return
	}
	groupID := chi.URLParam(r, "id")

	var req CreateCustomSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	memberIDs, err := h.service.Members(r.Context(), groupID)
	if err != nil {
		if errors.Is(err, groups.ErrNotFound) {
			http.Error(w, "Group not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to load group", http.StatusInternalServerError)
		return
	}
	if len(memberIDs) == 0 {
		http.Error(w, "group has no members", http.StatusBadRequest)
		return
	}

	session, err := h.createCustomSession(r.Context(), user.ID, req.Name, memberIDs, req.EndTime)
	if err != nil {
		http.Error(w, "Failed to create session", http.StatusInternalServerError)
		return
	}
	setSessionQRVisibility(&session, user)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(SessionResponse{AttendanceSession: session})
}

// SaveSessionAsGroup snapshots an existing session's participants into a new
// named group.
// POST /api/sessions/{id}/group
func (h *GroupHandler) SaveSessionAsGroup(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.GetUserFromContext(r.Context())
	if !ok {
		http.Error(w, "Not authenticated", http.StatusUnauthorized)
		return
	}
	sessionID := chi.URLParam(r, "id")

	var req CreateGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	group, err := h.service.SaveSessionAsGroup(r.Context(), sessionID, req.Name, user.ID)
	if err != nil {
		writeGroupError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(GroupResponse{ParticipantGroup: group})
}

// DuplicateSession creates a new custom_list session copying another session's
// participants (and optional name/end time).
// POST /api/sessions/{id}/duplicate
func (h *GroupHandler) DuplicateSession(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.GetUserFromContext(r.Context())
	if !ok {
		http.Error(w, "Not authenticated", http.StatusUnauthorized)
		return
	}
	sourceID := chi.URLParam(r, "id")

	var req CreateCustomSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	rows, err := h.db.Pool.Query(r.Context(), `
		SELECT user_id FROM session_participants WHERE session_id = $1 ORDER BY user_id
	`, sourceID)
	if err != nil {
		http.Error(w, "Failed to load source session", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var memberIDs []string
	for rows.Next() {
		var uid string
		if err := rows.Scan(&uid); err != nil {
			http.Error(w, "Failed to load source session", http.StatusInternalServerError)
			return
		}
		memberIDs = append(memberIDs, uid)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "Failed to load source session", http.StatusInternalServerError)
		return
	}
	if len(memberIDs) == 0 {
		http.Error(w, "source session has no participants to duplicate", http.StatusBadRequest)
		return
	}

	name := req.Name
	if name == "" {
		// Default to the source session's name with a " (copy)" suffix.
		var srcName string
		if err := h.db.Pool.QueryRow(r.Context(), `SELECT name FROM attendance_session WHERE id = $1`, sourceID).Scan(&srcName); err != nil {
			http.Error(w, "Source session not found", http.StatusNotFound)
			return
		}
		name = srcName + " (copy)"
	}

	session, err := h.createCustomSession(r.Context(), user.ID, name, memberIDs, req.EndTime)
	if err != nil {
		http.Error(w, "Failed to create session", http.StatusInternalServerError)
		return
	}
	setSessionQRVisibility(&session, user)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(SessionResponse{AttendanceSession: session})
}

// createCustomSession inserts a custom_list session with its participants
// atomically. It is shared by the group and duplicate flows.
func (h *GroupHandler) createCustomSession(ctx context.Context, createdBy, name string, participantIDs []string, endTime *time.Time) (models.AttendanceSession, error) {
	tx, err := h.db.Pool.Begin(ctx)
	if err != nil {
		return models.AttendanceSession{}, fmt.Errorf("begin session creation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	sessionID := generateID()
	qrSecret := generateSessionToken()
	deeplinkCode, err := deeplink.GenerateCode()
	if err != nil {
		return models.AttendanceSession{}, fmt.Errorf("generate deep-link code: %w", err)
	}
	qrCode := fmt.Sprintf("%s:%s", sessionID, qrSecret)
	now := time.Now()

	_, err = tx.Exec(ctx, `
		INSERT INTO attendance_session (
			id, name, qr_code, qr_code_secret, scope, batteries,
			status, created_by, start_time, end_time, deeplink_code, "createdAt", "updatedAt"
		)
		VALUES ($1, $2, $3, $4, 'custom_list', '{}', 'active', $5, NOW(), $6, $7, $8, $9)
	`, sessionID, name, qrCode, qrSecret, createdBy, endTime, deeplinkCode, now, now)
	if err != nil {
		return models.AttendanceSession{}, fmt.Errorf("insert session: %w", err)
	}

	for _, pid := range participantIDs {
		if _, err := tx.Exec(ctx, `
			INSERT INTO session_participants (session_id, user_id) VALUES ($1, $2)
			ON CONFLICT DO NOTHING
		`, sessionID, pid); err != nil {
			return models.AttendanceSession{}, fmt.Errorf("insert participant: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return models.AttendanceSession{}, fmt.Errorf("commit session: %w", err)
	}

	count := len(participantIDs)
	return models.AttendanceSession{
		ID:               sessionID,
		Name:             name,
		QRCode:           qrCode,
		Scope:            models.SessionScopeCustomList,
		Batteries:        []string{},
		Status:           models.SessionStatusActive,
		CreatedBy:        createdBy,
		StartTime:        now,
		EndTime:          endTime,
		ParticipantCount: &count,
		CreatedAt:        now,
		UpdatedAt:        now,
	}, nil
}

func writeGroupError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, groups.ErrInvalidRequest):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, groups.ErrDuplicateName):
		http.Error(w, "A group with this name already exists", http.StatusConflict)
	default:
		http.Error(w, "Failed to save group", http.StatusInternalServerError)
	}
}
