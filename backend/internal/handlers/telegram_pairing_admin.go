package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/davidlivingston/go-nextjs-starter/backend/internal/middleware"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/services/matching"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

type TelegramPairingRecord struct {
	TelegramID    int64  `json:"telegramId"`
	UserID        string `json:"userId"`
	FullName      string `json:"fullName"`
	SelfConfirmed bool   `json:"selfConfirmed"`
	ConfirmedBy   string `json:"confirmedBy,omitempty"`
	CreatedAt     string `json:"createdAt"`
}

type TelegramPairingAttemptRecord struct {
	TelegramID int64  `json:"telegramId"`
	Outcome    string `json:"outcome"`
	UserID     string `json:"userId,omitempty"`
	FullName   string `json:"fullName,omitempty"`
	CreatedAt  string `json:"createdAt"`
}

type TelegramPairingRequestRecord struct {
	TelegramID  int64                   `json:"telegramId"`
	DisplayName string                  `json:"displayName"`
	AttemptID   string                  `json:"attemptId"`
	CreatedAt   string                  `json:"createdAt"`
	Candidates  []RegistrationCandidate `json:"candidates"`
}

type TelegramPairingReviewResponse struct {
	Pairings []TelegramPairingRecord        `json:"pairings"`
	Attempts []TelegramPairingAttemptRecord `json:"attempts"`
	Requests []TelegramPairingRequestRecord `json:"requests"`
}

func requireTelegramSuperadmin(w http.ResponseWriter, r *http.Request) bool {
	actor, ok := middleware.GetUserFromContext(r.Context())
	if !ok {
		http.Error(w, "Not authenticated", http.StatusUnauthorized)
		return false
	}
	if !actor.IsSuperadmin {
		http.Error(w, "Insufficient permissions", http.StatusForbidden)
		return false
	}
	return true
}

// ListTelegramPairings returns pairings and the exception attempts first so
// the review page does not bury conflicts below routine self-confirmations.
func (h *AdminHandler) ListTelegramPairings(w http.ResponseWriter, r *http.Request) {
	if !requireTelegramSuperadmin(w, r) {
		return
	}
	ctx := r.Context()
	pairings := make([]TelegramPairingRecord, 0)
	rows, err := h.db.Pool.Query(ctx, `
		SELECT p.telegram_id, p.user_id, COALESCE(u."full_name", ''), p.self_confirmed,
		       COALESCE(c."full_name", ''), p."createdAt"
		FROM telegram_pairing p JOIN "user" u ON u.id = p.user_id
		LEFT JOIN "user" c ON c.id = p.confirmed_by ORDER BY p."createdAt" DESC
	`)
	if err != nil {
		http.Error(w, "Failed to fetch Telegram pairings", http.StatusInternalServerError)
		return
	}
	for rows.Next() {
		var record TelegramPairingRecord
		var createdAt time.Time
		if err := rows.Scan(&record.TelegramID, &record.UserID, &record.FullName, &record.SelfConfirmed, &record.ConfirmedBy, &createdAt); err != nil {
			rows.Close()
			http.Error(w, "Failed to read Telegram pairings", http.StatusInternalServerError)
			return
		}
		record.CreatedAt = createdAt.Format(time.RFC3339)
		pairings = append(pairings, record)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		http.Error(w, "Failed to fetch Telegram pairings", http.StatusInternalServerError)
		return
	}

	attempts := make([]TelegramPairingAttemptRecord, 0)
	rows, err = h.db.Pool.Query(ctx, `
		SELECT a.telegram_id, a.outcome, COALESCE(a.user_id, ''), COALESCE(u."full_name", ''), a."createdAt"
		FROM telegram_pairing_attempt a LEFT JOIN "user" u ON u.id = a.user_id
		ORDER BY CASE WHEN a.outcome = 'refused_conflict' THEN 0 WHEN a.outcome = 'no_match' THEN 1 ELSE 2 END,
		         a."createdAt" DESC LIMIT 100
	`)
	if err != nil {
		http.Error(w, "Failed to fetch Telegram pairing attempts", http.StatusInternalServerError)
		return
	}
	for rows.Next() {
		var record TelegramPairingAttemptRecord
		var createdAt time.Time
		if err := rows.Scan(&record.TelegramID, &record.Outcome, &record.UserID, &record.FullName, &createdAt); err != nil {
			rows.Close()
			http.Error(w, "Failed to read Telegram pairing attempts", http.StatusInternalServerError)
			return
		}
		record.CreatedAt = createdAt.Format(time.RFC3339)
		attempts = append(attempts, record)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		http.Error(w, "Failed to fetch Telegram pairing attempts", http.StatusInternalServerError)
		return
	}

	requests, err := h.listTelegramPairingRequests(ctx)
	if err != nil {
		http.Error(w, "Failed to fetch Telegram pairing requests", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(TelegramPairingReviewResponse{Pairings: pairings, Attempts: attempts, Requests: requests})
}

func (h *AdminHandler) listTelegramPairingRequests(ctx context.Context) ([]TelegramPairingRequestRecord, error) {
	rosterRows, err := h.db.Pool.Query(ctx, `
		SELECT id, COALESCE("full_name", ''), COALESCE(rank, ''), COALESCE(battery, '')
		FROM "user" WHERE verified = true
	`)
	if err != nil {
		return nil, err
	}
	roster := make([]matching.RosterRow, 0)
	for rosterRows.Next() {
		var row matching.RosterRow
		if err := rosterRows.Scan(&row.ID, &row.Name, &row.Rank, &row.Battery); err != nil {
			rosterRows.Close()
			return nil, err
		}
		roster = append(roster, row)
	}
	rosterRows.Close()

	heldRows, err := h.db.Pool.Query(ctx, `SELECT user_id FROM telegram_pairing`)
	if err != nil {
		return nil, err
	}
	held := map[string]bool{}
	for heldRows.Next() {
		var userID string
		if err := heldRows.Scan(&userID); err != nil {
			heldRows.Close()
			return nil, err
		}
		held[userID] = true
	}
	heldRows.Close()

	rows, err := h.db.Pool.Query(ctx, `
		SELECT telegram_id, display_name, COALESCE(attempt_id, ''), "createdAt"
		FROM telegram_pairing_request ORDER BY "createdAt" DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	requests := make([]TelegramPairingRequestRecord, 0)
	for rows.Next() {
		var request TelegramPairingRequestRecord
		var createdAt time.Time
		if err := rows.Scan(&request.TelegramID, &request.DisplayName, &request.AttemptID, &createdAt); err != nil {
			return nil, err
		}
		request.CreatedAt = createdAt.Format(time.RFC3339)
		candidates := matching.RankCandidates(matching.Registration{Name: request.DisplayName}, roster)
		request.Candidates = make([]RegistrationCandidate, 0, len(candidates))
		for _, candidate := range candidates {
			if held[candidate.ID] {
				candidate.AlreadyClaimed = true
				candidate.Selectable = false
			}
			request.Candidates = append(request.Candidates, registrationCandidate(candidate))
		}
		requests = append(requests, request)
	}
	return requests, rows.Err()
}

func registrationCandidate(candidate matching.Candidate) RegistrationCandidate {
	return RegistrationCandidate{
		ID: candidate.ID, FullName: candidate.Name, Rank: candidate.Rank, Battery: candidate.Battery,
		Score: candidate.Score, Strong: candidate.Strong, AlreadyClaimed: candidate.AlreadyClaimed,
		ClaimedBy: candidate.ClaimedUsername, Selectable: candidate.Selectable, Preselected: candidate.Preselected,
	}
}

type confirmTelegramPairingRequest struct {
	UserID string `json:"userId"`
}

// ConfirmTelegramPairing is the exceptional commander path for a weak match.
func (h *AdminHandler) ConfirmTelegramPairing(w http.ResponseWriter, r *http.Request) {
	actor, ok := middleware.GetUserFromContext(r.Context())
	if !ok {
		http.Error(w, "Not authenticated", http.StatusUnauthorized)
		return
	}
	if !actor.IsSuperadmin {
		http.Error(w, "Insufficient permissions", http.StatusForbidden)
		return
	}
	telegramID, err := strconv.ParseInt(chi.URLParam(r, "telegramID"), 10, 64)
	if err != nil || telegramID <= 0 {
		http.Error(w, "Invalid Telegram account", http.StatusBadRequest)
		return
	}
	var req confirmTelegramPairingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.UserID) == "" {
		http.Error(w, "userId is required", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	tx, err := h.db.Pool.Begin(ctx)
	if err != nil {
		http.Error(w, "Failed to pair Telegram account", http.StatusInternalServerError)
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, telegramID); err != nil {
		http.Error(w, "Failed to lock Telegram pairing", http.StatusInternalServerError)
		return
	}
	var displayName, attemptID string
	if err := tx.QueryRow(ctx, `
		SELECT display_name, COALESCE(attempt_id, '')
		FROM telegram_pairing_request WHERE telegram_id = $1 FOR UPDATE
	`, telegramID).Scan(&displayName, &attemptID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "Pairing request not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to fetch pairing request", http.StatusInternalServerError)
		return
	}
	if strings.TrimSpace(attemptID) == "" {
		http.Error(w, "Pairing attempt not found", http.StatusNotFound)
		return
	}
	result, err := tx.Exec(ctx, `
		INSERT INTO telegram_pairing (id, telegram_id, user_id, display_name, confirmed_by, self_confirmed)
		VALUES ($1, $2, $3, $4, $5, false) ON CONFLICT DO NOTHING
	`, generateID(), telegramID, req.UserID, displayName, actor.ID)
	if err != nil {
		http.Error(w, "Failed to pair Telegram account", http.StatusInternalServerError)
		return
	}
	if result.RowsAffected() == 0 {
		http.Error(w, "Telegram account or roster row is already paired", http.StatusConflict)
		return
	}
	result, err = tx.Exec(ctx, `
		UPDATE telegram_pairing_attempt SET outcome = 'confirmed', user_id = $2
		WHERE id = $1 AND telegram_id = $3 AND outcome = 'no_match'
	`, attemptID, req.UserID, telegramID)
	if err != nil {
		http.Error(w, "Failed to record pairing", http.StatusInternalServerError)
		return
	}
	if result.RowsAffected() != 1 {
		http.Error(w, "Pairing attempt not found", http.StatusNotFound)
		return
	}
	if _, err := tx.Exec(ctx, `DELETE FROM telegram_pairing_request WHERE telegram_id = $1`, telegramID); err != nil {
		http.Error(w, "Failed to clear pairing request", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		http.Error(w, "Failed to pair Telegram account", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"message": "Telegram pairing confirmed"})
}

// UnpairTelegramAccount removes only the Telegram relationship; attendance
// history remains attached to the roster row.
func (h *AdminHandler) UnpairTelegramAccount(w http.ResponseWriter, r *http.Request) {
	if !requireTelegramSuperadmin(w, r) {
		return
	}
	telegramID, err := strconv.ParseInt(chi.URLParam(r, "telegramID"), 10, 64)
	if err != nil || telegramID <= 0 {
		http.Error(w, "Invalid Telegram account", http.StatusBadRequest)
		return
	}
	tx, err := h.db.Pool.Begin(r.Context())
	if err != nil {
		http.Error(w, "Failed to unpair Telegram account", http.StatusInternalServerError)
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()
	// Keep the same order as self-confirmation: Telegram account, roster
	// user, pairing row, then pairing attempts. In particular, take the
	// account lock before reading the row so an unpair cannot hold the row
	// while a confirmation waits for the account or roster lock.
	if _, err := tx.Exec(r.Context(), `SELECT pg_advisory_xact_lock($1)`, telegramID); err != nil {
		http.Error(w, "Failed to lock Telegram pairing", http.StatusInternalServerError)
		return
	}
	var userID string
	if err := tx.QueryRow(r.Context(), `SELECT user_id FROM telegram_pairing WHERE telegram_id = $1`, telegramID).Scan(&userID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "Pairing not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to fetch Telegram pairing", http.StatusInternalServerError)
		return
	}
	if _, err := tx.Exec(r.Context(), `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, userID); err != nil {
		http.Error(w, "Failed to lock Telegram pairing", http.StatusInternalServerError)
		return
	}
	if err := tx.QueryRow(r.Context(), `SELECT user_id FROM telegram_pairing WHERE telegram_id = $1 FOR UPDATE`, telegramID).Scan(&userID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "Pairing not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to lock Telegram pairing row", http.StatusInternalServerError)
		return
	}
	result, err := tx.Exec(r.Context(), `DELETE FROM telegram_pairing WHERE telegram_id = $1`, telegramID)
	if err != nil {
		http.Error(w, "Failed to unpair Telegram account", http.StatusInternalServerError)
		return
	}
	if result.RowsAffected() == 0 {
		http.Error(w, "Pairing not found", http.StatusNotFound)
		return
	}
	if _, err := tx.Exec(r.Context(), `
		UPDATE telegram_pairing_attempt
		SET user_id = NULL
		WHERE user_id = $1 AND outcome = 'proposed'
	`, userID); err != nil {
		http.Error(w, "Failed to invalidate Telegram proposals", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		http.Error(w, "Failed to unpair Telegram account", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"message": "Telegram account unpaired"})
}
