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
type SignInOutcome string

const (
	SignInOutcomeAuthenticated    SignInOutcome = "authenticated"
	SignInOutcomeSignupRequired   SignInOutcome = "signup_required"
	SignInOutcomePendingApproval  SignInOutcome = "pending_approval"
)

// SignInResponse is returned for every successful (2xx) sign-in call.
type SignInResponse struct {
	Outcome  SignInOutcome `json:"outcome"`
	User     *models.User `json:"user,omitempty"`
	Session  *string      `json:"session,omitempty"`
	FullName string       `json:"fullName,omitempty"`
}

type SignUpRequest struct {
	FullName         string `json:"fullName"`
	NRICLast5        string `json:"nricLast5"`
	ConfirmNRICLast5 string `json:"confirmNricLast5"`
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

	var ur userRow
	err := h.db.Pool.QueryRow(ctx, `
		SELECT
			u.id, u."full_name", u.rank, u.battery, u."nric_last5", u.dob, u."is_superadmin",
			u.tier_override, u.verified,
			u."createdAt", u."updatedAt", u.password
		FROM "user" u
		WHERE upper(u."full_name") = upper($1) AND u."nric_last5" IS NOT DISTINCT FROM $2
	`, req.Identifier, nricLast5Val).Scan(
		&ur.id, &ur.fullName, &ur.rank, &ur.battery, &ur.nricLast5, &ur.dob, &ur.isSuperadmin,
		&ur.tierOverride, &ur.verified,
		&ur.createdAt, &ur.updatedAt, &ur.password,
	)

	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			log.Printf("[SignIn] Error querying user table: %v", err)
			http.Error(w, "Failed to query user", http.StatusInternalServerError)
			return
		}

		if req.Identifier == "admin" {
			log.Printf("[SignIn] Admin login failed - invalid credentials")
			http.Error(w, "Invalid identifier or password", http.StatusUnauthorized)
			return
		}

		// Name-leak protection: if the name exists with a different NRIC, return generic error.
		var nameExists bool
		_ = h.db.Pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM "user" WHERE upper("full_name") = upper($1))`, req.Identifier,
		).Scan(&nameExists)
		if nameExists {
			log.Printf("[SignIn] Name exists but NRIC mismatch for %q — returning generic error", req.Identifier)
			http.Error(w, "Invalid identifier or password", http.StatusUnauthorized)
			return
		}

		log.Printf("[SignIn] Unknown name %q — returning signup_required", req.Identifier)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(SignInResponse{
			Outcome:  SignInOutcomeSignupRequired,
			FullName: req.Identifier,
		})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(ur.password), []byte(passwordForAuth)); err != nil {
		log.Printf("[SignIn] Password verification failed for user: %s", req.Identifier)
		http.Error(w, "Invalid identifier or password", http.StatusUnauthorized)
		return
	}

	// Block unverified (pending) users.
	if !ur.verified {
		log.Printf("[SignIn] User %q is pending approval", req.Identifier)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(SignInResponse{Outcome: SignInOutcomePendingApproval})
		return
	}

	log.Printf("[SignIn] User authenticated successfully: %s", req.Identifier)

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

// SignUp handles POST /api/auth/sign-up.
// Called after sign-in returns signup_required (name not in DB).
// Creates account with verified=false; returns pending_approval.
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

	var nameExists bool
	_ = h.db.Pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM "user" WHERE upper("full_name") = upper($1))`, req.FullName,
	).Scan(&nameExists)
	if nameExists {
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
		INSERT INTO "user" (id, "full_name", "nric_last5", password, extras, "createdAt", "updatedAt", "is_superadmin", verified)
		VALUES ($1, $2, $3, $4, '{}'::jsonb, $5, $6, false, false)
	`, userID, req.FullName, nric, string(hash), now, now)
	if err != nil {
		log.Printf("[SignUp] Failed to create user %q: %v", req.FullName, err)
		http.Error(w, "Failed to create account", http.StatusInternalServerError)
		return
	}
	log.Printf("[SignUp] Created pending account for %q", req.FullName)

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
