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
	id           string
	fullName     *string
	rank         *string
	battery      *string
	nricLast5    *string
	dob          *string
	isSuperadmin bool
	tierOverride *int16
	verified     bool
	createdAt    time.Time
	updatedAt    time.Time
	password     string
}

type pendingRegistrationRow struct {
	passwordHash string
}

const passwordHashCost = 12

// signInUserColumns is the SELECT list shared by the sign-in lookups so the
// primary query and the word-subset fallback scan an identical set of columns.
const signInUserColumns = `u.id, u."full_name", u.rank, u.battery, u."nric_last5", u.dob, u."is_superadmin", u.tier_override, u.verified, u."createdAt", u."updatedAt", u.password`

// scanUserRow scans signInUserColumns (in order) into ur. It accepts a pgx.Row,
// which both Pool.QueryRow and Pool.Query rows satisfy.
func scanUserRow(row pgx.Row, ur *userRow) error {
	return row.Scan(
		&ur.id, &ur.fullName, &ur.rank, &ur.battery, &ur.nricLast5, &ur.dob, &ur.isSuperadmin,
		&ur.tierOverride, &ur.verified, &ur.createdAt, &ur.updatedAt, &ur.password,
	)
}

// findUsersByNRICAndName returns users whose nric_last5 equals nricLast5 AND whose
// full name contains every word in identifier (case-insensitive, any order). Used
// as a sign-in fallback so personnel can type part of their name.
func (h *AuthHandler) findUsersByNRICAndName(ctx context.Context, nricLast5, identifier string) ([]userRow, error) {
	rows, err := h.db.Pool.Query(ctx, `SELECT `+signInUserColumns+` FROM "user" u WHERE u."nric_last5" = $1`, nricLast5)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var matches []userRow
	for rows.Next() {
		var ur userRow
		if err := scanUserRow(rows, &ur); err != nil {
			return nil, err
		}
		if nameMatchesIdentifier(ur.fullName, identifier) {
			matches = append(matches, ur)
		}
	}
	return matches, rows.Err()
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
	passwordForAuth, nricLast5Val, legacyPasswordOK := prepareSignInCredential(req.Identifier, req.Password)
	if !legacyPasswordOK {
		passwordForAuth = strings.ToUpper(req.Password)
	}
	legacyErr := scanUserRow(h.db.Pool.QueryRow(ctx, `
		SELECT `+signInUserColumns+`
		FROM "user" u
		WHERE upper(u."full_name") = upper($1) AND u."nric_last5" IS NOT DISTINCT FROM $2
	`, req.Identifier, nricLast5Val), &ur)
	if legacyErr != nil && errors.Is(legacyErr, pgx.ErrNoRows) && nricLast5Val != nil {
		matches, fallbackErr := h.findUsersByNRICAndName(ctx, *nricLast5Val, req.Identifier)
		switch {
		case fallbackErr != nil:
			log.Printf("[SignIn] Name fallback query failed for %q: %v", req.Identifier, fallbackErr)
		case len(matches) > 1:
			writeInvalidCredentials(w, req.Identifier)
			return
		case len(matches) == 1:
			ur = matches[0]
			legacyErr = nil
		}
	}
	if legacyErr != nil {
		if !errors.Is(legacyErr, pgx.ErrNoRows) {
			log.Printf("[SignIn] Error querying legacy user: %v", legacyErr)
			http.Error(w, "Failed to query user", http.StatusInternalServerError)
			return
		}
		// Keep unknown identifiers indistinguishable from wrong passwords.
		writeInvalidCredentials(w, req.Identifier)
		return
	}

	if !comparePassword(ur.password, passwordForAuth) {
		writeInvalidCredentials(w, req.Identifier)
		return
	}
	h.finishSignIn(w, r, ctx, req.Identifier, ur)
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
		ID:           ur.id,
		FullName:     ur.fullName,
		Rank:         ur.rank,
		Battery:      ur.battery,
		NRICLast5:    ur.nricLast5,
		DOB:          ur.dob,
		TierOverride: ur.tierOverride,
		Verified:     ur.verified,
		IsSuperadmin: ur.isSuperadmin,
		CreatedAt:    ur.createdAt,
		UpdatedAt:    ur.updatedAt,
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
			u.id, u."full_name", u.rank, u.battery, u."nric_last5", u.dob, u."is_superadmin",
			u.tier_override, u.verified,
			u."createdAt", u."updatedAt"
		FROM "user" u
		JOIN session s ON s."userId" = u.id
		WHERE s.token = $1 AND s."expiresAt" > NOW()
	`, cookie.Value).Scan(
		&ur.id, &ur.fullName, &ur.rank, &ur.battery, &ur.nricLast5, &ur.dob, &ur.isSuperadmin,
		&ur.tierOverride, &ur.verified,
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
		ID:           ur.id,
		FullName:     ur.fullName,
		Rank:         ur.rank,
		Battery:      ur.battery,
		NRICLast5:    ur.nricLast5,
		DOB:          ur.dob,
		TierOverride: ur.tierOverride,
		Verified:     ur.verified,
		IsSuperadmin: ur.isSuperadmin,
		CreatedAt:    ur.createdAt,
		UpdatedAt:    ur.updatedAt,
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
