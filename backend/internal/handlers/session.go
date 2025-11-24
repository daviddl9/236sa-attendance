package handlers

import (
	"context"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/davidlivingston/go-nextjs-starter/backend/internal/database"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/middleware"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/models"
	"github.com/go-chi/chi/v5"
	"github.com/skip2/go-qrcode"
	"github.com/xuri/excelize/v2"
)

type SessionHandler struct {
	db *database.DB
}

func NewSessionHandler(db *database.DB) *SessionHandler {
	return &SessionHandler{db: db}
}

type CreateSessionRequest struct {
	Name        string     `json:"name"`
	SessionType string     `json:"sessionType"`
	Scope       string     `json:"scope"`
	Batteries   []string   `json:"batteries,omitempty"`
	StartTime   time.Time  `json:"startTime"`
	EndTime     *time.Time `json:"endTime,omitempty"`
}

type SessionResponse struct {
	models.AttendanceSession
	QRCodeImage string `json:"qrCodeImage"` // Base64 encoded PNG
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

	// Validate session type
	validType := false
	for _, t := range []string{models.SessionTypeFirstParade, models.SessionTypeMorningFormation, models.SessionTypeCustom} {
		if req.SessionType == t {
			validType = true
			break
		}
	}
	if !validType {
		http.Error(w, "Invalid session type", http.StatusBadRequest)
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

	// Create QR code data: session_id:secret:timestamp
	qrData := fmt.Sprintf("%s:%s:%d", sessionID, qrSecret, now.Unix())
	qrCode := qrData // Store the QR data directly

	// Generate QR code PNG
	qrPNG, err := qrcode.Encode(qrData, qrcode.Medium, 256)
	if err != nil {
		http.Error(w, "Failed to generate QR code", http.StatusInternalServerError)
		return
	}
	qrCodeImage := base64.StdEncoding.EncodeToString(qrPNG)

	// Insert session into database
	_, err = h.db.Pool.Exec(ctx, `
		INSERT INTO attendance_session (
			id, name, session_type, qr_code, qr_code_secret, scope, batteries,
			status, created_by, start_time, end_time, "createdAt", "updatedAt"
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`, sessionID, req.Name, req.SessionType, qrCode, qrSecret, req.Scope,
		req.Batteries, models.SessionStatusActive, user.ID, req.StartTime, req.EndTime, now, now)

	if err != nil {
		http.Error(w, "Failed to create session", http.StatusInternalServerError)
		return
	}

	session := models.AttendanceSession{
		ID:           sessionID,
		Name:         req.Name,
		SessionType:  req.SessionType,
		QRCode:       qrCode,
		QRCodeSecret: qrSecret,
		Scope:        req.Scope,
		Batteries:    req.Batteries,
		Status:       models.SessionStatusActive,
		CreatedBy:    user.ID,
		StartTime:    req.StartTime,
		EndTime:      req.EndTime,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	response := SessionResponse{
		AttendanceSession: session,
		QRCodeImage:       qrCodeImage,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
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
			id, name, session_type, qr_code, qr_code_secret, scope, batteries,
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
		argIndex++
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
			&session.SessionType,
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
	json.NewEncoder(w).Encode(sessions)
}

// GetSession retrieves a single session by ID
func (h *SessionHandler) GetSession(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	sessionID := chi.URLParam(r, "id")

	var session models.AttendanceSession
	var closedAt *time.Time

	err := h.db.Pool.QueryRow(ctx, `
		SELECT 
			id, name, session_type, qr_code, qr_code_secret, scope, batteries,
			status, created_by, start_time, end_time, closed_at,
			"createdAt", "updatedAt"
		FROM attendance_session
		WHERE id = $1
	`, sessionID).Scan(
		&session.ID,
		&session.Name,
		&session.SessionType,
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
	json.NewEncoder(w).Encode(session)
}

// GetActiveSessions retrieves all active sessions
func (h *SessionHandler) GetActiveSessions(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()

	rows, err := h.db.Pool.Query(ctx, `
		SELECT 
			id, name, session_type, qr_code, qr_code_secret, scope, batteries,
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
			&session.SessionType,
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
	json.NewEncoder(w).Encode(sessions)
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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Session closed successfully"})
}

// GetSessionQR retrieves the QR code image for a session
func (h *SessionHandler) GetSessionQR(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	sessionID := chi.URLParam(r, "id")

	var qrCode, qrSecret string
	err := h.db.Pool.QueryRow(ctx, `
		SELECT qr_code, qr_code_secret FROM attendance_session WHERE id = $1
	`, sessionID).Scan(&qrCode, &qrSecret)

	if err != nil {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}

	// Reconstruct QR data
	qrData := fmt.Sprintf("%s:%s:%d", sessionID, qrSecret, time.Now().Unix())

	// Generate QR code PNG
	qrPNG, err := qrcode.Encode(qrData, qrcode.Medium, 256)
	if err != nil {
		http.Error(w, "Failed to generate QR code", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.Write(qrPNG)
}

// ExportSessionCSV exports session attendance to CSV
func (h *SessionHandler) ExportSessionCSV(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	sessionID := chi.URLParam(r, "id")

	// Get session name
	var sessionName string
	err := h.db.Pool.QueryRow(ctx, `SELECT name FROM attendance_session WHERE id = $1`, sessionID).Scan(&sessionName)
	if err != nil {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}

	// Get attendance records with user info
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

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s_attendance.csv\"", sessionName))
	writer := csv.NewWriter(w)
	defer writer.Flush()

	// Write header
	writer.Write([]string{"Full Name", "Rank", "Battery", "Status", "Marked At", "Method"})

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

		writer.Write([]string{
			fullNameStr,
			rankStr,
			batteryStr,
			"Present",
			markedAt.Format("2006-01-02 15:04:05"),
			markingMethod,
		})
	}
}

// ExportSessionExcel exports session attendance to Excel
func (h *SessionHandler) ExportSessionExcel(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	sessionID := chi.URLParam(r, "id")

	// Get session name
	var sessionName string
	err := h.db.Pool.QueryRow(ctx, `SELECT name FROM attendance_session WHERE id = $1`, sessionID).Scan(&sessionName)
	if err != nil {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}

	// Create Excel file
	f := excelize.NewFile()
	defer f.Close()

	sheetName := "Attendance"
	f.NewSheet(sheetName)
	f.DeleteSheet("Sheet1")

	// Set headers
	headers := []string{"Full Name", "Rank", "Battery", "Status", "Marked At", "Method"}
	for i, header := range headers {
		cell := fmt.Sprintf("%c1", 'A'+i)
		f.SetCellValue(sheetName, cell, header)
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

		f.SetCellValue(sheetName, fmt.Sprintf("A%d", rowNum), fullNameStr)
		f.SetCellValue(sheetName, fmt.Sprintf("B%d", rowNum), rankStr)
		f.SetCellValue(sheetName, fmt.Sprintf("C%d", rowNum), batteryStr)
		f.SetCellValue(sheetName, fmt.Sprintf("D%d", rowNum), "Present")
		f.SetCellValue(sheetName, fmt.Sprintf("E%d", rowNum), markedAt.Format("2006-01-02 15:04:05"))
		f.SetCellValue(sheetName, fmt.Sprintf("F%d", rowNum), markingMethod)
		rowNum++
	}

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s_attendance.xlsx\"", sessionName))
	f.Write(w)
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
	json.NewEncoder(w).Encode(map[string]interface{}{
		"sessionName": sessionName,
		"records":     records,
		"note":        "PDF export coming soon. Use CSV or Excel export for now.",
	})
}
