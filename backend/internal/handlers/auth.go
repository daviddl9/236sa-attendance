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
	Identifier string `json:"identifier"` // Can be full_name or admin username
	Password   string `json:"password"`
}

// SignInOutcome is the machine-readable result of a sign-in attempt.
// "authenticated"   — credentials matched, session created.
// "signup_required" — full_name is not in the DB; admin should import
//
//	or the user can create their own account.
type SignInOutcome string

const (
	SignInOutcomeAuthenticated   SignInOutcome = "authenticated"
	SignInOutcomeSignupRequired  SignInOutcome = "signup_required"
)

// SignInResponse is returned for every successful (2xx) sign-in call.
// When Outcome=="authenticated", User and Session are populated.
// When Outcome=="signup_required", FullName is populated.
type SignInResponse struct {
	Outcome  SignInOutcome `json:"outcome"`
	User     *models.User `json:"user,omitempty"`
	Session  *string      `json:"session,omitempty"`
	FullName string       `json:"fullName,omitempty"`
}

type SignUpRequest struct {
	FullName        string `json:"fullName"`
	NRICLast5       string `json:"nricLast5"`
	ConfirmNRICLast5 string `json:"confirmNricLast5"`
}

// AuthResponse is kept for backward-compatibility with the admin path
// which returns the same shape as before.
type AuthResponse = SignInResponse

func (h *AuthHandler) SignIn(w http.ResponseWriter, r *http.Request) {
	var req SignInRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.Identifier == "" || req.Password == "" {
		http.Error(w, "Identifier and password are required", http.StatusBadRequest)
		return
	}

	ctx := context.Background()

	passwordForAuth, nricLast5Val, ok := prepareSignInCredential(req.Identifier, req.Password)
	if !ok {
		http.Error(w, nricLast5FormatMessage, http.StatusBadRequest)
		return
	}

	var userID, hashedPassword string
	var fullName, rank, battery *string
	var nricLast5, dob *string
	var isSuperadmin bool
	var createdAt, updatedAt time.Time

	err := h.db.Pool.QueryRow(ctx, `
		SELECT
			u.id, u."full_name", u.rank, u.battery, u."nric_last5", u.dob, u."is_superadmin",
			u."createdAt", u."updatedAt", u.password
		FROM "user" u
		WHERE u."full_name" = $1 AND u."nric_last5" IS NOT DISTINCT FROM $2
	`, req.Identifier, nricLast5Val).Scan(
		&userID, &fullName, &rank, &battery, &nricLast5, &dob, &isSuperadmin,
		&createdAt, &updatedAt, &hashedPassword,
	)

	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			log.Printf("[SignIn] Error querying user table: %v", err)
			http.Error(w, "Failed to query user", http.StatusInternalServerError)
			return
		}

		// Admin path: no auto-create, no signup_required.
		if req.Identifier == "admin" {
			log.Printf("[SignIn] Admin login failed - invalid credentials")
			http.Error(w, "Invalid identifier or password", http.StatusUnauthorized)
			return
		}

		// Name-leak protection (FR-021): if the full_name exists in the DB
		// with a different NRIC, return generic invalid_credentials rather than
		// signup_required so we don't reveal that the name is registered.
		var nameExists bool
		_ = h.db.Pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM "user" WHERE "full_name" = $1)`, req.Identifier,
		).Scan(&nameExists)
		if nameExists {
			log.Printf("[SignIn] Name exists but NRIC mismatch for %q — returning generic error", req.Identifier)
			http.Error(w, "Invalid identifier or password", http.StatusUnauthorized)
			return
		}

		// Name not in DB: explicit signup required.
		log.Printf("[SignIn] Unknown name %q — returning signup_required", req.Identifier)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(SignInResponse{
			Outcome:  SignInOutcomeSignupRequired,
			FullName: req.Identifier,
		})
		return
	}

	// User found — verify password.
	if err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(passwordForAuth)); err != nil {
		log.Printf("[SignIn] Password verification failed for user: %s", req.Identifier)
		http.Error(w, "Invalid identifier or password", http.StatusUnauthorized)
		return
	}
	log.Printf("[SignIn] User authenticated successfully: %s", req.Identifier)

	user, session, err := h.createSession(ctx, r, userID, fullName, rank, battery, nricLast5, dob, isSuperadmin, createdAt, updatedAt)
	if err != nil {
		log.Printf("[SignIn] Failed to create session for user %s: %v", userID, err)
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

// SignUp handles POST /api/auth/sign-up.
// It creates a new user account when the full_name is not already in the
// DB. Both nricLast5 values must match and pass the feature-001 format
// check. The account is immediately authenticated (session created).
func (h *AuthHandler) SignUp(w http.ResponseWriter, r *http.Request) {
	var req SignUpRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.FullName == "" {
		http.Error(w, "fullName is required", http.StatusBadRequest)
		return
	}

	nric, ok := normalizeNRICLast5(req.NRICLast5)
	if !ok {
		http.Error(w, nricLast5FormatMessage, http.StatusBadRequest)
		return
	}
	confirmNric, ok := normalizeNRICLast5(req.ConfirmNRICLast5)
	if !ok {
		http.Error(w, nricLast5FormatMessage, http.StatusBadRequest)
		return
	}
	if nric != confirmNric {
		http.Error(w, "NRIC Last 5 values do not match", http.StatusBadRequest)
		return
	}

	ctx := context.Background()

	// Guard against creating a duplicate account.
	var nameExists bool
	_ = h.db.Pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM "user" WHERE "full_name" = $1)`, req.FullName,
	).Scan(&nameExists)
	if nameExists {
		// Return generic error — do not reveal that the name exists.
		http.Error(w, "Invalid identifier or password", http.StatusUnauthorized)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(nric), 4)
	if err != nil {
		http.Error(w, "Failed to hash password", http.StatusInternalServerError)
		return
	}

	userID := generateID()
	now := time.Now()
	_, err = h.db.Pool.Exec(ctx, `
		INSERT INTO "user" (id, "full_name", "nric_last5", password, extras, "createdAt", "updatedAt", "is_superadmin")
		VALUES ($1, $2, $3, $4, '{}'::jsonb, $5, $6, false)
	`, userID, req.FullName, nric, string(hash), now, now)
	if err != nil {
		log.Printf("[SignUp] Failed to create user %q: %v", req.FullName, err)
		http.Error(w, "Failed to create account", http.StatusInternalServerError)
		return
	}
	log.Printf("[SignUp] Created account for %q", req.FullName)

	fullName := req.FullName
	user, session, err := h.createSession(ctx, r, userID, &fullName, nil, nil, &nric, nil, false, now, now)
	if err != nil {
		log.Printf("[SignUp] Failed to create session for %s: %v", userID, err)
		http.Error(w, "Failed to create session", http.StatusInternalServerError)
		return
	}

	setSessionCookie(w, session)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(SignInResponse{
		Outcome: SignInOutcomeAuthenticated,
		User:    &user,
		Session: &session,
	})
}

// createSession inserts a session row, returns the user model and token.
func (h *AuthHandler) createSession(
	ctx context.Context, r *http.Request,
	userID string, fullName, rank, battery, nricLast5, dob *string,
	isSuperadmin bool, createdAt, updatedAt time.Time,
) (models.User, string, error) {
	token := generateSessionToken()
	sessionID := generateID()
	expiresAt := time.Now().Add(30 * 24 * time.Hour)
	now := time.Now()

	_, err := h.db.Pool.Exec(ctx, `
		INSERT INTO session (id, "expiresAt", token, "userId", "createdAt", "updatedAt", "ipAddress", "userAgent")
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, sessionID, expiresAt, token, userID, now, now, r.RemoteAddr, r.UserAgent())
	if err != nil {
		return models.User{}, "", err
	}
	log.Printf("[SignIn] Session created successfully for user: %s", userID)

	user := models.User{
		ID:           userID,
		FullName:     fullName,
		Rank:         rank,
		Battery:      battery,
		NRICLast5:    nricLast5,
		DOB:          dob,
		IsSuperadmin: isSuperadmin,
		CreatedAt:    createdAt,
		UpdatedAt:    updatedAt,
	}
	return user, token, nil
}

func (h *AuthHandler) SignOut(w http.ResponseWriter, r *http.Request) {
	// Get session token from cookie
	cookie, err := r.Cookie("session")
	if err != nil {
		http.Error(w, "Not authenticated", http.StatusUnauthorized)
		return
	}

	ctx := context.Background()

	// Delete session from database
	_, err = h.db.Pool.Exec(ctx, `DELETE FROM session WHERE token = $1`, cookie.Value)
	if err != nil {
		http.Error(w, "Failed to delete session", http.StatusInternalServerError)
		return
	}

	// Clear cookie
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

func (h *AuthHandler) GetSession(w http.ResponseWriter, r *http.Request) {
	// Get session token from cookie
	cookie, err := r.Cookie("session")
	if err != nil {
		http.Error(w, "Not authenticated", http.StatusUnauthorized)
		return
	}

	ctx := context.Background()

	// Find session and user with all fields
	var userID string
	var fullName, rank, battery *string
	var nricLast5, dob *string
	var isSuperadmin bool
	var createdAt, updatedAt time.Time

	err = h.db.Pool.QueryRow(ctx, `
		SELECT 
			u.id, u."full_name", u.rank, u.battery, u."nric_last5", u.dob, u."is_superadmin",
			u."createdAt", u."updatedAt"
		FROM "user" u
		JOIN session s ON s."userId" = u.id
		WHERE s.token = $1 AND s."expiresAt" > NOW()
	`, cookie.Value).Scan(
		&userID, &fullName, &rank, &battery, &nricLast5, &dob, &isSuperadmin,
		&createdAt, &updatedAt,
	)

	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			log.Printf("[GetSession] Error querying user table: %v", err)
		} else {
			log.Printf("[GetSession] Session not found or expired for token")
		}
		http.Error(w, "Invalid or expired session", http.StatusUnauthorized)
		return
	}
	log.Printf("[GetSession] Found user session for user: %s", userID)

	user := models.User{
		ID:           userID,
		FullName:     fullName,
		Rank:         rank,
		Battery:      battery,
		NRICLast5:    nricLast5,
		DOB:          dob,
		IsSuperadmin: isSuperadmin,
		CreatedAt:    createdAt,
		UpdatedAt:    updatedAt,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(user)
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
