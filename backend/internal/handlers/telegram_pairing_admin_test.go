package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/davidlivingston/go-nextjs-starter/backend/internal/database"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/middleware"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/models"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/telegram"
	"github.com/go-chi/chi/v5"
)

func TestTelegramPairingReviewRequiresUnitCommanderAndLeaksNoName(t *testing.T) {
	db, prefix, telegramID := openTelegramPairingDB(t)
	seedUser(t, db, prefix+"-target", "PRIVATE ROSTER NAME", "PTE", "Alpha", "", true)
	h := NewAdminHandler(db)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/telegram/pairings", nil)
	rec := httptest.NewRecorder()
	h.ListTelegramPairings(rec, req)
	if rec.Code != http.StatusUnauthorized || strings.Contains(rec.Body.String(), "PRIVATE ROSTER NAME") {
		t.Fatalf("unauthenticated review = %d %q", rec.Code, rec.Body.String())
	}

	for _, tc := range []struct {
		name  string
		actor *models.User
	}{
		{name: "enlisted", actor: &models.User{ID: "soldier", Rank: stringPtr("PTE")}},
		{name: "battery NCO", actor: &models.User{ID: "battery-nco", Rank: stringPtr("3SG")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/admin/telegram/pairings", nil)
			req = req.WithContext(context.WithValue(req.Context(), middleware.UserKey, tc.actor))
			rec := httptest.NewRecorder()
			h.ListTelegramPairings(rec, req)
			if rec.Code != http.StatusForbidden || strings.Contains(rec.Body.String(), "PRIVATE ROSTER NAME") {
				t.Fatalf("%s review = %d %q", tc.name, rec.Code, rec.Body.String())
			}
		})
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
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserKey, &models.User{ID: "commander", Rank: stringPtr("SSG")}))
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

	// Unpair through the same unit-commander boundary used by the API.
	req = httptest.NewRequest(http.MethodDelete, "/api/admin/telegram/pairings/"+itoa(telegramID), nil)
	req = withTelegramRoute(req, telegramID)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserKey, &models.User{ID: "commander", Rank: stringPtr("SSG")}))
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

func TestTelegramPairingUnpairInvalidatesPendingProposal(t *testing.T) {
	db, prefix, telegramID := openTelegramPairingDB(t)
	targetID := prefix + "-target"
	seedUser(t, db, targetID, "PRIVATE ROSTER NAME", "PTE", "Alpha", "", true)
	store := &TelegramPairingStore{db: db}
	first, err := store.ProposePairing(context.Background(), telegramID, "PRIVATE ROSTER NAME")
	if err != nil {
		t.Fatal(err)
	}
	admin := NewAdminHandler(db)
	req := httptest.NewRequest(http.MethodDelete, "/api/admin/telegram/pairings/"+itoa(telegramID), nil)
	req = withTelegramRoute(req, telegramID)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserKey, &models.User{ID: "admin", IsSuperadmin: true}))
	rec := httptest.NewRecorder()
	// Seed the pairing that the stale proposal was made against.
	if _, err := db.Pool.Exec(context.Background(), `
		INSERT INTO telegram_pairing (id, telegram_id, user_id, self_confirmed) VALUES ($1, $2, $3, true)
	`, prefix+"-pairing", telegramID+1, targetID); err != nil {
		t.Fatal(err)
	}
	admin.UnpairTelegramAccount(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unpair status = %d: %s", rec.Code, rec.Body.String())
	}
	req = httptest.NewRequest(http.MethodDelete, "/api/admin/telegram/pairings/"+itoa(telegramID+1), nil)
	req = withTelegramRoute(req, telegramID+1)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserKey, &models.User{ID: "admin", IsSuperadmin: true}))
	rec = httptest.NewRecorder()
	admin.UnpairTelegramAccount(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("target unpair status = %d: %s", rec.Code, rec.Body.String())
	}
	confirmation, err := store.ConfirmPairing(context.Background(), telegramID, first.AttemptID)
	if err != nil || confirmation.Outcome != telegram.PairingStale {
		t.Fatalf("invalidated confirmation = %+v, err=%v", confirmation, err)
	}
}

func TestTelegramPairingAdminResolutionTargetsRequestAttempt(t *testing.T) {
	db, prefix, telegramID := openTelegramPairingDB(t)
	actorID := prefix + "-commander"
	seedUser(t, db, actorID, "TEST COMMANDER", "SSG", "HQ", "", true)
	seedUser(t, db, prefix+"-target", "PRIVATE ROSTER NAME", "PTE", "Alpha", "", true)
	store := &TelegramPairingStore{db: db}
	if _, err := store.ProposePairing(context.Background(), telegramID, "weak request"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ProposePairing(context.Background(), telegramID, "another weak request"); err != nil {
		t.Fatal(err)
	}
	var requestAttempt string
	if err := db.Pool.QueryRow(context.Background(), `SELECT attempt_id FROM telegram_pairing_request WHERE telegram_id = $1`, telegramID).Scan(&requestAttempt); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool.Exec(context.Background(), `
		INSERT INTO telegram_pairing_attempt (id, telegram_id, outcome) VALUES ($1, $2, 'no_match')
	`, prefix+"-later-refusal", telegramID); err != nil {
		t.Fatal(err)
	}
	admin := NewAdminHandler(db)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/telegram/pairings/"+itoa(telegramID)+"/confirm", strings.NewReader(`{"userId":"`+prefix+`-target"}`))
	req = withTelegramRoute(req, telegramID)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserKey, &models.User{ID: actorID, Rank: stringPtr("SSG")}))
	rec := httptest.NewRecorder()
	admin.ConfirmTelegramPairing(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin resolution status = %d: %s", rec.Code, rec.Body.String())
	}
	var intended, later string
	if err := db.Pool.QueryRow(context.Background(), `SELECT outcome FROM telegram_pairing_attempt WHERE id = $1`, requestAttempt).Scan(&intended); err != nil {
		t.Fatal(err)
	}
	if err := db.Pool.QueryRow(context.Background(), `SELECT outcome FROM telegram_pairing_attempt WHERE id = $1`, prefix+"-later-refusal").Scan(&later); err != nil {
		t.Fatal(err)
	}
	if intended != "confirmed" || later != "no_match" {
		t.Fatalf("attempt outcomes = intended %q, later %q", intended, later)
	}
}

func TestTelegramPairingConfirmAndUnpairDoNotDeadlock(t *testing.T) {
	db, prefix, telegramID := openTelegramPairingDB(t)
	targetID := prefix + "-target"
	seedUser(t, db, targetID, "PAIRING LOCK TARGET", "PTE", "Alpha", "", true)
	if _, err := db.Pool.Exec(context.Background(), `
		INSERT INTO telegram_pairing (id, telegram_id, user_id, self_confirmed)
		VALUES ($1, $2, $3, true)
	`, prefix+"-pairing", telegramID, targetID); err != nil {
		t.Fatal(err)
	}
	store := &TelegramPairingStore{db: db}
	proposal, err := store.ProposePairing(context.Background(), telegramID, "PAIRING LOCK TARGET")
	if err != nil {
		t.Fatal(err)
	}

	// Hold the roster lock while admin unpair reaches it first. With the old
	// order, confirmation then held the attempt row and waited behind unpair;
	// releasing the roster lock formed the inverse-lock cycle. With the shared
	// order, confirmation waits on the Telegram-account lock instead and never
	// waits for the pairing row while unpair owns that account lock.
	blocker, err := db.Pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = blocker.Rollback(context.Background()) }()
	if _, err := blocker.Exec(context.Background(), `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, targetID); err != nil {
		t.Fatal(err)
	}

	admin := NewAdminHandler(db)
	unpairDone := make(chan int, 1)
	go func() {
		req := httptest.NewRequest(http.MethodDelete, "/api/admin/telegram/pairings/"+itoa(telegramID), nil)
		req = withTelegramRoute(req, telegramID)
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserKey, &models.User{ID: prefix + "-admin", IsSuperadmin: true}))
		rec := httptest.NewRecorder()
		admin.UnpairTelegramAccount(rec, req)
		unpairDone <- rec.Code
	}()
	if !waitForAdvisoryWaiters(db, telegramID, targetID, 1, 0, 2*time.Second) {
		t.Fatal("admin unpair did not reach the roster advisory lock")
	}

	confirmDone := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, err := store.ConfirmPairing(ctx, telegramID, proposal.AttemptID)
		confirmDone <- err
	}()
	// Old code makes unpair the first waiter on the target lock and
	// confirmation the second, forming the inverse-lock cycle after the
	// blocker is released. The fixed code makes confirmation wait on the
	// account lock instead, so accept either observed state.
	if !waitForAdvisoryContention(db, telegramID, targetID, 2*time.Second) {
		t.Fatal("self-confirmation and unpair did not reach the shared lock ordering")
	}
	if err := blocker.Rollback(context.Background()); err != nil {
		t.Fatal(err)
	}

	select {
	case err := <-confirmDone:
		if err != nil {
			t.Fatalf("confirmation returned an error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("confirmation timed out")
	}
	select {
	case status := <-unpairDone:
		if status != http.StatusOK {
			t.Fatalf("unpair status = %d, want %d", status, http.StatusOK)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("unpair timed out")
	}
}

func waitForAdvisoryWaiters(db *database.DB, telegramID int64, targetID string, targetWant, accountWant int, timeout time.Duration) bool {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var targetCount, accountCount int
		err := db.Pool.QueryRow(ctx, `
			SELECT
				COUNT(*) FILTER (
					WHERE classid = (((hashtextextended($2, 0) >> 32) & 4294967295)::oid)
					  AND objid = ((hashtextextended($2, 0) & 4294967295)::oid)
				),
				COUNT(*) FILTER (
					WHERE classid = (((($1::bigint >> 32) & 4294967295))::oid)
					  AND objid = ((($1::bigint & 4294967295))::oid)
				)
			FROM pg_locks
			WHERE locktype = 'advisory' AND NOT granted
		`, telegramID, targetID).Scan(&targetCount, &accountCount)
		if err == nil && targetCount >= targetWant && accountCount >= accountWant {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

func waitForAdvisoryContention(db *database.DB, telegramID int64, targetID string, timeout time.Duration) bool {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var targetCount, accountCount int
		err := db.Pool.QueryRow(ctx, `
			SELECT
				COUNT(*) FILTER (
					WHERE classid = (((hashtextextended($2, 0) >> 32) & 4294967295)::oid)
					  AND objid = ((hashtextextended($2, 0) & 4294967295)::oid)
				),
				COUNT(*) FILTER (
					WHERE classid = (((($1::bigint >> 32) & 4294967295))::oid)
					  AND objid = ((($1::bigint & 4294967295))::oid)
				)
			FROM pg_locks
			WHERE locktype = 'advisory' AND NOT granted
		`, telegramID, targetID).Scan(&targetCount, &accountCount)
		// Old code waits twice on the roster lock; shared ordering waits once
		// there and once on the account lock.
		if err == nil && (targetCount >= 2 || (targetCount >= 1 && accountCount >= 1)) {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

func TestTelegramPairingActionsRequireUnitCommander(t *testing.T) {
	const telegramID int64 = 970000000999
	h := NewAdminHandler(nil)
	actors := []struct {
		name   string
		actor  *models.User
		status int
	}{
		{name: "unauthenticated", status: http.StatusUnauthorized},
		{name: "enlisted", actor: &models.User{ID: "soldier", Rank: stringPtr("PTE")}, status: http.StatusForbidden},
		{name: "battery NCO", actor: &models.User{ID: "battery-nco", Rank: stringPtr("3SG")}, status: http.StatusForbidden},
	}
	actions := []struct {
		name   string
		method string
		path   string
		body   string
		route  bool
		call   func(http.ResponseWriter, *http.Request)
	}{
		{name: "list", method: http.MethodGet, path: "/api/admin/telegram/pairings", call: h.ListTelegramPairings},
		{name: "confirm", method: http.MethodPost, path: "/api/admin/telegram/pairings/" + itoa(telegramID) + "/confirm", body: `{"userId":"target"}`, route: true, call: h.ConfirmTelegramPairing},
		{name: "unpair", method: http.MethodDelete, path: "/api/admin/telegram/pairings/" + itoa(telegramID), route: true, call: h.UnpairTelegramAccount},
	}
	for _, action := range actions {
		for _, tc := range actors {
			t.Run(action.name+"/"+tc.name, func(t *testing.T) {
				req := httptest.NewRequest(action.method, action.path, strings.NewReader(action.body))
				if action.route {
					req = withTelegramRoute(req, telegramID)
				}
				if tc.actor != nil {
					req = req.WithContext(context.WithValue(req.Context(), middleware.UserKey, tc.actor))
				}
				rec := httptest.NewRecorder()
				action.call(rec, req)
				if rec.Code != tc.status || strings.Contains(rec.Body.String(), "target") {
					t.Fatalf("%s = %d %q, want %d without target disclosure", action.name, rec.Code, rec.Body.String(), tc.status)
				}
			})
		}
	}
}

func TestTelegramPairingSuperadminStillCanReviewConfirmAndUnpair(t *testing.T) {
	db, prefix, telegramID := openTelegramPairingDB(t)
	actorID := prefix + "-superadmin"
	targetID := prefix + "-target"
	seedUser(t, db, actorID, "TEST SUPERADMIN", "CPT", "HQ", "", true)
	seedUser(t, db, targetID, "PRIVATE ROSTER NAME", "PTE", "Alpha", "", true)
	store := &TelegramPairingStore{db: db}
	proposal, err := store.ProposePairing(context.Background(), telegramID, "unmatched request")
	if err != nil || !proposal.NoMatch || proposal.AttemptID == "" {
		t.Fatalf("proposal = %+v, err=%v", proposal, err)
	}
	actor := &models.User{ID: actorID, IsSuperadmin: true}
	h := NewAdminHandler(db)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/telegram/pairings", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserKey, actor))
	rec := httptest.NewRecorder()
	h.ListTelegramPairings(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("superadmin review = %d: %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/admin/telegram/pairings/"+itoa(telegramID)+"/confirm", strings.NewReader(`{"userId":"`+targetID+`"}`))
	req = withTelegramRoute(req, telegramID)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserKey, actor))
	rec = httptest.NewRecorder()
	h.ConfirmTelegramPairing(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("superadmin confirm = %d: %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodDelete, "/api/admin/telegram/pairings/"+itoa(telegramID), nil)
	req = withTelegramRoute(req, telegramID)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserKey, actor))
	rec = httptest.NewRecorder()
	h.UnpairTelegramAccount(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("superadmin unpair = %d: %s", rec.Code, rec.Body.String())
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
