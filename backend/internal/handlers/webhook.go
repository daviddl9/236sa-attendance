package handlers

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/davidlivingston/go-nextjs-starter/backend/internal/database"
)

type WebhookHandler struct {
	db *database.DB
}

func NewWebhookHandler(db *database.DB) *WebhookHandler {
	return &WebhookHandler{db: db}
}

type PolarWebhookEvent struct {
	Type string                 `json:"type"`
	Data map[string]interface{} `json:"data"`
}

type PolarSubscription struct {
	ID                          string                 `json:"id"`
	CreatedAt                   time.Time              `json:"created_at"`
	ModifiedAt                  time.Time              `json:"modified_at"`
	Amount                      int                    `json:"amount"`
	Currency                    string                 `json:"currency"`
	RecurringInterval           string                 `json:"recurring_interval"`
	Status                      string                 `json:"status"`
	CurrentPeriodStart          time.Time              `json:"current_period_start"`
	CurrentPeriodEnd            time.Time              `json:"current_period_end"`
	CancelAtPeriodEnd           bool                   `json:"cancel_at_period_end"`
	CanceledAt                  *time.Time             `json:"canceled_at"`
	StartedAt                   time.Time              `json:"started_at"`
	EndsAt                      *time.Time             `json:"ends_at"`
	EndedAt                     *time.Time             `json:"ended_at"`
	CustomerID                  string                 `json:"customer_id"`
	ProductID                   string                 `json:"product_id"`
	DiscountID                  *string                `json:"discount_id"`
	CheckoutID                  string                 `json:"checkout_id"`
	CustomerCancellationReason  *string                `json:"customer_cancellation_reason"`
	CustomerCancellationComment *string                `json:"customer_cancellation_comment"`
	Metadata                    map[string]interface{} `json:"metadata"`
	CustomFieldData             map[string]interface{} `json:"custom_field_data"`
}

func (h *WebhookHandler) PolarWebhook(w http.ResponseWriter, r *http.Request) {
	// Read the raw body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer func() { _ = r.Body.Close() }()

	// Verify webhook signature
	webhookSecret := os.Getenv("POLAR_WEBHOOK_SECRET")
	if webhookSecret != "" {
		signature := r.Header.Get("X-Polar-Signature")
		if !verifyPolarSignature(body, signature, webhookSecret) {
			http.Error(w, "Invalid signature", http.StatusUnauthorized)
			return
		}
	}

	// Parse the webhook event
	var event PolarWebhookEvent
	if err := json.Unmarshal(body, &event); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Handle different event types
	switch event.Type {
	case "subscription.created", "subscription.updated":
		_ = h.handleSubscriptionEvent(event.Data)
	case "subscription.canceled":
		_ = h.handleSubscriptionCanceled(event.Data)
	case "subscription.ended":
		_ = h.handleSubscriptionEnded(event.Data)
	default:
		// Unknown event type - just acknowledge it
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func (h *WebhookHandler) handleSubscriptionEvent(data map[string]interface{}) error {
	// Marshal and unmarshal to convert map to struct
	jsonData, _ := json.Marshal(data)
	var sub PolarSubscription
	if err := json.Unmarshal(jsonData, &sub); err != nil {
		return err
	}

	ctx := context.Background()

	// Convert metadata and custom field data to JSON strings
	var metadataStr, customFieldStr *string
	if sub.Metadata != nil {
		if jsonMeta, err := json.Marshal(sub.Metadata); err == nil {
			str := string(jsonMeta)
			metadataStr = &str
		}
	}
	if sub.CustomFieldData != nil {
		if jsonCustom, err := json.Marshal(sub.CustomFieldData); err == nil {
			str := string(jsonCustom)
			customFieldStr = &str
		}
	}

	// Find user by customer ID (assuming metadata contains user_id)
	var userID *string
	if sub.Metadata != nil {
		if uid, ok := sub.Metadata["user_id"].(string); ok {
			userID = &uid
		}
	}

	// Upsert subscription
	_, err := h.db.Pool.Exec(ctx, `
		INSERT INTO subscription (
			id, "createdAt", "modifiedAt", amount, currency, "recurringInterval",
			status, "currentPeriodStart", "currentPeriodEnd", "cancelAtPeriodEnd",
			"canceledAt", "startedAt", "endsAt", "endedAt", "customerId",
			"productId", "discountId", "checkoutId", "customerCancellationReason",
			"customerCancellationComment", metadata, "customFieldData", "userId"
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
			$11, $12, $13, $14, $15, $16, $17, $18, $19,
			$20, $21, $22, $23
		)
		ON CONFLICT (id) DO UPDATE SET
			"modifiedAt" = EXCLUDED."modifiedAt",
			amount = EXCLUDED.amount,
			status = EXCLUDED.status,
			"currentPeriodStart" = EXCLUDED."currentPeriodStart",
			"currentPeriodEnd" = EXCLUDED."currentPeriodEnd",
			"cancelAtPeriodEnd" = EXCLUDED."cancelAtPeriodEnd",
			"canceledAt" = EXCLUDED."canceledAt",
			"endsAt" = EXCLUDED."endsAt",
			"endedAt" = EXCLUDED."endedAt",
			"customerCancellationReason" = EXCLUDED."customerCancellationReason",
			"customerCancellationComment" = EXCLUDED."customerCancellationComment",
			metadata = EXCLUDED.metadata,
			"customFieldData" = EXCLUDED."customFieldData"
	`, sub.ID, sub.CreatedAt, sub.ModifiedAt, sub.Amount, sub.Currency,
		sub.RecurringInterval, sub.Status, sub.CurrentPeriodStart, sub.CurrentPeriodEnd,
		sub.CancelAtPeriodEnd, sub.CanceledAt, sub.StartedAt, sub.EndsAt, sub.EndedAt,
		sub.CustomerID, sub.ProductID, sub.DiscountID, sub.CheckoutID,
		sub.CustomerCancellationReason, sub.CustomerCancellationComment,
		metadataStr, customFieldStr, userID)

	return err
}

func (h *WebhookHandler) handleSubscriptionCanceled(data map[string]interface{}) error {
	subscriptionID, ok := data["id"].(string)
	if !ok {
		return fmt.Errorf("subscription ID not found")
	}

	ctx := context.Background()
	now := time.Now()

	_, err := h.db.Pool.Exec(ctx, `
		UPDATE subscription
		SET "canceledAt" = $1, "cancelAtPeriodEnd" = true, "modifiedAt" = $1
		WHERE id = $2
	`, now, subscriptionID)

	return err
}

func (h *WebhookHandler) handleSubscriptionEnded(data map[string]interface{}) error {
	subscriptionID, ok := data["id"].(string)
	if !ok {
		return fmt.Errorf("subscription ID not found")
	}

	ctx := context.Background()
	now := time.Now()

	_, err := h.db.Pool.Exec(ctx, `
		UPDATE subscription
		SET "endedAt" = $1, status = 'ended', "modifiedAt" = $1
		WHERE id = $2
	`, now, subscriptionID)

	return err
}

func verifyPolarSignature(payload []byte, signature, secret string) bool {
	if signature == "" {
		return false
	}

	// Compute HMAC
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	expectedMAC := hex.EncodeToString(mac.Sum(nil))

	// Compare signatures
	return hmac.Equal([]byte(signature), []byte(expectedMAC))
}
