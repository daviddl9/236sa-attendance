package handlers

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/davidlivingston/go-nextjs-starter/backend/internal/database"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/middleware"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/models"
	outservice "github.com/davidlivingston/go-nextjs-starter/backend/internal/services/out"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/sse"
	"github.com/go-chi/chi/v5"
)

// OutHandler serves the "Out" section: sessions soldiers enrol themselves into
// by scanning at the gate on their way out and again on their way back in.
type OutHandler struct {
	db      *database.DB
	hub     *sse.Hub
	service *outservice.Service
}

// NewOutHandler constructs an Out session handler.
func NewOutHandler(db *database.DB, hub *sse.Hub) *OutHandler {
	return &OutHandler{db: db, hub: hub, service: outservice.NewService(db)}
}

// CreateOutSessionRequest is the body accepted by POST /api/sessions/out.
// ExpectedReturnAt is optional; omitting it selects the sub-type's SGT default.
type CreateOutSessionRequest struct {
	Name             string     `json:"name"`
	Subtype          string     `json:"subtype"`
	ExpectedReturnAt *time.Time `json:"expectedReturnAt,omitempty"`
}

// OutSubtypeOption describes one selectable sub-type and the return time it
// would default to, so the create form can pre-populate the field without
// duplicating the rule on the client.
type OutSubtypeOption struct {
	Subtype         string    `json:"subtype"`
	Label           string    `json:"label"`
	DefaultReturnAt time.Time `json:"defaultReturnAt"`
}

// ListOutSubtypes handles GET /api/sessions/out/subtypes.
func (h *OutHandler) ListOutSubtypes(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	options := make([]OutSubtypeOption, 0, len(models.ValidOutSubtypes))
	for _, subtype := range models.ValidOutSubtypes {
		options = append(options, OutSubtypeOption{
			Subtype:         subtype,
			Label:           models.OutSubtypeDisplayName(subtype),
			DefaultReturnAt: outservice.DefaultReturnAt(subtype, now),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"subtypes": options})
}

// CreateOutSession handles POST /api/sessions/out. Tier 3+ only; enforced by
// the route's middleware.
func (h *OutHandler) CreateOutSession(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.GetUserFromContext(r.Context())
	if !ok {
		http.Error(w, "Not authenticated", http.StatusUnauthorized)
		return
	}

	var req CreateOutSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	session, err := h.service.Create(r.Context(), outservice.CreateRequest{
		Name:             req.Name,
		Subtype:          req.Subtype,
		ExpectedReturnAt: req.ExpectedReturnAt,
		CreatedBy:        user.ID,
	})
	if err != nil {
		if errors.Is(err, outservice.ErrInvalidRequest) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, "Failed to create out session", http.StatusInternalServerError)
		return
	}

	setOutSessionQRVisibility(&session, user)
	writeJSON(w, http.StatusCreated, session)
}

// GetOutSession handles GET /api/sessions/out/{id}.
func (h *OutHandler) GetOutSession(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.GetUserFromContext(r.Context())
	if !ok {
		http.Error(w, "Not authenticated", http.StatusUnauthorized)
		return
	}

	session, err := h.service.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		if errors.Is(err, outservice.ErrSessionNotFound) {
			http.Error(w, "Out session not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to load out session", http.StatusInternalServerError)
		return
	}

	setOutSessionQRVisibility(&session, user)
	writeJSON(w, http.StatusOK, session)
}

// setOutSessionQRVisibility withholds the scannable code from anyone below
// commander tier, matching setSessionQRVisibility for attendance sessions.
func setOutSessionQRVisibility(session *models.OutSession, user *models.User) {
	if session == nil {
		return
	}
	if user == nil || !user.IsCommander() {
		session.QRCode = ""
	}
}

// outScanCookiePrefix names the short-lived cookie that carries the scanned QR
// secret from the redirect to the confirm screen, so the secret never travels
// in a URL where it would land in browser history, Referer headers or the
// request log.
const outScanCookiePrefix = "out_scan_"
const outScanCookieTTL = 5 * time.Minute

func outScanCookieName(sessionID string) string { return outScanCookiePrefix + sessionID }

// SetOutScanCookie stores the scanned secret for one Out session.
func SetOutScanCookie(w http.ResponseWriter, sessionID, secret string) {
	http.SetCookie(w, &http.Cookie{
		Name:     outScanCookieName(sessionID),
		Value:    secret,
		Path:     "/",
		MaxAge:   int(outScanCookieTTL.Seconds()),
		HttpOnly: true,
		Secure:   os.Getenv("ENVIRONMENT") == "production",
		SameSite: http.SameSiteLaxMode,
	})
}

// scanAuthorized reports whether the caller presented the session's QR secret.
// Comparison is constant-time, matching the Telegram webhook's handling.
func scanAuthorized(r *http.Request, sessionID, secret string) bool {
	cookie, err := r.Cookie(outScanCookieName(sessionID))
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(secret)) == 1
}

// OutSelfState is what the confirm screen needs: what this scan will do, and
// what the person's last movement was.
type OutSelfState struct {
	Session       models.OutSession   `json:"session"`
	NextDirection string              `json:"nextDirection"`
	Last          *models.OutMovement `json:"last,omitempty"`
	Scanned       bool                `json:"scanned"`
	// NextMovementAt is set while the repeat-scan window is still open. The
	// screen shows the wait instead of offering a confirm the server would
	// refuse.
	NextMovementAt *time.Time `json:"nextMovementAt,omitempty"`
}

// GetOutSelfState handles GET /api/out/sessions/{id}/me.
func (h *OutHandler) GetOutSelfState(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.GetUserFromContext(r.Context())
	if !ok {
		http.Error(w, "Not authenticated", http.StatusUnauthorized)
		return
	}
	sessionID := chi.URLParam(r, "id")
	session, err := h.service.Get(r.Context(), sessionID)
	if err != nil {
		h.writeSessionError(w, err)
		return
	}
	last, err := h.service.LatestMovement(r.Context(), sessionID, user.ID)
	if err != nil {
		http.Error(w, "Failed to load your status", http.StatusInternalServerError)
		return
	}
	scanned := scanAuthorized(r, sessionID, session.QRCodeSecret)
	session.QRCodeSecret = ""
	if !user.IsCommander() {
		session.QRCode = ""
	}
	writeJSON(w, http.StatusOK, OutSelfState{
		Session:        session,
		NextDirection:  outservice.NextDirection(last),
		Last:           last,
		Scanned:        scanned,
		NextMovementAt: outservice.NextMovementAt(last, time.Now()),
	})
}

// RecordOutMovementRequest is the body of POST /api/out/sessions/{id}/movement.
type RecordOutMovementRequest struct {
	// ExpectedDirection is the direction the confirm screen displayed. The
	// server re-derives it and refuses on mismatch, so a stale page cannot
	// flip someone the wrong way.
	ExpectedDirection string `json:"expectedDirection"`
}

// RecordOutMovement handles POST /api/out/sessions/{id}/movement.
func (h *OutHandler) RecordOutMovement(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.GetUserFromContext(r.Context())
	if !ok {
		http.Error(w, "Not authenticated", http.StatusUnauthorized)
		return
	}
	sessionID := chi.URLParam(r, "id")
	session, err := h.service.Get(r.Context(), sessionID)
	if err != nil {
		h.writeSessionError(w, err)
		return
	}
	if !scanAuthorized(r, sessionID, session.QRCodeSecret) {
		http.Error(w, "Scan the session QR code to record a movement", http.StatusForbidden)
		return
	}

	var req RecordOutMovementRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	result, err := h.service.RecordMovement(r.Context(), outservice.RecordRequest{
		SessionID:         sessionID,
		UserID:            user.ID,
		Method:            models.MarkingMethodQRScan,
		ExpectedDirection: req.ExpectedDirection,
	})
	if err != nil {
		http.Error(w, "Failed to record movement", http.StatusInternalServerError)
		return
	}
	h.writeMovementResult(w, r, sessionID, user, result)
}

// ManualOutMovementRequest records a movement on someone else's behalf.
type ManualOutMovementRequest struct {
	UserID            string `json:"userId"`
	ExpectedDirection string `json:"expectedDirection,omitempty"`
}

// ManualOutMovement handles POST /api/out/sessions/{id}/manual. Tier 2+.
func (h *OutHandler) ManualOutMovement(w http.ResponseWriter, r *http.Request) {
	actor, ok := middleware.GetUserFromContext(r.Context())
	if !ok {
		http.Error(w, "Not authenticated", http.StatusUnauthorized)
		return
	}
	var req ManualOutMovementRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.UserID) == "" {
		http.Error(w, "userId is required", http.StatusBadRequest)
		return
	}
	sessionID := chi.URLParam(r, "id")
	actorID := actor.ID
	result, err := h.service.RecordMovement(r.Context(), outservice.RecordRequest{
		SessionID:         sessionID,
		UserID:            req.UserID,
		Method:            models.MarkingMethodManual,
		ExpectedDirection: req.ExpectedDirection,
		RecordedBy:        &actorID,
	})
	if err != nil {
		http.Error(w, "Failed to record movement", http.StatusInternalServerError)
		return
	}
	h.writeMovementResult(w, r, sessionID, actor, result)
}

// VoidOutMovementRequest carries the reason a movement is being reversed.
type VoidOutMovementRequest struct {
	Reason string `json:"reason,omitempty"`
}

// VoidOutMovement handles POST /api/out/movements/{movementId}/void. Tier 2+.
func (h *OutHandler) VoidOutMovement(w http.ResponseWriter, r *http.Request) {
	actor, ok := middleware.GetUserFromContext(r.Context())
	if !ok {
		http.Error(w, "Not authenticated", http.StatusUnauthorized)
		return
	}
	var req VoidOutMovementRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	err := h.service.Void(r.Context(), chi.URLParam(r, "movementId"), actor.ID, req.Reason)
	if errors.Is(err, outservice.ErrMovementNotFound) {
		http.Error(w, "Movement not found or already reversed", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "Failed to reverse movement", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "Movement reversed"})
}

// GetOutBoard handles GET /api/out/sessions/{id}/board. Tier 2+.
func (h *OutHandler) GetOutBoard(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.GetUserFromContext(r.Context())
	if !ok {
		http.Error(w, "Not authenticated", http.StatusUnauthorized)
		return
	}
	board, err := h.service.GetBoard(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		h.writeSessionError(w, err)
		return
	}
	board.Session.QRCodeSecret = ""
	setOutSessionQRVisibility(&board.Session, user)
	writeJSON(w, http.StatusOK, board)
}

// ListOutSessions handles GET /api/sessions/out. Tier 2+.
func (h *OutHandler) ListOutSessions(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.GetUserFromContext(r.Context())
	if !ok {
		http.Error(w, "Not authenticated", http.StatusUnauthorized)
		return
	}
	includeClosed := r.URL.Query().Get("includeClosed") == "true"
	sessions, err := h.service.List(r.Context(), includeClosed)
	if err != nil {
		http.Error(w, "Failed to list out sessions", http.StatusInternalServerError)
		return
	}
	for i := range sessions {
		sessions[i].QRCodeSecret = ""
		setOutSessionQRVisibility(&sessions[i], user)
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": sessions})
}

// CloseOutSessionRequest acknowledges closing while people are still out.
type CloseOutSessionRequest struct {
	AcknowledgeStillOut bool `json:"acknowledgeStillOut"`
}

// CloseOutSession handles PUT /api/sessions/out/{id}/close. Tier 3+.
func (h *OutHandler) CloseOutSession(w http.ResponseWriter, r *http.Request) {
	actor, ok := middleware.GetUserFromContext(r.Context())
	if !ok {
		http.Error(w, "Not authenticated", http.StatusUnauthorized)
		return
	}
	var req CloseOutSessionRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	board, err := h.service.Close(r.Context(), chi.URLParam(r, "id"), actor.ID, req.AcknowledgeStillOut)
	if errors.Is(err, outservice.ErrStillOut) {
		// Mirrors the strong-match gate in registration approval: report what
		// the commander is about to accept rather than silently closing.
		board.Session.QRCodeSecret = ""
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":    "still_out",
			"message":  "Personnel have not scanned back in. Close anyway?",
			"stillOut": board.OutCount,
			"overdue":  board.OverdueCount,
			"members":  board.Members,
		})
		return
	}
	if errors.Is(err, outservice.ErrSessionNotActive) {
		http.Error(w, "Out session is already closed", http.StatusConflict)
		return
	}
	if err != nil {
		h.writeSessionError(w, err)
		return
	}
	board.Session.QRCodeSecret = ""
	setOutSessionQRVisibility(&board.Session, actor)
	h.broadcast(chi.URLParam(r, "id"), sse.Event{
		Type:    sse.EventTypeSessionClosed,
		Payload: sse.SessionClosedPayload{SessionID: board.Session.ID},
	})
	writeJSON(w, http.StatusOK, board)
}

// writeMovementResult renders one movement outcome and broadcasts on success.
func (h *OutHandler) writeMovementResult(w http.ResponseWriter, r *http.Request,
	sessionID string, actor *models.User, result outservice.RecordResult) {

	switch result.Outcome {
	case outservice.SessionUnavailable:
		http.Error(w, "This Out session is not open", http.StatusBadRequest)
		return
	case outservice.NotVerified:
		http.Error(w, "Your account is not approved yet", http.StatusForbidden)
		return
	case outservice.DirectionChanged:
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":     "direction_changed",
			"message":   "Your status changed since this screen loaded. Check it and try again.",
			"direction": result.Direction,
		})
		return
	case outservice.Duplicate:
		writeJSON(w, http.StatusOK, map[string]any{
			"outcome":   "duplicate",
			"message":   "Already recorded a moment ago — nothing changed.",
			"direction": result.Direction,
			"last":      result.Previous,
		})
		return
	}

	board, err := h.service.GetBoard(r.Context(), sessionID)
	if err == nil {
		h.broadcast(sessionID, sse.Event{
			Type: sse.EventTypeOutMovement,
			Payload: sse.OutMovementPayload{
				UserID:        result.Movement.UserID,
				Direction:     result.Direction,
				OccurredAt:    result.Movement.OccurredAt,
				MarkingMethod: result.Movement.Method,
				OutCount:      board.OutCount,
				ReturnedCount: board.ReturnedCount,
				OverdueCount:  board.OverdueCount,
			},
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"outcome":   "recorded",
		"direction": result.Direction,
		"movement":  result.Movement,
	})
}

func (h *OutHandler) broadcast(sessionID string, event sse.Event) {
	if h.hub != nil {
		h.hub.Broadcast(sessionID, event)
	}
}

func (h *OutHandler) writeSessionError(w http.ResponseWriter, err error) {
	if errors.Is(err, outservice.ErrSessionNotFound) {
		http.Error(w, "Out session not found", http.StatusNotFound)
		return
	}
	http.Error(w, "Failed to load out session", http.StatusInternalServerError)
}
