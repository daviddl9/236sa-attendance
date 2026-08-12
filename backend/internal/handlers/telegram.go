package handlers

import (
	"bytes"
	"context"
	"crypto/subtle"
	"errors"
	"io"
	"log"
	"net/http"

	"github.com/davidlivingston/go-nextjs-starter/backend/internal/database"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/sse"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/telegram"
	"github.com/jackc/pgx/v5"
)

const (
	telegramSecretHeader = "X-Telegram-Bot-Api-Secret-Token"
	maxTelegramUpdate    = 1 << 20
)

// TelegramHandler authenticates and decodes Telegram webhook updates. Pairing
// and attendance actions are delegated to the bot's service adapters.
type TelegramHandler struct {
	bot        *telegram.Bot
	secret     string
	actionSink telegram.ActionSink
	enabled    bool
}

func NewTelegramHandler(bot *telegram.Bot, webhookSecret string, actionSink telegram.ActionSink) *TelegramHandler {
	return &TelegramHandler{
		bot:        bot,
		secret:     webhookSecret,
		actionSink: actionSink,
		enabled:    bot != nil && actionSink != nil,
	}
}

// Webhook handles one Telegram update. Authentication failures deliberately
// look like a missing endpoint and are checked before reading or decoding the
// request body, so forged requests cannot cause outbound messages.
func (h *TelegramHandler) Webhook(w http.ResponseWriter, r *http.Request) {
	if !h.enabled || !h.authenticated(r) {
		h.reject(w)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxTelegramUpdate+1))
	if err != nil {
		h.serverError(w)
		return
	}
	if len(body) > maxTelegramUpdate {
		w.WriteHeader(http.StatusOK)
		return
	}

	update, err := telegram.DecodeUpdate(bytes.NewReader(body))
	if err != nil {
		// The request is authentic but cannot be acted on. A 200 response
		// prevents Telegram from retrying malformed input forever.
		w.WriteHeader(http.StatusOK)
		return
	}
	actions, err := h.bot.HandleUpdate(r.Context(), update)
	if err != nil {
		log.Printf("Telegram update handling failed")
		h.serverError(w)
		return
	}
	if !h.actionSink.Enqueue(actions) {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *TelegramHandler) authenticated(r *http.Request) bool {
	if len(h.secret) == 0 {
		return false
	}
	provided := r.Header.Get(telegramSecretHeader)
	return subtle.ConstantTimeCompare([]byte(provided), []byte(h.secret)) == 1
}

func (h *TelegramHandler) reject(w http.ResponseWriter) {
	// Do not distinguish a disabled bot, a wrong path, or a wrong header.
	w.WriteHeader(http.StatusNotFound)
}

func (h *TelegramHandler) serverError(w http.ResponseWriter) {
	http.Error(w, "Internal server error", http.StatusInternalServerError)
}

// TelegramPairingStore adapts the pairing tables for the bot. The join is
// constrained by Telegram account ID, so the only name returned to a paired
// soldier is that account's own name.
type TelegramPairingStore struct {
	db  *database.DB
	hub *sse.Hub
}

func NewTelegramPairingStore(db *database.DB) *TelegramPairingStore {
	return NewTelegramPairingStoreWithHub(db, nil)
}

// NewTelegramPairingStoreWithHub wires the optional live attendance hub. The
// nil-hub constructor remains useful for tests and deployments that do not
// expose SSE.
func NewTelegramPairingStoreWithHub(db *database.DB, hub *sse.Hub) *TelegramPairingStore {
	return &TelegramPairingStore{db: db, hub: hub}
}

func (s *TelegramPairingStore) FindPairing(ctx context.Context, telegramID int64) (telegram.Pairing, bool, error) {
	if s == nil || s.db == nil || s.db.Pool == nil {
		return telegram.Pairing{}, false, errors.New("telegram pairing store is not configured")
	}

	var pairing telegram.Pairing
	err := s.db.Pool.QueryRow(ctx, `
		SELECT p.telegram_id, p.user_id, COALESCE(u."full_name", '')
		FROM telegram_pairing p
		JOIN "user" u ON u.id = p.user_id
		WHERE p.telegram_id = $1
	`, telegramID).Scan(&pairing.TelegramID, &pairing.UserID, &pairing.FullName)
	if errors.Is(err, pgx.ErrNoRows) {
		return telegram.Pairing{}, false, nil
	}
	if err != nil {
		return telegram.Pairing{}, false, err
	}
	return pairing, true, nil
}
