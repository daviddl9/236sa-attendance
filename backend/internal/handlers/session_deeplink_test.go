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

func TestSessionTelegramLinkIsCommanderOnlyAndDeepLinkNeverSerializes(t *testing.T) {
	t.Setenv("TELEGRAM_BOT_TOKEN", "synthetic-test-token")
	t.Setenv("TELEGRAM_WEBHOOK_SECRET", "synthetic-webhook-secret")
	t.Setenv("TELEGRAM_BOT_USERNAME", "synthetic_attendance_bot")
	t.Setenv("TELEGRAM_WEBHOOK_PATH", "synthetic-webhook-path")
	db, prefix := openRegistrationDB(t)
	creatorID := prefix + "-creator"
	seedUser(t, db, creatorID, "SESSION CREATOR", "3SG", "HQ", prefix+"-creator", true)

	handler := NewSessionHandler(db, nil)
	sessionID := createStandardSession(t, handler, creatorID, "same name")
	code := assertSessionDeepLinkCode(t, db, sessionID)
	wantLink := "https://t.me/synthetic_attendance_bot?start=" + code

	tests := []struct {
		name             string
		user             *models.User
		wantTelegramLink bool
		wantQRCode       bool
	}{
		{
			name:             "battery commander",
			user:             sessionTestUser(creatorID, models.Rank3SG),
			wantTelegramLink: true,
			wantQRCode:       true,
		},
		{
			name:             "enlisted",
			user:             sessionTestUser(prefix+"-enlisted", models.RankPTE),
			wantTelegramLink: false,
			wantQRCode:       false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/sessions/"+sessionID, nil)
			req = withSessionUser(req, tc.user, sessionID)
			rec := httptest.NewRecorder()
			handler.GetSession(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("get session status = %d: %s", rec.Code, rec.Body.String())
			}

			body := rec.Body.String()
			var response map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if _, ok := response["deeplinkCode"]; ok {
				t.Fatalf("response serialized the internal deep-link field: %s", body)
			}
			if tc.wantTelegramLink {
				if got, _ := response["telegramLink"].(string); got != wantLink {
					t.Fatalf("Telegram link = %q, want %q", got, wantLink)
				}
				if !strings.Contains(body, wantLink) {
					t.Fatalf("authorized response did not contain configured Telegram link: %s", body)
				}
			} else {
				if _, ok := response["telegramLink"]; ok {
					t.Fatalf("enlisted response exposed Telegram link: %s", body)
				}
				if strings.Contains(body, code) {
					t.Fatalf("enlisted response exposed raw deep-link code: %s", body)
				}
			}
			_, hasQRCode := response["qrCode"]
			if hasQRCode != tc.wantQRCode {
				t.Fatalf("qrCode visibility = %v, want %v: %s", hasQRCode, tc.wantQRCode, body)
			}
		})
	}
}

func sessionTestUser(id, rank string) *models.User {
	return &models.User{ID: id, Rank: &rank}
}

func withSessionUser(req *http.Request, user *models.User, sessionID string) *http.Request {
	rc := chi.NewRouteContext()
	rc.URLParams.Add("id", sessionID)
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rc)
	return req.WithContext(context.WithValue(ctx, middleware.UserKey, user))
}

func TestTelegramSessionLinkRequiresCompleteConfiguration(t *testing.T) {
	code := strings.Repeat("A", 22)
	cases := []struct {
		name     string
		secret   string
		username string
		path     string
		wantLink string
	}{
		{name: "missing secret", username: "synthetic_attendance_bot", path: "synthetic-webhook-path"},
		{name: "missing username", secret: "synthetic-webhook-secret", path: "synthetic-webhook-path"},
		{name: "missing path", secret: "synthetic-webhook-secret", username: "synthetic_attendance_bot"},
		{name: "fully configured", secret: "synthetic-webhook-secret", username: "synthetic_attendance_bot", path: "synthetic-webhook-path", wantLink: "https://t.me/synthetic_attendance_bot?start=" + code},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("TELEGRAM_BOT_TOKEN", "synthetic-test-token")
			t.Setenv("TELEGRAM_WEBHOOK_SECRET", tc.secret)
			t.Setenv("TELEGRAM_BOT_USERNAME", tc.username)
			t.Setenv("TELEGRAM_WEBHOOK_PATH", tc.path)
			if got := telegramSessionLink(code); got != tc.wantLink {
				t.Fatalf("Telegram link = %q, want %q", got, tc.wantLink)
			}
		})
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
