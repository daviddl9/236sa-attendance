package handlers

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
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
	if loaded.State != "draft" || loaded.Draft.Name != draft.Draft.Name || loaded.Version != 1 {
		t.Fatalf("loaded draft = %+v, want first save at version 1", loaded)
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
	if updated.Version != 2 || updated.State != "choosing_duration" || updated.Draft.Battery != models.BatteryAlpha {
		t.Fatalf("updated context = %+v, want second save at version 2", updated)
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
	if cleared.State != "idle" || cleared.Version != 3 || cleared.SessionID != "" {
		t.Fatalf("cleared context = %+v, want idle tombstone at version 3", cleared)
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

func TestTelegramAdminContextConcurrentInitialSavesAndConditionalClear(t *testing.T) {
	db, prefix := openTelegramAdminDB(t)
	store := NewTelegramAdminStore(db, nil)
	ctx := context.Background()
	telegramID := telegramAdminID(prefix, 20)
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			results <- store.SaveContext(ctx, telegram.AdminContext{
				TelegramID: telegramID, State: fmt.Sprintf("draft-%d", index), Version: 0,
			})
		}(i)
	}
	close(start)
	wg.Wait()
	close(results)
	var successes, conflicts int
	for err := range results {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, telegram.ErrAdminContextConflict):
			conflicts++
		default:
			t.Fatalf("concurrent initial save error = %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent initial saves = successes %d conflicts %d, want 1/1", successes, conflicts)
	}

	sessionID := prefix + "-context-session"
	seedUser(t, db, prefix+"-creator", "CONTEXT CREATOR", models.RankSSG, models.BatteryHQ, "", true)
	seedAdminSession(t, db, sessionID, "Context session", models.SessionScopeUnitWide, nil, prefix+"-creator", time.Hour)
	loaded, err := store.LoadContext(ctx, telegramID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Version != 1 {
		t.Fatalf("concurrent initial save loaded version = %d, want 1", loaded.Version)
	}
	loaded.SessionID = sessionID
	loaded.State = "selected_session"
	if err := store.SaveContext(ctx, loaded); err != nil {
		t.Fatal(err)
	}
	selected, err := store.LoadContext(ctx, telegramID)
	if err != nil {
		t.Fatal(err)
	}
	if selected.Version != 2 {
		t.Fatalf("concurrent initial save update version = %d, want 2", selected.Version)
	}
	if err := store.ClearContext(ctx, telegramID); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveContext(ctx, selected); !errors.Is(err, telegram.ErrAdminContextConflict) {
		t.Fatalf("stale save after clear = %v, want conflict", err)
	}
	if err := store.ClearContextForSession(ctx, telegramID, sessionID, selected.Version); !errors.Is(err, telegram.ErrAdminContextConflict) {
		t.Fatalf("stale conditional clear = %v, want conflict", err)
	}
	current, err := store.LoadContext(ctx, telegramID)
	if err != nil {
		t.Fatal(err)
	}
	if current.SessionID != "" || current.State != "idle" || current.Version != 3 {
		t.Fatalf("cleared tombstone = %+v, want idle version 3", current)
	}

	missingID := telegramAdminID(prefix, 25)
	if err := store.ClearContext(ctx, missingID); err != nil {
		t.Fatal(err)
	}
	tombstone, err := store.LoadContext(ctx, missingID)
	if err != nil {
		t.Fatal(err)
	}
	if tombstone.State != "idle" || tombstone.Version != 1 {
		t.Fatalf("missing-row clear tombstone = %+v, want idle version 1", tombstone)
	}
	if err := store.SaveContext(ctx, telegram.AdminContext{TelegramID: missingID, State: "draft", Version: 0}); !errors.Is(err, telegram.ErrAdminContextConflict) {
		t.Fatalf("save over missing-row tombstone = %v, want conflict", err)
	}
}

func TestTelegramAdminCustomListTierTwoAndSSETotals(t *testing.T) {
	db, prefix := openTelegramAdminDB(t)
	ctx := context.Background()
	actorID := prefix + "-actor"
	alphaID := prefix + "-alpha"
	bravoID := prefix + "-bravo"
	seedUser(t, db, actorID, "ALPHA NCO", models.Rank3SG, models.BatteryAlpha, "", true)
	seedUser(t, db, alphaID, "ALPHA TARGET", models.RankPTE, models.BatteryAlpha, "", true)
	seedUser(t, db, bravoID, "BRAVO TARGET", models.RankPTE, models.BatteryBravo, "", true)
	telegramID := telegramAdminID(prefix, 21)
	seedTelegramAdminPairing(t, db, prefix+"-pairing", telegramID, actorID)
	sessionID := prefix + "-custom-session"
	seedCustomAdminSession(t, db, sessionID, "Custom event", actorID, time.Hour, []string{alphaID, bravoID})
	hub := sse.NewHub()
	client := sse.NewClient(prefix+"-sse", actorID)
	hub.Subscribe(sessionID, client)
	t.Cleanup(func() { hub.Unsubscribe(sessionID, client) })
	store := NewTelegramAdminStore(db, hub)
	actor, found, err := store.Actor(ctx, telegram.Pairing{TelegramID: telegramID, UserID: actorID})
	if err != nil || !found {
		t.Fatalf("Actor = %+v found=%v err=%v", actor, found, err)
	}
	events, err := store.ActiveEvents(ctx, actor)
	if err != nil || !containsAdminEvent(events, sessionID) {
		t.Fatalf("custom active events = %+v err=%v", events, err)
	}
	page, err := store.Status(ctx, actor, sessionID, "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || page.Missing != 1 || !containsAdminUser(page.Rows, alphaID) || containsAdminUser(page.Rows, bravoID) {
		t.Fatalf("Tier 2 custom status = %+v, want only Alpha participant", page)
	}
	if err := store.MarkManual(ctx, actor, sessionID, alphaID); err != nil {
		t.Fatalf("custom Alpha mark: %v", err)
	}
	select {
	case event := <-client.Send:
		payload, ok := event.Payload.(sse.AttendanceMarkedPayload)
		if !ok || payload.UserID != alphaID || payload.UserName != "ALPHA TARGET" || payload.TotalUsers != 1 || payload.PresentCount != 1 {
			t.Fatalf("custom mark SSE = %#v, want preserved metadata and 1/1 totals", event.Payload)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for custom mark SSE")
	}
	if err := store.MarkManual(ctx, actor, sessionID, bravoID); !errors.Is(err, telegram.ErrAdminUnavailable) {
		t.Fatalf("Tier 2 custom Bravo mark = %v, want unavailable", err)
	}
	if err := store.UndoManual(ctx, actor, sessionID, alphaID); err != nil {
		t.Fatalf("custom Alpha undo: %v", err)
	}
	select {
	case event := <-client.Send:
		payload, ok := event.Payload.(sse.AttendanceRemovedPayload)
		if !ok || payload.UserID != alphaID || payload.TotalUsers != 1 || payload.PresentCount != 0 {
			t.Fatalf("custom undo SSE = %#v, want 0/1 totals", event.Payload)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for custom undo SSE")
	}
}

func TestTelegramAdminExpiryAndCloseVsMarkRace(t *testing.T) {
	db, prefix := openTelegramAdminDB(t)
	ctx := context.Background()
	actorID := prefix + "-actor"
	targetID := prefix + "-target"
	seedUser(t, db, actorID, "EXPIRY COMMANDER", models.RankSSG, models.BatteryAlpha, "", true)
	seedUser(t, db, targetID, "EXPIRY TARGET", models.RankPTE, models.BatteryAlpha, "", true)
	telegramID := telegramAdminID(prefix, 23)
	seedTelegramAdminPairing(t, db, prefix+"-pairing", telegramID, actorID)
	store := NewTelegramAdminStore(db, nil)
	actor, found, err := store.Actor(ctx, telegram.Pairing{TelegramID: telegramID, UserID: actorID})
	if err != nil || !found {
		t.Fatalf("Actor = %+v found=%v err=%v", actor, found, err)
	}
	expiredID := prefix + "-expired"
	seedAdminSession(t, db, expiredID, "Expired event", models.SessionScopeUnitWide, nil, actorID, -time.Minute)
	if err := store.MarkManual(ctx, actor, expiredID, targetID); !errors.Is(err, telegram.ErrAdminUnavailable) {
		t.Fatalf("expired mark = %v, want unavailable", err)
	}
	if _, err := store.Status(ctx, actor, expiredID, "", 1); !errors.Is(err, telegram.ErrAdminUnavailable) {
		t.Fatalf("expired status = %v, want unavailable", err)
	}
	if err := store.CloseEvent(ctx, actor, expiredID); !errors.Is(err, telegram.ErrAdminUnavailable) {
		t.Fatalf("expired close = %v, want unavailable", err)
	}

	raceID := prefix + "-race"
	seedAdminSession(t, db, raceID, "Close versus mark", models.SessionScopeUnitWide, nil, actorID, time.Hour)
	start := make(chan struct{})
	markResult := make(chan error, 1)
	closeResult := make(chan error, 1)
	go func() { <-start; markResult <- store.MarkManual(ctx, actor, raceID, targetID) }()
	go func() { <-start; closeResult <- store.CloseEvent(ctx, actor, raceID) }()
	close(start)
	markErr := <-markResult
	closeErr := <-closeResult
	if closeErr != nil {
		t.Fatalf("close-vs-mark close error = %v, want close to win or follow a committed mark", closeErr)
	}
	if markErr != nil && !errors.Is(markErr, telegram.ErrAdminUnavailable) {
		t.Fatalf("close-vs-mark mark error = %v", markErr)
	}
	var status string
	if err := db.Pool.QueryRow(ctx, `SELECT status FROM attendance_session WHERE id = $1`, raceID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != models.SessionStatusClosed {
		t.Fatalf("close-vs-mark status = %q, want closed", status)
	}
}

func TestTelegramAdminTargetBatteryMutationIsRecheckedInWriteTransaction(t *testing.T) {
	db, prefix := openTelegramAdminDB(t)
	ctx := context.Background()
	actorID := prefix + "-actor"
	targetID := prefix + "-target"
	seedUser(t, db, actorID, "BATTERY COMMANDER", models.Rank3SG, models.BatteryAlpha, "", true)
	seedUser(t, db, targetID, "MOVING TARGET", models.RankPTE, models.BatteryAlpha, "", true)
	telegramID := telegramAdminID(prefix, 24)
	seedTelegramAdminPairing(t, db, prefix+"-pairing", telegramID, actorID)
	sessionID := prefix + "-battery-race"
	seedAdminSession(t, db, sessionID, "Battery race", models.SessionScopeUnitWide, nil, actorID, time.Hour)
	store := NewTelegramAdminStore(db, nil)
	actor, found, err := store.Actor(ctx, telegram.Pairing{TelegramID: telegramID, UserID: actorID})
	if err != nil || !found {
		t.Fatalf("Actor = %+v found=%v err=%v", actor, found, err)
	}
	moveTx, err := db.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := moveTx.Exec(ctx, `UPDATE "user" SET battery = $2 WHERE id = $1`, targetID, models.BatteryBravo); err != nil {
		_ = moveTx.Rollback(ctx)
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() { result <- store.MarkManual(ctx, actor, sessionID, targetID) }()
	// The roster move is uncommitted while the admin operation starts. Whether
	// the operation reaches the target lock before or after this commit, its
	// row lock must make it evaluate the committed Bravo state rather than the
	// stale Alpha lookup.
	if err := moveTx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-result; !errors.Is(err, telegram.ErrAdminUnavailable) {
		t.Fatalf("mark after target battery move = %v, want unavailable", err)
	}
	var count int
	if err := db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM attendance_record WHERE session_id = $1 AND user_id = $2`, sessionID, targetID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("stale target was marked after battery move: %d records", count)
	}
}

func TestTelegramAdminSuperadminClosesAnotherCreatorEvent(t *testing.T) {
	db, prefix := openTelegramAdminDB(t)
	ctx := context.Background()
	creatorID := prefix + "-creator"
	superID := prefix + "-super"
	seedUser(t, db, creatorID, "CREATOR", models.RankSSG, models.BatteryAlpha, "", true)
	seedUser(t, db, superID, "SUPERADMIN", models.RankCPT, models.BatteryHQ, "", true)
	if _, err := db.Pool.Exec(ctx, `UPDATE "user" SET is_superadmin = true WHERE id = $1`, superID); err != nil {
		t.Fatal(err)
	}
	telegramID := telegramAdminID(prefix, 22)
	seedTelegramAdminPairing(t, db, prefix+"-pairing", telegramID, superID)
	sessionID := prefix + "-other-creator-event"
	seedAdminSession(t, db, sessionID, "Other creator event", models.SessionScopeUnitWide, nil, creatorID, time.Hour)
	store := NewTelegramAdminStore(db, nil)
	actor, found, err := store.Actor(ctx, telegram.Pairing{TelegramID: telegramID, UserID: superID})
	if err != nil || !found {
		t.Fatalf("superadmin Actor = %+v found=%v err=%v", actor, found, err)
	}
	if err := store.CloseEvent(ctx, actor, sessionID); err != nil {
		t.Fatalf("superadmin close = %v", err)
	}
	var status string
	if err := db.Pool.QueryRow(ctx, `SELECT status FROM attendance_session WHERE id = $1`, sessionID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != models.SessionStatusClosed {
		t.Fatalf("superadmin closed status = %q", status)
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
		if _, err := db.Pool.Exec(ctx, `DELETE FROM telegram_chat_context WHERE telegram_id BETWEEN $1 AND $2`, telegramAdminID(prefix, 0), telegramAdminID(prefix, 100)); err != nil {
			t.Errorf("cleanup Telegram admin contexts: %v", err)
		}
		if _, err := db.Pool.Exec(ctx, `DELETE FROM attendance_session WHERE id LIKE $1 OR created_by LIKE $1`, prefix+"-%"); err != nil {
			t.Errorf("cleanup Telegram admin sessions: %v", err)
		}
		if _, err := db.Pool.Exec(ctx, `DELETE FROM telegram_pairing WHERE id LIKE $1`, prefix+"-%"); err != nil {
			t.Errorf("cleanup Telegram admin pairings: %v", err)
		}
		if _, err := db.Pool.Exec(ctx, `DELETE FROM "user" WHERE id LIKE $1`, prefix+"-%"); err != nil {
			t.Errorf("cleanup Telegram admin users: %v", err)
		}
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

func seedCustomAdminSession(t *testing.T, db *database.DB, id, name, creatorID string, duration time.Duration, participants []string) {
	t.Helper()
	if _, err := db.Pool.Exec(context.Background(), `
		INSERT INTO attendance_session (id, name, qr_code, qr_code_secret, scope, batteries, status, created_by, start_time, end_time, deeplink_code)
		VALUES ($1, $2, $3, $4, $5, '{}', 'active', $6, NOW(), $7, NULL)
	`, id, name, id+"-qr", id+"-secret", models.SessionScopeCustomList, creatorID, time.Now().Add(duration)); err != nil {
		t.Fatalf("seed custom Telegram admin session: %v", err)
	}
	for _, participantID := range participants {
		if _, err := db.Pool.Exec(context.Background(), `
			INSERT INTO session_participants (session_id, user_id) VALUES ($1, $2)
		`, id, participantID); err != nil {
			t.Fatalf("seed custom participant %s: %v", participantID, err)
		}
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
