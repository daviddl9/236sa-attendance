package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/davidlivingston/go-nextjs-starter/backend/internal/database"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/middleware"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/models"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/services/attendance"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/sse"
	"github.com/go-chi/chi/v5"
)

type AttendanceHandler struct {
	db  *database.DB
	hub *sse.Hub
}

func NewAttendanceHandler(db *database.DB, hub *sse.Hub) *AttendanceHandler {
	return &AttendanceHandler{db: db, hub: hub}
}

type MarkAttendanceRequest struct {
	QRData string `json:"qrData"`
}

// HandleQRScan handles QR code scan from URL (public endpoint)
func (h *AttendanceHandler) HandleQRScan(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	token := chi.URLParam(r, "token")

	// Parse token: sessionID:secret
	parts := strings.Split(token, ":")
	if len(parts) != 2 {
		http.Error(w, "Invalid QR code format", http.StatusBadRequest)
		return
	}

	sessionID := parts[0]
	secret := parts[1]

	// Verify the session exists and authorize the QR secret. The attendance
	// service owns the session-validity decision below.
	var qrSecret string
	err := h.db.Pool.QueryRow(ctx, `
		SELECT qr_code_secret FROM attendance_session WHERE id = $1
	`, sessionID).Scan(&qrSecret)

	if err != nil {
		http.Error(w, "Invalid session", http.StatusNotFound)
		return
	}

	// Verify secret matches
	if secret != qrSecret {
		http.Error(w, "Invalid QR code", http.StatusUnauthorized)
		return
	}

	// Check if user is authenticated (optional - check cookie directly)
	frontendURL := getEnv("FRONTEND_URL", "http://localhost:5173")
	var userID string
	var user *models.User

	cookie, err := r.Cookie("session")
	if err == nil {
		// Try to validate session
		err = h.db.Pool.QueryRow(ctx, `
			SELECT "userId" FROM session
			WHERE token = $1 AND "expiresAt" > NOW()
		`, cookie.Value).Scan(&userID)

		if err == nil {
			// User is authenticated - load user data
			user = &models.User{}
			err = h.db.Pool.QueryRow(ctx, `
				SELECT id, "full_name", rank, battery, is_superadmin, "createdAt", "updatedAt"
				FROM "user" WHERE id = $1
			`, userID).Scan(
				&user.ID, &user.FullName, &user.Rank, &user.Battery,
				&user.IsSuperadmin,
				&user.CreatedAt, &user.UpdatedAt,
			)
			if err != nil {
				user = nil
			}
		}
	}

	if user == nil {
		// Not authenticated - redirect to sign-in page with QR token for auto-marking after login
		redirectURL := fmt.Sprintf("%s/sign-in?redirect=/qr/%s&qrToken=%s", frontendURL, token, token)
		http.Redirect(w, r, redirectURL, http.StatusFound)
		return
	}

	// Mark attendance in a caller-owned transaction.
	tx, err := h.db.Pool.Begin(ctx)
	if err != nil {
		http.Error(w, "Failed to mark attendance", http.StatusInternalServerError)
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()

	outcome, err := attendance.Mark(ctx, tx, attendance.MarkRequest{
		SessionID: sessionID,
		UserID:    userID,
		Method:    models.MarkingMethodQRScan,
	})
	if err != nil {
		http.Error(w, "Failed to mark attendance", http.StatusInternalServerError)
		return
	}
	switch outcome {
	case attendance.AlreadyMarked:
		// Already marked - land on the session page with the confirmation modal.
		http.Redirect(w, r, fmt.Sprintf("%s/dashboard/sessions/%s?scanned=true", frontendURL, sessionID), http.StatusFound)
		return
	case attendance.SessionClosed:
		http.Error(w, "Session is not active", http.StatusBadRequest)
		return
	}

	var markedAt time.Time
	err = tx.QueryRow(ctx, `
		SELECT marked_at FROM attendance_record WHERE session_id = $1 AND user_id = $2
	`, sessionID, userID).Scan(&markedAt)
	if err != nil {
		http.Error(w, "Failed to mark attendance", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		http.Error(w, "Failed to mark attendance", http.StatusInternalServerError)
		return
	}

	// Broadcast SSE event for live updates after the mark commits.
	h.broadcastAttendanceMarked(ctx, sessionID, user, models.MarkingMethodQRScan, markedAt)

	// Land on the session page with the confirmation modal, matching the
	// in-app camera scan flow.
	http.Redirect(w, r, fmt.Sprintf("%s/dashboard/sessions/%s?scanned=true", frontendURL, sessionID), http.StatusFound)
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
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
	if _, err := fmt.Sscanf(timestampStr, "%d", &timestamp); err != nil {
		http.Error(w, "Invalid timestamp in QR code", http.StatusBadRequest)
		return
	}
	qrTime := time.Unix(timestamp, 0)
	if time.Since(qrTime) > 5*time.Minute {
		http.Error(w, "QR code expired", http.StatusBadRequest)
		return
	}

	// Verify the session exists and authorize the QR secret. The attendance
	// service owns the session-validity decision below.
	var qrSecret string
	err := h.db.Pool.QueryRow(ctx, `
		SELECT qr_code_secret FROM attendance_session WHERE id = $1
	`, sessionID).Scan(&qrSecret)

	if err != nil {
		http.Error(w, "Invalid session", http.StatusNotFound)
		return
	}

	// Verify secret matches
	if secret != qrSecret {
		http.Error(w, "Invalid QR code", http.StatusUnauthorized)
		return
	}

	tx, err := h.db.Pool.Begin(ctx)
	if err != nil {
		http.Error(w, "Failed to mark attendance", http.StatusInternalServerError)
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()

	outcome, err := attendance.Mark(ctx, tx, attendance.MarkRequest{
		SessionID: sessionID,
		UserID:    user.ID,
		Method:    models.MarkingMethodQRScan,
	})
	if err != nil {
		http.Error(w, "Failed to mark attendance", http.StatusInternalServerError)
		return
	}
	switch outcome {
	case attendance.AlreadyMarked:
		http.Error(w, "Attendance already marked for this session", http.StatusConflict)
		return
	case attendance.SessionClosed:
		http.Error(w, "Session is not active", http.StatusBadRequest)
		return
	}

	var recordID string
	var markedAt time.Time
	err = tx.QueryRow(ctx, `
		SELECT id, marked_at FROM attendance_record WHERE session_id = $1 AND user_id = $2
	`, sessionID, user.ID).Scan(&recordID, &markedAt)
	if err != nil {
		http.Error(w, "Failed to mark attendance", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		http.Error(w, "Failed to mark attendance", http.StatusInternalServerError)
		return
	}

	// Broadcast SSE event for live updates after the mark commits.
	h.broadcastAttendanceMarked(ctx, sessionID, user, models.MarkingMethodQRScan, markedAt)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{
		"message":  "Attendance marked successfully",
		"recordId": recordID,
	}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
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

	// Verify the session exists for the commander's transport authorization.
	// The attendance service owns the session-validity decision below.
	var session models.AttendanceSession
	err := h.db.Pool.QueryRow(ctx, `
		SELECT id, scope, batteries FROM attendance_session WHERE id = $1
	`, sessionID).Scan(&session.ID, &session.Scope, &session.Batteries)

	if err != nil {
		http.Error(w, "Session not found", http.StatusNotFound)
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

	batchMarkedAt := time.Now()
	var successCount int
	var closedCount int
	var errors []string

	// Mark attendance for each user. Each mark keeps the existing partial-success
	// behaviour while giving the service a caller-owned transaction.
	for _, targetUserID := range req.UserIDs {
		// Get target user details for broadcast.
		var targetUser models.User
		err := h.db.Pool.QueryRow(ctx, `
			SELECT id, "full_name", rank, battery FROM "user" WHERE id = $1
		`, targetUserID).Scan(&targetUser.ID, &targetUser.FullName, &targetUser.Rank, &targetUser.Battery)
		if err != nil {
			errors = append(errors, fmt.Sprintf("User %s not found", targetUserID))
			continue
		}

		tx, err := h.db.Pool.Begin(ctx)
		if err != nil {
			errors = append(errors, fmt.Sprintf("Failed to mark user %s: %v", targetUserID, err))
			continue
		}

		markedBy := user.ID
		outcome, err := attendance.Mark(ctx, tx, attendance.MarkRequest{
			SessionID: sessionID,
			UserID:    targetUserID,
			Method:    models.MarkingMethodManual,
			MarkedBy:  &markedBy,
			MarkedAt:  &batchMarkedAt,
		})
		if err != nil {
			_ = tx.Rollback(ctx)
			errors = append(errors, fmt.Sprintf("Failed to mark user %s: %v", targetUserID, err))
			continue
		}

		switch outcome {
		case attendance.AlreadyMarked:
			_ = tx.Rollback(ctx)
			errors = append(errors, fmt.Sprintf("User %s already marked", targetUserID))
			continue
		case attendance.SessionClosed:
			_ = tx.Rollback(ctx)
			closedCount++
			errors = append(errors, "Session is not active")
			continue
		}

		var markedAt time.Time
		if err := tx.QueryRow(ctx, `
			SELECT marked_at FROM attendance_record WHERE session_id = $1 AND user_id = $2
		`, sessionID, targetUserID).Scan(&markedAt); err != nil {
			_ = tx.Rollback(ctx)
			errors = append(errors, fmt.Sprintf("Failed to mark user %s: %v", targetUserID, err))
			continue
		}
		if err := tx.Commit(ctx); err != nil {
			errors = append(errors, fmt.Sprintf("Failed to mark user %s: %v", targetUserID, err))
			continue
		}

		// Broadcast SSE event for this user after the mark commits.
		h.broadcastAttendanceMarked(ctx, sessionID, &targetUser, models.MarkingMethodManual, markedAt)
		successCount++
	}

	// Preserve the legacy transport response when every requested target is
	// rejected by the service because the session is closed. Mixed batches keep
	// their per-target details in the JSON response below.
	if closedCount == len(req.UserIDs) {
		http.Error(w, "Session is not active", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"message":      fmt.Sprintf("Marked attendance for %d user(s)", successCount),
		"successCount": successCount,
		"errors":       errors,
	}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
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

	// Broadcast SSE event for live updates
	h.broadcastAttendanceRemoved(ctx, sessionID, targetUserID)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"message": "Attendance removed successfully"}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// broadcastAttendanceMarked broadcasts an attendance marked event via SSE
func (h *AttendanceHandler) broadcastAttendanceMarked(ctx context.Context, sessionID string, user *models.User, method string, markedAt time.Time) {
	if h.hub == nil {
		return
	}

	// Get updated counts for the session
	presentCount, totalUsers := h.getSessionCounts(ctx, sessionID)

	userName := ""
	if user.FullName != nil {
		userName = *user.FullName
	}
	userRank := ""
	if user.Rank != nil {
		userRank = *user.Rank
	}
	userBattery := ""
	if user.Battery != nil {
		userBattery = *user.Battery
	}

	h.hub.Broadcast(sessionID, sse.Event{
		Type: sse.EventTypeAttendanceMarked,
		Payload: sse.AttendanceMarkedPayload{
			UserID:        user.ID,
			UserName:      userName,
			UserRank:      userRank,
			UserBattery:   userBattery,
			MarkingMethod: method,
			MarkedAt:      markedAt,
			PresentCount:  presentCount,
			TotalUsers:    totalUsers,
		},
	})
}

// broadcastAttendanceRemoved broadcasts an attendance removed event via SSE
func (h *AttendanceHandler) broadcastAttendanceRemoved(ctx context.Context, sessionID, userID string) {
	if h.hub == nil {
		return
	}

	// Get updated counts for the session
	presentCount, totalUsers := h.getSessionCounts(ctx, sessionID)

	h.hub.Broadcast(sessionID, sse.Event{
		Type: sse.EventTypeAttendanceRemoved,
		Payload: sse.AttendanceRemovedPayload{
			UserID:       userID,
			PresentCount: presentCount,
			TotalUsers:   totalUsers,
		},
	})
}

// getSessionCounts returns present count and total users for a session
func (h *AttendanceHandler) getSessionCounts(ctx context.Context, sessionID string) (presentCount, totalUsers int) {
	// Get session scope and batteries to determine total users
	var scope string
	var batteries []string
	err := h.db.Pool.QueryRow(ctx, `
		SELECT scope, batteries FROM attendance_session WHERE id = $1
	`, sessionID).Scan(&scope, &batteries)
	if err != nil {
		return 0, 0
	}

	// Count present users
	err = h.db.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM attendance_record WHERE session_id = $1
	`, sessionID).Scan(&presentCount)
	if err != nil {
		presentCount = 0
	}

	// Count total eligible users based on session scope
	if scope == models.SessionScopeUnitWide {
		err = h.db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM "user"`).Scan(&totalUsers)
	} else {
		err = h.db.Pool.QueryRow(ctx, `
			SELECT COUNT(*) FROM "user" WHERE battery = ANY($1)
		`, batteries).Scan(&totalUsers)
	}
	if err != nil {
		totalUsers = 0
	}

	return presentCount, totalUsers
}
