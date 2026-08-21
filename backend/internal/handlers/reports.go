package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/davidlivingston/go-nextjs-starter/backend/internal/database"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/middleware"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/models"
	reportservice "github.com/davidlivingston/go-nextjs-starter/backend/internal/services/reports"
	"github.com/go-chi/chi/v5"
)

type ReportsHandler struct {
	db      *database.DB
	service *reportservice.Service
}

func NewReportsHandler(db *database.DB) *ReportsHandler {
	return &ReportsHandler{db: db, service: reportservice.NewService(db)}
}

type SessionAnalytics struct {
	TotalUsers           int     `json:"totalUsers"`
	PresentCount         int     `json:"presentCount"`
	AttendancePercentage float64 `json:"attendancePercentage"`
	// ExtraCount is the number of walk-ins (marked users outside the roster).
	// They appear in PresentUsers but are excluded from TotalUsers/PresentCount
	// so those stay anchored to the expected roster.
	ExtraCount   int                     `json:"extraCount"`
	MissingUsers []UserInfo              `json:"missingUsers"`
	PresentUsers []UserInfo              `json:"presentUsers"`
	ByBattery    map[string]BatteryStats `json:"byBattery"`
	ByRank       map[string]RankStats    `json:"byRank"`
}

type UserInfo struct {
	ID           string             `json:"id"`
	FullName     *string            `json:"fullName,omitempty"`
	Rank         *string            `json:"rank,omitempty"`
	Battery      *string            `json:"battery,omitempty"`
	ActiveStatus *models.StatusInfo `json:"activeStatus,omitempty"`
}

type BatteryStats struct {
	Total   int `json:"total"`
	Present int `json:"present"`
}

type RankStats struct {
	Total   int `json:"total"`
	Present int `json:"present"`
}

// batteryScopeForAnalytics returns the battery an analytics view is restricted
// to, or nil for no restriction. Enlisted soldiers (Tier 1) and battery NCOs
// (Tier 2) see only their own battery; unit commanders and superadmins (Tier
// 3+) see all batteries.
func batteryScopeForAnalytics(user *models.User) *string {
	if user == nil || user.GetTier() >= models.TierUnitCommander {
		return nil
	}
	// Tier 1–2 are restricted to their own battery. A user with no battery has
	// no roster to see, so scope to a sentinel that matches no one (empty
	// board) rather than falling through to the unrestricted unit-wide query.
	if user.Battery != nil {
		return user.Battery
	}
	empty := ""
	return &empty
}

// GetSessionAnalytics returns detailed analytics for a session.
func (h *ReportsHandler) GetSessionAnalytics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessionID := chi.URLParam(r, "id")
	currentUser, _ := middleware.GetUserFromContext(ctx)

	// Use the shared service for both authorization and the scoped roster. The
	// web response still enriches missing users with active personnel status;
	// Telegram callers only receive the service's status-free rows.
	summary, err := h.service.Summary(ctx, sessionID, currentUser)
	if err != nil {
		if errors.Is(err, reportservice.ErrSessionNotFound) {
			http.Error(w, "Session not found", http.StatusNotFound)
		} else {
			http.Error(w, "Failed to fetch users", http.StatusInternalServerError)
		}
		return
	}
	roster, err := h.service.EligibleUsers(ctx, sessionID, currentUser)
	if err != nil {
		http.Error(w, "Failed to fetch users", http.StatusInternalServerError)
		return
	}
	// Walk-ins: verified users who marked without being on the roster are
	// present extras. Appending them to the roster lets the split loop and the
	// breakdowns count them as present; they can never be missing.
	extras, err := h.service.Extras(ctx, sessionID, currentUser)
	if err != nil {
		http.Error(w, "Failed to fetch users", http.StatusInternalServerError)
		return
	}
	roster = append(roster, extras...)

	allUsers := make([]UserInfo, 0, len(roster))
	for _, row := range roster {
		user := UserInfo{ID: row.ID}
		if row.Name != "" {
			name := row.Name
			user.FullName = &name
		}
		if row.Rank != "" {
			rank := row.Rank
			user.Rank = &rank
		}
		if row.Battery != "" {
			battery := row.Battery
			user.Battery = &battery
		}
		allUsers = append(allUsers, user)
	}

	// Get marked attendance.
	rows, err := h.db.Pool.Query(ctx, `
		SELECT user_id FROM attendance_record WHERE session_id = $1
	`, sessionID)
	if err != nil {
		http.Error(w, "Failed to fetch attendance", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	presentUserIDs := make(map[string]bool)
	for rows.Next() {
		var userID string
		if err := rows.Scan(&userID); err != nil {
			continue // Skip invalid rows
		}
		presentUserIDs[userID] = true
	}

	// Find missing and present users.
	var missingUsers []UserInfo
	var presentUsers []UserInfo
	var missingUserIDs []string
	for _, user := range allUsers {
		if presentUserIDs[user.ID] {
			presentUsers = append(presentUsers, user)
		} else {
			missingUsers = append(missingUsers, user)
			missingUserIDs = append(missingUserIDs, user.ID)
		}
	}

	// Fetch active statuses for missing users only in the existing web
	// response; the shared Telegram rows intentionally contain no statuses.
	activeStatuses := GetActiveStatusesForUsers(ctx, h.db, missingUserIDs)
	for i := range missingUsers {
		if status, ok := activeStatuses[missingUsers[i].ID]; ok {
			missingUsers[i].ActiveStatus = &status
		}
	}

	// Keep the existing dashboard's per-battery and per-rank breakdowns.
	byBattery := make(map[string]BatteryStats)
	for _, user := range allUsers {
		if user.Battery == nil {
			continue
		}
		battery := *user.Battery
		stats := byBattery[battery]
		stats.Total++
		if presentUserIDs[user.ID] {
			stats.Present++
		}
		byBattery[battery] = stats
	}

	byRank := make(map[string]RankStats)
	for _, user := range allUsers {
		if user.Rank == nil {
			continue
		}
		rank := *user.Rank
		stats := byRank[rank]
		stats.Total++
		if presentUserIDs[user.ID] {
			stats.Present++
		}
		byRank[rank] = stats
	}

	analytics := SessionAnalytics{
		TotalUsers:           summary.Total,
		PresentCount:         summary.Present,
		AttendancePercentage: summary.Percentage,
		ExtraCount:           summary.Extras,
		MissingUsers:         missingUsers,
		PresentUsers:         presentUsers,
		ByBattery:            byBattery,
		ByRank:               byRank,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(analytics); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// GetMissingUsers returns list of users who haven't marked attendance.
func (h *ReportsHandler) GetMissingUsers(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "id")
	actor, _ := middleware.GetUserFromContext(r.Context())
	search := r.URL.Query().Get("q")

	// The shared service bounds each query to ten rows for Telegram. The web
	// dashboard historically returned the complete list, so walk all pages here
	// and keep the existing JSON response shape.
	page := 1
	var missingUsers []UserInfo
	for {
		result, err := h.service.Missing(r.Context(), sessionID, actor, search, page, 10)
		if err != nil {
			if errors.Is(err, reportservice.ErrSessionNotFound) {
				http.Error(w, "Session not found", http.StatusNotFound)
			} else {
				http.Error(w, "Failed to fetch missing users", http.StatusInternalServerError)
			}
			return
		}
		for _, row := range result.Rows {
			fullName, rank, battery := row.Name, row.Rank, row.Battery
			missingUsers = append(missingUsers, UserInfo{
				ID:       row.ID,
				FullName: &fullName,
				Rank:     &rank,
				Battery:  &battery,
			})
		}
		if !result.HasNext {
			break
		}
		page++
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(missingUsers); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

type UserReport struct {
	User           UserInfo        `json:"user"`
	TotalSessions  int             `json:"totalSessions"`
	Attended       int             `json:"attended"`
	AttendanceRate float64         `json:"attendanceRate"`
	RecentSessions []SessionRecord `json:"recentSessions"`
}

type SessionRecord struct {
	SessionID     string `json:"sessionId"`
	SessionName   string `json:"sessionName"`
	MarkedAt      string `json:"markedAt"`
	MarkingMethod string `json:"markingMethod"`
}

// GetUserReport returns attendance history for a specific user
func (h *ReportsHandler) GetUserReport(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	userID := chi.URLParam(r, "userId")

	// Get user info
	var user UserInfo
	var fullName, rank, battery *string
	err := h.db.Pool.QueryRow(ctx, `
		SELECT id, "full_name", rank, battery FROM "user" WHERE id = $1
	`, userID).Scan(&user.ID, &fullName, &rank, &battery)

	if err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}
	user.FullName = fullName
	user.Rank = rank
	user.Battery = battery

	// Get total sessions user is eligible for (based on battery)
	var totalSessions int
	if err := h.db.Pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM attendance_session
		WHERE scope = 'unit_wide' OR $1 = ANY(batteries)
	`, battery).Scan(&totalSessions); err != nil {
		http.Error(w, "Failed to get total sessions", http.StatusInternalServerError)
		return
	}

	// Get attended sessions
	rows, err := h.db.Pool.Query(ctx, `
		SELECT 
			ar.session_id, s.name, ar.marked_at, ar.marking_method
		FROM attendance_record ar
		JOIN attendance_session s ON s.id = ar.session_id
		WHERE ar.user_id = $1
		ORDER BY ar.marked_at DESC
		LIMIT 50
	`, userID)
	if err != nil {
		http.Error(w, "Failed to fetch attendance records", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var recentSessions []SessionRecord
	attended := 0
	for rows.Next() {
		var record SessionRecord
		var markedAt string
		err := rows.Scan(&record.SessionID, &record.SessionName, &markedAt, &record.MarkingMethod)
		if err != nil {
			continue
		}
		record.MarkedAt = markedAt
		recentSessions = append(recentSessions, record)
		attended++
	}

	var attendanceRate float64
	if totalSessions > 0 {
		attendanceRate = float64(attended) / float64(totalSessions) * 100
	}

	report := UserReport{
		User:           user,
		TotalSessions:  totalSessions,
		Attended:       attended,
		AttendanceRate: attendanceRate,
		RecentSessions: recentSessions,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(report); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// GetBatteryReport returns attendance statistics for a battery
func (h *ReportsHandler) GetBatteryReport(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	battery := chi.URLParam(r, "battery")

	// Validate battery
	if battery != "HQ" && battery != "Alpha" && battery != "Bravo" {
		http.Error(w, "Invalid battery", http.StatusBadRequest)
		return
	}

	// Get all sessions (unit-wide or for this battery)
	rows, err := h.db.Pool.Query(ctx, `
		SELECT id, name, start_time
		FROM attendance_session
		WHERE scope = 'unit_wide' OR $1 = ANY(batteries)
		ORDER BY start_time DESC
	`, battery)
	if err != nil {
		http.Error(w, "Failed to fetch sessions", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type BatterySessionStats struct {
		SessionID            string  `json:"sessionId"`
		SessionName          string  `json:"sessionName"`
		StartTime            string  `json:"startTime"`
		TotalUsers           int     `json:"totalUsers"`
		PresentCount         int     `json:"presentCount"`
		AttendancePercentage float64 `json:"attendancePercentage"`
	}

	type BatteryReportResponse struct {
		Battery  string                `json:"battery"`
		Sessions []BatterySessionStats `json:"sessions"`
	}

	var sessions []BatterySessionStats
	for rows.Next() {
		var sessionID, sessionName, startTime string
		err := rows.Scan(&sessionID, &sessionName, &startTime)
		if err != nil {
			continue
		}

		// Count total users in battery (excluding superadmins)
		var totalUsers int
		if err := h.db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM "user" WHERE battery = $1 AND "is_superadmin" = false`, battery).Scan(&totalUsers); err != nil {
			continue // Skip session if count fails
		}

		// Count present users (excluding superadmins)
		var presentCount int
		if err := h.db.Pool.QueryRow(ctx, `
			SELECT COUNT(DISTINCT ar.user_id)
			FROM attendance_record ar
			JOIN "user" u ON u.id = ar.user_id
			WHERE ar.session_id = $1 AND u.battery = $2 AND u."is_superadmin" = false
		`, sessionID, battery).Scan(&presentCount); err != nil {
			continue // Skip session if count fails
		}

		var attendancePercentage float64
		if totalUsers > 0 {
			attendancePercentage = float64(presentCount) / float64(totalUsers) * 100
		}

		sessions = append(sessions, BatterySessionStats{
			SessionID:            sessionID,
			SessionName:          sessionName,
			StartTime:            startTime,
			TotalUsers:           totalUsers,
			PresentCount:         presentCount,
			AttendancePercentage: attendancePercentage,
		})
	}

	response := BatteryReportResponse{
		Battery:  battery,
		Sessions: sessions,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}
