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
	"golang.org/x/crypto/bcrypt"
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
	NRICLast5    *string `json:"nricLast5,omitempty"`
	DOB          *string `json:"dob,omitempty"`
	IsSuperadmin bool    `json:"isSuperadmin"`
	CreatedAt    string  `json:"createdAt"`
	UpdatedAt    string  `json:"updatedAt"`
}

type UpdateProfileRequest struct {
	FullName  *string `json:"fullName,omitempty"`
	Rank      *string `json:"rank,omitempty"`
	Battery   *string `json:"battery,omitempty"`
	NRICLast5 *string `json:"nricLast5,omitempty"`
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
	var fullName, rank, battery, nricLast5, dob *string
	var createdAt, updatedAt time.Time

	err := h.db.Pool.QueryRow(
		context.Background(),
		`SELECT 
			id, "full_name", rank, battery, "nric_last5", dob, "is_superadmin",
			"createdAt", "updatedAt"
		 FROM "user" WHERE id = $1`,
		userID,
	).Scan(
		&profile.ID,
		&fullName,
		&rank,
		&battery,
		&nricLast5,
		&dob,
		&profile.IsSuperadmin,
		&createdAt,
		&updatedAt,
	)

	if err != nil {
		http.Error(w, "Failed to fetch profile", http.StatusInternalServerError)
		return
	}

	profile.FullName = fullName
	profile.Rank = rank
	profile.Battery = battery
	profile.NRICLast5 = nricLast5
	profile.DOB = dob
	profile.CreatedAt = createdAt.Format(time.RFC3339)
	profile.UpdatedAt = updatedAt.Format(time.RFC3339)

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
	// Exclude admin user by ID
	adminID := "00000000000000000000000000000000"
	whereClause := fmt.Sprintf(`WHERE id != $%d`, 1)
	args := []interface{}{adminID}
	argIndex := 2

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
		id, "full_name", rank, battery, "nric_last5", dob, "is_superadmin",
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

	users := make([]UserProfile, 0) // Initialize as empty slice, not nil
	for rows.Next() {
		var profile UserProfile
		var fullName, rank, battery, nricLast5, dob *string
		var createdAt, updatedAt time.Time

		err := rows.Scan(
			&profile.ID,
			&fullName,
			&rank,
			&battery,
			&nricLast5,
			&dob,
			&profile.IsSuperadmin,
			&createdAt,
			&updatedAt,
		)
		if err != nil {
			continue
		}

		profile.FullName = fullName
		profile.Rank = rank
		profile.Battery = battery
		profile.NRICLast5 = nricLast5
		profile.DOB = dob
		profile.CreatedAt = createdAt.Format(time.RFC3339)
		profile.UpdatedAt = updatedAt.Format(time.RFC3339)

		users = append(users, profile)
	}

	response := UserListResponse{
		Users: users,
		Total: total,
		Page:  page,
		Limit: limit,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// GetUser retrieves a single user by ID (excludes admin users - they should not be accessible via this endpoint)
func (h *UserHandler) GetUser(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	userID := chi.URLParam(r, "id")

	var profile UserProfile
	var fullName, rank, battery, nricLast5, dob *string
	var createdAt, updatedAt time.Time

	// Exclude admin user by ID
	adminID := "00000000000000000000000000000000"
	if userID == adminID {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	err := h.db.Pool.QueryRow(ctx, `
		SELECT 
			id, "full_name", rank, battery, "nric_last5", dob, "is_superadmin",
			"createdAt", "updatedAt"
		FROM "user"
		WHERE id = $1
	`, userID).Scan(
		&profile.ID,
		&fullName,
		&rank,
		&battery,
		&nricLast5,
		&dob,
		&profile.IsSuperadmin,
		&createdAt,
		&updatedAt,
	)

	if err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	profile.FullName = fullName
	profile.Rank = rank
	profile.Battery = battery
	profile.NRICLast5 = nricLast5
	profile.DOB = dob
	profile.CreatedAt = createdAt.Format(time.RFC3339)
	profile.UpdatedAt = updatedAt.Format(time.RFC3339)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(profile); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
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
	if req.NRICLast5 != nil && currentUser.IsSuperadmin {
		updates = append(updates, fmt.Sprintf("\"nric_last5\" = $%d", argIndex))
		args = append(args, *req.NRICLast5)
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
	if err := json.NewEncoder(w).Encode(map[string]string{"message": "User updated successfully"}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
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
	if err := json.NewEncoder(w).Encode(map[string]string{"message": "User deleted successfully"}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
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

	if req.Rank != nil {
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

	if req.Battery != nil {
		if *req.Battery != models.BatteryHQ && *req.Battery != models.BatteryAlpha && *req.Battery != models.BatteryBravo {
			http.Error(w, "Invalid battery (must be HQ, Alpha, or Bravo)", http.StatusBadRequest)
			return
		}
		updates = append(updates, fmt.Sprintf("battery = $%d", argIndex))
		args = append(args, *req.Battery)
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

type RegisterUserRequest struct {
	FullName  string `json:"fullName"`
	Rank      string `json:"rank"`
	Battery   string `json:"battery"`
	NRICLast5 string `json:"nricLast5"`
	DOB       string `json:"dob"` // DDMMYY format
}

type RegisterUserResponse struct {
	User    models.User `json:"user"`
	Session string      `json:"session"`
}

// RegisterUser creates a new user account and automatically logs them in
func (h *UserHandler) RegisterUser(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()

	var req RegisterUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.FullName == "" || req.Rank == "" || req.Battery == "" || req.NRICLast5 == "" || req.DOB == "" {
		http.Error(w, "All fields are required", http.StatusBadRequest)
		return
	}

	// Validate rank
	validRank := false
	for _, rank := range models.ValidRanks {
		if rank == req.Rank {
			validRank = true
			break
		}
	}
	if !validRank {
		http.Error(w, "Invalid rank", http.StatusBadRequest)
		return
	}

	// Validate battery
	if req.Battery != models.BatteryHQ && req.Battery != models.BatteryAlpha && req.Battery != models.BatteryBravo {
		http.Error(w, "Invalid battery (must be HQ, Alpha, or Bravo)", http.StatusBadRequest)
		return
	}

	// Validate DOB format (DDMMYY, 6 characters)
	if len(req.DOB) != 6 {
		http.Error(w, "Invalid DOB format (must be DDMMYY)", http.StatusBadRequest)
		return
	}

	// Validate NRIC Last 4 (4 characters)
	if len(req.NRICLast5) != 4 {
		http.Error(w, "Invalid NRIC Last 4 (must be 4 characters)", http.StatusBadRequest)
		return
	}

	// Check if user already exists using composite key: (full_name, nricLast5, dob)
	var existingID string
	err := h.db.Pool.QueryRow(ctx, `
		SELECT id FROM "user" 
		WHERE "full_name" = $1 AND "nric_last5" = $2 AND dob = $3
		LIMIT 1
	`, req.FullName, req.NRICLast5, req.DOB).Scan(&existingID)

	if err == nil {
		// User already exists - return error suggesting sign-in
		http.Error(w, "User already exists. Please sign in instead.", http.StatusConflict)
		return
	}

	// Generate password: NRIC Last 4 + DOB
	password := req.NRICLast5 + req.DOB
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "Failed to hash password", http.StatusInternalServerError)
		return
	}

	// Generate user ID
	userID := generateID()
	now := time.Now()

	// Create user account
	_, err = h.db.Pool.Exec(ctx, `
		INSERT INTO "user" (
			id, "full_name", rank, battery, "nric_last5", dob, password,
			"createdAt", "updatedAt"
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, userID, req.FullName, req.Rank, req.Battery, req.NRICLast5, req.DOB, string(hashedPassword), now, now)

	if err != nil {
		// Check for unique constraint violation (shouldn't happen after our check, but handle it)
		if strings.Contains(err.Error(), "unique constraint") || strings.Contains(err.Error(), "duplicate") {
			http.Error(w, "User already exists. Please sign in instead.", http.StatusConflict)
			return
		}
		http.Error(w, "Failed to create user", http.StatusInternalServerError)
		return
	}

	// Auto-login: Create session
	sessionToken := generateSessionToken()
	sessionID := generateID()
	expiresAt := now.Add(30 * 24 * time.Hour)

	_, err = h.db.Pool.Exec(ctx, `
		INSERT INTO session (id, "expiresAt", token, "userId", "createdAt", "updatedAt", "ipAddress", "userAgent")
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, sessionID, expiresAt, sessionToken, userID, now, now, r.RemoteAddr, r.UserAgent())

	if err != nil {
		http.Error(w, "Failed to create session", http.StatusInternalServerError)
		return
	}

	// Set session cookie
	setSessionCookie(w, sessionToken)

	// Return user and session
	fullName := req.FullName
	rank := req.Rank
	battery := req.Battery
	nricLast5 := req.NRICLast5
	dob := req.DOB

	user := models.User{
		ID:           userID,
		FullName:     &fullName,
		Rank:         &rank,
		Battery:      &battery,
		NRICLast5:    &nricLast5,
		DOB:          &dob,
		IsSuperadmin: false,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	response := RegisterUserResponse{
		User:    user,
		Session: sessionToken,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}
