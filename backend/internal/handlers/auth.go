package handlers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/davidlivingston/go-nextjs-starter/backend/internal/database"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/models"
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

type AuthResponse struct {
	User    models.User `json:"user"`
	Session string      `json:"session"`
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

	// Find user by full_name (or admin identifier)
	var userID, hashedPassword string
	var fullName, rank, battery *string
	var nricLast4, dob *string
	var isSuperadmin bool
	var createdAt, updatedAt time.Time

	// Try to find user by full_name
	err := h.db.Pool.QueryRow(ctx, `
		SELECT 
			u.id, u."full_name", u.rank, u.battery, u."nric_last4", u.dob, u."is_superadmin",
			u."createdAt", u."updatedAt", u.password
		FROM "user" u
		WHERE u."full_name" = $1
		LIMIT 1
	`, req.Identifier).Scan(
		&userID, &fullName, &rank, &battery, &nricLast4, &dob, &isSuperadmin, &createdAt, &updatedAt, &hashedPassword,
	)

	if err != nil {
		// User not found - automatically create a new user
		userID = generateID()
		now := time.Now()

		// Extract NRIC Last 4 and DOB from password if it's 10 characters (format: NRICLast4 + DOB)
		var nricLast4Val, dobVal *string
		if len(req.Password) == 10 {
			nricLast4Str := req.Password[:4]
			dobStr := req.Password[4:]
			nricLast4Val = &nricLast4Str
			dobVal = &dobStr
		}

		// Hash the password
		hashedPasswordBytes, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			http.Error(w, "Failed to hash password", http.StatusInternalServerError)
			return
		}
		hashedPassword = string(hashedPasswordBytes)

		// Create the user account
		_, err = h.db.Pool.Exec(ctx, `
			INSERT INTO "user" (
				id, "full_name", rank, battery, "nric_last4", dob, password,
				"createdAt", "updatedAt", "is_superadmin"
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		`, userID, req.Identifier, nil, nil, nricLast4Val, dobVal, hashedPassword, now, now, false)

		if err != nil {
			http.Error(w, "Failed to create user", http.StatusInternalServerError)
			return
		}

		// Set user fields for response
		fullName = &req.Identifier
		rank = nil
		battery = nil
		nricLast4 = nricLast4Val
		dob = dobVal
		isSuperadmin = false
		createdAt = now
		updatedAt = now
	} else {
		// User exists - verify password
		if err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(req.Password)); err != nil {
			http.Error(w, "Invalid identifier or password", http.StatusUnauthorized)
			return
		}
	}

	// Create new session
	sessionToken := generateSessionToken()
	sessionID := generateID()
	expiresAt := time.Now().Add(30 * 24 * time.Hour)
	now := time.Now()

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
	user := models.User{
		ID:           userID,
		FullName:     fullName,
		Rank:         rank,
		Battery:      battery,
		NRICLast4:    nricLast4,
		DOB:          dob,
		IsSuperadmin: isSuperadmin,
		CreatedAt:    createdAt,
		UpdatedAt:    updatedAt,
	}

	response := AuthResponse{
		User:    user,
		Session: sessionToken,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
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
	var nricLast4, dob *string
	var isSuperadmin bool
	var createdAt, updatedAt time.Time

	err = h.db.Pool.QueryRow(ctx, `
		SELECT 
			u.id, u."full_name", u.rank, u.battery, u."nric_last4", u.dob, u."is_superadmin",
			u."createdAt", u."updatedAt"
		FROM "user" u
		JOIN session s ON s."userId" = u.id
		WHERE s.token = $1 AND s."expiresAt" > NOW()
	`, cookie.Value).Scan(
		&userID, &fullName, &rank, &battery, &nricLast4, &dob, &isSuperadmin,
		&createdAt, &updatedAt,
	)

	if err != nil {
		http.Error(w, "Invalid or expired session", http.StatusUnauthorized)
		return
	}

	user := models.User{
		ID:           userID,
		FullName:     fullName,
		Rank:         rank,
		Battery:      battery,
		NRICLast4:    nricLast4,
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
