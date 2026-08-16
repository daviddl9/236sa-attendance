package handlers

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"math/big"
	"net/http"
	"strings"

	"github.com/davidlivingston/go-nextjs-starter/backend/internal/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

const temporaryPasswordAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789"
const temporaryPasswordLength = 16

type provisionCredentialsRequest struct {
	Username string `json:"username"`
}
type ProvisionCredentialsResponse struct {
	Username          string `json:"username"`
	TemporaryPassword string `json:"temporaryPassword"`
}

func recordAdminAction(ctx context.Context, tx pgx.Tx, actorID, targetID, action string) error {
	if actorID == "" {
		return nil
	}
	_, err := tx.Exec(ctx, `INSERT INTO credential_audit (id, actor_user_id, target_user_id, action)
		VALUES ($1, $2, $3, $4)`, generateID(), actorID, targetID, action)
	return err
}

func generateTemporaryPassword() (string, error) {
	password := make([]byte, temporaryPasswordLength)
	max := big.NewInt(int64(len(temporaryPasswordAlphabet)))
	for i := range password {
		index, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		password[i] = temporaryPasswordAlphabet[index.Int64()]
	}
	return string(password), nil
}

// ProvisionCredentials replaces a user's username and password. The plaintext
// password is returned only in this response; it is never persisted or logged.
func (h *AdminHandler) ProvisionCredentials(w http.ResponseWriter, r *http.Request) {
	actor, ok := middleware.GetUserFromContext(r.Context())
	if !ok {
		http.Error(w, "Not authenticated", http.StatusUnauthorized)
		return
	}
	var req provisionCredentialsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" {
		http.Error(w, "Username is required", http.StatusBadRequest)
		return
	}
	if strings.HasPrefix(normalizeUsername(req.Username), migratedPendingUsernamePrefix) {
		http.Error(w, "Username is unavailable", http.StatusBadRequest)
		return
	}
	password, err := generateTemporaryPassword()
	if err != nil {
		http.Error(w, "Failed to generate temporary password", http.StatusInternalServerError)
		return
	}
	if err := validatePassword(password, password); err != nil {
		http.Error(w, "Failed to validate temporary password", http.StatusInternalServerError)
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), passwordHashCost)
	if err != nil {
		http.Error(w, "Failed to hash temporary password", http.StatusInternalServerError)
		return
	}

	ctx, targetID := r.Context(), chi.URLParam(r, "id")
	tx, err := h.db.Pool.Begin(ctx)
	if err != nil {
		http.Error(w, "Failed to provision credentials", http.StatusInternalServerError)
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()
	normalized := normalizeUsername(req.Username)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, normalized); err != nil {
		http.Error(w, "Failed to provision credentials", http.StatusInternalServerError)
		return
	}
	var taken bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(
		SELECT 1 FROM "user" WHERE lower(trim(username)) = $1 AND id <> $2
		UNION ALL SELECT 1 FROM pending_registration WHERE lower(trim(username)) = $1
	)`, normalized, targetID).Scan(&taken)
	if err != nil {
		http.Error(w, "Failed to check username", http.StatusInternalServerError)
		return
	}
	if taken {
		http.Error(w, "Username is unavailable", http.StatusConflict)
		return
	}
	result, err := tx.Exec(ctx, `UPDATE "user"
		SET username = $1, password = $2, "updatedAt" = NOW()
		WHERE id = $3`, req.Username, string(hash), targetID)
	if err != nil {
		http.Error(w, "Failed to provision credentials", http.StatusInternalServerError)
		return
	}
	if result.RowsAffected() == 0 {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}
	if err := recordAdminAction(ctx, tx, actor.ID, targetID, "credential_provision"); err != nil {
		http.Error(w, "Failed to record credential provision", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		http.Error(w, "Failed to provision credentials", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(ProvisionCredentialsResponse{Username: req.Username, TemporaryPassword: password})
}
