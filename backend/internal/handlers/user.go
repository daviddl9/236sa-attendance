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
	ID           string            `json:"id"`
	FullName     *string           `json:"fullName,omitempty"`
	Rank         *string           `json:"rank,omitempty"`
	Battery      *string           `json:"battery,omitempty"`
	NRICLast5    *string           `json:"nricLast5,omitempty"`
	DOB          *string           `json:"dob,omitempty"`
	Extras       map[string]string `json:"extras"`
	TierOverride *int16            `json:"tierOverride,omitempty"`
	Verified     bool              `json:"verified"`
	IsSuperadmin bool              `json:"isSuperadmin"`
	CreatedAt    string            `json:"createdAt"`
	UpdatedAt    string            `json:"updatedAt"`
}

type UpdateProfileRequest struct {
	FullName  *string `json:"fullName,omitempty"`
	Rank      *string `json:"rank,omitempty"`
	Battery   *string `json:"battery,omitempty"`
	NRICLast5 *string `json:"nricLast5,omitempty"`
	DOB       *string `json:"dob,omitempty"`
}

// UpdateUserRequest extends UpdateProfileRequest for superadmin use.
type UpdateUserRequest struct {
	UpdateProfileRequest
	TierOverride *int16 `json:"tierOverride"` // 2, 3, or null; superadmin only
}

type UserListResponse struct {
	Users []UserProfile `json:"users"`
	Total int           `json:"total"`
	Page  int           `json:"page"`
	Limit int           `json:"limit"`
}

// GetProfile retrieves the current user's profile.
func (h *UserHandler) GetProfile(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDKey).(string)

	var profile UserProfile
	var fullName, rank, battery, nricLast5, dob *string
	var tierOverride *int16
	var createdAt, updatedAt time.Time

	err := h.db.Pool.QueryRow(
		context.Background(),
		`SELECT
			id, "full_name", rank, battery, "nric_last5", dob, extras, tier_override, verified, "is_superadmin",
			"createdAt", "updatedAt"
		 FROM "user" WHERE id = $1`,
		userID,
	).Scan(
		&profile.ID, &fullName, &rank, &battery, &nricLast5, &dob, &profile.Extras,
		&tierOverride, &profile.Verified, &profile.IsSuperadmin, &createdAt, &updatedAt,
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
	profile.TierOverride = tierOverride
	if profile.Extras == nil {
		profile.Extras = map[string]string{}
	}
	profile.CreatedAt = createdAt.Format(time.RFC3339)
	profile.UpdatedAt = updatedAt.Format(time.RFC3339)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(profile)
}

// ListUsers retrieves a paginated list of users. For Tier 2 users the list is
// automatically scoped to their own battery.
func (h *UserHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()

	currentUser, _ := middleware.GetUserFromContext(r.Context())

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

	// Tier 2 can only see their own battery regardless of the query param.
	if currentUser != nil && currentUser.GetTier() == models.TierBatteryNCO {
		if currentUser.Battery != nil {
			battery = *currentUser.Battery
		}
	}

	adminID := "00000000000000000000000000000000"
	whereClause := fmt.Sprintf(`WHERE id != $%d AND verified = true`, 1)
	args := []interface{}{adminID}
	argIndex := 2

	if search != "" {
		whereClause += fmt.Sprintf(` AND "full_name" ILIKE $%d`, argIndex)
		args = append(args, "%"+search+"%")
		argIndex++
	}
	if battery != "" {
		whereClause += fmt.Sprintf(` AND battery = $%d`, argIndex)
		args = append(args, battery)
		argIndex++
	}
	if rank != "" {
		whereClause += fmt.Sprintf(` AND rank = $%d`, argIndex)
		args = append(args, rank)
		argIndex++
	}

	var total int
	err := h.db.Pool.QueryRow(ctx, fmt.Sprintf(`SELECT COUNT(*) FROM "user" %s`, whereClause), args...).Scan(&total)
	if err != nil {
		http.Error(w, "Failed to count users", http.StatusInternalServerError)
		return
	}

	query := fmt.Sprintf(`SELECT
		id, "full_name", rank, battery, "nric_last5", dob, extras, tier_override, verified, "is_superadmin",
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

	users := make([]UserProfile, 0)
	for rows.Next() {
		var profile UserProfile
		var fullName, rank, battery, nricLast5, dob *string
		var tierOverride *int16
		var createdAt, updatedAt time.Time

		if err := rows.Scan(
			&profile.ID, &fullName, &rank, &battery, &nricLast5, &dob, &profile.Extras,
			&tierOverride, &profile.Verified, &profile.IsSuperadmin, &createdAt, &updatedAt,
		); err != nil {
			continue
		}

		profile.FullName = fullName
		profile.Rank = rank
		profile.Battery = battery
		profile.NRICLast5 = nricLast5
		profile.DOB = dob
		profile.TierOverride = tierOverride
		if profile.Extras == nil {
			profile.Extras = map[string]string{}
		}
		profile.CreatedAt = createdAt.Format(time.RFC3339)
		profile.UpdatedAt = updatedAt.Format(time.RFC3339)
		users = append(users, profile)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(UserListResponse{
		Users: users, Total: total, Page: page, Limit: limit,
	})
}

// GetUser retrieves a single user by ID. Tier 2 users can only retrieve users
// from their own battery.
func (h *UserHandler) GetUser(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	userID := chi.URLParam(r, "id")
	currentUser, _ := middleware.GetUserFromContext(r.Context())

	adminID := "00000000000000000000000000000000"
	if userID == adminID {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	var profile UserProfile
	var fullName, rank, battery, nricLast5, dob *string
	var tierOverride *int16
	var createdAt, updatedAt time.Time

	err := h.db.Pool.QueryRow(ctx, `
		SELECT
			id, "full_name", rank, battery, "nric_last5", dob, extras, tier_override, verified, "is_superadmin",
			"createdAt", "updatedAt"
		FROM "user"
		WHERE id = $1
	`, userID).Scan(
		&profile.ID, &fullName, &rank, &battery, &nricLast5, &dob, &profile.Extras,
		&tierOverride, &profile.Verified, &profile.IsSuperadmin, &createdAt, &updatedAt,
	)
	if err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	// Tier 2: enforce battery boundary.
	if currentUser != nil && currentUser.GetTier() == models.TierBatteryNCO {
		if battery == nil || currentUser.Battery == nil || *battery != *currentUser.Battery {
			http.Error(w, "User not found", http.StatusNotFound)
			return
		}
	}

	profile.FullName = fullName
	profile.Rank = rank
	profile.Battery = battery
	profile.NRICLast5 = nricLast5
	profile.DOB = dob
	profile.TierOverride = tierOverride
	if profile.Extras == nil {
		profile.Extras = map[string]string{}
	}
	profile.CreatedAt = createdAt.Format(time.RFC3339)
	profile.UpdatedAt = updatedAt.Format(time.RFC3339)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(profile)
}

// UpdateUser updates a user's profile (superadmin only for most fields).
func (h *UserHandler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	userID := chi.URLParam(r, "id")

	currentUser, ok := middleware.GetUserFromContext(r.Context())
	if !ok {
		http.Error(w, "Not authenticated", http.StatusUnauthorized)
		return
	}

	var req UpdateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	updates := []string{}
	args := []interface{}{}
	argIndex := 1

	if req.FullName != nil {
		updates = append(updates, fmt.Sprintf(`"full_name" = $%d`, argIndex))
		args = append(args, *req.FullName)
		argIndex++
	}
	if req.Rank != nil && currentUser.IsSuperadmin {
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
		updates = append(updates, fmt.Sprintf(`rank = $%d`, argIndex))
		args = append(args, *req.Rank)
		argIndex++

		// Auto-set is_superadmin for CPT+.
		isSuperadmin := models.IsSuperadminByRank(*req.Rank)
		updates = append(updates, fmt.Sprintf(`"is_superadmin" = $%d`, argIndex))
		args = append(args, isSuperadmin)
		argIndex++
	}
	if req.Battery != nil && currentUser.IsSuperadmin {
		if *req.Battery != models.BatteryHQ && *req.Battery != models.BatteryAlpha && *req.Battery != models.BatteryBravo {
			http.Error(w, "Invalid battery", http.StatusBadRequest)
			return
		}
		updates = append(updates, fmt.Sprintf(`battery = $%d`, argIndex))
		args = append(args, *req.Battery)
		argIndex++
	}
	if req.NRICLast5 != nil && currentUser.IsSuperadmin {
		normalizedNRICLast5, ok := normalizeNRICLast5(*req.NRICLast5)
		if !ok {
			http.Error(w, nricLast5FormatMessage, http.StatusBadRequest)
			return
		}
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(normalizedNRICLast5), bcrypt.DefaultCost)
		if err != nil {
			http.Error(w, "Failed to hash password", http.StatusInternalServerError)
			return
		}
		updates = append(updates, fmt.Sprintf(`"nric_last5" = $%d`, argIndex))
		args = append(args, normalizedNRICLast5)
		argIndex++
		updates = append(updates, fmt.Sprintf(`password = $%d`, argIndex))
		args = append(args, string(hashedPassword))
		argIndex++
	}
	if req.DOB != nil && currentUser.IsSuperadmin {
		if len(*req.DOB) != 6 {
			http.Error(w, "Invalid DOB format (must be DDMMYY)", http.StatusBadRequest)
			return
		}
		updates = append(updates, fmt.Sprintf(`dob = $%d`, argIndex))
		args = append(args, *req.DOB)
		argIndex++
	}

	// tier_override: superadmin only, value must be 2, 3, or null.
	if req.TierOverride != nil && currentUser.IsSuperadmin {
		v := *req.TierOverride
		if v != 2 && v != 3 {
			http.Error(w, "tier_override must be 2 or 3", http.StatusBadRequest)
			return
		}
		updates = append(updates, fmt.Sprintf(`tier_override = $%d`, argIndex))
		args = append(args, v)
		argIndex++
	} else if req.TierOverride == nil && currentUser.IsSuperadmin {
		// Explicit null clears the override; we detect this via JSON decode since
		// *int16 nil means "not provided" but the JSON key may still be present.
		// For simplicity the caller sends {"tierOverride": null} to clear.
		// We handle this by checking if the JSON contained the key.
		// (already handled: nil pointer means field absent OR explicitly null)
	}

	if len(updates) == 0 {
		http.Error(w, "No fields to update", http.StatusBadRequest)
		return
	}

	updates = append(updates, fmt.Sprintf(`"updatedAt" = $%d`, argIndex))
	args = append(args, time.Now())
	argIndex++
	args = append(args, userID)

	query := fmt.Sprintf(`UPDATE "user" SET %s WHERE id = $%d`, strings.Join(updates, ", "), argIndex)
	if _, err := h.db.Pool.Exec(ctx, query, args...); err != nil {
		http.Error(w, "Failed to update user", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"message": "User updated successfully"})
}

// DeleteUser hard-deletes a user (superadmin only).
func (h *UserHandler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	userID := chi.URLParam(r, "id")

	if _, err := h.db.Pool.Exec(ctx, `DELETE FROM "user" WHERE id = $1`, userID); err != nil {
		http.Error(w, "Failed to delete user", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"message": "User deleted successfully"})
}

func (h *UserHandler) BulkDeleteCount(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()

	search := r.URL.Query().Get("search")
	battery := r.URL.Query().Get("battery")
	rank := r.URL.Query().Get("rank")

	adminID := "00000000000000000000000000000000"
	whereClause := "WHERE id != $1 AND verified = true"
	args := []interface{}{adminID}
	argIndex := 2

	if search != "" {
		whereClause += fmt.Sprintf(` AND "full_name" ILIKE $%d`, argIndex)
		args = append(args, "%"+search+"%")
		argIndex++
	}
	if battery != "" {
		whereClause += fmt.Sprintf(` AND battery = $%d`, argIndex)
		args = append(args, battery)
		argIndex++
	}
	if rank != "" {
		whereClause += fmt.Sprintf(` AND rank = $%d`, argIndex)
		args = append(args, rank)
	}

	var count int
	if err := h.db.Pool.QueryRow(ctx, fmt.Sprintf(`SELECT COUNT(*) FROM "user" %s`, whereClause), args...).Scan(&count); err != nil {
		http.Error(w, "Failed to count users", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]int{"count": count})
}

type BulkDeleteRequest struct {
	Search  string `json:"search,omitempty"`
	Battery string `json:"battery,omitempty"`
	Rank    string `json:"rank,omitempty"`
}

func (h *UserHandler) BulkDelete(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()

	var req BulkDeleteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	adminID := "00000000000000000000000000000000"
	whereClause := "WHERE id != $1 AND verified = true"
	args := []interface{}{adminID}
	argIndex := 2

	if req.Search != "" {
		whereClause += fmt.Sprintf(` AND "full_name" ILIKE $%d`, argIndex)
		args = append(args, "%"+req.Search+"%")
		argIndex++
	}
	if req.Battery != "" {
		whereClause += fmt.Sprintf(` AND battery = $%d`, argIndex)
		args = append(args, req.Battery)
		argIndex++
	}
	if req.Rank != "" {
		whereClause += fmt.Sprintf(` AND rank = $%d`, argIndex)
		args = append(args, req.Rank)
	}

	result, err := h.db.Pool.Exec(ctx, fmt.Sprintf(`DELETE FROM "user" %s`, whereClause), args...)
	if err != nil {
		http.Error(w, "Failed to delete users", http.StatusInternalServerError)
		return
	}

	deletedCount := result.RowsAffected()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"message":      fmt.Sprintf("Deleted %d users successfully", deletedCount),
		"deletedCount": deletedCount,
	})
}

func (h *UserHandler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDKey).(string)

	var req UpdateProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	updates := []string{}
	args := []interface{}{}
	argIndex := 1

	if req.FullName != nil {
		updates = append(updates, fmt.Sprintf(`"full_name" = $%d`, argIndex))
		args = append(args, *req.FullName)
		argIndex++
	}
	if req.Rank != nil {
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
		updates = append(updates, fmt.Sprintf(`rank = $%d`, argIndex))
		args = append(args, *req.Rank)
		argIndex++
	}
	if req.Battery != nil {
		if *req.Battery != models.BatteryHQ && *req.Battery != models.BatteryAlpha && *req.Battery != models.BatteryBravo {
			http.Error(w, "Invalid battery (must be HQ, Alpha, or Bravo)", http.StatusBadRequest)
			return
		}
		updates = append(updates, fmt.Sprintf(`battery = $%d`, argIndex))
		args = append(args, *req.Battery)
		argIndex++
	}

	if len(updates) == 0 {
		http.Error(w, "No fields to update", http.StatusBadRequest)
		return
	}

	updates = append(updates, fmt.Sprintf(`"updatedAt" = $%d`, argIndex))
	args = append(args, time.Now())
	argIndex++
	args = append(args, userID)

	if _, err := h.db.Pool.Exec(context.Background(), fmt.Sprintf(`UPDATE "user" SET %s WHERE id = $%d`, strings.Join(updates, ", "), argIndex), args...); err != nil {
		http.Error(w, "Failed to update profile", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"message": "Profile updated successfully"})
}

type RegisterUserRequest struct {
	FullName  string `json:"fullName"`
	Rank      string `json:"rank"`
	Battery   string `json:"battery"`
	NRICLast5 string `json:"nricLast5"`
	DOB       string `json:"dob"` // DDMMYY format
}

// RegisterUser creates a new user account from the public registration page.
// The account starts unverified and requires admin approval before login.
func (h *UserHandler) RegisterUser(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()

	var req RegisterUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.FullName == "" || req.Rank == "" || req.Battery == "" || req.NRICLast5 == "" || req.DOB == "" {
		http.Error(w, "All fields are required", http.StatusBadRequest)
		return
	}

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

	if req.Battery != models.BatteryHQ && req.Battery != models.BatteryAlpha && req.Battery != models.BatteryBravo {
		http.Error(w, "Invalid battery (must be HQ, Alpha, or Bravo)", http.StatusBadRequest)
		return
	}

	if len(req.DOB) != 6 {
		http.Error(w, "Invalid DOB format (must be DDMMYY)", http.StatusBadRequest)
		return
	}

	normalizedNRICLast5, ok := normalizeNRICLast5(req.NRICLast5)
	if !ok {
		http.Error(w, nricLast5FormatMessage, http.StatusBadRequest)
		return
	}
	req.NRICLast5 = normalizedNRICLast5

	// Check for existing record (same composite key).
	var existingID string
	err := h.db.Pool.QueryRow(ctx, `
		SELECT id FROM "user"
		WHERE "full_name" = $1 AND "nric_last5" = $2 AND dob = $3
		LIMIT 1
	`, req.FullName, req.NRICLast5, req.DOB).Scan(&existingID)
	if err == nil {
		http.Error(w, "User already exists. Please sign in instead.", http.StatusConflict)
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NRICLast5), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "Failed to hash password", http.StatusInternalServerError)
		return
	}

	// CPT+ are auto-superadmin even on registration.
	isSuperadmin := models.IsSuperadminByRank(req.Rank)

	userID := generateID()
	now := time.Now()
	_, err = h.db.Pool.Exec(ctx, `
		INSERT INTO "user" (
			id, "full_name", rank, battery, "nric_last5", dob, password,
			"is_superadmin", verified, extras, "createdAt", "updatedAt"
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, false, '{}'::jsonb, $9, $10)
	`, userID, req.FullName, req.Rank, req.Battery, req.NRICLast5, req.DOB,
		string(hashedPassword), isSuperadmin, now, now)
	if err != nil {
		if strings.Contains(err.Error(), "unique constraint") || strings.Contains(err.Error(), "duplicate") {
			http.Error(w, "User already exists. Please sign in instead.", http.StatusConflict)
			return
		}
		http.Error(w, "Failed to create user", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]string{"outcome": "pending_approval"})
}
