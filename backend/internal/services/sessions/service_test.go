package sessions

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/davidlivingston/go-nextjs-starter/backend/internal/database"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/models"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/services/deeplink"
)

func TestCreateValidatesScopesAndGeneratesCapabilities(t *testing.T) {
	db, prefix := openSessionServiceDB(t)
	creatorID := prefix + "-creator"
	seedSessionUser(t, db, creatorID, "3SG", "Alpha", false)

	svc := NewService(db, "@attendance_bot")
	for _, req := range []CreateRequest{
		{Name: "bad scope", Scope: "custom_list", CreatedBy: creatorID},
		{Name: "bad battery", Scope: models.SessionScopeBatterySpecific, Batteries: []string{"Delta"}, CreatedBy: creatorID},
		{Name: "missing battery", Scope: models.SessionScopeBatterySpecific, CreatedBy: creatorID},
	} {
		if _, err := svc.Create(context.Background(), req); err == nil {
			t.Fatalf("Create(%+v) succeeded; want validation error", req)
		}
	}

	session, err := svc.Create(context.Background(), CreateRequest{
		Name:      "Morning parade",
		Scope:     models.SessionScopeBatterySpecific,
		Batteries: []string{models.BatteryAlpha},
		EndTime:   timePtr(time.Now().Add(time.Hour)),
		CreatedBy: creatorID,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if session.ID == "" || session.QRCodeSecret == "" || session.DeepLinkCode == "" {
		t.Fatalf("Create() did not return generated capabilities: %+v", session)
	}
	if !deeplink.IsValidCode(session.DeepLinkCode) {
		t.Fatalf("deep-link code %q is invalid", session.DeepLinkCode)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(session.DeepLinkCode)
	if err != nil || len(decoded) != 16 {
		t.Fatalf("deep-link code is not 128-bit base64url: %q (%v)", session.DeepLinkCode, err)
	}
	if got, want := session.TelegramLink, "https://t.me/attendance_bot?start="+session.DeepLinkCode; got != want {
		t.Fatalf("TelegramLink = %q, want %q", got, want)
	}
	if !strings.HasPrefix(session.QRCode, session.ID+":") {
		t.Fatalf("QRCode = %q, want session ID prefix", session.QRCode)
	}

	var count int
	if err := db.Pool.QueryRow(context.Background(), `SELECT count(*) FROM attendance_session WHERE id = $1`, session.ID).Scan(&count); err != nil {
		t.Fatalf("count created session: %v", err)
	}
	if count != 1 {
		t.Fatalf("created session rows = %d, want 1", count)
	}
}

func TestListActiveRestrictsBatteryNCO(t *testing.T) {
	db, prefix := openSessionServiceDB(t)
	creatorID := prefix + "-creator"
	seedSessionUser(t, db, creatorID, "SSG", "HQ", false)
	seedSessionUser(t, db, prefix+"-alpha", "3SG", "Alpha", false)

	seedSessionRow(t, db, prefix+"-unit", models.SessionScopeUnitWide, nil, models.SessionStatusActive, creatorID)
	seedSessionRow(t, db, prefix+"-alpha-session", models.SessionScopeBatterySpecific, []string{models.BatteryAlpha}, models.SessionStatusActive, creatorID)
	seedSessionRow(t, db, prefix+"-bravo-session", models.SessionScopeBatterySpecific, []string{models.BatteryBravo}, models.SessionStatusActive, creatorID)
	seedSessionRow(t, db, prefix+"-closed", models.SessionScopeBatterySpecific, []string{models.BatteryAlpha}, models.SessionStatusClosed, creatorID)

	svc := NewService(db, "bot")
	actor := &models.User{ID: prefix + "-alpha", Rank: stringPtr(models.Rank3SG), Battery: stringPtr(models.BatteryAlpha)}
	active, err := svc.ListActive(context.Background(), actor)
	if err != nil {
		t.Fatalf("ListActive() error = %v", err)
	}
	got := filterSessionIDs(sessionIDs(active), prefix)
	if !sameStrings(got, []string{prefix + "-alpha-session", prefix + "-unit"}) {
		t.Fatalf("Tier 2 active sessions = %v, want unit + own battery", got)
	}

	unitActor := &models.User{ID: creatorID, Rank: stringPtr(models.RankSSG), Battery: stringPtr(models.BatteryHQ)}
	all, err := svc.ListActive(context.Background(), unitActor)
	if err != nil {
		t.Fatalf("ListActive(unit commander) error = %v", err)
	}
	if got := len(filterSessionIDs(sessionIDs(all), prefix)); got != 3 {
		t.Fatalf("Tier 3 active session count = %d, want 3", got)
	}
}

func TestCloseEnforcesOwnershipAndOnlyClosesActiveRows(t *testing.T) {
	db, prefix := openSessionServiceDB(t)
	creatorID := prefix + "-creator"
	otherID := prefix + "-other"
	adminID := prefix + "-admin"
	seedSessionUser(t, db, creatorID, "SSG", "HQ", false)
	seedSessionUser(t, db, otherID, "SSG", "HQ", false)
	seedSessionUser(t, db, adminID, "CPT", "HQ", true)
	sessionID := prefix + "-session"
	seedSessionRow(t, db, sessionID, models.SessionScopeUnitWide, nil, models.SessionStatusActive, creatorID)

	svc := NewService(db, "bot")
	if err := svc.Close(context.Background(), sessionID, &models.User{ID: otherID, Rank: stringPtr(models.RankSSG)}); err == nil {
		t.Fatal("Close() by another commander succeeded; want ownership error")
	}
	if err := svc.Close(context.Background(), sessionID, &models.User{ID: creatorID, Rank: stringPtr(models.RankSSG)}); err != nil {
		t.Fatalf("Close() by creator error = %v", err)
	}
	if err := svc.Close(context.Background(), sessionID, &models.User{ID: adminID, IsSuperadmin: true}); err == nil {
		t.Fatal("second Close() succeeded; want inactive-row error")
	}

	var status string
	if err := db.Pool.QueryRow(context.Background(), `SELECT status FROM attendance_session WHERE id = $1`, sessionID).Scan(&status); err != nil {
		t.Fatalf("read closed status: %v", err)
	}
	if status != models.SessionStatusClosed {
		t.Fatalf("status = %q, want closed", status)
	}
}

func TestListActiveExcludesCustomSessionWithOnlyUnverifiedParticipant(t *testing.T) {
	db, prefix := openSessionServiceDB(t)
	creatorID := prefix + "-creator"
	actorID := prefix + "-actor"
	participantID := prefix + "-unverified"
	sessionID := prefix + "-custom"
	seedSessionUser(t, db, creatorID, models.RankSSG, models.BatteryHQ, false)
	seedSessionUser(t, db, actorID, models.Rank3SG, models.BatteryAlpha, false)
	seedSessionUserVerified(t, db, participantID, models.RankPTE, models.BatteryAlpha, false, false)
	seedSessionRow(t, db, sessionID, models.SessionScopeCustomList, nil, models.SessionStatusActive, creatorID)
	if _, err := db.Pool.Exec(context.Background(), `
		INSERT INTO session_participants (session_id, user_id) VALUES ($1, $2)
	`, sessionID, participantID); err != nil {
		t.Fatalf("insert unverified participant: %v", err)
	}

	actor := &models.User{ID: actorID, Rank: stringPtr(models.Rank3SG), Battery: stringPtr(models.BatteryAlpha)}
	active, err := NewService(db, "bot").ListActive(context.Background(), actor)
	if err != nil {
		t.Fatalf("ListActive() error = %v", err)
	}
	for _, session := range active {
		if session.ID == sessionID {
			t.Fatalf("custom session with only unverified participant was listed: %+v", session)
		}
	}
}

func TestCloseConcurrentOnlyOneCallClosesSession(t *testing.T) {
	db, prefix := openSessionServiceDB(t)
	creatorID := prefix + "-creator"
	sessionID := prefix + "-session"
	seedSessionUser(t, db, creatorID, models.RankSSG, models.BatteryHQ, false)
	seedSessionRow(t, db, sessionID, models.SessionScopeUnitWide, nil, models.SessionStatusActive, creatorID)

	svc := NewService(db, "bot")
	actor := &models.User{ID: creatorID, Rank: stringPtr(models.RankSSG)}
	const callers = 2
	start := make(chan struct{})
	results := make(chan error, callers)
	var wg sync.WaitGroup
	wg.Add(callers)
	for i := 0; i < callers; i++ {
		go func() {
			defer wg.Done()
			<-start
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			results <- svc.Close(ctx, sessionID, actor)
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	var successes, closed int
	for err := range results {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrSessionClosed):
			closed++
		default:
			t.Fatalf("concurrent Close() error = %v, want inactive/closed outcome", err)
		}
	}
	if successes != 1 || closed != 1 {
		t.Fatalf("concurrent Close() outcomes = (%d success, %d closed), want (1, 1)", successes, closed)
	}

	var status string
	if err := db.Pool.QueryRow(context.Background(), `SELECT status FROM attendance_session WHERE id = $1`, sessionID).Scan(&status); err != nil {
		t.Fatalf("read concurrent close status: %v", err)
	}
	if status != models.SessionStatusClosed {
		t.Fatalf("status = %q, want closed", status)
	}
}

func openSessionServiceDB(t *testing.T) (*database.DB, string) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to run the sessions service integration test")
	}
	db, err := database.NewPostgresDB(url)
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	prefix := fmt.Sprintf("sessions-service-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = db.Pool.Exec(ctx, `DELETE FROM attendance_session WHERE id LIKE $1`, prefix+"-%")
		_, _ = db.Pool.Exec(ctx, `DELETE FROM "user" WHERE id LIKE $1`, prefix+"-%")
		db.Close()
	})
	return db, prefix
}

func seedSessionUser(t *testing.T, db *database.DB, id, rank, battery string, superadmin bool) {
	seedSessionUserVerified(t, db, id, rank, battery, superadmin, true)
}

func seedSessionUserVerified(t *testing.T, db *database.DB, id, rank, battery string, superadmin, verified bool) {
	t.Helper()
	_, err := db.Pool.Exec(context.Background(), `
		INSERT INTO "user" (id, "full_name", rank, battery, password, extras, "is_superadmin", verified, "createdAt", "updatedAt")
		VALUES ($1, $2, $3, $4, 'test', '{}'::jsonb, $5, $6, NOW(), NOW())
	`, id, id+" name", rank, battery, superadmin, verified)
	if err != nil {
		t.Fatalf("insert user %s: %v", id, err)
	}
}

func seedSessionRow(t *testing.T, db *database.DB, id, scope string, batteries []string, status, creatorID string) {
	t.Helper()
	if batteries == nil {
		batteries = []string{}
	}
	_, err := db.Pool.Exec(context.Background(), `
		INSERT INTO attendance_session (id, name, qr_code, qr_code_secret, scope, batteries, status, created_by, start_time)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
	`, id, id+" name", id+" qr", id+" secret", scope, batteries, status, creatorID)
	if err != nil {
		t.Fatalf("insert session %s: %v", id, err)
	}
}

func timePtr(v time.Time) *time.Time { return &v }
func stringPtr(v string) *string     { return &v }

func sessionIDs(sessions []models.AttendanceSession) []string {
	ids := make([]string, 0, len(sessions))
	for _, session := range sessions {
		ids = append(ids, session.ID)
	}
	return ids
}

func filterSessionIDs(ids []string, prefix string) []string {
	filtered := make([]string, 0)
	for _, id := range ids {
		if strings.HasPrefix(id, prefix+"-") {
			filtered = append(filtered, id)
		}
	}
	return filtered
}

func sameStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
