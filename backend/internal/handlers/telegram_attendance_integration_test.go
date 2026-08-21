package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/davidlivingston/go-nextjs-starter/backend/internal/database"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/middleware"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/models"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/services/deeplink"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/sse"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/telegram"
)

type captureTelegramSink struct {
	mu      sync.Mutex
	actions []telegram.Action
}

func (s *captureTelegramSink) Enqueue(actions []telegram.Action) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.actions = append(s.actions, actions...)
	return true
}

func (s *captureTelegramSink) Actions() []telegram.Action {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]telegram.Action(nil), s.actions...)
}

func (s *captureTelegramSink) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.actions = nil
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
	actions := sink.Actions()
	if len(actions) != 1 || actions[0].Text != "Attendance marked for Synthetic active parade." {
		t.Fatalf("first reply = %#v", actions)
	}
	assertTelegramRecord(t, db, prefix+"-active", soldierID, 1, models.MarkingMethodTelegramScan)

	sink.Reset()
	second := postTelegramMessage(t, handler, 970000000001, "/start "+code)
	if second.Code != http.StatusOK {
		t.Fatalf("repeated webhook status = %d, want 200", second.Code)
	}
	actions = sink.Actions()
	if len(actions) != 1 || actions[0].Text != "You are already marked for Synthetic active parade." {
		t.Fatalf("repeated reply = %#v", actions)
	}
	assertTelegramRecord(t, db, prefix+"-active", soldierID, 1, models.MarkingMethodTelegramScan)
}

func TestTelegramWebhookUnpairedDeepLinkUsesDisplayNameProposalWithoutMark(t *testing.T) {
	db, prefix := openTelegramAttendanceDB(t)
	targetID := prefix + "-target"
	seedUser(t, db, targetID, "SYNTHETIC DISPLAY SOLDIER", "PTE", "Alpha", prefix+"-target", true)
	code := seedTelegramSession(t, db, prefix+"-session", "Synthetic pairing parade", "unit_wide", nil, "active", targetID)
	telegramID := int64(970000000101)
	handler, sink := newTelegramAttendanceHandler(db)

	first := postTelegramUserMessage(t, handler, telegram.User{
		ID: telegramID, FirstName: "Synthetic", LastName: "Display Soldier",
	}, "/start "+code)
	if first.Code != http.StatusOK {
		t.Fatalf("valid deep-link webhook status = %d, want 200", first.Code)
	}
	actions := sink.Actions()
	if len(actions) != 1 || actions[0].Text != "Are you PTE SYNTHETIC DISPLAY SOLDIER, Alpha?" {
		t.Fatalf("valid deep-link pairing actions = %#v", actions)
	}
	if strings.Contains(actions[0].Text, code) {
		t.Fatalf("opaque deep-link payload was echoed in pairing reply: %q", actions[0].Text)
	}
	if actions[0].ReplyMarkup == nil {
		t.Fatal("strong display-name match did not require explicit confirmation")
	}
	assertTelegramRecord(t, db, prefix+"-session", targetID, 0, "")

	var outcome, proposedUserID string
	if err := db.Pool.QueryRow(context.Background(), `
		SELECT outcome, COALESCE(user_id, '') FROM telegram_pairing_attempt
		WHERE telegram_id = $1 ORDER BY "createdAt" DESC LIMIT 1
	`, telegramID).Scan(&outcome, &proposedUserID); err != nil {
		t.Fatalf("read pairing attempt: %v", err)
	}
	if outcome != "proposed" || proposedUserID != targetID {
		t.Fatalf("pairing attempt = (%q, %q), want proposed target %q", outcome, proposedUserID, targetID)
	}
	var payloadRows int
	if err := db.Pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM telegram_pairing_request
		WHERE telegram_id = $1 AND display_name LIKE '%' || $2 || '%'
	`, telegramID, code).Scan(&payloadRows); err != nil {
		t.Fatalf("search pairing request for opaque payload: %v", err)
	}
	if payloadRows != 0 {
		t.Fatalf("opaque payload was stored in pairing request display_name (%d rows)", payloadRows)
	}

	// An unknown capability gets the same normal display-name proposal rather
	// than a session-existence response. This keeps the code opaque while still
	// allowing a soldier to pair from the normal /start flow.
	sink.Reset()
	unknownCode := mustGenerateTelegramCode(t)
	secondID := telegramID + 1
	second := postTelegramUserMessage(t, handler, telegram.User{
		ID: secondID, FirstName: "Synthetic", LastName: "Display Soldier",
	}, "/start "+unknownCode)
	if second.Code != http.StatusOK {
		t.Fatalf("unknown deep-link webhook status = %d, want 200", second.Code)
	}
	unknownActions := sink.Actions()
	if len(unknownActions) != 1 || unknownActions[0].Text != actions[0].Text {
		t.Fatalf("unknown deep-link proposal = %#v, want same normal proposal text", unknownActions)
	}
	if strings.Contains(unknownActions[0].Text, unknownCode) {
		t.Fatalf("unknown opaque payload was echoed in pairing reply: %q", unknownActions[0].Text)
	}
	assertTelegramRecord(t, db, prefix+"-session", targetID, 0, "")
}

func TestTelegramWebhookNoPayloadStartUsesDisplayNameAndEmptyPrompts(t *testing.T) {
	db, prefix := openTelegramAttendanceDB(t)
	targetID := prefix + "-no-payload-target"
	sessionID := prefix + "-no-payload-session"
	seedUser(t, db, targetID, "SYNTHETIC DISPLAY SOLDIER", "PTE", "Alpha", prefix+"-no-payload-target", true)
	seedTelegramSession(t, db, sessionID, "Synthetic no-payload parade", "unit_wide", nil, "active", targetID)
	telegramID := int64(970000000141)
	emptyNameTelegramID := telegramID + 1
	handler, sink := newTelegramAttendanceHandler(db)

	first := postTelegramUserMessage(t, handler, telegram.User{
		ID: telegramID, FirstName: "Synthetic", LastName: "Display Soldier",
	}, "/start")
	if first.Code != http.StatusOK {
		t.Fatalf("no-payload webhook status = %d, want 200", first.Code)
	}
	actions := sink.Actions()
	if len(actions) != 1 || actions[0].Text != "Are you PTE SYNTHETIC DISPLAY SOLDIER, Alpha?" {
		t.Fatalf("no-payload pairing actions = %#v", actions)
	}
	if actions[0].ReplyMarkup == nil {
		t.Fatal("no-payload strong match did not require explicit confirmation")
	}
	assertTelegramRecord(t, db, sessionID, targetID, 0, "")

	var requestCount int
	var displayName, attemptID string
	if err := db.Pool.QueryRow(context.Background(), `
		SELECT COUNT(*), COALESCE(MAX(display_name), ''), COALESCE(MAX(attempt_id), '')
		FROM telegram_pairing_request WHERE telegram_id = $1
	`, telegramID).Scan(&requestCount, &displayName, &attemptID); err != nil {
		t.Fatalf("read no-payload pairing request: %v", err)
	}
	if requestCount != 1 || displayName != "Synthetic Display Soldier" || attemptID == "" {
		t.Fatalf("no-payload pairing request = (%d, %q, %q), want persisted display name and attempt", requestCount, displayName, attemptID)
	}
	var outcome, proposedUserID string
	if err := db.Pool.QueryRow(context.Background(), `
		SELECT outcome, COALESCE(user_id, '') FROM telegram_pairing_attempt WHERE id = $1
	`, attemptID).Scan(&outcome, &proposedUserID); err != nil {
		t.Fatalf("read no-payload pairing attempt: %v", err)
	}
	if outcome != "proposed" || proposedUserID != targetID {
		t.Fatalf("no-payload pairing attempt = (%q, %q), want proposed target %q", outcome, proposedUserID, targetID)
	}

	sink.Reset()
	empty := postTelegramUserMessage(t, handler, telegram.User{ID: emptyNameTelegramID}, "/start")
	if empty.Code != http.StatusOK {
		t.Fatalf("empty-display webhook status = %d, want 200", empty.Code)
	}
	if actions = sink.Actions(); len(actions) != 1 || actions[0].Text != telegram.NamePromptReply {
		t.Fatalf("empty-display pairing actions = %#v, want name prompt", actions)
	}
	if err := db.Pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM telegram_pairing_request WHERE telegram_id = $1
	`, emptyNameTelegramID).Scan(&requestCount); err != nil {
		t.Fatalf("read empty-display pairing request: %v", err)
	}
	if requestCount != 0 {
		t.Fatalf("empty-display pairing request count = %d, want 0", requestCount)
	}
	assertTelegramRecord(t, db, sessionID, targetID, 0, "")
}

func TestTelegramScanAndAdminManualMarkCompleteWithoutDeadlock(t *testing.T) {
	db, prefix := openTelegramAttendanceDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	actorID := prefix + "-deadlock-actor"
	sessionID := prefix + "-deadlock-session"
	telegramID := int64(970000001500)
	seedUser(t, db, actorID, "SSG DEADLOCK ACTOR", models.RankSSG, models.BatteryAlpha, prefix+"-deadlock", true)
	code := seedTelegramSession(t, db, sessionID, "Synthetic deadlock parade", "unit_wide", nil, "active", actorID)
	seedTelegramPairing(t, db, prefix+"-deadlock-pairing", telegramID, actorID)

	adminStore := NewTelegramAdminStore(db, nil)
	actor, found, err := adminStore.Actor(ctx, telegram.Pairing{TelegramID: telegramID, UserID: actorID})
	if err != nil || !found {
		t.Fatalf("deadlock regression actor = %+v, found=%v, err=%v", actor, found, err)
	}

	// Hold the pairing row so the admin operation queues on its first lock. The
	// scan then locks the session and queues behind the admin. Releasing this
	// blocker makes the old session-then-pairing order form a deterministic
	// cycle, while pairing-then-session lets both operations drain.
	blocker, err := db.Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin deadlock blocker: %v", err)
	}
	defer func() { _ = blocker.Rollback(context.Background()) }()
	var pairingID string
	if err := blocker.QueryRow(ctx, `
		SELECT id FROM telegram_pairing WHERE telegram_id = $1 FOR UPDATE
	`, telegramID).Scan(&pairingID); err != nil {
		t.Fatalf("lock deadlock regression pairing: %v", err)
	}

	type attendanceCall struct {
		result telegram.AttendanceResult
		err    error
	}
	adminDone := make(chan error, 1)
	go func() {
		adminDone <- adminStore.MarkManual(ctx, actor, sessionID, actorID)
	}()
	if err := waitForTelegramPairingWaiters(ctx, db, 1); err != nil {
		t.Fatalf("admin did not queue on pairing lock: %v", err)
	}

	scanDone := make(chan attendanceCall, 1)
	attendanceStore := NewTelegramPairingStore(db)
	go func() {
		result, err := attendanceStore.MarkAttendance(ctx, telegramID, code)
		scanDone <- attendanceCall{result: result, err: err}
	}()
	if err := waitForTelegramPairingWaiters(ctx, db, 2); err != nil {
		t.Fatalf("scan and admin did not queue on pairing lock: %v", err)
	}

	if err := blocker.Commit(ctx); err != nil {
		t.Fatalf("release deadlock regression blocker: %v", err)
	}
	adminErr := <-adminDone
	scan := <-scanDone
	if adminErr != nil && !errors.Is(adminErr, telegram.ErrAdminUnavailable) {
		t.Fatalf("admin manual mark error = %v, want success or idempotent unavailable", adminErr)
	}
	if scan.err != nil {
		t.Fatalf("paired scan error = %v, want success or idempotent outcome", scan.err)
	}
	if scan.result.Outcome != telegram.AttendanceMarked && scan.result.Outcome != telegram.AttendanceAlreadyMarked {
		t.Fatalf("paired scan outcome = %v, want marked or already marked", scan.result.Outcome)
	}
	var records int
	if err := db.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM attendance_record WHERE session_id = $1 AND user_id = $2
	`, sessionID, actorID).Scan(&records); err != nil {
		t.Fatalf("count deadlock regression attendance: %v", err)
	}
	if records != 1 {
		t.Fatalf("deadlock regression attendance records = %d, want one idempotent record", records)
	}
}

func waitForTelegramPairingWaiters(ctx context.Context, db *database.DB, want int) error {
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		var waiting int
		err := db.Pool.QueryRow(ctx, `
			SELECT COUNT(*)
			FROM pg_stat_activity
			WHERE datname = current_database()
			  AND pid <> pg_backend_pid()
			  AND wait_event_type = 'Lock'
			  AND query ILIKE '%telegram_pairing%'
		`).Scan(&waiting)
		if err != nil {
			return fmt.Errorf("inspect PostgreSQL pairing waiters: %w", err)
		}
		if waiting >= want {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("waiting for %d pairing waiters: %w", want, ctx.Err())
		case <-ticker.C:
		}
	}
}

func TestTelegramWebhookConcurrentDuplicateDeliveryCreatesOneRecord(t *testing.T) {
	db, prefix := openTelegramAttendanceDB(t)
	soldierID := prefix + "-concurrent-soldier"
	telegramID := int64(970000000111)
	seedUser(t, db, soldierID, "PTE SYNTHETIC CONCURRENT", "PTE", "Alpha", prefix+"-concurrent", true)
	code := seedTelegramSession(t, db, prefix+"-concurrent", "Synthetic concurrent parade", "unit_wide", nil, "active", soldierID)
	seedTelegramPairing(t, db, prefix+"-concurrent-pairing", telegramID, soldierID)
	handler, sink := newTelegramAttendanceHandler(db)

	const deliveries = 32
	statuses := make(chan int, deliveries)
	var wg sync.WaitGroup
	for i := 0; i < deliveries; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			statuses <- postTelegramMessage(t, handler, telegramID, "/start "+code).Code
		}()
	}
	wg.Wait()
	close(statuses)
	for status := range statuses {
		if status != http.StatusOK {
			t.Fatalf("duplicate delivery status = %d, want 200", status)
		}
	}

	actions := sink.Actions()
	if len(actions) != deliveries {
		t.Fatalf("duplicate delivery replies = %d, want %d", len(actions), deliveries)
	}
	marked, already := 0, 0
	for _, action := range actions {
		switch action.Text {
		case "Attendance marked for Synthetic concurrent parade.":
			marked++
		case "You are already marked for Synthetic concurrent parade.":
			already++
		default:
			t.Fatalf("unsafe duplicate delivery reply = %q", action.Text)
		}
	}
	if marked != 1 || already != deliveries-1 {
		t.Fatalf("duplicate delivery outcomes = marked %d already %d, want 1/%d", marked, already, deliveries-1)
	}
	assertTelegramRecord(t, db, prefix+"-concurrent", soldierID, 1, models.MarkingMethodTelegramScan)
}

func TestTelegramWebhookRejectsClosedUnknownAndUnpairedWithoutRecords(t *testing.T) {
	db, prefix := openTelegramAttendanceDB(t)
	closedUserID := prefix + "-closed-user"
	scannerUserID := prefix + "-scanner-user"
	unpairedUserID := prefix + "-unpaired-user"
	seedUser(t, db, closedUserID, "PTE CLOSED SCANNER", "PTE", "Alpha", prefix+"-closed", true)
	seedUser(t, db, scannerUserID, "PTE OUTSIDE SCANNER", "PTE", "Bravo", prefix+"-outside", true)
	seedUser(t, db, unpairedUserID, "PTE UNPAIRED SCANNER", "PTE", "Alpha", prefix+"-unpaired", true)
	closedCode := seedTelegramSession(t, db, prefix+"-closed", "Synthetic closed parade", "unit_wide", nil, "closed", closedUserID)
	unpairedCode := seedTelegramSession(t, db, prefix+"-unpaired", "Synthetic unpaired parade", "unit_wide", nil, "active", unpairedUserID)
	seedTelegramPairing(t, db, prefix+"-closed-pairing", 970000000002, closedUserID)
	seedTelegramPairing(t, db, prefix+"-outside-pairing", 970000000003, scannerUserID)

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
		{name: "unknown code", telegramID: 970000000003, code: mustGenerateTelegramCode(t), want: telegram.AttendanceInvalidReply, sessionID: prefix + "-closed", userID: scannerUserID},
		{name: "unpaired", telegramID: 970000000004, code: unpairedCode, want: telegram.NamePromptReply, sessionID: prefix + "-unpaired", userID: unpairedUserID},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sink.Reset()
			rec := postTelegramMessage(t, handler, tc.telegramID, "/start "+tc.code)
			if rec.Code != http.StatusOK {
				t.Fatalf("webhook status = %d, want 200", rec.Code)
			}
			actions := sink.Actions()
			if len(actions) != 1 || actions[0].Text != tc.want {
				t.Fatalf("reply = %#v, want %q", actions, tc.want)
			}
			assertTelegramRecord(t, db, tc.sessionID, tc.userID, 0, "")
		})
	}
}

func TestTelegramWebhookOutOfScopeScanMarksWalkIn(t *testing.T) {
	db, prefix := openTelegramAttendanceDB(t)
	userID := prefix + "-walk-in"
	seedUser(t, db, userID, "PTE WALK IN", "PTE", "Bravo", prefix+"-walk-in", true)
	code := seedTelegramSession(t, db, prefix+"-scoped", "Synthetic walk-in parade", "battery_specific", []string{"Alpha"}, "active", userID)
	seedTelegramPairing(t, db, prefix+"-walk-in-pairing", 970000000008, userID)

	handler, sink := newTelegramAttendanceHandler(db)
	rec := postTelegramMessage(t, handler, 970000000008, "/start "+code)
	if rec.Code != http.StatusOK {
		t.Fatalf("webhook status = %d, want 200", rec.Code)
	}
	actions := sink.Actions()
	want := "Attendance marked for Synthetic walk-in parade. Note: you are not on this session's list, but your presence is counted."
	if len(actions) != 1 || actions[0].Text != want {
		t.Fatalf("reply = %#v, want %q", actions, want)
	}
	assertTelegramRecord(t, db, prefix+"-scoped", userID, 1, models.MarkingMethodTelegramScan)

	// A second scan is a plain duplicate, not another walk-in note.
	sink.Reset()
	postTelegramMessage(t, handler, 970000000008, "/start "+code)
	if actions := sink.Actions(); len(actions) != 1 || actions[0].Text != "You are already marked for Synthetic walk-in parade." {
		t.Fatalf("duplicate reply = %#v", actions)
	}
	assertTelegramRecord(t, db, prefix+"-scoped", userID, 1, models.MarkingMethodTelegramScan)
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
	if rec.Code != http.StatusOK || len(sink.Actions()) != 0 {
		t.Fatalf("group webhook = status %d actions %#v, want 200/no actions", rec.Code, sink.Actions())
	}
	assertTelegramRecord(t, db, prefix+"-group", soldierID, 0, "")
}

func TestTelegramWebhookBroadcastsTelegramAttributionAfterCommit(t *testing.T) {
	db, prefix := openTelegramAttendanceDB(t)
	soldierID := prefix + "-sse-soldier"
	telegramID := int64(970000000121)
	seedUser(t, db, soldierID, "PTE SYNTHETIC SSE", "PTE", "Alpha", prefix+"-sse", true)
	code := seedTelegramSession(t, db, prefix+"-sse", "Synthetic SSE parade", "unit_wide", nil, "active", soldierID)
	seedTelegramPairing(t, db, prefix+"-sse-pairing", telegramID, soldierID)

	hub := sse.NewHub()
	client := sse.NewClient("synthetic-sse-client", prefix+"-commander")
	hub.Subscribe(prefix+"-sse", client)
	t.Cleanup(func() { hub.Unsubscribe(prefix+"-sse", client) })
	handler, sink := newTelegramAttendanceHandlerWithHub(db, hub)
	response := postTelegramMessage(t, handler, telegramID, "/start "+code)
	if response.Code != http.StatusOK {
		t.Fatalf("SSE Telegram webhook status = %d, want 200", response.Code)
	}
	if actions := sink.Actions(); len(actions) != 1 || actions[0].Text != "Attendance marked for Synthetic SSE parade." {
		t.Fatalf("SSE Telegram reply = %#v", actions)
	}

	select {
	case event := <-client.Send:
		if event.Type != sse.EventTypeAttendanceMarked {
			t.Fatalf("SSE event type = %q, want %q", event.Type, sse.EventTypeAttendanceMarked)
		}
		payload, ok := event.Payload.(sse.AttendanceMarkedPayload)
		if !ok {
			t.Fatalf("SSE payload type = %T, want AttendanceMarkedPayload", event.Payload)
		}
		if payload.MarkingMethod != models.MarkingMethodTelegramScan {
			t.Fatalf("SSE marking method = %q, want %q", payload.MarkingMethod, models.MarkingMethodTelegramScan)
		}
		if payload.UserID != soldierID {
			t.Fatalf("SSE user ID = %q, want %q", payload.UserID, soldierID)
		}
		var count int
		if err := db.Pool.QueryRow(context.Background(), `
			SELECT COUNT(*) FROM attendance_record WHERE session_id = $1 AND user_id = $2
		`, prefix+"-sse", soldierID).Scan(&count); err != nil {
			t.Fatalf("read committed attendance from SSE assertion: %v", err)
		}
		if count != 1 {
			t.Fatalf("attendance visible with SSE event = %d, want 1", count)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Telegram attendance SSE event")
	}
}

func TestTelegramWebhookRollsBackOnAttendanceInsertError(t *testing.T) {
	db, prefix := openTelegramAttendanceDB(t)
	soldierID := prefix + "-rollback-soldier"
	telegramID := int64(970000000131)
	sessionID := prefix + "-rollback"
	seedUser(t, db, soldierID, "PTE SYNTHETIC ROLLBACK", "PTE", "Alpha", prefix+"-rollback", true)
	code := seedTelegramSession(t, db, sessionID, "Synthetic rollback parade", "unit_wide", nil, "active", soldierID)
	seedTelegramPairing(t, db, prefix+"-rollback-pairing", telegramID, soldierID)

	_, err := db.Pool.Exec(context.Background(), `
		CREATE OR REPLACE FUNCTION tg_synthetic_fail_telegram_mark() RETURNS trigger
		LANGUAGE plpgsql AS $$
		BEGIN
			IF NEW.marking_method = 'telegram_scan' THEN
				RAISE EXCEPTION 'synthetic Telegram attendance insert failure';
			END IF;
			RETURN NEW;
		END;
		$$;
		CREATE TRIGGER tg_synthetic_fail_telegram_mark
		BEFORE INSERT ON attendance_record
		FOR EACH ROW EXECUTE FUNCTION tg_synthetic_fail_telegram_mark()
	`)
	if err != nil {
		t.Fatalf("install synthetic rollback trigger: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(context.Background(), `DROP TRIGGER IF EXISTS tg_synthetic_fail_telegram_mark ON attendance_record`)
		_, _ = db.Pool.Exec(context.Background(), `DROP FUNCTION IF EXISTS tg_synthetic_fail_telegram_mark()`)
	})

	handler, sink := newTelegramAttendanceHandler(db)
	response := postTelegramMessage(t, handler, telegramID, "/start "+code)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("rollback webhook status = %d, want 500", response.Code)
	}
	if actions := sink.Actions(); len(actions) != 0 {
		t.Fatalf("rollback error queued unsafe reply: %#v", actions)
	}
	assertTelegramRecord(t, db, sessionID, soldierID, 0, "")
}

func TestTelegramAndWebAttendanceShareOutcomes(t *testing.T) {
	db, prefix := openTelegramAttendanceDB(t)
	soldierID := prefix + "-soldier"
	outsideID := prefix + "-outside"
	outsideWebID := prefix + "-outside-web"
	seedUser(t, db, soldierID, "PTE PARITY SCANNER", "PTE", "Alpha", prefix+"-soldier", true)
	seedUser(t, db, outsideID, "PTE PARITY OUTSIDE", "PTE", "Bravo", prefix+"-outside", true)
	seedUser(t, db, outsideWebID, "PTE PARITY OUTSIDE WEB", "PTE", "Bravo", prefix+"-outside-web", true)
	activeID := prefix + "-active"
	closedID := prefix + "-closed"
	activeCode := seedTelegramSession(t, db, activeID, "Synthetic parity active", "unit_wide", nil, "active", soldierID)
	closedCode := seedTelegramSession(t, db, closedID, "Synthetic parity closed", "unit_wide", nil, "closed", soldierID)
	scopedCode := seedTelegramSession(t, db, prefix+"-scoped", "Synthetic parity scoped", "battery_specific", []string{"Alpha"}, "active", outsideID)
	seedTelegramPairing(t, db, prefix+"-pairing", 970000000006, soldierID)
	seedTelegramPairing(t, db, prefix+"-outside-pairing", 970000000007, outsideID)

	handler, sink := newTelegramAttendanceHandler(db)
	postTelegramMessage(t, handler, 970000000006, "/start "+activeCode)
	if actions := sink.Actions(); len(actions) != 1 || actions[0].Text != "Attendance marked for Synthetic parity active." {
		t.Fatalf("Telegram active reply = %#v", actions)
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
	sink.Reset()
	postTelegramMessage(t, handler, 970000000006, "/start "+closedCode)
	if actions := sink.Actions(); len(actions) != 1 || actions[0].Text != telegram.AttendanceClosedReply {
		t.Fatalf("Telegram closed reply = %#v", actions)
	}
	webClosed := markWebForTest(t, web, closedID, closedID+"-secret", soldierID)
	if webClosed.Code != http.StatusBadRequest {
		t.Fatalf("web closed status = %d, want %d", webClosed.Code, http.StatusBadRequest)
	}

	// An out-of-scope scan is a walk-in on both channels: it marks attendance
	// and counts the soldier as present, never absent.
	sink.Reset()
	postTelegramMessage(t, handler, 970000000007, "/start "+scopedCode)
	wantWalkIn := "Attendance marked for Synthetic parity scoped. Note: you are not on this session's list, but your presence is counted."
	if actions := sink.Actions(); len(actions) != 1 || actions[0].Text != wantWalkIn {
		t.Fatalf("Telegram walk-in reply = %#v, want %q", actions, wantWalkIn)
	}
	webScoped := markWebForTest(t, web, prefix+"-scoped", prefix+"-scoped-secret", outsideWebID)
	if webScoped.Code != http.StatusOK {
		t.Fatalf("web walk-in status = %d, want %d", webScoped.Code, http.StatusOK)
	}

	assertTelegramRecord(t, db, activeID, soldierID, 1, models.MarkingMethodTelegramScan)
	assertTelegramRecord(t, db, closedID, soldierID, 0, "")
	assertTelegramRecord(t, db, prefix+"-scoped", outsideID, 1, models.MarkingMethodTelegramScan)
	assertTelegramRecord(t, db, prefix+"-scoped", outsideWebID, 1, models.MarkingMethodQRScan)
}

func TestTelegramWebhookSynthetic431UserLoadHasNoFailuresOrDuplicates(t *testing.T) {
	db, prefix := openTelegramAttendanceDB(t)
	const users = 431
	firstTelegramID := int64(970000001000)
	firstUserID := prefix + "-load-user-000"
	seedUser(t, db, firstUserID, "SYNTHETIC LOAD SOLDIER 000", "PTE", "Alpha", prefix+"-load-000", true)
	for i := 1; i < users; i++ {
		userID := fmt.Sprintf("%s-load-user-%03d", prefix, i)
		seedUser(t, db, userID, fmt.Sprintf("SYNTHETIC LOAD SOLDIER %03d", i), "PTE", "Alpha", fmt.Sprintf("%s-load-%03d", prefix, i), true)
	}
	sessionID := prefix + "-load-session"
	code := seedTelegramSession(t, db, sessionID, "Synthetic 431-user parade", "unit_wide", nil, "active", firstUserID)
	for i := 0; i < users; i++ {
		userID := fmt.Sprintf("%s-load-user-%03d", prefix, i)
		seedTelegramPairing(t, db, fmt.Sprintf("%s-load-pairing-%03d", prefix, i), firstTelegramID+int64(i), userID)
	}

	handler, sink := newTelegramAttendanceHandler(db)
	statuses := make(chan int, users)
	var wg sync.WaitGroup
	for i := 0; i < users; i++ {
		telegramID := firstTelegramID + int64(i)
		wg.Add(1)
		go func() {
			defer wg.Done()
			statuses <- postTelegramMessage(t, handler, telegramID, "/start "+code).Code
		}()
	}
	wg.Wait()
	close(statuses)
	for status := range statuses {
		if status != http.StatusOK {
			t.Fatalf("431-user load webhook status = %d, want 200", status)
		}
	}
	if actions := sink.Actions(); len(actions) != users {
		t.Fatalf("431-user load replies = %d, want %d", len(actions), users)
	}

	var count, distinctUsers, telegramMarks int
	if err := db.Pool.QueryRow(context.Background(), `
		SELECT COUNT(*), COUNT(DISTINCT user_id), COUNT(*) FILTER (WHERE marking_method = $2)
		FROM attendance_record WHERE session_id = $1
	`, sessionID, models.MarkingMethodTelegramScan).Scan(&count, &distinctUsers, &telegramMarks); err != nil {
		t.Fatalf("count 431-user attendance: %v", err)
	}
	if count != users || distinctUsers != users || telegramMarks != users {
		t.Fatalf("431-user load records = total %d distinct %d Telegram %d, want %d/%d/%d", count, distinctUsers, telegramMarks, users, users, users)
	}
	var duplicateGroups int
	if err := db.Pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM (
			SELECT user_id FROM attendance_record WHERE session_id = $1 GROUP BY user_id HAVING COUNT(*) > 1
		) duplicates
	`, sessionID).Scan(&duplicateGroups); err != nil {
		t.Fatalf("count 431-user duplicate groups: %v", err)
	}
	if duplicateGroups != 0 {
		t.Fatalf("431-user load duplicate groups = %d, want 0", duplicateGroups)
	}
}

// These fixtures intentionally use generated row IDs, deterministic synthetic
// names/Telegram IDs, and freshly generated test-only deep-link codes. The
// webhook-to-database behavior does not depend on production-shaped data.
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
		_, _ = db.Pool.Exec(ctx, `DELETE FROM telegram_pairing_request WHERE telegram_id BETWEEN $1 AND $2`, int64(970000000000), int64(970000002000))
		_, _ = db.Pool.Exec(ctx, `DELETE FROM telegram_pairing_attempt WHERE telegram_id BETWEEN $1 AND $2`, int64(970000000000), int64(970000002000))
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
	return newTelegramAttendanceHandlerWithHub(db, nil)
}

func newTelegramAttendanceHandlerWithHub(db *database.DB, hub *sse.Hub) (*TelegramHandler, *captureTelegramSink) {
	sink := &captureTelegramSink{}
	store := NewTelegramPairingStoreWithHub(db, hub)
	return NewTelegramHandler(telegram.NewBot(store), "synthetic-webhook-secret", sink), sink
}

func postTelegramMessage(t *testing.T, handler *TelegramHandler, telegramID int64, text string) *httptest.ResponseRecorder {
	t.Helper()
	return postTelegramUserMessage(t, handler, telegram.User{ID: telegramID}, text)
}

func postTelegramUserMessage(t *testing.T, handler *TelegramHandler, user telegram.User, text string) *httptest.ResponseRecorder {
	t.Helper()
	body := fmt.Sprintf(`{"message":{"from":{"id":%d,"first_name":%q,"last_name":%q},"chat":{"id":%d,"type":"private"},"text":%q}}`, user.ID, user.FirstName, user.LastName, user.ID, text)
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
