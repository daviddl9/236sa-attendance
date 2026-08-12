package handlers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/davidlivingston/go-nextjs-starter/backend/internal/database"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/middleware"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/models"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

type AuthHandler struct {
	db *database.DB
}

func NewAuthHandler(db *database.DB) *AuthHandler {
	return &AuthHandler{db: db}
}

type SignInRequest struct {
	Identifier string `json:"identifier"` // Username, or legacy full name during rollout.
	Password   string `json:"password"`
}

// SignInOutcome is the machine-readable result of a sign-in attempt.
type SignInOutcome string

const (
	SignInOutcomeAuthenticated   SignInOutcome = "authenticated"
	SignInOutcomePendingApproval SignInOutcome = "pending_approval"
)

// SignInResponse is returned for every successful (2xx) sign-in call.
type SignInResponse struct {
	Outcome SignInOutcome `json:"outcome"`
	User    *models.User  `json:"user,omitempty"`
	Session *string       `json:"session,omitempty"`
}

type SignUpRequest struct {
	Username        string `json:"username"`
	Password        string `json:"password"`
	ConfirmPassword string `json:"confirmPassword"`
	FullName        string `json:"fullName"`
	Rank            string `json:"rank"`
	Battery         string `json:"battery"`
}

// AuthResponse is kept for backward-compatibility.
type AuthResponse = SignInResponse

// userRow holds all columns needed when scanning a user for sign-in.
type userRow struct {
	id                     string
	fullName               *string
	rank                   *string
	battery                *string
	isSuperadmin           bool
	tierOverride           *int16
	verified               bool
	passwordChangeRequired bool
	createdAt              time.Time
	updatedAt              time.Time
	password               string
}

type pendingRegistrationRow struct {
	passwordHash string
}

const passwordHashCost = 12

// signInUserColumns is the SELECT list shared by the sign-in lookups so the
// primary query and the word-subset fallback scan an identical set of columns.
const signInUserColumns = `u.id, u."full_name", u.rank, u.battery, u."is_superadmin", u.tier_override, u.verified, u.password_change_required, u."createdAt", u."updatedAt", u.password`

// scanUserRow scans signInUserColumns (in order) into ur. It accepts a pgx.Row,
// which both Pool.QueryRow and Pool.Query rows satisfy.
func scanUserRow(row pgx.Row, ur *userRow) error {
	return row.Scan(
		&ur.id, &ur.fullName, &ur.rank, &ur.battery, &ur.isSuperadmin,
		&ur.tierOverride, &ur.verified, &ur.passwordChangeRequired, &ur.createdAt, &ur.updatedAt, &ur.password,
	)
}

func (h *AuthHandler) SignIn(w http.ResponseWriter, r *http.Request) {
	var req SignInRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	req.Identifier = strings.TrimSpace(req.Identifier)
	if req.Identifier == "" || req.Password == "" {
		http.Error(w, "Identifier and password are required", http.StatusBadRequest)
		return
	}

	ctx := context.Background()
	var ur userRow
	userErr := h.findUserByUsername(ctx, req.Identifier, &ur)
	if userErr == nil {
		if !comparePassword(ur.password, req.Password) {
			writeInvalidCredentials(w, req.Identifier)
			return
		}
		h.finishSignIn(w, r, ctx, req.Identifier, ur)
		return
	}
	if !errors.Is(userErr, pgx.ErrNoRows) {
		log.Printf("[SignIn] Error querying user by username: %v", userErr)
		http.Error(w, "Failed to query user", http.StatusInternalServerError)
		return
	}

	var pending pendingRegistrationRow
	pendingErr := h.findPendingByUsername(ctx, req.Identifier, &pending)
	if pendingErr == nil {
		if !comparePassword(pending.passwordHash, req.Password) {
			writeInvalidCredentials(w, req.Identifier)
			return
		}
		writePendingApproval(w, req.Identifier)
		return
	}
	if !errors.Is(pendingErr, pgx.ErrNoRows) {
		log.Printf("[SignIn] Error querying pending registrations: %v", pendingErr)
		http.Error(w, "Failed to query pending registration", http.StatusInternalServerError)
		return
	}

	// Compatibility path for existing roster rows that have no username yet.
	//
	// Identity is established by the bcrypt hash in `password`, never by a stored
	// copy of the secret itself. Pre-008 credentials were uppercased before
	// hashing, so the typed value is tried as-is and uppercased.
	matched, legacyErr := h.authenticateLegacyByName(ctx, req.Identifier, req.Password)
	if legacyErr != nil {
		log.Printf("[SignIn] Error querying legacy user: %v", legacyErr)
		http.Error(w, "Failed to query user", http.StatusInternalServerError)
		return
	}
	if matched == nil {
		// Keep unknown identifiers indistinguishable from wrong passwords.
		writeInvalidCredentials(w, req.Identifier)
		return
	}
	ur = *matched
	h.finishSignIn(w, r, ctx, req.Identifier, ur)
}

// maxLegacyNameCandidates bounds how many same-named rows we will verify against.
// The roster contains a handful of shared full names, and each candidate costs a
// bcrypt comparison, so an unbounded loop would be a denial-of-service lever.
const maxLegacyNameCandidates = 5

// authenticateLegacyByName resolves a pre-008 credential using the full name and
// the bcrypt hash alone. It returns the single row whose hash verifies, or nil
// when none or more than one does. An ambiguous match is treated as a failure:
// two people sharing a name and a secret must never silently resolve to one of
// them.
func (h *AuthHandler) authenticateLegacyByName(ctx context.Context, identifier, password string) (*userRow, error) {
	rows, err := h.db.Pool.Query(ctx, `
		SELECT `+signInUserColumns+`
		FROM "user" u
		WHERE upper(u."full_name") = upper($1)
		LIMIT $2
	`, identifier, maxLegacyNameCandidates)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	candidates := make([]userRow, 0, maxLegacyNameCandidates)
	for rows.Next() {
		var candidate userRow
		if err := scanUserRow(rows, &candidate); err != nil {
			return nil, err
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	attempts := []string{password, strings.ToUpper(password)}
	var matched *userRow
	for i := range candidates {
		for _, attempt := range attempts {
			if !comparePassword(candidates[i].password, attempt) {
				continue
			}
			if matched != nil {
				return nil, nil
			}
			matched = &candidates[i]
			break
		}
	}
	if matched == nil {
		return nil, nil
	}
	h.upgradeLegacyHash(ctx, matched.id, matched.password, password)
	return matched, nil
}

// upgradeLegacyHash rehashes a verified credential at the current cost. Pre-008
// hashes were written at bcrypt cost 4, which is the minimum the library allows
// and is far too weak for a secret drawn from a small keyspace. Upgrading on a
// successful sign-in retires those hashes without asking anyone to do anything.
func (h *AuthHandler) upgradeLegacyHash(ctx context.Context, userID, storedHash, plaintext string) {
	cost, err := bcrypt.Cost([]byte(storedHash))
	if err != nil || cost >= passwordHashCost {
		return
	}
	upgraded, err := bcrypt.GenerateFromPassword([]byte(plaintext), passwordHashCost)
	if err != nil {
		log.Printf("[SignIn] Failed to upgrade password hash: %v", err)
		return
	}
	if _, err := h.db.Pool.Exec(ctx, `UPDATE "user" SET password = $1 WHERE id = $2`, string(upgraded), userID); err != nil {
		log.Printf("[SignIn] Failed to persist upgraded password hash: %v", err)
	}
}

func (h *AuthHandler) findUserByUsername(ctx context.Context, username string, ur *userRow) error {
	return scanUserRow(h.db.Pool.QueryRow(ctx, `
		SELECT `+signInUserColumns+` FROM "user" u
		WHERE lower(trim(u.username)) = lower(trim($1))
		  AND left(lower(trim(u.username)), length($2)) <> $2
	`, username, migratedPendingUsernamePrefix), ur)
}

func (h *AuthHandler) findPendingByUsername(ctx context.Context, username string, pending *pendingRegistrationRow) error {
	return h.db.Pool.QueryRow(ctx, `
		SELECT password_hash FROM pending_registration
		WHERE lower(trim(username)) = lower(trim($1))
		  AND left(lower(trim(username)), length($2)) <> $2
	`, username, migratedPendingUsernamePrefix).Scan(&pending.passwordHash)
}

func comparePassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

func writeInvalidCredentials(w http.ResponseWriter, identifier string) {
	log.Printf("[SignIn] Password verification failed for user: %s", identifier)
	http.Error(w, "Invalid identifier or password", http.StatusUnauthorized)
}

func writePendingApproval(w http.ResponseWriter, identifier string) {
	log.Printf("[SignIn] User %q is pending approval", identifier)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(SignInResponse{Outcome: SignInOutcomePendingApproval})
}

func (h *AuthHandler) finishSignIn(w http.ResponseWriter, r *http.Request, ctx context.Context, identifier string, ur userRow) {
	if !ur.verified {
		writePendingApproval(w, identifier)
		return
	}

	user, session, err := h.createSession(ctx, r, ur)
	if err != nil {
		log.Printf("[SignIn] Failed to create session for user %s: %v", ur.id, err)
		http.Error(w, "Failed to create session", http.StatusInternalServerError)
		return
	}
	setSessionCookie(w, session)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(SignInResponse{
		Outcome: SignInOutcomeAuthenticated,
		User:    &user,
		Session: &session,
	})
}

// SignUp handles POST /api/auth/sign-up. It stores a pending registration,
// not a roster user, until the PR3 approval flow resolves it.
func (h *AuthHandler) SignUp(w http.ResponseWriter, r *http.Request) {
	var req SignUpRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	req, err := validateSignUpRequest(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), passwordHashCost)
	if err != nil {
		http.Error(w, "Failed to hash password", http.StatusInternalServerError)
		return
	}

	ctx := context.Background()
	tx, err := h.db.Pool.Begin(ctx)
	if err != nil {
		http.Error(w, "Failed to create account", http.StatusInternalServerError)
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()

	normalizedUsername := normalizeUsername(req.Username)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, normalizedUsername); err != nil {
		http.Error(w, "Failed to create account", http.StatusInternalServerError)
		return
	}
	var usernameTaken bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM "user" WHERE lower(trim(username)) = $1
			UNION ALL
			SELECT 1 FROM pending_registration WHERE lower(trim(username)) = $1
		)
	`, normalizedUsername).Scan(&usernameTaken); err != nil {
		http.Error(w, "Failed to check username", http.StatusInternalServerError)
		return
	}
	if usernameTaken {
		http.Error(w, "Username is unavailable", http.StatusConflict)
		return
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO pending_registration (
			id, username, password_hash, claimed_name, claimed_rank, claimed_battery
		)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, generateID(), req.Username, string(hash), req.FullName, req.Rank, req.Battery)
	if err != nil {
		if strings.Contains(err.Error(), "unique") || strings.Contains(err.Error(), "duplicate") {
			http.Error(w, "Username is unavailable", http.StatusConflict)
			return
		}
		log.Printf("[SignUp] Failed to create pending registration for %q: %v", req.Username, err)
		http.Error(w, "Failed to create account", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		log.Printf("[SignUp] Failed to commit pending registration for %q: %v", req.Username, err)
		http.Error(w, "Failed to create account", http.StatusInternalServerError)
		return
	}

	log.Printf("[SignUp] Created pending registration for %q", req.Username)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(SignInResponse{Outcome: SignInOutcomePendingApproval})
}

// createSession inserts a session row and returns the populated user model and token.
func (h *AuthHandler) createSession(ctx context.Context, r *http.Request, ur userRow) (models.User, string, error) {
	token := generateSessionToken()
	sessionID := generateID()
	expiresAt := time.Now().Add(30 * 24 * time.Hour)
	now := time.Now()

	_, err := h.db.Pool.Exec(ctx, `
		INSERT INTO session (id, "expiresAt", token, "userId", "createdAt", "updatedAt", "ipAddress", "userAgent")
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, sessionID, expiresAt, token, ur.id, now, now, r.RemoteAddr, r.UserAgent())
	if err != nil {
		return models.User{}, "", err
	}
	log.Printf("[SignIn] Session created successfully for user: %s", ur.id)

	user := models.User{
		ID:                     ur.id,
		FullName:               ur.fullName,
		Rank:                   ur.rank,
		Battery:                ur.battery,
		TierOverride:           ur.tierOverride,
		Verified:               ur.verified,
		PasswordChangeRequired: ur.passwordChangeRequired,
		IsSuperadmin:           ur.isSuperadmin,
		CreatedAt:              ur.createdAt,
		UpdatedAt:              ur.updatedAt,
	}
	return user, token, nil
}

func (h *AuthHandler) SignOut(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("session")
	if err != nil {
		http.Error(w, "Not authenticated", http.StatusUnauthorized)
		return
	}

	ctx := context.Background()

	_, err = h.db.Pool.Exec(ctx, `DELETE FROM session WHERE token = $1`, cookie.Value)
	if err != nil {
		http.Error(w, "Failed to delete session", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   os.Getenv("ENVIRONMENT") == "production",
		SameSite: http.SameSiteLaxMode,
	})

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"success": true}`))
}

// sessionUserResponse wraps the user model with the computed tier for frontend use.
type sessionUserResponse struct {
	models.User
	Tier int `json:"tier"`
}

type ChangePasswordRequest struct {
	Password        string `json:"password"`
	ConfirmPassword string `json:"confirmPassword"`
}

// ChangePassword replaces the current password and clears the forced-change flag.
func (h *AuthHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(string)
	if !ok || userID == "" {
		http.Error(w, "Not authenticated", http.StatusUnauthorized)
		return
	}
	var req ChangePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if err := validatePassword(req.Password, req.ConfirmPassword); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var currentHash *string
	if err := h.db.Pool.QueryRow(r.Context(), `SELECT password FROM "user" WHERE id = $1`, userID).Scan(&currentHash); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "User not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to fetch current password", http.StatusInternalServerError)
		return
	}
	if currentHash != nil && comparePassword(*currentHash, req.Password) {
		http.Error(w, "New password must differ from your current password", http.StatusBadRequest)
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), passwordHashCost)
	if err != nil {
		http.Error(w, "Failed to hash password", http.StatusInternalServerError)
		return
	}
	result, err := h.db.Pool.Exec(r.Context(), `
		UPDATE "user" SET password = $1, password_change_required = false, "updatedAt" = NOW()
		WHERE id = $2
	`, string(hash), userID)
	if err != nil {
		http.Error(w, "Failed to change password", http.StatusInternalServerError)
		return
	}
	if result.RowsAffected() == 0 {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"message": "Password changed successfully"})
}

func (h *AuthHandler) GetSession(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("session")
	if err != nil {
		http.Error(w, "Not authenticated", http.StatusUnauthorized)
		return
	}

	ctx := context.Background()

	var ur userRow
	err = h.db.Pool.QueryRow(ctx, `
		SELECT
			u.id, u."full_name", u.rank, u.battery, u."is_superadmin",
			u.tier_override, u.verified, u.password_change_required,
			u."createdAt", u."updatedAt"
		FROM "user" u
		JOIN session s ON s."userId" = u.id
		WHERE s.token = $1 AND s."expiresAt" > NOW()
	`, cookie.Value).Scan(
		&ur.id, &ur.fullName, &ur.rank, &ur.battery, &ur.isSuperadmin,
		&ur.tierOverride, &ur.verified, &ur.passwordChangeRequired,
		&ur.createdAt, &ur.updatedAt,
	)

	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			log.Printf("[GetSession] Error querying user table: %v", err)
		}
		http.Error(w, "Invalid or expired session", http.StatusUnauthorized)
		return
	}
	log.Printf("[GetSession] Found user session for user: %s", ur.id)

	user := models.User{
		ID:                     ur.id,
		FullName:               ur.fullName,
		Rank:                   ur.rank,
		Battery:                ur.battery,
		TierOverride:           ur.tierOverride,
		Verified:               ur.verified,
		PasswordChangeRequired: ur.passwordChangeRequired,
		IsSuperadmin:           ur.isSuperadmin,
		CreatedAt:              ur.createdAt,
		UpdatedAt:              ur.updatedAt,
	}

	resp := sessionUserResponse{
		User: user,
		Tier: int(user.GetTier()),
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func generateID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func generateSessionToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    token,
		Path:     "/",
		MaxAge:   30 * 24 * 60 * 60, // 30 days
		HttpOnly: true,
		Secure:   os.Getenv("ENVIRONMENT") == "production",
		SameSite: http.SameSiteLaxMode,
	})
}
