package handlers

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/davidlivingston/go-nextjs-starter/backend/internal/database"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/middleware"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/models"
	"github.com/go-chi/chi/v5"
)

type ReportsHandler struct {
	db *database.DB
}

func NewReportsHandler(db *database.DB) *ReportsHandler {
	return &ReportsHandler{db: db}
}

type SessionAnalytics struct {
	TotalUsers           int                     `json:"totalUsers"`
	PresentCount         int                     `json:"presentCount"`
	AttendancePercentage float64                 `json:"attendancePercentage"`
	MissingUsers         []UserInfo              `json:"missingUsers"`
	PresentUsers         []UserInfo              `json:"presentUsers"`
	ByBattery            map[string]BatteryStats `json:"byBattery"`
	ByRank               map[string]RankStats    `json:"byRank"`
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

// GetSessionAnalytics returns detailed analytics for a session
func (h *ReportsHandler) GetSessionAnalytics(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	sessionID := chi.URLParam(r, "id")

	// Verify session exists
	var sessionScope string
	var sessionBatteries []string
	err := h.db.Pool.QueryRow(ctx, `
		SELECT scope, batteries FROM attendance_session WHERE id = $1
	`, sessionID).Scan(&sessionScope, &sessionBatteries)

	if err != nil {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}

	// Tier 2 users see only their own battery's slice of the analytics.
	currentUser, _ := middleware.GetUserFromContext(r.Context())
	var batteryScope *string
	if currentUser != nil && currentUser.GetTier() == models.TierBatteryNCO && currentUser.Battery != nil {
		batteryScope = currentUser.Battery
	}

	// Build user query based on session scope + optional battery scope for Tier 2.
	var userQuery string
	var userArgs []any
	switch sessionScope {
	case models.SessionScopeCustomList:
		if batteryScope != nil {
			userQuery = `SELECT u.id, u."full_name", u.rank, u.battery FROM "user" u
				JOIN session_participants sp ON sp.user_id = u.id
				WHERE sp.session_id = $1 AND u."is_superadmin" = false AND u.battery = $2`
			userArgs = []any{sessionID, *batteryScope}
		} else {
			userQuery = `SELECT u.id, u."full_name", u.rank, u.battery FROM "user" u
				JOIN session_participants sp ON sp.user_id = u.id
				WHERE sp.session_id = $1 AND u."is_superadmin" = false`
			userArgs = []any{sessionID}
		}
	case models.SessionScopeUnitWide:
		if batteryScope != nil {
			userQuery = `SELECT id, "full_name", rank, battery FROM "user" WHERE "is_superadmin" = false AND battery = $1`
			userArgs = []any{*batteryScope}
		} else {
			userQuery = `SELECT id, "full_name", rank, battery FROM "user" WHERE "is_superadmin" = false`
		}
	default: // battery_specific
		if batteryScope != nil {
			userQuery = `SELECT id, "full_name", rank, battery FROM "user" WHERE "is_superadmin" = false AND battery = $1`
			userArgs = []any{*batteryScope}
		} else {
			userQuery = `SELECT id, "full_name", rank, battery FROM "user" WHERE "is_superadmin" = false AND battery = ANY($1)`
			userArgs = []any{sessionBatteries}
		}
	}

	// Get all eligible users
	rows, err := h.db.Pool.Query(ctx, userQuery, userArgs...)
	if err != nil {
		http.Error(w, "Failed to fetch users", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var allUsers []UserInfo
	userMap := make(map[string]UserInfo)
	for rows.Next() {
		var user UserInfo
		var fullName, rank, battery *string
		err := rows.Scan(&user.ID, &fullName, &rank, &battery)
		if err != nil {
			continue
		}
		user.FullName = fullName
		user.Rank = rank
		user.Battery = battery
		allUsers = append(allUsers, user)
		userMap[user.ID] = user
	}

	// Get marked attendance
	rows, err = h.db.Pool.Query(ctx, `
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

	// Calculate statistics
	totalUsers := len(allUsers)
	presentCount := len(presentUserIDs)
	var attendancePercentage float64
	if totalUsers > 0 {
		attendancePercentage = float64(presentCount) / float64(totalUsers) * 100
	}

	// Find missing and present users
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

	// Fetch active statuses for missing users
	activeStatuses := GetActiveStatusesForUsers(ctx, h.db, missingUserIDs)
	for i := range missingUsers {
		if status, ok := activeStatuses[missingUsers[i].ID]; ok {
			missingUsers[i].ActiveStatus = &status
		}
	}

	// Calculate by battery
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

	// Calculate by rank
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
		TotalUsers:           totalUsers,
		PresentCount:         presentCount,
		AttendancePercentage: attendancePercentage,
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

// GetMissingUsers returns list of users who haven't marked attendance
func (h *ReportsHandler) GetMissingUsers(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	sessionID := chi.URLParam(r, "id")

	// Get session scope
	var sessionScope string
	var sessionBatteries []string
	err := h.db.Pool.QueryRow(ctx, `
		SELECT scope, batteries FROM attendance_session WHERE id = $1
	`, sessionID).Scan(&sessionScope, &sessionBatteries)

	if err != nil {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}

	// Tier 2 battery scoping for missing list.
	missingUser, _ := middleware.GetUserFromContext(r.Context())
	var missingBatteryScope *string
	if missingUser != nil && missingUser.GetTier() == models.TierBatteryNCO && missingUser.Battery != nil {
		missingBatteryScope = missingUser.Battery
	}

	var userQuery string
	var userArgs []any
	switch sessionScope {
	case models.SessionScopeCustomList:
		if missingBatteryScope != nil {
			userQuery = `
				SELECT u.id, u."full_name", u.rank, u.battery
				FROM "user" u
				JOIN session_participants sp ON sp.user_id = u.id
				WHERE sp.session_id = $1 AND u."is_superadmin" = false AND u.battery = $2
				AND NOT EXISTS (SELECT 1 FROM attendance_record ar WHERE ar.session_id = $1 AND ar.user_id = u.id)
			`
			userArgs = []any{sessionID, *missingBatteryScope}
		} else {
			userQuery = `
				SELECT u.id, u."full_name", u.rank, u.battery
				FROM "user" u
				JOIN session_participants sp ON sp.user_id = u.id
				WHERE sp.session_id = $1 AND u."is_superadmin" = false
				AND NOT EXISTS (SELECT 1 FROM attendance_record ar WHERE ar.session_id = $1 AND ar.user_id = u.id)
			`
			userArgs = []any{sessionID}
		}
	case models.SessionScopeUnitWide:
		if missingBatteryScope != nil {
			userQuery = `
				SELECT u.id, u."full_name", u.rank, u.battery FROM "user" u
				WHERE u."is_superadmin" = false AND u.battery = $1
				AND NOT EXISTS (SELECT 1 FROM attendance_record ar WHERE ar.session_id = $2 AND ar.user_id = u.id)
			`
			userArgs = []any{*missingBatteryScope, sessionID}
		} else {
			userQuery = `
				SELECT u.id, u."full_name", u.rank, u.battery FROM "user" u
				WHERE u."is_superadmin" = false
				AND NOT EXISTS (SELECT 1 FROM attendance_record ar WHERE ar.session_id = $1 AND ar.user_id = u.id)
			`
			userArgs = []any{sessionID}
		}
	default: // battery_specific
		if missingBatteryScope != nil {
			userQuery = `
				SELECT u.id, u."full_name", u.rank, u.battery FROM "user" u
				WHERE u."is_superadmin" = false AND u.battery = $1
				AND NOT EXISTS (SELECT 1 FROM attendance_record ar WHERE ar.session_id = $2 AND ar.user_id = u.id)
			`
			userArgs = []any{*missingBatteryScope, sessionID}
		} else {
			userQuery = `
				SELECT u.id, u."full_name", u.rank, u.battery FROM "user" u
				WHERE u."is_superadmin" = false AND u.battery = ANY($1)
				AND NOT EXISTS (SELECT 1 FROM attendance_record ar WHERE ar.session_id = $2 AND ar.user_id = u.id)
			`
			userArgs = []any{sessionBatteries, sessionID}
		}
	}

	rows, err := h.db.Pool.Query(ctx, userQuery, userArgs...)
	if err != nil {
		http.Error(w, "Failed to fetch missing users", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var missingUsers []UserInfo
	for rows.Next() {
		var user UserInfo
		var fullName, rank, battery *string
		err := rows.Scan(&user.ID, &fullName, &rank, &battery)
		if err != nil {
			continue
		}
		user.FullName = fullName
		user.Rank = rank
		user.Battery = battery
		missingUsers = append(missingUsers, user)
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
