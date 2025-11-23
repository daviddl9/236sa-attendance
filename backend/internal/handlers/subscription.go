package handlers

import (
	"net/http"

	"github.com/davidlivingston/go-nextjs-starter/backend/internal/database"
)

type SubscriptionHandler struct {
	db *database.DB
}

func NewSubscriptionHandler(db *database.DB) *SubscriptionHandler {
	return &SubscriptionHandler{db: db}
}

func (h *SubscriptionHandler) GetUserSubscription(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not implemented", http.StatusNotImplemented)
}
