package handlers

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/davidlivingston/go-nextjs-starter/backend/internal/database"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/models"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/sse"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/telegram"
)

func TestTelegramAdminActorReloadsVerifiedRosterRole(t *testing.T) {
	db, prefix := openTelegramAdminDB(t)
	userID := prefix + "-actor"
	telegramID := telegramAdminID(prefix, 1)
	seedUser(t, db, userID, "CURRENT COMMANDER", models.RankSSG, models.BatteryAlpha, "", true)
	seedTelegramAdminPairing(t, db, prefix+"-pairing", telegramID, userID)
	store := NewTelegramAdminStore(db, nil)

	pairing := telegram.Pairing{TelegramID: telegramID, UserID: userID, FullName: "STALE NAME"}
	actor, found, err := store.Actor(context.Background(), pairing)
	if err != nil || !found {
		t.Fatalf("Actor = %+v, found=%v, err=%v", actor, found, err)
	}
	if actor.FullName != "CURRENT COMMANDER" || actor.Tier != models.TierUnitCommander || actor.Battery != models.BatteryAlpha {
		t.Fatalf("initial actor = %+v", actor)
	}
	if _, err := db.Pool.Exec(context.Background(), `UPDATE "user" SET rank = $2, is_superadmin = true WHERE id = $1`, userID, models.RankCPT); err != nil {
		t.Fatal(err)
	}
	actor, found, err = store.Actor(context.Background(), pairing)
	if err != nil || !found || actor.Tier != models.TierSuperadmin || !actor.IsSuperadmin {
		t.Fatalf("superadmin actor = %+v, found=%v, err=%v", actor, found, err)
	}

	if _, err := db.Pool.Exec(context.Background(), `
		UPDATE "user" SET "full_name" = 'DEMOTED CURRENT USER', rank = $2, battery = $3, is_superadmin = false WHERE id = $1
	`, userID, models.RankPTE, models.BatteryBravo); err != nil {
		t.Fatal(err)
	}
	actor, found, err = store.Actor(context.Background(), pairing)
	if err != nil || !found {
		t.Fatalf("reloaded Actor = %+v, found=%v, err=%v", actor, found, err)
	}
	if actor.FullName != "DEMOTED CURRENT USER" || actor.Tier != models.TierEnlisted || actor.Battery != models.BatteryBravo || actor.IsSuperadmin {
		t.Fatalf("reloaded actor did not use current roster state: %+v", actor)
	}
	if _, err := db.Pool.Exec(context.Background(), `UPDATE "user" SET verified = false WHERE id = $1`, userID); err != nil {
		t.Fatal(err)
	}
	if _, found, err := store.Actor(context.Background(), pairing); err != nil || found {
		t.Fatalf("unverified Actor = found %v, err %v; want unavailable", found, err)
	}
}

func TestTelegramAdminContextPersistsRejectsStaleVersionAndExpiresDraft(t *testing.T) {
	db, prefix := openTelegramAdminDB(t)
	store := NewTelegramAdminStore(db, nil)
	telegramID := telegramAdminID(prefix, 2)
	ctx := context.Background()

	initial, err := store.LoadContext(ctx, telegramID)
	if err != nil {
		t.Fatal(err)
	}
	if initial.State != "idle" || initial.Version != 0 {
		t.Fatalf("initial context = %+v", initial)
	}
	endTime := time.Now().Add(time.Hour).Truncate(time.Microsecond)
	draft := telegram.AdminContext{
		TelegramID: telegramID, State: "draft",
		Draft:   telegram.AdminDraft{Name: "Persisted parade", Scope: models.SessionScopeUnitWide, EndTime: endTime},
		Version: initial.Version, ExpiresAt: ptrTime(time.Now().Add(10 * time.Minute)),
	}
	if err := store.SaveContext(ctx, draft); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadContext(ctx, telegramID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.State != "draft" || loaded.Draft.Name != draft.Draft.Name || loaded.Version != 0 {
		t.Fatalf("loaded draft = %+v", loaded)
	}
	loaded.State = "choosing_duration"
	loaded.Draft.Battery = models.BatteryAlpha
	if err := store.SaveContext(ctx, loaded); err != nil {
		t.Fatal(err)
	}
	updated, err := store.LoadContext(ctx, telegramID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Version != 1 || updated.State != "choosing_duration" || updated.Draft.Battery != models.BatteryAlpha {
		t.Fatalf("updated context = %+v", updated)
	}
	if err := store.SaveContext(ctx, loaded); !errors.Is(err, telegram.ErrAdminContextConflict) {
		t.Fatalf("stale SaveContext error = %v, want ErrAdminContextConflict", err)
	}

	expiredID := telegramAdminID(prefix, 3)
	if err := store.SaveContext(ctx, telegram.AdminContext{
		TelegramID: expiredID, State: "draft",
		Draft:     telegram.AdminDraft{Name: "Expired draft", Scope: models.SessionScopeUnitWide},
		ExpiresAt: ptrTime(time.Now().Add(-time.Minute)),
	}); err != nil {
		t.Fatal(err)
	}
	expired, err := store.LoadContext(ctx, expiredID)
	if err != nil {
		t.Fatal(err)
	}
	if expired.State != "idle" || expired.SessionID != "" || expired.Draft.Name != "" || expired.ExpiresAt != nil {
		t.Fatalf("expired draft = %+v, want idle empty context", expired)
	}
	if err := store.ClearContext(ctx, telegramID); err != nil {
		t.Fatal(err)
	}
	cleared, err := store.LoadContext(ctx, telegramID)
	if err != nil {
		t.Fatal(err)
	}
	if cleared.State != "idle" || cleared.Version != 0 {
		t.Fatalf("cleared context = %+v", cleared)
	}
}

func TestTelegramAdminMigrationColumns(t *testing.T) {
	db, _ := openTelegramAdminDB(t)
	rows, err := db.Pool.Query(context.Background(), `
		SELECT column_name, is_nullable FROM information_schema.columns
		WHERE table_name = 'telegram_chat_context'
	`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	want := map[string]bool{
		"session_id": true, "updatedAt": true, "state": true, "draft_name": true,
		"draft_scope": true, "draft_battery": true, "draft_end_time": true,
		"expires_at": true, "version": true,
	}
	seen := map[string]bool{}
	for rows.Next() {
		var name, nullable string
		if err := rows.Scan(&name, &nullable); err != nil {
			t.Fatal(err)
		}
		if want[name] {
			seen[name] = true
		}
		if name == "session_id" && nullable != "YES" {
			t.Fatalf("session_id is_nullable = %q, want YES", nullable)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	for name := range want {
		if !seen[name] {
			t.Errorf("telegram_chat_context is missing column %q", name)
		}
	}
}

func TestTelegramAdminTierTwoScopeAndCreatorClose(t *testing.T) {
	t.Setenv("TELEGRAM_BOT_USERNAME", "synthetic_bot")
	db, prefix := openTelegramAdminDB(t)
	ctx := context.Background()
	tierTwoID, creatorID := prefix+"-tier-two", prefix+"-creator"
	seedUser(t, db, tierTwoID, "ALPHA NCO", models.Rank3SG, models.BatteryAlpha, "", true)
	seedUser(t, db, creatorID, "UNIT CREATOR", models.RankSSG, models.BatteryAlpha, "", true)
	tierTwoTelegramID, creatorTelegramID := telegramAdminID(prefix, 4), telegramAdminID(prefix, 5)
	seedTelegramAdminPairing(t, db, prefix+"-tier-two-pairing", tierTwoTelegramID, tierTwoID)
	seedTelegramAdminPairing(t, db, prefix+"-creator-pairing", creatorTelegramID, creatorID)

	alphaID, bravoID, unitID := prefix+"-alpha-event", prefix+"-bravo-event", prefix+"-unit-event"
	seedAdminSession(t, db, alphaID, "Alpha event", models.SessionScopeBatterySpecific, []string{models.BatteryAlpha}, tierTwoID, time.Hour)
	seedAdminSession(t, db, bravoID, "Bravo event", models.SessionScopeBatterySpecific, []string{models.BatteryBravo}, creatorID, time.Hour)
	seedAdminSession(t, db, unitID, "Unit event", models.SessionScopeUnitWide, nil, creatorID, time.Hour)
	store := NewTelegramAdminStore(db, nil)

	tierTwo, found, err := store.Actor(ctx, telegram.Pairing{TelegramID: tierTwoTelegramID, UserID: tierTwoID})
	if err != nil || !found {
		t.Fatalf("tier-two Actor = %+v, found=%v, err=%v", tierTwo, found, err)
	}
	events, err := store.ActiveEvents(ctx, tierTwo)
	if err != nil {
		t.Fatal(err)
	}
	if !containsAdminEvent(events, alphaID) || !containsAdminEvent(events, unitID) || containsAdminEvent(events, bravoID) {
		t.Fatalf("tier-two active events = %+v", events)
	}
	if _, err := store.CreateEvent(ctx, tierTwo, telegram.AdminDraft{Name: "forbidden", Scope: models.SessionScopeUnitWide, EndTime: time.Now().Add(time.Hour)}); !errors.Is(err, telegram.ErrAdminUnavailable) {
		t.Fatalf("tier-two CreateEvent = %v, want unavailable", err)
	}
	if err := store.CloseEvent(ctx, tierTwo, alphaID); !errors.Is(err, telegram.ErrAdminUnavailable) {
		t.Fatalf("tier-two CloseEvent = %v, want unavailable", err)
	}

	creator, found, err := store.Actor(ctx, telegram.Pairing{TelegramID: creatorTelegramID, UserID: creatorID})
	if err != nil || !found {
		t.Fatalf("creator Actor = %+v, found=%v, err=%v", creator, found, err)
	}
	created, err := store.CreateEvent(ctx, creator, telegram.AdminDraft{
		Name: "Created through Telegram", Scope: models.SessionScopeBatterySpecific,
		Battery: models.BatteryAlpha, EndTime: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("creator CreateEvent: %v", err)
	}
	if created.ID == "" || created.Scope != models.SessionScopeBatterySpecific || len(created.Batteries) != 1 || created.Batteries[0] != models.BatteryAlpha || !strings.HasPrefix(created.TelegramLink, "https://t.me/synthetic_bot?start=") {
		t.Fatalf("created event = %+v", created)
	}
	if err := store.CloseEvent(ctx, creator, alphaID); !errors.Is(err, telegram.ErrAdminUnavailable) {
		t.Fatalf("creator closing another user's event = %v, want unavailable", err)
	}
	if err := store.CloseEvent(ctx, creator, unitID); err != nil {
		t.Fatalf("creator closing own event: %v", err)
	}
	var status string
	if err := db.Pool.QueryRow(ctx, `SELECT status FROM attendance_session WHERE id = $1`, unitID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != models.SessionStatusClosed {
		t.Fatalf("closed event status = %q", status)
	}
}

func TestTelegramAdminStatusManualMarkUndoAndSSE(t *testing.T) {
	db, prefix := openTelegramAdminDB(t)
	ctx := context.Background()
	commanderID, targetID, outsideID := prefix+"-commander", prefix+"-target", prefix+"-outside"
	seedUser(t, db, commanderID, "ALPHA COMMANDER", models.RankSSG, models.BatteryAlpha, "", true)
	seedUser(t, db, targetID, "ALPHA TARGET", models.RankPTE, models.BatteryAlpha, "", true)
	seedUser(t, db, outsideID, "BRAVO TARGET", models.RankPTE, models.BatteryBravo, "", true)
	telegramID := telegramAdminID(prefix, 6)
	seedTelegramAdminPairing(t, db, prefix+"-commander-pairing", telegramID, commanderID)
	sessionID := prefix + "-scoped-event"
	seedAdminSession(t, db, sessionID, "Scoped event", models.SessionScopeBatterySpecific, []string{models.BatteryAlpha}, commanderID, time.Hour)

	hub := sse.NewHub()
	client := sse.NewClient(prefix+"-sse-client", commanderID)
	hub.Subscribe(sessionID, client)
	t.Cleanup(func() { hub.Unsubscribe(sessionID, client) })
	store := NewTelegramAdminStore(db, hub)
	actor, found, err := store.Actor(ctx, telegram.Pairing{TelegramID: telegramID, UserID: commanderID})
	if err != nil || !found {
		t.Fatalf("Actor = %+v, found=%v, err=%v", actor, found, err)
	}
	page, err := store.Status(ctx, actor, sessionID, "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if page.SessionName != "Scoped event" || page.Total < 2 || page.Present != 0 || page.Missing != page.Total || !containsAdminUser(page.Rows, commanderID) || !containsAdminUser(page.Rows, targetID) {
		t.Fatalf("status before mark = %+v", page)
	}
	if err := store.MarkManual(ctx, actor, sessionID, targetID); err != nil {
		t.Fatalf("MarkManual target: %v", err)
	}
	var method string
	var markedBy *string
	if err := db.Pool.QueryRow(ctx, `SELECT marking_method, marked_by FROM attendance_record WHERE session_id = $1 AND user_id = $2`, sessionID, targetID).Scan(&method, &markedBy); err != nil {
		t.Fatal(err)
	}
	if method != models.MarkingMethodManual || markedBy == nil || *markedBy != commanderID {
		t.Fatalf("manual record = method %q marked_by %v", method, markedBy)
	}
	select {
	case event := <-client.Send:
		if event.Type != sse.EventTypeAttendanceMarked {
			t.Fatalf("mark SSE event = %q", event.Type)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for manual mark SSE event")
	}
	marks, err := store.OwnManualMarks(ctx, actor, sessionID, 1)
	if err != nil || len(marks) != 1 || marks[0].ID != targetID {
		t.Fatalf("own manual marks = %+v, err=%v", marks, err)
	}
	if err := store.MarkManual(ctx, actor, sessionID, outsideID); !errors.Is(err, telegram.ErrAdminUnavailable) {
		t.Fatalf("out-of-scope MarkManual = %v, want unavailable", err)
	}
	if err := store.UndoManual(ctx, actor, sessionID, targetID); err != nil {
		t.Fatalf("UndoManual target: %v", err)
	}
	select {
	case event := <-client.Send:
		if event.Type != sse.EventTypeAttendanceRemoved {
			t.Fatalf("undo SSE event = %q", event.Type)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for manual undo SSE event")
	}
	var count int
	if err := db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM attendance_record WHERE session_id = $1 AND user_id = $2`, sessionID, targetID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("manual record count after undo = %d", count)
	}
}

func TestTelegramAdminClosedSelectedContextIsCleanedAndUnavailable(t *testing.T) {
	db, prefix := openTelegramAdminDB(t)
	ctx := context.Background()
	commanderID := prefix + "-commander"
	telegramID := telegramAdminID(prefix, 7)
	seedUser(t, db, commanderID, "CLOSURE COMMANDER", models.RankSSG, models.BatteryAlpha, "", true)
	seedTelegramAdminPairing(t, db, prefix+"-pairing", telegramID, commanderID)
	sessionID := prefix + "-closed-event"
	seedAdminSession(t, db, sessionID, "Closed event", models.SessionScopeUnitWide, nil, commanderID, -time.Hour)
	if _, err := db.Pool.Exec(ctx, `UPDATE attendance_session SET status = 'closed' WHERE id = $1`, sessionID); err != nil {
		t.Fatal(err)
	}
	store := NewTelegramAdminStore(db, nil)
	if err := store.SaveContext(ctx, telegram.AdminContext{TelegramID: telegramID, SessionID: sessionID, State: "selected_session"}); err != nil {
		t.Fatal(err)
	}
	actor, found, err := store.Actor(ctx, telegram.Pairing{TelegramID: telegramID, UserID: commanderID})
	if err != nil || !found {
		t.Fatalf("Actor = %+v, found=%v, err=%v", actor, found, err)
	}
	if _, err := store.Status(ctx, actor, sessionID, "", 1); !errors.Is(err, telegram.ErrAdminUnavailable) {
		t.Fatalf("closed Status = %v, want unavailable", err)
	}
	loaded, err := store.LoadContext(ctx, telegramID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.SessionID != "" || loaded.State != "idle" {
		t.Fatalf("closed selected context = %+v, want idle/empty", loaded)
	}
}

func openTelegramAdminDB(t *testing.T) (*database.DB, string) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to run the Telegram admin integration test")
	}
	db, err := database.NewPostgresDB(url)
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	prefix := fmt.Sprintf("tg-admin-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = db.Pool.Exec(ctx, `DELETE FROM telegram_chat_context WHERE telegram_id BETWEEN $1 AND $2`, telegramAdminID(prefix, 0), telegramAdminID(prefix, 100))
		_, _ = db.Pool.Exec(ctx, `DELETE FROM attendance_session WHERE id LIKE $1 OR created_by LIKE $1`, prefix+"-%")
		_, _ = db.Pool.Exec(ctx, `DELETE FROM telegram_pairing WHERE id LIKE $1`, prefix+"-%")
		_, _ = db.Pool.Exec(ctx, `DELETE FROM "user" WHERE id LIKE $1`, prefix+"-%")
		db.Close()
	})
	return db, prefix
}

func telegramAdminID(prefix string, offset int64) int64 {
	var value int64
	for _, char := range prefix {
		value = (value*31 + int64(char)) % 900000000000000000
	}
	if value < 1000000000 {
		value += 1000000000
	}
	return value + offset
}

func seedTelegramAdminPairing(t *testing.T, db *database.DB, id string, telegramID int64, userID string) {
	t.Helper()
	if _, err := db.Pool.Exec(context.Background(), `
		INSERT INTO telegram_pairing (id, telegram_id, user_id, display_name, self_confirmed)
		VALUES ($1, $2, $3, 'Synthetic Telegram Admin', true)
	`, id, telegramID, userID); err != nil {
		t.Fatalf("seed Telegram admin pairing: %v", err)
	}
}

func seedAdminSession(t *testing.T, db *database.DB, id, name, scope string, batteries []string, creatorID string, duration time.Duration) {
	t.Helper()
	if batteries == nil {
		batteries = []string{}
	}
	if _, err := db.Pool.Exec(context.Background(), `
		INSERT INTO attendance_session (id, name, qr_code, qr_code_secret, scope, batteries, status, created_by, start_time, end_time, deeplink_code)
		VALUES ($1, $2, $3, $4, $5, $6, 'active', $7, NOW(), $8, NULL)
	`, id, name, id+"-qr", id+"-secret", scope, batteries, creatorID, time.Now().Add(duration)); err != nil {
		t.Fatalf("seed Telegram admin session: %v", err)
	}
}

func ptrTime(value time.Time) *time.Time { return &value }

func containsAdminEvent(events []telegram.ActiveEvent, id string) bool {
	for _, event := range events {
		if event.ID == id {
			return true
		}
	}
	return false
}

func containsAdminUser(users []telegram.AdminUser, id string) bool {
	for _, user := range users {
		if user.ID == id {
			return true
		}
	}
	return false
}
