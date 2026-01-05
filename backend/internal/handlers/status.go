package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/davidlivingston/go-nextjs-starter/backend/internal/database"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/middleware"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/models"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/sse"
	"github.com/go-chi/chi/v5"
)

type StatusHandler struct {
	db     *database.DB
	sseHub *sse.Hub
}

func NewStatusHandler(db *database.DB, sseHub *sse.Hub) *StatusHandler {
	return &StatusHandler{db: db, sseHub: sseHub}
}

type CreateStatusRequest struct {
	UserID         string                    `json:"userId"`
	StatusType     string                    `json:"statusType"`
	DailyAfterTime *string                   `json:"dailyAfterTime,omitempty"`
	StartDate      *string                   `json:"startDate,omitempty"`
	EndDate        *string                   `json:"endDate,omitempty"`
	Notes          *string                   `json:"notes,omitempty"`
	Dates          []CreateStatusDateRequest `json:"dates,omitempty"`
}

type CreateStatusDateRequest struct {
	StartDate string `json:"startDate"`
	StartTime string `json:"startTime"`
	EndDate   string `json:"endDate"`
	EndTime   string `json:"endTime"`
}

type UpdateStatusRequest struct {
	StatusType     *string                   `json:"statusType,omitempty"`
	DailyAfterTime *string                   `json:"dailyAfterTime,omitempty"`
	StartDate      *string                   `json:"startDate,omitempty"`
	EndDate        *string                   `json:"endDate,omitempty"`
	Notes          *string                   `json:"notes,omitempty"`
	Dates          []CreateStatusDateRequest `json:"dates,omitempty"`
}

type StatusListResponse struct {
	Statuses []models.UserStatusWithUser `json:"statuses"`
	Total    int                         `json:"total"`
	Page     int                         `json:"page"`
	Limit    int                         `json:"limit"`
}

// ListStatuses retrieves a paginated list of statuses with optional filters
func (h *StatusHandler) ListStatuses(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()

	// Parse query parameters
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 || limit > 100 {
		limit = 20
	}
	offset := (page - 1) * limit

	statusType := r.URL.Query().Get("statusType")
	userID := r.URL.Query().Get("userId")
	activeOnly := r.URL.Query().Get("active") == "true"

	// Build WHERE clause
	whereClause := "WHERE 1=1"
	args := []interface{}{}
	argIndex := 1

	if statusType != "" {
		whereClause += fmt.Sprintf(" AND s.status_type = $%d", argIndex)
		args = append(args, statusType)
		argIndex++
	}
	if userID != "" {
		whereClause += fmt.Sprintf(" AND s.user_id = $%d", argIndex)
		args = append(args, userID)
		argIndex++
	}

	// For active filtering, we need to check based on status type
	if activeOnly {
		whereClause += ` AND (
			(s.status_type = 'stay_out') OR
			(s.status_type = 'mc' AND CURRENT_DATE BETWEEN s.start_date AND s.end_date) OR
			(s.status_type IN ('off_pass', 'course', 'other') AND EXISTS (
				SELECT 1 FROM user_status_date sd
				WHERE sd.status_id = s.id AND sd.end_date >= CURRENT_DATE
			))
		)`
	}

	// Count total
	var total int
	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM user_status s %s`, whereClause)
	err := h.db.Pool.QueryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		http.Error(w, "Failed to count statuses", http.StatusInternalServerError)
		return
	}

	// Get statuses with user info
	query := fmt.Sprintf(`
		SELECT
			s.id, s.user_id, s.status_type, s.daily_after_time, s.start_date, s.end_date,
			s.notes, s.created_by, s."createdAt", s."updatedAt",
			u.full_name, u.rank, u.battery
		FROM user_status s
		LEFT JOIN "user" u ON s.user_id = u.id
		%s
		ORDER BY s."createdAt" DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, argIndex, argIndex+1)
	args = append(args, limit, offset)

	rows, err := h.db.Pool.Query(ctx, query, args...)
	if err != nil {
		http.Error(w, "Failed to fetch statuses", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	statuses := make([]models.UserStatusWithUser, 0)
	for rows.Next() {
		var status models.UserStatusWithUser
		var dailyAfterTime, startDate, endDate, notes *string
		var createdAt, updatedAt time.Time

		err := rows.Scan(
			&status.ID,
			&status.UserID,
			&status.StatusType,
			&dailyAfterTime,
			&startDate,
			&endDate,
			&notes,
			&status.CreatedBy,
			&createdAt,
			&updatedAt,
			&status.UserFullName,
			&status.UserRank,
			&status.UserBattery,
		)
		if err != nil {
			continue
		}

		status.DailyAfterTime = dailyAfterTime
		status.StartDate = startDate
		status.EndDate = endDate
		status.Notes = notes
		status.CreatedAt = createdAt
		status.UpdatedAt = updatedAt

		// Fetch dates for this status
		status.Dates = h.getStatusDates(ctx, status.ID)

		statuses = append(statuses, status)
	}

	response := StatusListResponse{
		Statuses: statuses,
		Total:    total,
		Page:     page,
		Limit:    limit,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// CreateStatus creates a new user status
func (h *StatusHandler) CreateStatus(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	currentUserID := r.Context().Value(middleware.UserIDKey).(string)

	var req CreateStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.UserID == "" || req.StatusType == "" {
		http.Error(w, "userId and statusType are required", http.StatusBadRequest)
		return
	}

	// Validate status type
	if !models.IsValidStatusType(req.StatusType) {
		http.Error(w, "Invalid status type", http.StatusBadRequest)
		return
	}

	// Validate type-specific fields
	switch req.StatusType {
	case models.StatusTypeStayOut:
		if req.DailyAfterTime == nil || *req.DailyAfterTime == "" {
			http.Error(w, "dailyAfterTime is required for stay_out status", http.StatusBadRequest)
			return
		}
	case models.StatusTypeMC:
		if req.StartDate == nil || req.EndDate == nil || *req.StartDate == "" || *req.EndDate == "" {
			http.Error(w, "startDate and endDate are required for mc status", http.StatusBadRequest)
			return
		}
	case models.StatusTypeOffPass, models.StatusTypeCourse, models.StatusTypeOther:
		if len(req.Dates) == 0 {
			http.Error(w, "dates are required for this status type", http.StatusBadRequest)
			return
		}
		// Validate notes for "other" type
		if req.StatusType == models.StatusTypeOther && (req.Notes == nil || *req.Notes == "") {
			http.Error(w, "notes are required for other status type", http.StatusBadRequest)
			return
		}
	}

	// Verify user exists
	var exists bool
	err := h.db.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM "user" WHERE id = $1)`, req.UserID).Scan(&exists)
	if err != nil || !exists {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	// Generate status ID
	statusID := generateID()
	now := time.Now()

	// Insert status
	_, err = h.db.Pool.Exec(ctx, `
		INSERT INTO user_status (
			id, user_id, status_type, daily_after_time, start_date, end_date,
			notes, created_by, "createdAt", "updatedAt"
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, statusID, req.UserID, req.StatusType, req.DailyAfterTime, req.StartDate, req.EndDate,
		req.Notes, currentUserID, now, now)

	if err != nil {
		http.Error(w, "Failed to create status", http.StatusInternalServerError)
		return
	}

	// Insert status dates if applicable
	for _, date := range req.Dates {
		dateID := generateID()
		_, err = h.db.Pool.Exec(ctx, `
			INSERT INTO user_status_date (id, status_id, start_date, start_time, end_date, end_time, "createdAt")
			VALUES ($1, $2, $3, $4, $5, $6, $7)
		`, dateID, statusID, date.StartDate, date.StartTime, date.EndDate, date.EndTime, now)
		if err != nil {
			// Rollback by deleting the status
			_, _ = h.db.Pool.Exec(ctx, `DELETE FROM user_status WHERE id = $1`, statusID)
			http.Error(w, "Failed to create status dates", http.StatusInternalServerError)
			return
		}
	}

	// Return created status
	status := models.UserStatus{
		ID:             statusID,
		UserID:         req.UserID,
		StatusType:     req.StatusType,
		DailyAfterTime: req.DailyAfterTime,
		StartDate:      req.StartDate,
		EndDate:        req.EndDate,
		Notes:          req.Notes,
		CreatedBy:      currentUserID,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	// Broadcast status change to all connected clients
	h.sseHub.BroadcastToAll(sse.Event{
		Type: sse.EventTypeStatusChanged,
		Payload: sse.StatusChangedPayload{
			UserID:     req.UserID,
			StatusType: req.StatusType,
			Action:     "created",
		},
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(status); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// GetStatus retrieves a single status by ID
func (h *StatusHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	statusID := chi.URLParam(r, "id")

	var status models.UserStatusWithUser
	var dailyAfterTime, startDate, endDate, notes *string
	var createdAt, updatedAt time.Time

	err := h.db.Pool.QueryRow(ctx, `
		SELECT
			s.id, s.user_id, s.status_type, s.daily_after_time, s.start_date, s.end_date,
			s.notes, s.created_by, s."createdAt", s."updatedAt",
			u.full_name, u.rank, u.battery
		FROM user_status s
		LEFT JOIN "user" u ON s.user_id = u.id
		WHERE s.id = $1
	`, statusID).Scan(
		&status.ID,
		&status.UserID,
		&status.StatusType,
		&dailyAfterTime,
		&startDate,
		&endDate,
		&notes,
		&status.CreatedBy,
		&createdAt,
		&updatedAt,
		&status.UserFullName,
		&status.UserRank,
		&status.UserBattery,
	)

	if err != nil {
		http.Error(w, "Status not found", http.StatusNotFound)
		return
	}

	status.DailyAfterTime = dailyAfterTime
	status.StartDate = startDate
	status.EndDate = endDate
	status.Notes = notes
	status.CreatedAt = createdAt
	status.UpdatedAt = updatedAt
	status.Dates = h.getStatusDates(ctx, statusID)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(status); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// UpdateStatus updates a status
func (h *StatusHandler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	statusID := chi.URLParam(r, "id")

	var req UpdateStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Check status exists and get userId for broadcast
	var userID, currentStatusType string
	err := h.db.Pool.QueryRow(ctx, `SELECT user_id, status_type FROM user_status WHERE id = $1`, statusID).Scan(&userID, &currentStatusType)
	if err != nil {
		http.Error(w, "Status not found", http.StatusNotFound)
		return
	}

	// Validate status type if provided
	if req.StatusType != nil && !models.IsValidStatusType(*req.StatusType) {
		http.Error(w, "Invalid status type", http.StatusBadRequest)
		return
	}

	// Build update query dynamically
	updates := []string{}
	args := []interface{}{}
	argIndex := 1

	if req.StatusType != nil {
		updates = append(updates, fmt.Sprintf("status_type = $%d", argIndex))
		args = append(args, *req.StatusType)
		argIndex++
	}
	if req.DailyAfterTime != nil {
		updates = append(updates, fmt.Sprintf("daily_after_time = $%d", argIndex))
		args = append(args, *req.DailyAfterTime)
		argIndex++
	}
	if req.StartDate != nil {
		updates = append(updates, fmt.Sprintf("start_date = $%d", argIndex))
		args = append(args, *req.StartDate)
		argIndex++
	}
	if req.EndDate != nil {
		updates = append(updates, fmt.Sprintf("end_date = $%d", argIndex))
		args = append(args, *req.EndDate)
		argIndex++
	}
	if req.Notes != nil {
		updates = append(updates, fmt.Sprintf("notes = $%d", argIndex))
		args = append(args, *req.Notes)
		argIndex++
	}

	// Always update updatedAt
	updates = append(updates, fmt.Sprintf(`"updatedAt" = $%d`, argIndex))
	args = append(args, time.Now())
	argIndex++

	if len(updates) > 1 { // More than just updatedAt
		args = append(args, statusID)
		query := fmt.Sprintf(`UPDATE user_status SET %s WHERE id = $%d`,
			strings.Join(updates, ", "), argIndex)
		_, err = h.db.Pool.Exec(ctx, query, args...)
		if err != nil {
			http.Error(w, "Failed to update status", http.StatusInternalServerError)
			return
		}
	}

	// Update dates if provided
	if req.Dates != nil {
		// Delete existing dates
		_, err = h.db.Pool.Exec(ctx, `DELETE FROM user_status_date WHERE status_id = $1`, statusID)
		if err != nil {
			http.Error(w, "Failed to update status dates", http.StatusInternalServerError)
			return
		}

		// Insert new dates
		now := time.Now()
		for _, date := range req.Dates {
			dateID := generateID()
			_, err = h.db.Pool.Exec(ctx, `
				INSERT INTO user_status_date (id, status_id, start_date, start_time, end_date, end_time, "createdAt")
				VALUES ($1, $2, $3, $4, $5, $6, $7)
			`, dateID, statusID, date.StartDate, date.StartTime, date.EndDate, date.EndTime, now)
			if err != nil {
				http.Error(w, "Failed to update status dates", http.StatusInternalServerError)
				return
			}
		}
	}

	// Broadcast status change to all connected clients
	statusType := currentStatusType
	if req.StatusType != nil {
		statusType = *req.StatusType
	}
	h.sseHub.BroadcastToAll(sse.Event{
		Type: sse.EventTypeStatusChanged,
		Payload: sse.StatusChangedPayload{
			UserID:     userID,
			StatusType: statusType,
			Action:     "updated",
		},
	})

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"message": "Status updated successfully"}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// DeleteStatus deletes a status
func (h *StatusHandler) DeleteStatus(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	statusID := chi.URLParam(r, "id")

	// Get userId and statusType before deletion for broadcast
	var userID, statusType string
	err := h.db.Pool.QueryRow(ctx, `SELECT user_id, status_type FROM user_status WHERE id = $1`, statusID).Scan(&userID, &statusType)
	if err != nil {
		http.Error(w, "Status not found", http.StatusNotFound)
		return
	}

	_, err = h.db.Pool.Exec(ctx, `DELETE FROM user_status WHERE id = $1`, statusID)
	if err != nil {
		http.Error(w, "Failed to delete status", http.StatusInternalServerError)
		return
	}

	// Broadcast status change to all connected clients
	h.sseHub.BroadcastToAll(sse.Event{
		Type: sse.EventTypeStatusChanged,
		Payload: sse.StatusChangedPayload{
			UserID:     userID,
			StatusType: statusType,
			Action:     "deleted",
		},
	})

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"message": "Status deleted successfully"}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// getStatusDates fetches dates for a status
func (h *StatusHandler) getStatusDates(ctx context.Context, statusID string) []models.UserStatusDate {
	rows, err := h.db.Pool.Query(ctx, `
		SELECT id, status_id, start_date, start_time, end_date, end_time
		FROM user_status_date
		WHERE status_id = $1
		ORDER BY start_date ASC
	`, statusID)
	if err != nil {
		return []models.UserStatusDate{}
	}
	defer rows.Close()

	dates := make([]models.UserStatusDate, 0)
	for rows.Next() {
		var d models.UserStatusDate
		var startDateVal, endDateVal time.Time
		var startTime, endTime string

		if err := rows.Scan(&d.ID, &d.StatusID, &startDateVal, &startTime, &endDateVal, &endTime); err != nil {
			continue
		}

		d.StartDate = startDateVal.Format("2006-01-02")
		d.StartTime = startTime
		d.EndDate = endDateVal.Format("2006-01-02")
		d.EndTime = endTime
		dates = append(dates, d)
	}

	return dates
}

// GetActiveStatusesForUsers returns active statuses for a list of user IDs.
// Uses Singapore timezone (UTC+8) with FixedZone to avoid timezone database dependency in Docker.
func GetActiveStatusesForUsers(ctx context.Context, db *database.DB, userIDs []string) map[string]models.StatusInfo {
	if len(userIDs) == 0 {
		return make(map[string]models.StatusInfo)
	}

	// Build query with IN clause
	placeholders := make([]string, len(userIDs))
	args := make([]interface{}, len(userIDs))
	for i, id := range userIDs {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}

	// Get current time in Singapore timezone (UTC+8)
	// Use FixedZone to avoid dependency on timezone database in Docker containers
	loc := time.FixedZone("SGT", 8*60*60)
	now := time.Now().In(loc)
	currentDate := now.Format("2006-01-02")
	currentTime := now.Format("15:04:05")

	query := fmt.Sprintf(`
		SELECT DISTINCT ON (s.user_id) s.user_id, s.status_type
		FROM user_status s
		LEFT JOIN user_status_date sd ON s.id = sd.status_id
		WHERE s.user_id IN (%s)
		AND (
			(s.status_type = 'stay_out' AND s.daily_after_time <= '%s'::time) OR
			(s.status_type = 'mc' AND '%s'::date BETWEEN s.start_date AND s.end_date) OR
			(s.status_type IN ('off_pass', 'course', 'other')
				AND '%s'::date BETWEEN sd.start_date AND sd.end_date
				AND '%s'::time BETWEEN sd.start_time AND sd.end_time)
		)
		ORDER BY s.user_id, s."createdAt" DESC
	`, strings.Join(placeholders, ", "), currentTime, currentDate, currentDate, currentTime)

	rows, err := db.Pool.Query(ctx, query, args...)
	if err != nil {
		return make(map[string]models.StatusInfo)
	}
	defer rows.Close()

	result := make(map[string]models.StatusInfo)
	for rows.Next() {
		var userID, statusType string
		if err := rows.Scan(&userID, &statusType); err != nil {
			continue
		}
		result[userID] = models.StatusInfo{
			StatusType:  statusType,
			DisplayName: models.StatusTypeDisplayNames[statusType],
		}
	}

	return result
}
