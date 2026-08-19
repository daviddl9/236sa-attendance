package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/davidlivingston/go-nextjs-starter/backend/internal/database"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/middleware"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/models"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

const seededAdminID = "00000000000000000000000000000000"

type UserHandler struct {
	db *database.DB
}

func NewUserHandler(db *database.DB) *UserHandler {
	return &UserHandler{db: db}
}

type UserProfile struct {
	ID           string            `json:"id"`
	Username     *string           `json:"username,omitempty"`
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
	var username, fullName, rank, battery, nricLast5, dob *string
	var tierOverride *int16
	var createdAt, updatedAt time.Time

	err := h.db.Pool.QueryRow(
		context.Background(),
		`SELECT
			id, username, "full_name", rank, battery, "nric_last5", dob, extras, tier_override,
			verified, "is_superadmin", "createdAt", "updatedAt"
		 FROM "user" WHERE id = $1`,
		userID,
	).Scan(
		&profile.ID, &username, &fullName, &rank, &battery, &nricLast5, &dob, &profile.Extras,
		&tierOverride, &profile.Verified, &profile.IsSuperadmin, &createdAt, &updatedAt,
	)
	if err != nil {
		http.Error(w, "Failed to fetch profile", http.StatusInternalServerError)
		return
	}

	profile.Username = username
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

const (
	defaultListUsersLimit = 20
	// maxListUsersLimit must exceed the roster size: the groups "add people"
	// dialog fetches the full roster in a single page to avoid a serial
	// request waterfall on high-latency links.
	maxListUsersLimit = 1000
)

// parseListUsersLimit clamps the requested page size; out-of-range or
// unparseable values fall back to the default.
func parseListUsersLimit(raw string) int {
	limit, _ := strconv.Atoi(raw)
	if limit < 1 || limit > maxListUsersLimit {
		return defaultListUsersLimit
	}
	return limit
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
	limit := parseListUsersLimit(r.URL.Query().Get("limit"))
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
		id, username, "full_name", rank, battery, "nric_last5", dob, extras, tier_override,
		verified, "is_superadmin", "createdAt", "updatedAt"
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
		var username, fullName, rank, battery, nricLast5, dob *string
		var tierOverride *int16
		var createdAt, updatedAt time.Time

		if err := rows.Scan(
			&profile.ID, &username, &fullName, &rank, &battery, &nricLast5, &dob, &profile.Extras,
			&tierOverride, &profile.Verified, &profile.IsSuperadmin, &createdAt, &updatedAt,
		); err != nil {
			continue
		}

		profile.Username = username
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
	var username, fullName, rank, battery, nricLast5, dob *string
	var tierOverride *int16
	var createdAt, updatedAt time.Time

	err := h.db.Pool.QueryRow(ctx, `
		SELECT
			id, username, "full_name", rank, battery, "nric_last5", dob, extras, tier_override,
			verified, "is_superadmin", "createdAt", "updatedAt"
		FROM "user"
		WHERE id = $1
	`, userID).Scan(
		&profile.ID, &username, &fullName, &rank, &battery, &nricLast5, &dob, &profile.Extras,
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

	profile.Username = username
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

// BulkDeleteRequest is the body accepted by POST /api/users/bulk-delete.
// Replaces the old filter-shaped request — every target user must now be
// named explicitly by id.
type BulkDeleteRequest struct {
	UserIDs []string `json:"userIds"`
}

// BulkDeleteSkip represents a user id that was intentionally skipped. The
// Reason field is one of: "self", "system_admin", or "not_found".
type BulkDeleteSkip struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

// BulkDeleteFailure represents an unexpected per-id error. The Code field
// is a short machine-readable token; free-text messages are logged but not
// returned to avoid leaking DB internals.
type BulkDeleteFailure struct {
	ID   string `json:"id"`
	Code string `json:"code"`
}

// BulkDeleteSummary is a convenience payload for the frontend's toast.
type BulkDeleteSummary struct {
	Requested int `json:"requested"`
	Deleted   int `json:"deleted"`
	Skipped   int `json:"skipped"`
	Failed    int `json:"failed"`
}

// BulkDeleteResponse is the per-id outcome shape returned by the handler.
type BulkDeleteResponse struct {
	Deleted []string            `json:"deleted"`
	Skipped []BulkDeleteSkip    `json:"skipped"`
	Failed  []BulkDeleteFailure `json:"failed"`
	Summary BulkDeleteSummary   `json:"summary"`
}

// partitionBulkDeleteIDs splits the requested id list into deletable
// targets, ids skipped because they are the requester themselves, and ids
// skipped because they are the seeded system admin. Duplicate ids in the
// input are de-duplicated; order is preserved by first occurrence. Pure
// function — no DB access — testable in unit mode.
func partitionBulkDeleteIDs(requested []string, selfID string) (targets []string, skipped []BulkDeleteSkip) {
	seen := make(map[string]struct{}, len(requested))
	for _, id := range requested {
		if id == "" {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		switch id {
		case selfID:
			skipped = append(skipped, BulkDeleteSkip{ID: id, Reason: "self"})
		case seededAdminID:
			skipped = append(skipped, BulkDeleteSkip{ID: id, Reason: "system_admin"})
		default:
			targets = append(targets, id)
		}
	}
	return targets, skipped
}

// BulkDeleteUsers deletes a best-effort batch of users named by explicit
// id. Superadmin only. Returns a per-id outcome shape so the frontend can
// surface deleted / skipped / failed counts and identities.
func (h *UserHandler) BulkDeleteUsers(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()

	currentUser, ok := middleware.GetUserFromContext(r.Context())
	if !ok {
		http.Error(w, "Not authenticated", http.StatusUnauthorized)
		return
	}

	var req BulkDeleteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if len(req.UserIDs) == 0 {
		http.Error(w, "userIds must be a non-empty array", http.StatusBadRequest)
		return
	}

	targets, skipped := partitionBulkDeleteIDs(req.UserIDs, currentUser.ID)

	deleted := []string{}
	failed := []BulkDeleteFailure{}

	if len(targets) > 0 {
		rows, err := h.db.Pool.Query(ctx,
			`DELETE FROM "user" WHERE id = ANY($1) RETURNING id`, targets)
		if err != nil {
			log.Printf("bulk delete: requester=%s requested=%d db_error=%v",
				currentUser.ID, len(req.UserIDs), err)
			for _, id := range targets {
				failed = append(failed, BulkDeleteFailure{ID: id, Code: "db_error"})
			}
		} else {
			deletedSet := make(map[string]struct{}, len(targets))
			for rows.Next() {
				var id string
				if scanErr := rows.Scan(&id); scanErr != nil {
					log.Printf("bulk delete: requester=%s scan_error=%v", currentUser.ID, scanErr)
					continue
				}
				deleted = append(deleted, id)
				deletedSet[id] = struct{}{}
			}
			rows.Close()
			if rowsErr := rows.Err(); rowsErr != nil {
				log.Printf("bulk delete: requester=%s rows_error=%v", currentUser.ID, rowsErr)
			}
			for _, id := range targets {
				if _, was := deletedSet[id]; !was {
					skipped = append(skipped, BulkDeleteSkip{ID: id, Reason: "not_found"})
				}
			}
		}
	}

	log.Printf("bulk delete: requester=%s requested=%d deleted=%d skipped=%d failed=%d",
		currentUser.ID, len(req.UserIDs), len(deleted), len(skipped), len(failed))

	resp := BulkDeleteResponse{
		Deleted: deleted,
		Skipped: skipped,
		Failed:  failed,
		Summary: BulkDeleteSummary{
			Requested: len(req.UserIDs),
			Deleted:   len(deleted),
			Skipped:   len(skipped),
			Failed:    len(failed),
		},
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
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

// CreateUserRequest is the body accepted by POST /api/users (superadmin only).
// DOB is required: it is the sign-in credential (full name + date of birth).
type CreateUserRequest struct {
	FullName string            `json:"fullName"`
	Rank     string            `json:"rank"`
	Battery  string            `json:"battery"`
	DOB      string            `json:"dob"`
	Extras   map[string]string `json:"extras,omitempty"`
}

// createUserConflict is the 409 response shape when a user already exists
// for the (full_name, nric_last5) pair. The frontend uses Verified to
// decide whether to deep-link to the user detail page or the registrations
// approval queue.
type createUserConflict struct {
	Error          string `json:"error"`
	ExistingUserID string `json:"existingUserId"`
	Verified       bool   `json:"verified"`
	FullName       string `json:"fullName"`
}

// normalizeFullName trims leading/trailing whitespace and collapses any
// internal whitespace run to a single space. "  John   Doe  " → "John Doe".
func normalizeFullName(name string) string {
	return strings.Join(strings.Fields(name), " ")
}

// createUserValidationError carries a user-facing message; the handler maps
// it to a 400 response with the message in the body.
type createUserValidationError struct{ message string }

func (e *createUserValidationError) Error() string { return e.message }

// validateCreateUser normalises and validates a CreateUserRequest. The
// returned value is safe to insert directly. Pure function: no DB access,
// no I/O — testable in unit mode without a database.
func validateCreateUser(req CreateUserRequest) (CreateUserRequest, error) {
	req.FullName = normalizeFullName(req.FullName)
	if req.FullName == "" {
		return req, &createUserValidationError{message: "Full name is required"}
	}
	req.Rank = strings.TrimSpace(req.Rank)
	if req.Rank == "" {
		return req, &createUserValidationError{message: "Rank is required"}
	}
	validRank := false
	for _, r := range models.ValidRanks {
		if r == req.Rank {
			validRank = true
			break
		}
	}
	if !validRank {
		return req, &createUserValidationError{message: "Invalid rank"}
	}
	req.Battery = strings.TrimSpace(req.Battery)
	if req.Battery != models.BatteryHQ && req.Battery != models.BatteryAlpha && req.Battery != models.BatteryBravo {
		return req, &createUserValidationError{message: "Invalid battery (must be HQ, Alpha, or Bravo)"}
	}
	// DOB is the sign-in credential, so it is required and must be a real,
	// canonicalisable date. The auth layer compares it via canonicalDOB, so any
	// format it accepts (YYYY-MM-DD, DD.MM.YYYY, …) works; we store it
	// canonicalised so the duplicate check below compares like-for-like.
	req.DOB = strings.TrimSpace(req.DOB)
	canonical, ok := canonicalDOB(req.DOB)
	if !ok {
		return req, &createUserValidationError{message: "Date of birth is required"}
	}
	req.DOB = canonical
	return req, nil
}

// CreateUser inserts a single new user as a verified, immediately-usable
// account. Superadmin only. Mirrors the validation rules of RegisterUser
// and the password-hashing path of BulkCreateUsers but for one row.
func (h *UserHandler) CreateUser(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()

	var req CreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	normalized, err := validateCreateUser(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	req = normalized

	// Duplicate detection on (upper(full_name), dob). No DB-level unique
	// constraint exists today on this pair; the application layer is
	// authoritative. DOB is stored canonicalised by validateCreateUser, so the
	// comparison is exact.
	var (
		existingID       string
		existingVerified bool
	)
	err = h.db.Pool.QueryRow(ctx, `
		SELECT id, verified FROM "user"
		WHERE upper("full_name") = upper($1) AND dob = $2
		LIMIT 1
	`, req.FullName, req.DOB).Scan(&existingID, &existingVerified)
	switch {
	case err == nil:
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(createUserConflict{
			Error:          "user_exists",
			ExistingUserID: existingID,
			Verified:       existingVerified,
			FullName:       req.FullName,
		})
		return
	case errors.Is(err, pgx.ErrNoRows):
		// Happy path — no duplicate. Proceed.
	default:
		http.Error(w, "Failed to check for existing user", http.StatusInternalServerError)
		return
	}

	isSuperadmin := models.IsSuperadminByRank(req.Rank)
	extras := req.Extras
	if extras == nil {
		extras = map[string]string{}
	}
	extrasJSON, err := json.Marshal(extras)
	if err != nil {
		http.Error(w, "Failed to encode extras", http.StatusInternalServerError)
		return
	}

	userID := generateID()
	now := time.Now()

	// No bcrypt password is set: the DOB column is the sign-in credential and
	// the auth layer's DOB path (verifyLegacyCandidates) is primary. password
	// stays NULL, which the sign-in code already treats as "no match" on the
	// password fallback path.
	_, err = h.db.Pool.Exec(ctx, `
		INSERT INTO "user" (
			id, "full_name", rank, battery, dob,
			"is_superadmin", verified, extras, "createdAt", "updatedAt"
		)
		VALUES ($1, $2, $3, $4, $5, $6, true, $7::jsonb, $8, $9)
	`, userID, req.FullName, req.Rank, req.Battery, req.DOB,
		isSuperadmin, string(extrasJSON), now, now)
	if err != nil {
		// Race: another writer may have inserted between the duplicate check
		// and the insert. Surface as 409 with the minimum useful payload.
		if strings.Contains(err.Error(), "unique constraint") || strings.Contains(err.Error(), "duplicate") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(createUserConflict{
				Error:    "user_exists",
				FullName: req.FullName,
			})
			return
		}
		http.Error(w, "Failed to create user", http.StatusInternalServerError)
		return
	}

	profile := UserProfile{
		ID:           userID,
		FullName:     &req.FullName,
		Rank:         &req.Rank,
		Battery:      &req.Battery,
		DOB:          &req.DOB,
		Extras:       extras,
		Verified:     true,
		IsSuperadmin: isSuperadmin,
		CreatedAt:    now.Format(time.RFC3339),
		UpdatedAt:    now.Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(profile)
}
