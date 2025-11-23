package handlers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
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

type SignUpRequest struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type SignInRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type AuthResponse struct {
	User    models.User `json:"user"`
	Session string      `json:"session"`
}

func (h *AuthHandler) SignUp(w http.ResponseWriter, r *http.Request) {
	var req SignUpRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate input
	if req.Email == "" || req.Password == "" || req.Name == "" {
		http.Error(w, "Name, email and password are required", http.StatusBadRequest)
		return
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "Failed to hash password", http.StatusInternalServerError)
		return
	}

	// Generate user ID
	userID := generateID()
	accountID := generateID()

	ctx := context.Background()

	// Start transaction
	tx, err := h.db.Pool.Begin(ctx)
	if err != nil {
		http.Error(w, "Failed to start transaction", http.StatusInternalServerError)
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Create user
	now := time.Now()
	_, err = tx.Exec(ctx, `
		INSERT INTO "user" (id, name, email, "emailVerified", "createdAt", "updatedAt")
		VALUES ($1, $2, $3, $4, $5, $6)
	`, userID, req.Name, req.Email, false, now, now)
	if err != nil {
		http.Error(w, "Email already exists", http.StatusConflict)
		return
	}

	// Create account with password
	_, err = tx.Exec(ctx, `
		INSERT INTO account (id, "accountId", "providerId", "userId", password, "createdAt", "updatedAt")
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, accountID, userID, "credential", userID, string(hashedPassword), now, now)
	if err != nil {
		http.Error(w, "Failed to create account", http.StatusInternalServerError)
		return
	}

	// Create session
	sessionToken := generateSessionToken()
	sessionID := generateID()
	expiresAt := time.Now().Add(30 * 24 * time.Hour) // 30 days

	_, err = tx.Exec(ctx, `
		INSERT INTO session (id, "expiresAt", token, "userId", "createdAt", "updatedAt", "ipAddress", "userAgent")
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, sessionID, expiresAt, sessionToken, userID, now, now, r.RemoteAddr, r.UserAgent())
	if err != nil {
		http.Error(w, "Failed to create session", http.StatusInternalServerError)
		return
	}

	if err := tx.Commit(ctx); err != nil {
		http.Error(w, "Failed to commit transaction", http.StatusInternalServerError)
		return
	}

	// Set session cookie
	setSessionCookie(w, sessionToken)

	// Return user and session
	user := models.User{
		ID:            userID,
		Name:          req.Name,
		Email:         req.Email,
		EmailVerified: false,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	response := AuthResponse{
		User:    user,
		Session: sessionToken,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

func (h *AuthHandler) SignIn(w http.ResponseWriter, r *http.Request) {
	var req SignInRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	ctx := context.Background()

	// Find user by email
	var userID, name, email, hashedPassword string
	var emailVerified bool
	var image *string
	var createdAt, updatedAt time.Time

	err := h.db.Pool.QueryRow(ctx, `
		SELECT u.id, u.name, u.email, u."emailVerified", u.image, u."createdAt", u."updatedAt", a.password
		FROM "user" u
		JOIN account a ON a."userId" = u.id AND a."providerId" = 'credential'
		WHERE u.email = $1
	`, req.Email).Scan(&userID, &name, &email, &emailVerified, &image, &createdAt, &updatedAt, &hashedPassword)

	if err != nil {
		http.Error(w, "Invalid email or password", http.StatusUnauthorized)
		return
	}

	// Verify password
	if err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(req.Password)); err != nil {
		http.Error(w, "Invalid email or password", http.StatusUnauthorized)
		return
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
		ID:            userID,
		Name:          name,
		Email:         email,
		EmailVerified: emailVerified,
		Image:         image,
		CreatedAt:     createdAt,
		UpdatedAt:     updatedAt,
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

	// Find session and user
	var userID, name, email string
	var emailVerified bool
	var image *string
	var createdAt, updatedAt time.Time

	err = h.db.Pool.QueryRow(ctx, `
		SELECT u.id, u.name, u.email, u."emailVerified", u.image, u."createdAt", u."updatedAt"
		FROM "user" u
		JOIN session s ON s."userId" = u.id
		WHERE s.token = $1 AND s."expiresAt" > NOW()
	`, cookie.Value).Scan(&userID, &name, &email, &emailVerified, &image, &createdAt, &updatedAt)

	if err != nil {
		http.Error(w, "Invalid or expired session", http.StatusUnauthorized)
		return
	}

	user := models.User{
		ID:            userID,
		Name:          name,
		Email:         email,
		EmailVerified: emailVerified,
		Image:         image,
		CreatedAt:     createdAt,
		UpdatedAt:     updatedAt,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(user)
}

func (h *AuthHandler) GoogleOAuthRedirect(w http.ResponseWriter, r *http.Request) {
	clientID := os.Getenv("GOOGLE_CLIENT_ID")
	if clientID == "" {
		http.Error(w, "Google OAuth client ID not configured", http.StatusInternalServerError)
		return
	}

	backendURL := os.Getenv("BACKEND_URL")
	if backendURL == "" {
		http.Error(w, "BACKEND_URL not configured", http.StatusInternalServerError)
		return
	}

	redirectURI := backendURL + "/api/auth/oauth/callback"

	// Build OAuth URL with proper encoding
	authURL, err := url.Parse("https://accounts.google.com/o/oauth2/v2/auth")
	if err != nil {
		http.Error(w, "Failed to parse OAuth URL", http.StatusInternalServerError)
		return
	}

	params := url.Values{}
	params.Set("client_id", clientID)
	params.Set("redirect_uri", redirectURI)
	params.Set("response_type", "code")
	params.Set("scope", "openid profile email")
	authURL.RawQuery = params.Encode()

	http.Redirect(w, r, authURL.String(), http.StatusFound)
}

func (h *AuthHandler) GoogleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "Authorization code not found", http.StatusBadRequest)
		return
	}

	// Exchange code for access token
	clientID := os.Getenv("GOOGLE_CLIENT_ID")
	clientSecret := os.Getenv("GOOGLE_CLIENT_SECRET")
	redirectURI := os.Getenv("BACKEND_URL") + "/api/auth/oauth/callback"

	tokenURL := "https://oauth2.googleapis.com/token"
	data := fmt.Sprintf("code=%s&client_id=%s&client_secret=%s&redirect_uri=%s&grant_type=authorization_code",
		code, clientID, clientSecret, redirectURI)

	resp, err := http.Post(tokenURL, "application/x-www-form-urlencoded", strings.NewReader(data))
	if err != nil {
		http.Error(w, "Failed to exchange code for token", http.StatusInternalServerError)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		IDToken     string `json:"id_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		http.Error(w, "Failed to decode token response", http.StatusInternalServerError)
		return
	}

	// Get user info from Google
	userInfoURL := "https://www.googleapis.com/oauth2/v2/userinfo"
	req, _ := http.NewRequest("GET", userInfoURL, nil)
	req.Header.Set("Authorization", "Bearer "+tokenResp.AccessToken)

	userInfoResp, err := http.DefaultClient.Do(req)
	if err != nil {
		http.Error(w, "Failed to get user info", http.StatusInternalServerError)
		return
	}
	defer func() { _ = userInfoResp.Body.Close() }()

	var userInfo struct {
		ID            string `json:"id"`
		Email         string `json:"email"`
		VerifiedEmail bool   `json:"verified_email"`
		Name          string `json:"name"`
		Picture       string `json:"picture"`
	}
	if err := json.NewDecoder(userInfoResp.Body).Decode(&userInfo); err != nil {
		http.Error(w, "Failed to decode user info", http.StatusInternalServerError)
		return
	}

	ctx := context.Background()
	tx, err := h.db.Pool.Begin(ctx)
	if err != nil {
		http.Error(w, "Failed to start transaction", http.StatusInternalServerError)
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Find or create user
	var userID string
	err = tx.QueryRow(ctx, `SELECT id FROM "user" WHERE email = $1`, userInfo.Email).Scan(&userID)

	now := time.Now()
	if err != nil {
		// User doesn't exist, create new user
		userID = generateID()
		_, err = tx.Exec(ctx, `
			INSERT INTO "user" (id, name, email, "emailVerified", image, "createdAt", "updatedAt")
			VALUES ($1, $2, $3, $4, $5, $6, $7)
		`, userID, userInfo.Name, userInfo.Email, userInfo.VerifiedEmail, userInfo.Picture, now, now)
		if err != nil {
			log.Printf("Failed to create user: %v", err)
			http.Error(w, "Failed to create user", http.StatusInternalServerError)
			return
		}
	}

	// Create or update OAuth account
	accountID := generateID()
	_, err = tx.Exec(ctx, `
		INSERT INTO account (id, "accountId", "providerId", "userId", "accessToken", "idToken", "createdAt", "updatedAt")
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT ("userId", "providerId") DO UPDATE SET
			"accessToken" = EXCLUDED."accessToken",
			"idToken" = EXCLUDED."idToken",
			"updatedAt" = EXCLUDED."updatedAt"
	`, accountID, userInfo.ID, "google", userID, tokenResp.AccessToken, tokenResp.IDToken, now, now)
	if err != nil {
		log.Printf("Failed to create/update OAuth account: %v", err)
		http.Error(w, "Failed to create account", http.StatusInternalServerError)
		return
	}

	// Create session
	sessionToken := generateSessionToken()
	sessionID := generateID()
	expiresAt := time.Now().Add(30 * 24 * time.Hour)

	_, err = tx.Exec(ctx, `
		INSERT INTO session (id, "expiresAt", token, "userId", "createdAt", "updatedAt", "ipAddress", "userAgent")
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, sessionID, expiresAt, sessionToken, userID, now, now, r.RemoteAddr, r.UserAgent())
	if err != nil {
		log.Printf("Failed to create session: %v", err)
		http.Error(w, "Failed to create session", http.StatusInternalServerError)
		return
	}

	if err := tx.Commit(ctx); err != nil {
		log.Printf("Failed to commit transaction: %v", err)
		http.Error(w, "Failed to commit transaction", http.StatusInternalServerError)
		return
	}

	// Set session cookie
	setSessionCookie(w, sessionToken)

	// Redirect to frontend dashboard
	frontendURL := os.Getenv("FRONTEND_URL")
	http.Redirect(w, r, frontendURL+"/dashboard", http.StatusFound)
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
