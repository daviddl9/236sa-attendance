package sse

import (
	"sync"
)

// Client represents a connected SSE client
type Client struct {
	ID     string
	UserID string
	Send   chan Event
	Done   chan struct{}
}

// NewClient creates a new SSE client
func NewClient(id, userID string) *Client {
	return &Client{
		ID:     id,
		UserID: userID,
		Send:   make(chan Event, 10), // Buffered channel to avoid blocking
		Done:   make(chan struct{}),
	}
}

// Hub manages SSE client connections per session
type Hub struct {
	mu       sync.RWMutex
	sessions map[string]map[*Client]struct{} // sessionID -> set of clients
}

// NewHub creates a new SSE hub
func NewHub() *Hub {
	return &Hub{
		sessions: make(map[string]map[*Client]struct{}),
	}
}

// Subscribe adds a client to a session's subscribers
func (h *Hub) Subscribe(sessionID string, client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.sessions[sessionID] == nil {
		h.sessions[sessionID] = make(map[*Client]struct{})
	}
	h.sessions[sessionID][client] = struct{}{}
}

// Unsubscribe removes a client from a session's subscribers
func (h *Hub) Unsubscribe(sessionID string, client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if clients, ok := h.sessions[sessionID]; ok {
		delete(clients, client)
		if len(clients) == 0 {
			delete(h.sessions, sessionID)
		}
	}
	close(client.Send)
}

// Broadcast sends an event to all clients subscribed to a session
func (h *Hub) Broadcast(sessionID string, event Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	clients, ok := h.sessions[sessionID]
	if !ok {
		return
	}

	for client := range clients {
		select {
		case client.Send <- event:
			// Event sent successfully
		default:
			// Client buffer full, skip this event
		}
	}
}

// ClientCount returns the number of clients subscribed to a session
func (h *Hub) ClientCount(sessionID string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if clients, ok := h.sessions[sessionID]; ok {
		return len(clients)
	}
	return 0
}

// BroadcastToAll sends an event to all clients across all sessions
func (h *Hub) BroadcastToAll(event Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, clients := range h.sessions {
		for client := range clients {
			select {
			case client.Send <- event:
				// Event sent successfully
			default:
				// Client buffer full, skip this event
			}
		}
	}
}
