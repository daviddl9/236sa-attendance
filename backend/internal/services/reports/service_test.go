package reports

import (
	"context"
	"fmt"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/davidlivingston/go-nextjs-starter/backend/internal/database"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/models"
)

func TestSummaryAndMissingApplySessionAndBatteryScope(t *testing.T) {
	db, prefix := openReportsServiceDB(t)
	actorID := prefix + "-actor"
	seedReportUser(t, db, actorID, "COMMANDER", "3SG", "Alpha", false)
	alphaPresent := prefix + "-alpha-present"
	alphaMissing := prefix + "-alpha-missing"
	bravo := prefix + "-bravo"
	for _, person := range []struct{ id, name, battery string }{
		{alphaPresent, "Alice Present", models.BatteryAlpha},
		{alphaMissing, "Alicia Missing", models.BatteryAlpha},
		{bravo, "Bruno Bravo", models.BatteryBravo},
	} {
		seedReportUser(t, db, person.id, person.name, "PTE", person.battery, false)
	}
	sessionID := prefix + "-session"
	seedReportSession(t, db, sessionID, models.SessionScopeCustomList, nil, actorID)
	seedReportParticipant(t, db, sessionID, alphaPresent)
	seedReportParticipant(t, db, sessionID, alphaMissing)
	seedReportParticipant(t, db, sessionID, bravo)
	seedReportRecord(t, db, sessionID, alphaPresent, "qr_scan", "")

	svc := NewService(db)
	actor := &models.User{ID: actorID, Rank: stringPtr(models.Rank3SG), Battery: stringPtr(models.BatteryAlpha)}
	summary, err := svc.Summary(context.Background(), sessionID, actor)
	if err != nil {
		t.Fatalf("Summary() error = %v", err)
	}
	if summary.SessionName != sessionID+" name" || summary.Total != 2 || summary.Present != 1 || summary.Missing != 1 || summary.Percentage != 50 {
		t.Fatalf("summary = %+v, want total 2/present 1/missing 1/50%%", summary)
	}

	page, err := svc.Missing(context.Background(), sessionID, actor, "ali", 1, 50)
	if err != nil {
		t.Fatalf("Missing() error = %v", err)
	}
	if page.Total != 1 || page.PageCount != 1 || len(page.Rows) != 1 || page.Rows[0].ID != alphaMissing {
		t.Fatalf("missing page = %+v, want only authorized missing Alpha row", page)
	}
	for _, row := range page.Rows {
		if row.ID == bravo {
			t.Fatalf("unauthorized Bravo row leaked: %+v", row)
		}
	}
}

func TestMissingPaginatesSearchResultsDeterministicallyAndClampsPageSize(t *testing.T) {
	db, prefix := openReportsServiceDB(t)
	actorID := prefix + "-actor"
	seedReportUser(t, db, actorID, "COMMANDER", "3SG", "Alpha", false)
	sessionID := prefix + "-session"
	seedReportSession(t, db, sessionID, models.SessionScopeUnitWide, nil, actorID)

	for i := 0; i < 12; i++ {
		id := fmt.Sprintf("%s-target-%02d", prefix, i)
		seedReportUser(t, db, id, fmt.Sprintf("Target %02d", i), "PTE", models.BatteryAlpha, false)
	}
	actor := &models.User{ID: actorID, Rank: stringPtr(models.Rank3SG), Battery: stringPtr(models.BatteryAlpha)}
	svc := NewService(db)

	first, err := svc.Missing(context.Background(), sessionID, actor, "target", 1, 100)
	if err != nil {
		t.Fatalf("first Missing() error = %v", err)
	}
	if first.Page != 1 || first.PageCount != 2 || first.Total != 12 || len(first.Rows) != 10 || !first.HasNext || first.HasPrevious {
		t.Fatalf("first page = %+v, want 10-row page 1 of 2", first)
	}
	if got := rowIDs(first.Rows); !sort.StringsAreSorted(got) {
		t.Fatalf("first page IDs are not deterministic by name/ID: %v", got)
	}
	second, err := svc.Missing(context.Background(), sessionID, actor, "target", 2, 100)
	if err != nil {
		t.Fatalf("second Missing() error = %v", err)
	}
	if second.Page != 2 || len(second.Rows) != 2 || !second.HasPrevious || second.HasNext {
		t.Fatalf("second page = %+v, want 2-row page 2", second)
	}
}

func TestSuperadminAppearsInRosterOnlyWhenMarked(t *testing.T) {
	db, prefix := openReportsServiceDB(t)
	actorID := prefix + "-actor"
	seedReportUser(t, db, actorID, "COMMANDER", "3SG", "Alpha", false)
	markedAdmin := prefix + "-marked-admin"
	unmarkedAdmin := prefix + "-unmarked-admin"
	regular := prefix + "-regular"
	seedReportUser(t, db, markedAdmin, "Marked Admin", "CPT", "Alpha", true)
	seedReportUser(t, db, unmarkedAdmin, "Unmarked Admin", "CPT", "Alpha", true)
	seedReportUser(t, db, regular, "Regular Soldier", "PTE", "Alpha", false)

	sessionID := prefix + "-session"
	seedReportSession(t, db, sessionID, models.SessionScopeUnitWide, nil, actorID)
	seedReportRecord(t, db, sessionID, markedAdmin, "qr_scan", "")

	svc := NewService(db)
	actor := &models.User{ID: actorID, Rank: stringPtr(models.Rank3SG), Battery: stringPtr(models.BatteryAlpha)}

	roster, err := svc.EligibleUsers(context.Background(), sessionID, actor)
	if err != nil {
		t.Fatalf("EligibleUsers() error = %v", err)
	}
	ids := rowIDs(roster)
	if !containsString(ids, markedAdmin) {
		t.Fatalf("marked superadmin %s missing from roster: %v", markedAdmin, ids)
	}
	if containsString(ids, unmarkedAdmin) {
		t.Fatalf("unmarked superadmin %s should not be in roster: %v", unmarkedAdmin, ids)
	}
	if !containsString(ids, regular) {
		t.Fatalf("regular user %s missing from roster: %v", regular, ids)
	}

	summary, err := svc.Summary(context.Background(), sessionID, actor)
	if err != nil {
		t.Fatalf("Summary() error = %v", err)
	}
	// The marked superadmin is counted as present; the unmarked one is not in
	// the roster at all. Absolute totals are not asserted because other test
	// packages share this database and may seed additional Alpha users.
	if summary.Present != 1 {
		t.Fatalf("summary.Present = %d, want 1 (only the marked superadmin has a record)", summary.Present)
	}
	if summary.Missing != summary.Total-1 {
		t.Fatalf("summary = %+v, want Missing = Total-1", summary)
	}
}

func TestSummaryCountsWalkInsAsPresentExtrasNeverMissing(t *testing.T) {
	db, prefix := openReportsServiceDB(t)
	actorID := prefix + "-actor"
	seedReportUser(t, db, actorID, "COMMANDER", "3SG", "Alpha", false)
	present := prefix + "-present"
	missing := prefix + "-missing"
	walkIn := prefix + "-walk-in"
	seedReportUser(t, db, present, "Alice Present", "PTE", models.BatteryAlpha, false)
	seedReportUser(t, db, missing, "Alicia Missing", "PTE", models.BatteryAlpha, false)
	seedReportUser(t, db, walkIn, "Walter Walkin", "PTE", models.BatteryBravo, false)

	sessionID := prefix + "-session"
	seedReportSession(t, db, sessionID, models.SessionScopeCustomList, nil, actorID)
	seedReportParticipant(t, db, sessionID, present)
	seedReportParticipant(t, db, sessionID, missing)
	seedReportRecord(t, db, sessionID, present, "qr_scan", "")
	seedReportRecord(t, db, sessionID, walkIn, "telegram_scan", "")

	svc := NewService(db)
	actor := &models.User{ID: actorID, IsSuperadmin: true}

	// Roster is 2 (present + missing); the walk-in inflates the total and
	// counts as present.
	summary, err := svc.Summary(context.Background(), sessionID, actor)
	if err != nil {
		t.Fatalf("Summary() error = %v", err)
	}
	if summary.Total != 3 || summary.Present != 2 || summary.Missing != 1 {
		t.Fatalf("summary = %+v, want total 3/present 2/missing 1", summary)
	}

	// The walk-in is never absent.
	page, err := svc.Missing(context.Background(), sessionID, actor, "", 1, 10)
	if err != nil {
		t.Fatalf("Missing() error = %v", err)
	}
	if page.Total != 1 || len(page.Rows) != 1 || page.Rows[0].ID != missing {
		t.Fatalf("missing page = %+v, want only the roster missing user", page)
	}

	// Extras returns exactly the walk-in for an unrestricted actor.
	extras, err := svc.Extras(context.Background(), sessionID, actor)
	if err != nil {
		t.Fatalf("Extras() error = %v", err)
	}
	if len(extras) != 1 || extras[0].ID != walkIn {
		t.Fatalf("extras = %+v, want only the walk-in", extras)
	}

	// A battery-restricted Alpha actor never sees the Bravo walk-in.
	restricted := &models.User{ID: actorID, Rank: stringPtr(models.Rank3SG), Battery: stringPtr(models.BatteryAlpha)}
	restrictedExtras, err := svc.Extras(context.Background(), sessionID, restricted)
	if err != nil {
		t.Fatalf("Extras() restricted error = %v", err)
	}
	if len(restrictedExtras) != 0 {
		t.Fatalf("restricted extras = %+v, want none (walk-in is Bravo)", restrictedExtras)
	}
	restrictedSummary, err := svc.Summary(context.Background(), sessionID, restricted)
	if err != nil {
		t.Fatalf("Summary() restricted error = %v", err)
	}
	if restrictedSummary.Total != 2 || restrictedSummary.Present != 1 || restrictedSummary.Missing != 1 {
		t.Fatalf("restricted summary = %+v, want total 2/present 1/missing 1", restrictedSummary)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func openReportsServiceDB(t *testing.T) (*database.DB, string) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to run the reports service integration test")
	}
	db, err := database.NewPostgresDB(url)
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	prefix := fmt.Sprintf("reports-service-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = db.Pool.Exec(ctx, `DELETE FROM attendance_session WHERE id LIKE $1`, prefix+"-%")
		_, _ = db.Pool.Exec(ctx, `DELETE FROM "user" WHERE id LIKE $1`, prefix+"-%")
		db.Close()
	})
	return db, prefix
}

func seedReportUser(t *testing.T, db *database.DB, id, name, rank, battery string, superadmin bool) {
	t.Helper()
	_, err := db.Pool.Exec(context.Background(), `
		INSERT INTO "user" (id, "full_name", rank, battery, password, extras, "is_superadmin", verified, "createdAt", "updatedAt")
		VALUES ($1, $2, $3, $4, 'test', '{}'::jsonb, $5, true, NOW(), NOW())
	`, id, name, rank, battery, superadmin)
	if err != nil {
		t.Fatalf("insert user %s: %v", id, err)
	}
}

func seedReportSession(t *testing.T, db *database.DB, id, scope string, batteries []string, creatorID string) {
	t.Helper()
	if batteries == nil {
		batteries = []string{}
	}
	_, err := db.Pool.Exec(context.Background(), `
		INSERT INTO attendance_session (id, name, qr_code, qr_code_secret, scope, batteries, status, created_by, start_time)
		VALUES ($1, $2, $3, $4, $5, $6, 'active', $7, NOW())
	`, id, id+" name", id+" qr", id+" secret", scope, batteries, creatorID)
	if err != nil {
		t.Fatalf("insert session %s: %v", id, err)
	}
}

func seedReportParticipant(t *testing.T, db *database.DB, sessionID, userID string) {
	t.Helper()
	if _, err := db.Pool.Exec(context.Background(), `
		INSERT INTO session_participants (session_id, user_id) VALUES ($1, $2)
	`, sessionID, userID); err != nil {
		t.Fatalf("insert session participant: %v", err)
	}
}

func seedReportRecord(t *testing.T, db *database.DB, sessionID, userID, method, markedBy string) {
	t.Helper()
	var markedByValue *string
	if markedBy != "" {
		markedByValue = &markedBy
	}
	_, err := db.Pool.Exec(context.Background(), `
		INSERT INTO attendance_record (id, session_id, user_id, marked_at, marking_method, marked_by, "createdAt", "updatedAt")
		VALUES ($1, $2, $3, NOW(), $4, $5, NOW(), NOW())
	`, sessionID+"-"+userID, sessionID, userID, method, markedByValue)
	if err != nil {
		t.Fatalf("insert attendance record: %v", err)
	}
}

func rowIDs(rows []UserRow) []string {
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	return ids
}

func stringPtr(value string) *string { return &value }
