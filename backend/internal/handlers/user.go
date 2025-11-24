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
	"github.com/go-chi/chi/v5"
)

type UserHandler struct {
	db *database.DB
}

func NewUserHandler(db *database.DB) *UserHandler {
	return &UserHandler{db: db}
}

type UserProfile struct {
	ID           string  `json:"id"`
	FullName     *string `json:"fullName,omitempty"`
	Rank         *string `json:"rank,omitempty"`
	Battery      *string `json:"battery,omitempty"`
	NRICLast4    *string `json:"nricLast4,omitempty"`
	DOB          *string `json:"dob,omitempty"`
	IsSuperadmin bool    `json:"isSuperadmin"`
	CreatedAt    string  `json:"createdAt"`
	UpdatedAt    string  `json:"updatedAt"`
}

type UpdateProfileRequest struct {
	FullName  *string `json:"fullName,omitempty"`
	Rank      *string `json:"rank,omitempty"`
	Battery   *string `json:"battery,omitempty"`
	NRICLast4 *string `json:"nricLast4,omitempty"`
	DOB       *string `json:"dob,omitempty"`
}

type UserListResponse struct {
	Users []UserProfile `json:"users"`
	Total int           `json:"total"`
	Page  int           `json:"page"`
	Limit int           `json:"limit"`
}

// GetProfile retrieves the current user's profile
func (h *UserHandler) GetProfile(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDKey).(string)

	var profile UserProfile
	var fullName, rank, battery, nricLast4, dob *string

	err := h.db.Pool.QueryRow(
		context.Background(),
		`SELECT 
			id, "full_name", rank, battery, "nric_last4", dob, "is_superadmin",
			"createdAt", "updatedAt"
		 FROM "user" WHERE id = $1`,
		userID,
	).Scan(
		&profile.ID,
		&fullName,
		&rank,
		&battery,
		&nricLast4,
		&dob,
		&profile.IsSuperadmin,
		&profile.CreatedAt,
		&profile.UpdatedAt,
	)

	if err != nil {
		http.Error(w, "Failed to fetch profile", http.StatusInternalServerError)
		return
	}

	profile.FullName = fullName
	profile.Rank = rank
	profile.Battery = battery
	profile.NRICLast4 = nricLast4
	profile.DOB = dob

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(profile); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// ListUsers retrieves a paginated list of users with optional filters
func (h *UserHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
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

	search := r.URL.Query().Get("search")
	battery := r.URL.Query().Get("battery")
	rank := r.URL.Query().Get("rank")

	// Build WHERE clause and args
	whereClause := "WHERE 1=1"
	args := []interface{}{}
	argIndex := 1

	if search != "" {
		whereClause += fmt.Sprintf(" AND \"full_name\" ILIKE $%d", argIndex)
		args = append(args, "%"+search+"%")
		argIndex++
	}
	if battery != "" {
		whereClause += fmt.Sprintf(" AND battery = $%d", argIndex)
		args = append(args, battery)
		argIndex++
	}
	if rank != "" {
		whereClause += fmt.Sprintf(" AND rank = $%d", argIndex)
		args = append(args, rank)
		argIndex++
	}

	// Count total
	var total int
	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM "user" %s`, whereClause)
	err := h.db.Pool.QueryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		http.Error(w, "Failed to count users", http.StatusInternalServerError)
		return
	}

	// Get users
	query := fmt.Sprintf(`SELECT 
		id, "full_name", rank, battery, "nric_last4", dob, "is_superadmin",
		"createdAt", "updatedAt"
	FROM "user"
	%s ORDER BY "createdAt" DESC LIMIT $%d OFFSET $%d`, whereClause, argIndex, argIndex+1)
	args = append(args, limit, offset)

	rows, err := h.db.Pool.Query(ctx, query, args...)
	if err != nil {
		http.Error(w, "Failed to fetch users", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var users []UserProfile
	for rows.Next() {
		var profile UserProfile
		var fullName, rank, battery, nricLast4, dob *string

		err := rows.Scan(
			&profile.ID,
			&fullName,
			&rank,
			&battery,
			&nricLast4,
			&dob,
			&profile.IsSuperadmin,
			&profile.CreatedAt,
			&profile.UpdatedAt,
		)
		if err != nil {
			continue
		}

		profile.FullName = fullName
		profile.Rank = rank
		profile.Battery = battery
		profile.NRICLast4 = nricLast4
		profile.DOB = dob

		users = append(users, profile)
	}

	response := UserListResponse{
		Users: users,
		Total: total,
		Page:  page,
		Limit: limit,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// GetUser retrieves a single user by ID
func (h *UserHandler) GetUser(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	userID := chi.URLParam(r, "id")

	var profile UserProfile
	var fullName, rank, battery, nricLast4, dob *string

	err := h.db.Pool.QueryRow(ctx, `
		SELECT 
			id, "full_name", rank, battery, "nric_last4", dob, "is_superadmin",
			"createdAt", "updatedAt"
		FROM "user"
		WHERE id = $1
	`, userID).Scan(
		&profile.ID,
		&fullName,
		&rank,
		&battery,
		&nricLast4,
		&dob,
		&profile.IsSuperadmin,
		&profile.CreatedAt,
		&profile.UpdatedAt,
	)

	if err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	profile.FullName = fullName
	profile.Rank = rank
	profile.Battery = battery
	profile.NRICLast4 = nricLast4
	profile.DOB = dob

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(profile)
}

// UpdateUser updates a user's profile
func (h *UserHandler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	userID := chi.URLParam(r, "id")
	currentUserID := r.Context().Value(middleware.UserIDKey).(string)

	// Get current user to check permissions
	var currentUser models.User
	err := h.db.Pool.QueryRow(ctx, `
		SELECT "is_superadmin" FROM "user" WHERE id = $1
	`, currentUserID).Scan(&currentUser.IsSuperadmin)
	if err != nil {
		http.Error(w, "Failed to verify permissions", http.StatusInternalServerError)
		return
	}

	// Only superadmin can edit other users, users can edit themselves
	if userID != currentUserID && !currentUser.IsSuperadmin {
		http.Error(w, "Insufficient permissions", http.StatusForbidden)
		return
	}

	var req UpdateProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Build update query dynamically
	updates := []string{}
	args := []interface{}{}
	argIndex := 1

	if req.FullName != nil {
		updates = append(updates, fmt.Sprintf("\"full_name\" = $%d", argIndex))
		args = append(args, *req.FullName)
		argIndex++
	}
	if req.Rank != nil && currentUser.IsSuperadmin {
		// Validate rank
		validRank := false
		for _, r := range models.ValidRanks {
			if r == *req.Rank {
				validRank = true
				break
			}
		}
		if !validRank {
			http.Error(w, "Invalid rank", http.StatusBadRequest)
			return
		}
		updates = append(updates, fmt.Sprintf("rank = $%d", argIndex))
		args = append(args, *req.Rank)
		argIndex++
	}
	if req.Battery != nil && currentUser.IsSuperadmin {
		if *req.Battery != models.BatteryHQ && *req.Battery != models.BatteryAlpha && *req.Battery != models.BatteryBravo {
			http.Error(w, "Invalid battery", http.StatusBadRequest)
			return
		}
		updates = append(updates, fmt.Sprintf("battery = $%d", argIndex))
		args = append(args, *req.Battery)
		argIndex++
	}
	if req.NRICLast4 != nil && currentUser.IsSuperadmin {
		updates = append(updates, fmt.Sprintf("\"nric_last4\" = $%d", argIndex))
		args = append(args, *req.NRICLast4)
		argIndex++
	}
	if req.DOB != nil && currentUser.IsSuperadmin {
		if len(*req.DOB) != 6 {
			http.Error(w, "Invalid DOB format (must be DDMMYY)", http.StatusBadRequest)
			return
		}
		updates = append(updates, fmt.Sprintf("dob = $%d", argIndex))
		args = append(args, *req.DOB)
		argIndex++
	}

	if len(updates) == 0 {
		http.Error(w, "No fields to update", http.StatusBadRequest)
		return
	}

	updates = append(updates, fmt.Sprintf("\"updatedAt\" = $%d", argIndex))
	args = append(args, time.Now())
	argIndex++

	args = append(args, userID)

	query := fmt.Sprintf(`UPDATE "user" SET %s WHERE id = $%d`, strings.Join(updates, ", "), argIndex)
	_, err = h.db.Pool.Exec(ctx, query, args...)
	if err != nil {
		http.Error(w, "Failed to update user", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "User updated successfully"})
}

// DeleteUser soft deletes a user (superadmin only)
func (h *UserHandler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	userID := chi.URLParam(r, "id")

	// For now, hard delete. Can be changed to soft delete later
	_, err := h.db.Pool.Exec(ctx, `DELETE FROM "user" WHERE id = $1`, userID)
	if err != nil {
		http.Error(w, "Failed to delete user", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "User deleted successfully"})
}

// UpdateProfile updates the user's profile
func (h *UserHandler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDKey).(string)

	var req UpdateProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Build update query dynamically
	updates := []string{}
	args := []interface{}{}
	argIndex := 1

	if req.FullName != nil {
		updates = append(updates, fmt.Sprintf("\"full_name\" = $%d", argIndex))
		args = append(args, *req.FullName)
		argIndex++
	}

	if len(updates) == 0 {
		http.Error(w, "No fields to update", http.StatusBadRequest)
		return
	}

	updates = append(updates, fmt.Sprintf("\"updatedAt\" = $%d", argIndex))
	args = append(args, time.Now())
	argIndex++
	args = append(args, userID)

	query := fmt.Sprintf(`UPDATE "user" SET %s WHERE id = $%d`, strings.Join(updates, ", "), argIndex)
	_, err := h.db.Pool.Exec(context.Background(), query, args...)

	if err != nil {
		http.Error(w, "Failed to update profile", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"message": "Profile updated successfully"}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}
