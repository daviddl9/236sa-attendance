package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/davidlivingston/go-nextjs-starter/backend/internal/middleware"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/models"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/telegram"
	"github.com/go-chi/chi/v5"
)

func TestTelegramPairingReviewRequiresSuperadminAndLeaksNoName(t *testing.T) {
	db, prefix, telegramID := openTelegramPairingDB(t)
	seedUser(t, db, prefix+"-target", "PRIVATE ROSTER NAME", "PTE", "Alpha", "", true)
	h := NewAdminHandler(db)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/telegram/pairings", nil)
	rec := httptest.NewRecorder()
	h.ListTelegramPairings(rec, req)
	if rec.Code != http.StatusUnauthorized || strings.Contains(rec.Body.String(), "PRIVATE ROSTER NAME") {
		t.Fatalf("unauthenticated review = %d %q", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/admin/telegram/pairings", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserKey, &models.User{ID: "soldier", IsSuperadmin: false}))
	rec = httptest.NewRecorder()
	h.ListTelegramPairings(rec, req)
	if rec.Code != http.StatusForbidden || strings.Contains(rec.Body.String(), "PRIVATE ROSTER NAME") {
		t.Fatalf("soldier review = %d %q", rec.Code, rec.Body.String())
	}
	_ = telegramID
}

func TestTelegramPairingReviewPutsConflictsBeforeRoutineAndUnpairAllowsRepair(t *testing.T) {
	db, prefix, telegramID := openTelegramPairingDB(t)
	targetID := prefix + "-target"
	seedUser(t, db, targetID, "PRIVATE ROSTER NAME", "PTE", "Alpha", "", true)
	secondID := prefix + "-second"
	seedUser(t, db, secondID, "SECOND ROSTER NAME", "PTE", "Alpha", "", true)
	store := &TelegramPairingStore{db: db}
	first, err := store.ProposePairing(context.Background(), telegramID, "PRIVATE ROSTER NAME")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ConfirmPairing(context.Background(), telegramID, first.AttemptID); err != nil {
		t.Fatal(err)
	}
	conflictID := telegramID + 1
	conflict, err := store.ProposePairing(context.Background(), conflictID, "PRIVATE ROSTER NAME")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ConfirmPairing(context.Background(), conflictID, conflict.AttemptID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ProposePairing(context.Background(), telegramID+2, "unknown person"); err != nil {
		t.Fatal(err)
	}

	h := NewAdminHandler(db)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/telegram/pairings", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserKey, &models.User{ID: "admin", IsSuperadmin: true}))
	rec := httptest.NewRecorder()
	h.ListTelegramPairings(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("review status = %d: %s", rec.Code, rec.Body.String())
	}
	var response TelegramPairingReviewResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if len(response.Attempts) < 2 || response.Attempts[0].Outcome != "refused_conflict" {
		t.Fatalf("attempt order = %+v", response.Attempts)
	}
	if len(response.Pairings) != 1 || !response.Pairings[0].SelfConfirmed || response.Pairings[0].FullName != "PRIVATE ROSTER NAME" {
		t.Fatalf("pairings = %+v", response.Pairings)
	}

	// Unpair through the same superadmin boundary used by the API.
	req = httptest.NewRequest(http.MethodDelete, "/api/admin/telegram/pairings/"+itoa(telegramID), nil)
	req = withTelegramRoute(req, telegramID)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserKey, &models.User{ID: "admin", IsSuperadmin: true}))
	rec = httptest.NewRecorder()
	h.UnpairTelegramAccount(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("unpair status = %d: %s", rec.Code, rec.Body.String())
	}

	repaired, err := store.ProposePairing(context.Background(), telegramID, "SECOND ROSTER NAME")
	if err != nil {
		t.Fatal(err)
	}
	result, err := store.ConfirmPairing(context.Background(), telegramID, repaired.AttemptID)
	if err != nil || result.Outcome != telegram.PairingConfirmed || result.Pairing.UserID != secondID {
		t.Fatalf("repair = %+v, err=%v", result, err)
	}
}

func TestConfirmTelegramPairingRequiresSuperadmin(t *testing.T) {
	db, _, telegramID := openTelegramPairingDB(t)
	h := NewAdminHandler(db)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/telegram/pairings/"+itoa(telegramID)+"/confirm", strings.NewReader(`{"userId":"target"}`))
	req = withTelegramRoute(req, telegramID)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserKey, &models.User{ID: "soldier", IsSuperadmin: false}))
	rec := httptest.NewRecorder()
	h.ConfirmTelegramPairing(rec, req)
	if rec.Code != http.StatusForbidden || strings.Contains(rec.Body.String(), "target") {
		t.Fatalf("non-admin confirm = %d %q", rec.Code, rec.Body.String())
	}
}

func withTelegramRoute(req *http.Request, telegramID int64) *http.Request {
	rc := chi.NewRouteContext()
	rc.URLParams.Add("telegramID", itoa(telegramID))
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rc))
}

func itoa(value int64) string {
	return fmt.Sprintf("%d", value)
}
