package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/davidlivingston/go-nextjs-starter/backend/internal/database"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/middleware"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/models"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/services/deeplink"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/telegram"
)

type captureTelegramSink struct {
	actions []telegram.Action
}

func (s *captureTelegramSink) Enqueue(actions []telegram.Action) bool {
	s.actions = append(s.actions, actions...)
	return true
}

func TestTelegramWebhookMarksPairedSoldierExactlyOnce(t *testing.T) {
	db, prefix := openTelegramAttendanceDB(t)
	soldierID := prefix + "-soldier"
	seedUser(t, db, soldierID, "PTE SYNTHETIC SCANNER", "PTE", "Alpha", prefix+"-soldier", true)
	code := seedTelegramSession(t, db, prefix+"-active", "Synthetic active parade", "unit_wide", nil, "active", soldierID)
	seedTelegramPairing(t, db, prefix+"-pairing", 970000000001, soldierID)

	handler, sink := newTelegramAttendanceHandler(db)
	first := postTelegramMessage(t, handler, 970000000001, "/start "+code)
	if first.Code != http.StatusOK {
		t.Fatalf("first webhook status = %d, want 200", first.Code)
	}
	if len(sink.actions) != 1 || sink.actions[0].Text != "Attendance marked for Synthetic active parade." {
		t.Fatalf("first reply = %#v", sink.actions)
	}
	assertTelegramRecord(t, db, prefix+"-active", soldierID, 1, models.MarkingMethodTelegramScan)

	sink.actions = nil
	second := postTelegramMessage(t, handler, 970000000001, "/start "+code)
	if second.Code != http.StatusOK {
		t.Fatalf("repeated webhook status = %d, want 200", second.Code)
	}
	if len(sink.actions) != 1 || sink.actions[0].Text != "You are already marked for Synthetic active parade." {
		t.Fatalf("repeated reply = %#v", sink.actions)
	}
	assertTelegramRecord(t, db, prefix+"-active", soldierID, 1, models.MarkingMethodTelegramScan)
}

func TestTelegramWebhookRejectsClosedOutOfScopeUnknownAndUnpairedWithoutRecords(t *testing.T) {
	db, prefix := openTelegramAttendanceDB(t)
	closedUserID := prefix + "-closed-user"
	outOfScopeUserID := prefix + "-out-of-scope-user"
	unpairedUserID := prefix + "-unpaired-user"
	seedUser(t, db, closedUserID, "PTE CLOSED SCANNER", "PTE", "Alpha", prefix+"-closed", true)
	seedUser(t, db, outOfScopeUserID, "PTE OUTSIDE SCANNER", "PTE", "Bravo", prefix+"-outside", true)
	seedUser(t, db, unpairedUserID, "PTE UNPAIRED SCANNER", "PTE", "Alpha", prefix+"-unpaired", true)
	closedCode := seedTelegramSession(t, db, prefix+"-closed", "Synthetic closed parade", "unit_wide", nil, "closed", closedUserID)
	outOfScopeCode := seedTelegramSession(t, db, prefix+"-scoped", "Synthetic scoped parade", "battery_specific", []string{"Alpha"}, "active", outOfScopeUserID)
	unpairedCode := seedTelegramSession(t, db, prefix+"-unpaired", "Synthetic unpaired parade", "unit_wide", nil, "active", unpairedUserID)
	seedTelegramPairing(t, db, prefix+"-closed-pairing", 970000000002, closedUserID)
	seedTelegramPairing(t, db, prefix+"-outside-pairing", 970000000003, outOfScopeUserID)

	handler, sink := newTelegramAttendanceHandler(db)
	cases := []struct {
		name       string
		telegramID int64
		code       string
		want       string
		sessionID  string
		userID     string
	}{
		{name: "closed", telegramID: 970000000002, code: closedCode, want: telegram.AttendanceClosedReply, sessionID: prefix + "-closed", userID: closedUserID},
		{name: "out of scope", telegramID: 970000000003, code: outOfScopeCode, want: telegram.AttendanceOutOfScopeReply, sessionID: prefix + "-scoped", userID: outOfScopeUserID},
		{name: "unknown code", telegramID: 970000000003, code: mustGenerateTelegramCode(t), want: telegram.AttendanceInvalidReply, sessionID: prefix + "-scoped", userID: outOfScopeUserID},
		{name: "unpaired", telegramID: 970000000004, code: unpairedCode, want: telegram.NamePromptReply, sessionID: prefix + "-unpaired", userID: unpairedUserID},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sink.actions = nil
			rec := postTelegramMessage(t, handler, tc.telegramID, "/start "+tc.code)
			if rec.Code != http.StatusOK {
				t.Fatalf("webhook status = %d, want 200", rec.Code)
			}
			if len(sink.actions) != 1 || sink.actions[0].Text != tc.want {
				t.Fatalf("reply = %#v, want %q", sink.actions, tc.want)
			}
			assertTelegramRecord(t, db, tc.sessionID, tc.userID, 0, "")
		})
	}
}

func TestTelegramWebhookIgnoresGroupMessages(t *testing.T) {
	db, prefix := openTelegramAttendanceDB(t)
	soldierID := prefix + "-soldier"
	seedUser(t, db, soldierID, "PTE GROUP FILTER", "PTE", "Alpha", prefix+"-soldier", true)
	code := seedTelegramSession(t, db, prefix+"-group", "Synthetic group filter", "unit_wide", nil, "active", soldierID)
	seedTelegramPairing(t, db, prefix+"-pairing", 970000000005, soldierID)
	handler, sink := newTelegramAttendanceHandler(db)

	req := httptest.NewRequest(http.MethodPost, "/api/telegram/webhook/synthetic", strings.NewReader(fmt.Sprintf(`{"message":{"from":{"id":970000000005},"chat":{"id":-970000000005,"type":"group"},"text":"/start %s"}}`, code)))
	req.Header.Set(telegramSecretHeader, "synthetic-webhook-secret")
	rec := httptest.NewRecorder()
	handler.Webhook(rec, req)
	if rec.Code != http.StatusOK || len(sink.actions) != 0 {
		t.Fatalf("group webhook = status %d actions %#v, want 200/no actions", rec.Code, sink.actions)
	}
	assertTelegramRecord(t, db, prefix+"-group", soldierID, 0, "")
}

func TestTelegramAndWebAttendanceShareOutcomes(t *testing.T) {
	db, prefix := openTelegramAttendanceDB(t)
	soldierID := prefix + "-soldier"
	outsideID := prefix + "-outside"
	seedUser(t, db, soldierID, "PTE PARITY SCANNER", "PTE", "Alpha", prefix+"-soldier", true)
	seedUser(t, db, outsideID, "PTE PARITY OUTSIDE", "PTE", "Bravo", prefix+"-outside", true)
	activeID := prefix + "-active"
	closedID := prefix + "-closed"
	activeCode := seedTelegramSession(t, db, activeID, "Synthetic parity active", "unit_wide", nil, "active", soldierID)
	closedCode := seedTelegramSession(t, db, closedID, "Synthetic parity closed", "unit_wide", nil, "closed", soldierID)
	scopedCode := seedTelegramSession(t, db, prefix+"-scoped", "Synthetic parity scoped", "battery_specific", []string{"Alpha"}, "active", outsideID)
	seedTelegramPairing(t, db, prefix+"-pairing", 970000000006, soldierID)
	seedTelegramPairing(t, db, prefix+"-outside-pairing", 970000000007, outsideID)

	handler, sink := newTelegramAttendanceHandler(db)
	postTelegramMessage(t, handler, 970000000006, "/start "+activeCode)
	if len(sink.actions) != 1 || sink.actions[0].Text != "Attendance marked for Synthetic parity active." {
		t.Fatalf("Telegram active reply = %#v", sink.actions)
	}

	web := NewAttendanceHandler(db, nil)
	body := fmt.Sprintf(`{"qrData":%q}`, activeID+":"+activeID+"-secret:"+fmt.Sprint(time.Now().Unix()))
	req := httptest.NewRequest(http.MethodPost, "/api/attendance/mark", strings.NewReader(body))
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserKey, &models.User{ID: soldierID}))
	rec := httptest.NewRecorder()
	web.MarkAttendance(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("web duplicate status = %d, want %d", rec.Code, http.StatusConflict)
	}

	// A closed session and a scoped rejection must be the same service outcomes
	// regardless of whether the request arrives through Telegram or web.
	sink.actions = nil
	postTelegramMessage(t, handler, 970000000006, "/start "+closedCode)
	if len(sink.actions) != 1 || sink.actions[0].Text != telegram.AttendanceClosedReply {
		t.Fatalf("Telegram closed reply = %#v", sink.actions)
	}
	webClosed := markWebForTest(t, web, closedID, closedID+"-secret", soldierID)
	if webClosed.Code != http.StatusBadRequest {
		t.Fatalf("web closed status = %d, want %d", webClosed.Code, http.StatusBadRequest)
	}

	sink.actions = nil
	postTelegramMessage(t, handler, 970000000007, "/start "+scopedCode)
	if len(sink.actions) != 1 || sink.actions[0].Text != telegram.AttendanceOutOfScopeReply {
		t.Fatalf("Telegram out-of-scope reply = %#v", sink.actions)
	}
	webScoped := markWebForTest(t, web, prefix+"-scoped", prefix+"-scoped-secret", outsideID)
	if webScoped.Code != http.StatusForbidden {
		t.Fatalf("web out-of-scope status = %d, want %d", webScoped.Code, http.StatusForbidden)
	}

	assertTelegramRecord(t, db, activeID, soldierID, 1, models.MarkingMethodTelegramScan)
	assertTelegramRecord(t, db, closedID, soldierID, 0, "")
	assertTelegramRecord(t, db, prefix+"-scoped", outsideID, 0, "")
}

func openTelegramAttendanceDB(t *testing.T) (*database.DB, string) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to run the Telegram attendance integration test")
	}
	db, err := database.NewPostgresDB(url)
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	prefix := fmt.Sprintf("tg-att-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = db.Pool.Exec(ctx, `DELETE FROM attendance_session WHERE id LIKE $1`, prefix+"-%")
		_, _ = db.Pool.Exec(ctx, `DELETE FROM telegram_pairing WHERE id LIKE $1`, prefix+"-%")
		_, _ = db.Pool.Exec(ctx, `DELETE FROM "user" WHERE id LIKE $1`, prefix+"-%")
		db.Close()
	})
	return db, prefix
}

func seedTelegramSession(t *testing.T, db *database.DB, id, name, scope string, batteries []string, status, creatorID string) string {
	t.Helper()
	code := mustGenerateTelegramCode(t)
	if batteries == nil {
		batteries = []string{}
	}
	_, err := db.Pool.Exec(context.Background(), `
		INSERT INTO attendance_session (id, name, qr_code, qr_code_secret, scope, batteries, status, created_by, start_time, deeplink_code)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), $9)
	`, id, name, id+"-qr", id+"-secret", scope, batteries, status, creatorID, code)
	if err != nil {
		t.Fatalf("seed Telegram session: %v", err)
	}
	return code
}

func seedTelegramPairing(t *testing.T, db *database.DB, id string, telegramID int64, userID string) {
	t.Helper()
	_, err := db.Pool.Exec(context.Background(), `
		INSERT INTO telegram_pairing (id, telegram_id, user_id, display_name, self_confirmed)
		VALUES ($1, $2, $3, 'Synthetic Telegram User', true)
	`, id, telegramID, userID)
	if err != nil {
		t.Fatalf("seed Telegram pairing: %v", err)
	}
}

func newTelegramAttendanceHandler(db *database.DB) (*TelegramHandler, *captureTelegramSink) {
	sink := &captureTelegramSink{}
	store := NewTelegramPairingStore(db)
	return NewTelegramHandler(telegram.NewBot(store), "synthetic-webhook-secret", sink), sink
}

func postTelegramMessage(t *testing.T, handler *TelegramHandler, telegramID int64, text string) *httptest.ResponseRecorder {
	t.Helper()
	body := fmt.Sprintf(`{"message":{"from":{"id":%d},"chat":{"id":%d,"type":"private"},"text":%q}}`, telegramID, telegramID, text)
	req := httptest.NewRequest(http.MethodPost, "/api/telegram/webhook/synthetic", strings.NewReader(body))
	req.Header.Set(telegramSecretHeader, "synthetic-webhook-secret")
	rec := httptest.NewRecorder()
	handler.Webhook(rec, req)
	return rec
}

func markWebForTest(t *testing.T, handler *AttendanceHandler, sessionID, secret, userID string) *httptest.ResponseRecorder {
	t.Helper()
	body := fmt.Sprintf(`{"qrData":%q}`, sessionID+":"+secret+":"+fmt.Sprint(time.Now().Unix()))
	req := httptest.NewRequest(http.MethodPost, "/api/attendance/mark", strings.NewReader(body))
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserKey, &models.User{ID: userID}))
	rec := httptest.NewRecorder()
	handler.MarkAttendance(rec, req)
	return rec
}

func assertTelegramRecord(t *testing.T, db *database.DB, sessionID, userID string, wantCount int, wantMethod string) {
	t.Helper()
	var count int
	if err := db.Pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM attendance_record WHERE session_id = $1 AND user_id = $2
	`, sessionID, userID).Scan(&count); err != nil {
		t.Fatalf("count Telegram attendance records: %v", err)
	}
	if count != wantCount {
		t.Fatalf("attendance records for %s/%s = %d, want %d", sessionID, userID, count, wantCount)
	}
	if wantMethod != "" {
		var method string
		if err := db.Pool.QueryRow(context.Background(), `
			SELECT marking_method FROM attendance_record WHERE session_id = $1 AND user_id = $2
		`, sessionID, userID).Scan(&method); err != nil {
			t.Fatalf("read Telegram marking method: %v", err)
		}
		if method != wantMethod {
			t.Fatalf("marking method = %q, want %q", method, wantMethod)
		}
	}
}

func mustGenerateTelegramCode(t *testing.T) string {
	t.Helper()
	code, err := deeplink.GenerateCode()
	if err != nil {
		t.Fatalf("generate test Telegram code: %v", err)
	}
	return code
}
