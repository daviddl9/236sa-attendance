package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/davidlivingston/go-nextjs-starter/backend/internal/models"
)

func TestRequirePasswordChangeBlocksProtectedRequests(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler := RequirePasswordChange(next)
	user := &models.User{ID: "temporary-user", PasswordChangeRequired: true}

	req := httptest.NewRequest(http.MethodPost, "/api/attendance/mark", nil)
	req = req.WithContext(context.WithValue(req.Context(), UserKey, user))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("protected request status = %d, want 403", rec.Code)
	}

	change := httptest.NewRequest(http.MethodPost, "/api/auth/change-password", nil)
	change = change.WithContext(context.WithValue(change.Context(), UserKey, user))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, change)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("change-password status = %d, want 204", rec.Code)
	}
}
