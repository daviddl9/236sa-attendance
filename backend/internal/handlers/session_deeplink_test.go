package handlers

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/davidlivingston/go-nextjs-starter/backend/internal/database"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/middleware"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/models"
	"github.com/go-chi/chi/v5"
)

func TestCreateSessionGeneratesUniqueDeepLinkCodes(t *testing.T) {
	db, prefix := openRegistrationDB(t)
	creatorID := prefix + "-creator"
	seedUser(t, db, creatorID, "SESSION CREATOR", "3SG", "HQ", prefix+"-creator", true)

	handler := NewSessionHandler(db, nil)
	firstID := createStandardSession(t, handler, creatorID, "same name")
	secondID := createStandardSession(t, handler, creatorID, "same name")

	firstCode := assertSessionDeepLinkCode(t, db, firstID)
	secondCode := assertSessionDeepLinkCode(t, db, secondID)
	if firstCode == secondCode {
		t.Fatalf("two sessions created in the same request cycle received the same code: %q", firstCode)
	}
}

func TestCreateSessionExposesTelegramDeepLink(t *testing.T) {
	t.Setenv("TELEGRAM_BOT_USERNAME", "synthetic_attendance_bot")
	db, prefix := openRegistrationDB(t)
	creatorID := prefix + "-creator"
	seedUser(t, db, creatorID, "SESSION CREATOR", "3SG", "HQ", prefix+"-creator", true)

	handler := NewSessionHandler(db, nil)
	sessionID := createStandardSession(t, handler, creatorID, "same name")
	code := assertSessionDeepLinkCode(t, db, sessionID)

	// The response itself is checked by createStandardSession's helper in the
	// companion assertion below, while the persisted value remains the source
	// of truth for the URL.
	var response SessionResponse
	req := httptest.NewRequest(http.MethodGet, "/api/sessions/"+sessionID, nil)
	rc := chi.NewRouteContext()
	rc.URLParams.Add("id", sessionID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rc))
	rec := httptest.NewRecorder()
	handler.GetSession(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get session status = %d: %s", rec.Code, rec.Body.String())
	}
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.DeepLinkCode != code {
		t.Fatalf("response deep-link code = %q, want %q", response.DeepLinkCode, code)
	}
	wantLink := "https://t.me/synthetic_attendance_bot?start=" + code
	if response.TelegramLink != wantLink {
		t.Fatalf("response Telegram link = %q, want %q", response.TelegramLink, wantLink)
	}
}

func TestCreateCustomSessionGeneratesDeepLinkCode(t *testing.T) {
	db, prefix := openRegistrationDB(t)
	creatorID := prefix + "-creator"
	participantID := prefix + "-participant"
	seedUser(t, db, creatorID, "SESSION CREATOR", "3SG", "HQ", prefix+"-creator", true)
	seedUser(t, db, participantID, "SESSION PARTICIPANT", "PTE", "Alpha", prefix+"-participant", true)

	handler := NewSessionHandler(db, nil)
	body := fmt.Sprintf(`{"name":"same name","participantIds":[%q]}`, participantID)
	sessionID := createCustomSession(t, handler, creatorID, body)
	assertSessionDeepLinkCode(t, db, sessionID)
}

func createStandardSession(t *testing.T, handler *SessionHandler, creatorID, name string) string {
	t.Helper()
	body := fmt.Sprintf(`{"name":%q,"scope":"unit_wide"}`, name)
	req := httptest.NewRequest(http.MethodPost, "/api/sessions", strings.NewReader(body))
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserKey, &models.User{ID: creatorID}))
	rec := httptest.NewRecorder()
	handler.CreateSession(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create session status = %d, want %d: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var response SessionResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode created session: %v", err)
	}
	return response.ID
}

func createCustomSession(t *testing.T, handler *SessionHandler, creatorID, body string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/sessions/custom/create", strings.NewReader(body))
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserKey, &models.User{ID: creatorID}))
	rec := httptest.NewRecorder()
	handler.CreateCustomSession(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create custom session status = %d, want %d: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var response models.AttendanceSession
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode created custom session: %v", err)
	}
	return response.ID
}

func assertSessionDeepLinkCode(t *testing.T, db *database.DB, sessionID string) string {
	t.Helper()
	var code sql.NullString
	if err := db.Pool.QueryRow(context.Background(), `
		SELECT deeplink_code FROM attendance_session WHERE id = $1
	`, sessionID).Scan(&code); err != nil {
		t.Fatalf("read deep-link code: %v", err)
	}
	if !code.Valid {
		t.Fatalf("session %s has a null deep-link code immediately after creation", sessionID)
	}
	if len(code.String) != 22 {
		t.Fatalf("deep-link code length = %d, want 22 (%q)", len(code.String), code.String)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(code.String)
	if err != nil {
		t.Fatalf("deep-link code %q is not base64url: %v", code.String, err)
	}
	if len(decoded) != 16 {
		t.Fatalf("decoded deep-link code length = %d, want 16", len(decoded))
	}
	return code.String
}
