package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/davidlivingston/go-nextjs-starter/backend/internal/database"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/middleware"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/models"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/sse"
	"github.com/go-chi/chi/v5"
)

// SSEHandler handles Server-Sent Events for live attendance updates
type SSEHandler struct {
	db  *database.DB
	hub *sse.Hub
}

// NewSSEHandler creates a new SSE handler
func NewSSEHandler(db *database.DB, hub *sse.Hub) *SSEHandler {
	return &SSEHandler{db: db, hub: hub}
}

// StreamSession handles GET /api/sessions/{id}/stream
// Clients connect to this endpoint to receive real-time attendance updates
func (h *SSEHandler) StreamSession(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "id")

	user, ok := middleware.GetUserFromContext(r.Context())
	if !ok {
		http.Error(w, "Not authenticated", http.StatusUnauthorized)
		return
	}

	// Verify session exists and is active
	ctx := context.Background()
	var status string
	err := h.db.Pool.QueryRow(ctx, `
		SELECT status FROM attendance_session WHERE id = $1
	`, sessionID).Scan(&status)

	if err != nil {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}

	if status != models.SessionStatusActive {
		http.Error(w, "Session is not active", http.StatusBadRequest)
		return
	}

	// Check if response writer supports flushing
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // Disable nginx buffering

	// Create client and subscribe to session
	clientID := generateID()
	client := sse.NewClient(clientID, user.ID)
	h.hub.Subscribe(sessionID, client)

	log.Printf("[SSE] Client %s connected to session %s (user: %s)", clientID, sessionID, user.ID)

	// Ensure cleanup on disconnect
	defer func() {
		h.hub.Unsubscribe(sessionID, client)
		log.Printf("[SSE] Client %s disconnected from session %s", clientID, sessionID)
	}()

	// Send initial connection confirmation
	sendSSEEvent(w, flusher, sse.Event{
		Type:    "connected",
		Payload: map[string]string{"clientId": clientID},
	})

	// Heartbeat ticker (every 30 seconds)
	heartbeat := time.NewTicker(30 * time.Second)
	defer heartbeat.Stop()

	// Main event loop
	for {
		select {
		case <-r.Context().Done():
			// Client disconnected
			return
		case event, ok := <-client.Send:
			if !ok {
				// Channel closed, client unsubscribed
				return
			}
			sendSSEEvent(w, flusher, event)
		case <-heartbeat.C:
			sendSSEEvent(w, flusher, sse.Event{
				Type:    sse.EventTypeHeartbeat,
				Payload: map[string]int64{"timestamp": time.Now().Unix()},
			})
		}
	}
}

// sendSSEEvent writes an event to the SSE stream
func sendSSEEvent(w http.ResponseWriter, flusher http.Flusher, event sse.Event) {
	data, err := json.Marshal(event)
	if err != nil {
		log.Printf("[SSE] Failed to marshal event: %v", err)
		return
	}

	if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
		log.Printf("[SSE] Failed to write event: %v", err)
		return
	}
	flusher.Flush()
}
