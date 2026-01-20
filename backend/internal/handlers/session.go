package handlers

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/davidlivingston/go-nextjs-starter/backend/internal/database"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/middleware"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/models"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/sse"
	"github.com/go-chi/chi/v5"
	"github.com/xuri/excelize/v2"
)

type SessionHandler struct {
	db  *database.DB
	hub *sse.Hub
}

func NewSessionHandler(db *database.DB, hub *sse.Hub) *SessionHandler {
	return &SessionHandler{db: db, hub: hub}
}

type CreateSessionRequest struct {
	Name      string     `json:"name"`
	Scope     string     `json:"scope"`
	Batteries []string   `json:"batteries,omitempty"`
	EndTime   *time.Time `json:"endTime,omitempty"`
}

type SessionResponse struct {
	models.AttendanceSession
}

// CreateSession creates a new attendance session with QR code
func (h *SessionHandler) CreateSession(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.GetUserFromContext(r.Context())
	if !ok {
		http.Error(w, "Not authenticated", http.StatusUnauthorized)
		return
	}

	var req CreateSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate scope
	if req.Scope != models.SessionScopeUnitWide && req.Scope != models.SessionScopeBatterySpecific {
		http.Error(w, "Invalid scope", http.StatusBadRequest)
		return
	}

	// Validate batteries if battery_specific
	if req.Scope == models.SessionScopeBatterySpecific {
		if len(req.Batteries) == 0 {
			http.Error(w, "Batteries required for battery-specific sessions", http.StatusBadRequest)
			return
		}
		for _, battery := range req.Batteries {
			if battery != models.BatteryHQ && battery != models.BatteryAlpha && battery != models.BatteryBravo {
				http.Error(w, fmt.Sprintf("Invalid battery: %s", battery), http.StatusBadRequest)
				return
			}
		}
	}

	ctx := context.Background()
	sessionID := generateID()
	qrSecret := generateSessionToken() // Use session token generator for QR secret
	now := time.Now()

	// Store QR secret for frontend to construct URL
	qrCode := fmt.Sprintf("%s:%s", sessionID, qrSecret)

	// Insert session into database (start_time defaults to NOW() in database)
	_, err := h.db.Pool.Exec(ctx, `
		INSERT INTO attendance_session (
			id, name, qr_code, qr_code_secret, scope, batteries,
			status, created_by, start_time, end_time, "createdAt", "updatedAt"
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), $9, $10, $11)
	`, sessionID, req.Name, qrCode, qrSecret, req.Scope,
		req.Batteries, models.SessionStatusActive, user.ID, req.EndTime, now, now)

	if err != nil {
		http.Error(w, "Failed to create session", http.StatusInternalServerError)
		return
	}

	// Get the actual start_time from database (which defaults to NOW())
	var startTime time.Time
	err = h.db.Pool.QueryRow(ctx, `SELECT start_time FROM attendance_session WHERE id = $1`, sessionID).Scan(&startTime)
	if err != nil {
		http.Error(w, "Failed to retrieve session", http.StatusInternalServerError)
		return
	}

	session := models.AttendanceSession{
		ID:           sessionID,
		Name:         req.Name,
		QRCode:       qrCode,
		QRCodeSecret: qrSecret,
		Scope:        req.Scope,
		Batteries:    req.Batteries,
		Status:       models.SessionStatusActive,
		CreatedBy:    user.ID,
		StartTime:    startTime,
		EndTime:      req.EndTime,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	response := SessionResponse{
		AttendanceSession: session,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// ListSessions retrieves sessions with optional filters
func (h *SessionHandler) ListSessions(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()

	status := r.URL.Query().Get("status")
	battery := r.URL.Query().Get("battery")
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")

	query := `
		SELECT 
			id, name, qr_code, qr_code_secret, scope, batteries,
			status, created_by, start_time, end_time, closed_at,
			"createdAt", "updatedAt"
		FROM attendance_session
		WHERE 1=1
	`
	args := []interface{}{}
	argIndex := 1

	if status != "" {
		query += fmt.Sprintf(" AND status = $%d", argIndex)
		args = append(args, status)
		argIndex++
	}
	if battery != "" {
		query += fmt.Sprintf(" AND ($%d = ANY(batteries) OR scope = 'unit_wide')", argIndex)
		args = append(args, battery)
		argIndex++
	}
	if from != "" {
		query += fmt.Sprintf(" AND start_time >= $%d", argIndex)
		args = append(args, from)
		argIndex++
	}
	if to != "" {
		query += fmt.Sprintf(" AND start_time <= $%d", argIndex)
		args = append(args, to)
	}

	query += " ORDER BY start_time DESC"

	rows, err := h.db.Pool.Query(ctx, query, args...)
	if err != nil {
		http.Error(w, "Failed to fetch sessions", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var sessions []models.AttendanceSession
	for rows.Next() {
		var session models.AttendanceSession
		var closedAt *time.Time

		err := rows.Scan(
			&session.ID,
			&session.Name,
			&session.QRCode,
			&session.QRCodeSecret,
			&session.Scope,
			&session.Batteries,
			&session.Status,
			&session.CreatedBy,
			&session.StartTime,
			&session.EndTime,
			&closedAt,
			&session.CreatedAt,
			&session.UpdatedAt,
		)
		if err != nil {
			continue
		}

		session.ClosedAt = closedAt
		sessions = append(sessions, session)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(sessions); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// GetSession retrieves a single session by ID
func (h *SessionHandler) GetSession(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	sessionID := chi.URLParam(r, "id")

	var session models.AttendanceSession
	var closedAt *time.Time

	err := h.db.Pool.QueryRow(ctx, `
		SELECT 
			id, name, qr_code, qr_code_secret, scope, batteries,
			status, created_by, start_time, end_time, closed_at,
			"createdAt", "updatedAt"
		FROM attendance_session
		WHERE id = $1
	`, sessionID).Scan(
		&session.ID,
		&session.Name,
		&session.QRCode,
		&session.QRCodeSecret,
		&session.Scope,
		&session.Batteries,
		&session.Status,
		&session.CreatedBy,
		&session.StartTime,
		&session.EndTime,
		&closedAt,
		&session.CreatedAt,
		&session.UpdatedAt,
	)

	if err != nil {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}

	session.ClosedAt = closedAt

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(session); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// GetActiveSessions retrieves all active sessions
func (h *SessionHandler) GetActiveSessions(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()

	rows, err := h.db.Pool.Query(ctx, `
		SELECT 
			id, name, qr_code, qr_code_secret, scope, batteries,
			status, created_by, start_time, end_time, closed_at,
			"createdAt", "updatedAt"
		FROM attendance_session
		WHERE status = 'active'
		ORDER BY start_time DESC
	`)
	if err != nil {
		http.Error(w, "Failed to fetch active sessions", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var sessions []models.AttendanceSession
	for rows.Next() {
		var session models.AttendanceSession
		var closedAt *time.Time

		err := rows.Scan(
			&session.ID,
			&session.Name,
			&session.QRCode,
			&session.QRCodeSecret,
			&session.Scope,
			&session.Batteries,
			&session.Status,
			&session.CreatedBy,
			&session.StartTime,
			&session.EndTime,
			&closedAt,
			&session.CreatedAt,
			&session.UpdatedAt,
		)
		if err != nil {
			continue
		}

		session.ClosedAt = closedAt
		sessions = append(sessions, session)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(sessions); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// CloseSession closes an active session
func (h *SessionHandler) CloseSession(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	sessionID := chi.URLParam(r, "id")

	user, ok := middleware.GetUserFromContext(r.Context())
	if !ok {
		http.Error(w, "Not authenticated", http.StatusUnauthorized)
		return
	}

	// Check if session exists and get creator
	var createdBy string
	err := h.db.Pool.QueryRow(ctx, `
		SELECT created_by FROM attendance_session WHERE id = $1
	`, sessionID).Scan(&createdBy)

	if err != nil {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}

	// Only creator or superadmin can close
	if createdBy != user.ID && !user.IsSuperadmin {
		http.Error(w, "Insufficient permissions", http.StatusForbidden)
		return
	}

	now := time.Now()
	_, err = h.db.Pool.Exec(ctx, `
		UPDATE attendance_session
		SET status = 'closed', closed_at = $1, "updatedAt" = $2
		WHERE id = $3 AND status = 'active'
	`, now, now, sessionID)

	if err != nil {
		http.Error(w, "Failed to close session", http.StatusInternalServerError)
		return
	}

	// Broadcast SSE event for live updates
	if h.hub != nil {
		h.hub.Broadcast(sessionID, sse.Event{
			Type: sse.EventTypeSessionClosed,
			Payload: sse.SessionClosedPayload{
				SessionID: sessionID,
			},
		})
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"message": "Session closed successfully"}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// DeleteSession permanently deletes a session and its attendance records (superadmin only)
func (h *SessionHandler) DeleteSession(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	sessionID := chi.URLParam(r, "id")

	// Check if session exists
	var exists bool
	err := h.db.Pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM attendance_session WHERE id = $1)`,
		sessionID).Scan(&exists)
	if err != nil || !exists {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}

	// Delete session (attendance_records cascade delete via FK constraint)
	_, err = h.db.Pool.Exec(ctx, `DELETE FROM attendance_session WHERE id = $1`, sessionID)
	if err != nil {
		http.Error(w, "Failed to delete session", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"message": "Session deleted successfully"}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// ExportSessionCSV exports session attendance to CSV
func (h *SessionHandler) ExportSessionCSV(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	sessionID := chi.URLParam(r, "id")
	batteryFilter := r.URL.Query().Get("battery")

	// Get session name
	var sessionName string
	err := h.db.Pool.QueryRow(ctx, `SELECT name FROM attendance_session WHERE id = $1`, sessionID).Scan(&sessionName)
	if err != nil {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}

	// Build query with optional battery filter
	query := `
		SELECT
			u."full_name", u.rank, u.battery,
			ar.marked_at, ar.marking_method
		FROM attendance_record ar
		JOIN "user" u ON u.id = ar.user_id
		WHERE ar.session_id = $1`
	args := []interface{}{sessionID}

	if batteryFilter != "" {
		query += ` AND u.battery = $2`
		args = append(args, batteryFilter)
	}
	query += ` ORDER BY ar.marked_at`

	// Get attendance records with user info
	rows, err := h.db.Pool.Query(ctx, query, args...)
	if err != nil {
		http.Error(w, "Failed to fetch attendance", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s_attendance.csv\"", sessionName))
	writer := csv.NewWriter(w)
	defer writer.Flush()

	// Write header
	if err := writer.Write([]string{"Full Name", "Rank", "Battery", "Status", "Marked At", "Method"}); err != nil {
		http.Error(w, "Failed to write CSV header", http.StatusInternalServerError)
		return
	}

	// Write data
	for rows.Next() {
		var fullName, rank, battery *string
		var markingMethod string
		var markedAt time.Time

		err := rows.Scan(&fullName, &rank, &battery, &markedAt, &markingMethod)
		if err != nil {
			continue
		}

		fullNameStr := ""
		if fullName != nil {
			fullNameStr = *fullName
		}
		rankStr := ""
		if rank != nil {
			rankStr = *rank
		}
		batteryStr := ""
		if battery != nil {
			batteryStr = *battery
		}

		if err := writer.Write([]string{
			fullNameStr,
			rankStr,
			batteryStr,
			"Present",
			markedAt.Format("2006-01-02 15:04:05"),
			markingMethod,
		}); err != nil {
			continue
		}
	}
}

// ExportSessionExcel exports session attendance to Excel
func (h *SessionHandler) ExportSessionExcel(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	sessionID := chi.URLParam(r, "id")
	batteryFilter := r.URL.Query().Get("battery")

	// Get session name
	var sessionName string
	err := h.db.Pool.QueryRow(ctx, `SELECT name FROM attendance_session WHERE id = $1`, sessionID).Scan(&sessionName)
	if err != nil {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}

	// Create Excel file
	f := excelize.NewFile()
	defer func() {
		_ = f.Close()
	}()

	sheetName := "Attendance"
	if _, err := f.NewSheet(sheetName); err != nil {
		http.Error(w, "Failed to create Excel sheet", http.StatusInternalServerError)
		return
	}
	if err := f.DeleteSheet("Sheet1"); err != nil {
		http.Error(w, "Failed to delete default sheet", http.StatusInternalServerError)
		return
	}

	// Set headers
	headers := []string{"Full Name", "Rank", "Battery", "Status", "Marked At", "Method"}
	for i, header := range headers {
		cell := fmt.Sprintf("%c1", 'A'+i)
		if err := f.SetCellValue(sheetName, cell, header); err != nil {
			http.Error(w, "Failed to set Excel header", http.StatusInternalServerError)
			return
		}
	}

	// Build query with optional battery filter
	query := `
		SELECT
			u."full_name", u.rank, u.battery,
			ar.marked_at, ar.marking_method
		FROM attendance_record ar
		JOIN "user" u ON u.id = ar.user_id
		WHERE ar.session_id = $1`
	args := []interface{}{sessionID}

	if batteryFilter != "" {
		query += ` AND u.battery = $2`
		args = append(args, batteryFilter)
	}
	query += ` ORDER BY ar.marked_at`

	// Get attendance records
	rows, err := h.db.Pool.Query(ctx, query, args...)
	if err != nil {
		http.Error(w, "Failed to fetch attendance", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	rowNum := 2
	for rows.Next() {
		var fullName, rank, battery *string
		var markingMethod string
		var markedAt time.Time

		err := rows.Scan(&fullName, &rank, &battery, &markedAt, &markingMethod)
		if err != nil {
			continue
		}

		fullNameStr := ""
		if fullName != nil {
			fullNameStr = *fullName
		}
		rankStr := ""
		if rank != nil {
			rankStr = *rank
		}
		batteryStr := ""
		if battery != nil {
			batteryStr = *battery
		}

		if err := f.SetCellValue(sheetName, fmt.Sprintf("A%d", rowNum), fullNameStr); err != nil {
			continue
		}
		if err := f.SetCellValue(sheetName, fmt.Sprintf("B%d", rowNum), rankStr); err != nil {
			continue
		}
		if err := f.SetCellValue(sheetName, fmt.Sprintf("C%d", rowNum), batteryStr); err != nil {
			continue
		}
		if err := f.SetCellValue(sheetName, fmt.Sprintf("D%d", rowNum), "Present"); err != nil {
			continue
		}
		if err := f.SetCellValue(sheetName, fmt.Sprintf("E%d", rowNum), markedAt.Format("2006-01-02 15:04:05")); err != nil {
			continue
		}
		if err := f.SetCellValue(sheetName, fmt.Sprintf("F%d", rowNum), markingMethod); err != nil {
			continue
		}
		rowNum++
	}

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s_attendance.xlsx\"", sessionName))
	if err := f.Write(w); err != nil {
		http.Error(w, "Failed to write Excel file", http.StatusInternalServerError)
	}
}

// ExportSessionPDF exports session attendance to PDF (simplified - returns JSON for now)
func (h *SessionHandler) ExportSessionPDF(w http.ResponseWriter, r *http.Request) {
	// For now, return JSON. Full PDF implementation can be added later with gofpdf
	ctx := context.Background()
	sessionID := chi.URLParam(r, "id")

	var sessionName string
	err := h.db.Pool.QueryRow(ctx, `SELECT name FROM attendance_session WHERE id = $1`, sessionID).Scan(&sessionName)
	if err != nil {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}

	// Get attendance records
	rows, err := h.db.Pool.Query(ctx, `
		SELECT 
			u."full_name", u.rank, u.battery,
			ar.marked_at, ar.marking_method
		FROM attendance_record ar
		JOIN "user" u ON u.id = ar.user_id
		WHERE ar.session_id = $1
		ORDER BY ar.marked_at
	`, sessionID)
	if err != nil {
		http.Error(w, "Failed to fetch attendance", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type AttendanceRow struct {
		FullName      string `json:"fullName"`
		Rank          string `json:"rank"`
		Battery       string `json:"battery"`
		MarkedAt      string `json:"markedAt"`
		MarkingMethod string `json:"markingMethod"`
	}

	var records []AttendanceRow
	for rows.Next() {
		var fullName, rank, battery *string
		var markingMethod string
		var markedAt time.Time

		err := rows.Scan(&fullName, &rank, &battery, &markedAt, &markingMethod)
		if err != nil {
			continue
		}

		fullNameStr := ""
		if fullName != nil {
			fullNameStr = *fullName
		}
		rankStr := ""
		if rank != nil {
			rankStr = *rank
		}
		batteryStr := ""
		if battery != nil {
			batteryStr = *battery
		}

		records = append(records, AttendanceRow{
			FullName:      fullNameStr,
			Rank:          rankStr,
			Battery:       batteryStr,
			MarkedAt:      markedAt.Format("2006-01-02 15:04:05"),
			MarkingMethod: markingMethod,
		})
	}

	// Return JSON for now (PDF can be implemented later)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"sessionName": sessionName,
		"records":     records,
		"note":        "PDF export coming soon. Use CSV or Excel export for now.",
	}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}
