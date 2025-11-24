package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/davidlivingston/go-nextjs-starter/backend/internal/database"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/middleware"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/models"
	"github.com/go-chi/chi/v5"
)

type AttendanceHandler struct {
	db *database.DB
}

func NewAttendanceHandler(db *database.DB) *AttendanceHandler {
	return &AttendanceHandler{db: db}
}

type MarkAttendanceRequest struct {
	QRData string `json:"qrData"`
}

// MarkAttendance marks attendance via QR code scan
func (h *AttendanceHandler) MarkAttendance(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	user, ok := middleware.GetUserFromContext(r.Context())
	if !ok {
		http.Error(w, "Not authenticated", http.StatusUnauthorized)
		return
	}

	var req MarkAttendanceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Parse QR data: session_id:secret:timestamp
	parts := strings.Split(req.QRData, ":")
	if len(parts) != 3 {
		http.Error(w, "Invalid QR code format", http.StatusBadRequest)
		return
	}

	sessionID := parts[0]
	secret := parts[1]
	timestampStr := parts[2]

	// Validate timestamp (should be within 5 minutes)
	var timestamp int64
	fmt.Sscanf(timestampStr, "%d", &timestamp)
	qrTime := time.Unix(timestamp, 0)
	if time.Since(qrTime) > 5*time.Minute {
		http.Error(w, "QR code expired", http.StatusBadRequest)
		return
	}

	// Verify session exists and is active
	var qrSecret string
	var status string
	err := h.db.Pool.QueryRow(ctx, `
		SELECT qr_code_secret, status FROM attendance_session WHERE id = $1
	`, sessionID).Scan(&qrSecret, &status)

	if err != nil {
		http.Error(w, "Invalid session", http.StatusNotFound)
		return
	}

	if status != models.SessionStatusActive {
		http.Error(w, "Session is not active", http.StatusBadRequest)
		return
	}

	// Verify secret matches
	if secret != qrSecret {
		http.Error(w, "Invalid QR code", http.StatusUnauthorized)
		return
	}

	// Check if already marked
	var existingID string
	err = h.db.Pool.QueryRow(ctx, `
		SELECT id FROM attendance_record WHERE session_id = $1 AND user_id = $2
	`, sessionID, user.ID).Scan(&existingID)

	if err == nil {
		http.Error(w, "Attendance already marked for this session", http.StatusConflict)
		return
	}

	// Create attendance record
	recordID := generateID()
	now := time.Now()
	_, err = h.db.Pool.Exec(ctx, `
		INSERT INTO attendance_record (
			id, session_id, user_id, marked_at, marking_method, "createdAt", "updatedAt"
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, recordID, sessionID, user.ID, now, models.MarkingMethodQRScan, now, now)

	if err != nil {
		http.Error(w, "Failed to mark attendance", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Attendance marked successfully",
		"recordId": recordID,
	})
}

type ManualMarkRequest struct {
	UserIDs []string `json:"userIds"`
}

// ManualMarkAttendance marks attendance manually for users (commander only)
func (h *AttendanceHandler) ManualMarkAttendance(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	sessionID := chi.URLParam(r, "id")
	user, ok := middleware.GetUserFromContext(r.Context())
	if !ok {
		http.Error(w, "Not authenticated", http.StatusUnauthorized)
		return
	}

	// Verify session exists
	var session models.AttendanceSession
	err := h.db.Pool.QueryRow(ctx, `
		SELECT id, scope, batteries, status FROM attendance_session WHERE id = $1
	`, sessionID).Scan(&session.ID, &session.Scope, &session.Batteries, &session.Status)

	if err != nil {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}

	if session.Status != models.SessionStatusActive {
		http.Error(w, "Session is not active", http.StatusBadRequest)
		return
	}

	var req ManualMarkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if len(req.UserIDs) == 0 {
		http.Error(w, "No users specified", http.StatusBadRequest)
		return
	}

	// Check permissions: commanders can only mark for their battery, superadmins can mark for any
	if !user.IsSuperadmin {
		// Get user's battery
		var userBattery *string
		err := h.db.Pool.QueryRow(ctx, `SELECT battery FROM "user" WHERE id = $1`, user.ID).Scan(&userBattery)
		if err != nil || userBattery == nil {
			http.Error(w, "User battery not found", http.StatusBadRequest)
			return
		}

		// Check if session is unit-wide or includes user's battery
		if session.Scope == models.SessionScopeBatterySpecific {
			batteryAllowed := false
			for _, battery := range session.Batteries {
				if battery == *userBattery {
					batteryAllowed = true
					break
				}
			}
			if !batteryAllowed {
				http.Error(w, "Insufficient permissions: Cannot mark attendance for this battery", http.StatusForbidden)
				return
			}
		}
	}

	now := time.Now()
	var successCount int
	var errors []string

	// Mark attendance for each user
	for _, targetUserID := range req.UserIDs {
		// Check if already marked
		var existingID string
		err := h.db.Pool.QueryRow(ctx, `
			SELECT id FROM attendance_record WHERE session_id = $1 AND user_id = $2
		`, sessionID, targetUserID).Scan(&existingID)

		if err == nil {
			errors = append(errors, fmt.Sprintf("User %s already marked", targetUserID))
			continue
		}

		// Verify target user exists
		var targetUserExists bool
		err = h.db.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM "user" WHERE id = $1)`, targetUserID).Scan(&targetUserExists)
		if err != nil || !targetUserExists {
			errors = append(errors, fmt.Sprintf("User %s not found", targetUserID))
			continue
		}

		// Create attendance record
		recordID := generateID()
		markedBy := user.ID
		_, err = h.db.Pool.Exec(ctx, `
			INSERT INTO attendance_record (
				id, session_id, user_id, marked_at, marking_method, marked_by, "createdAt", "updatedAt"
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`, recordID, sessionID, targetUserID, now, models.MarkingMethodManual, markedBy, now, now)

		if err != nil {
			errors = append(errors, fmt.Sprintf("Failed to mark user %s: %v", targetUserID, err))
			continue
		}

		successCount++
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message":     fmt.Sprintf("Marked attendance for %d user(s)", successCount),
		"successCount": successCount,
		"errors":       errors,
	})
}

// RemoveAttendance removes an attendance record (commander/superadmin)
func (h *AttendanceHandler) RemoveAttendance(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	sessionID := chi.URLParam(r, "id")
	targetUserID := chi.URLParam(r, "userId")
	user, ok := middleware.GetUserFromContext(r.Context())
	if !ok {
		http.Error(w, "Not authenticated", http.StatusUnauthorized)
		return
	}

	// Only superadmin can remove any attendance, commanders can remove for their battery
	if !user.IsSuperadmin {
		// Get user's battery
		var userBattery *string
		err := h.db.Pool.QueryRow(ctx, `SELECT battery FROM "user" WHERE id = $1`, user.ID).Scan(&userBattery)
		if err != nil || userBattery == nil {
			http.Error(w, "User battery not found", http.StatusBadRequest)
			return
		}

		// Get target user's battery
		var targetBattery *string
		err = h.db.Pool.QueryRow(ctx, `SELECT battery FROM "user" WHERE id = $1`, targetUserID).Scan(&targetBattery)
		if err != nil {
			http.Error(w, "Target user not found", http.StatusNotFound)
			return
		}

		if targetBattery == nil || *targetBattery != *userBattery {
			http.Error(w, "Insufficient permissions", http.StatusForbidden)
			return
		}
	}

	_, err := h.db.Pool.Exec(ctx, `
		DELETE FROM attendance_record WHERE session_id = $1 AND user_id = $2
	`, sessionID, targetUserID)

	if err != nil {
		http.Error(w, "Failed to remove attendance", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Attendance removed successfully"})
}

