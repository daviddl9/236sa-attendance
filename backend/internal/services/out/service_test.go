package out

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/davidlivingston/go-nextjs-starter/backend/internal/database"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/models"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/services/attendance"
	sessionservice "github.com/davidlivingston/go-nextjs-starter/backend/internal/services/sessions"
)

func TestCreatePersistsOutSessionShape(t *testing.T) {
	db, prefix := openOutServiceDB(t)
	creatorID := prefix + "-creator"
	seedUser(t, db, creatorID, "SSG", models.BatteryAlpha, true)

	svc := NewService(db)
	session, err := svc.Create(context.Background(), CreateRequest{
		Name:      "Nights Out 21 Aug",
		Subtype:   models.OutSubtypeNightsOut,
		CreatedBy: creatorID,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if session.ID == "" || session.QRCodeSecret == "" {
		t.Fatalf("Create() returned incomplete session: %+v", session)
	}
	if session.Status != models.SessionStatusActive {
		t.Fatalf("status = %q; want active", session.Status)
	}
	if session.SubtypeLabel != "Nights Out" {
		t.Fatalf("subtype label = %q; want %q", session.SubtypeLabel, "Nights Out")
	}

	// The stored row must be an Out session with no end time: an Out session
	// that carried one could be closed by the expiry path overnight.
	var sessionType, subtype, scope string
	var endTime *time.Time
	var deeplinkCode *string
	err = db.Pool.QueryRow(context.Background(), `
		SELECT session_type, out_subtype, scope, end_time, deeplink_code
		FROM attendance_session WHERE id = $1
	`, session.ID).Scan(&sessionType, &subtype, &scope, &endTime, &deeplinkCode)
	if err != nil {
		t.Fatalf("read back session: %v", err)
	}
	if sessionType != models.SessionTypeOut {
		t.Fatalf("session_type = %q; want %q", sessionType, models.SessionTypeOut)
	}
	if subtype != models.OutSubtypeNightsOut {
		t.Fatalf("out_subtype = %q; want %q", subtype, models.OutSubtypeNightsOut)
	}
	if scope != models.SessionScopeUnitWide {
		t.Fatalf("scope = %q; want %q", scope, models.SessionScopeUnitWide)
	}
	if endTime != nil {
		t.Fatalf("end_time = %v; want NULL so the session survives overnight", endTime)
	}
	if deeplinkCode != nil {
		t.Fatalf("deeplink_code = %v; want NULL, Telegram is not part of this flow", *deeplinkCode)
	}
}

func TestCreateAppliesSubtypeDefaultReturnTime(t *testing.T) {
	db, prefix := openOutServiceDB(t)
	creatorID := prefix + "-creator"
	seedUser(t, db, creatorID, "SSG", models.BatteryAlpha, true)
	svc := NewService(db)

	for _, subtype := range models.ValidOutSubtypes {
		before := time.Now()
		session, err := svc.Create(context.Background(), CreateRequest{
			Name:      "Default " + subtype,
			Subtype:   subtype,
			CreatedBy: creatorID,
		})
		if err != nil {
			t.Fatalf("Create(%q) error = %v", subtype, err)
		}
		want := DefaultReturnAt(subtype, before)
		if !session.ExpectedReturnAt.Equal(want) {
			t.Fatalf("%s expected return = %s; want %s", subtype,
				session.ExpectedReturnAt.Format(time.RFC3339), want.Format(time.RFC3339))
		}
		if !session.ExpectedReturnAt.After(time.Now()) {
			t.Fatalf("%s expected return is not in the future", subtype)
		}
	}
}

func TestCreateHonoursExplicitReturnTime(t *testing.T) {
	db, prefix := openOutServiceDB(t)
	creatorID := prefix + "-creator"
	seedUser(t, db, creatorID, "SSG", models.BatteryAlpha, true)

	chosen := time.Now().Add(9 * time.Hour).Truncate(time.Second)
	session, err := NewService(db).Create(context.Background(), CreateRequest{
		Name:             "Stay Out with override",
		Subtype:          models.OutSubtypeStayOut,
		ExpectedReturnAt: &chosen,
		CreatedBy:        creatorID,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if !session.ExpectedReturnAt.Equal(chosen) {
		t.Fatalf("expected return = %s; want the override %s",
			session.ExpectedReturnAt.Format(time.RFC3339), chosen.Format(time.RFC3339))
	}
}

func TestCreateRejectsInvalidRequests(t *testing.T) {
	db, prefix := openOutServiceDB(t)
	creatorID := prefix + "-creator"
	seedUser(t, db, creatorID, "SSG", models.BatteryAlpha, true)
	svc := NewService(db)

	past := time.Now().Add(-time.Hour)
	for _, tc := range []struct {
		name string
		req  CreateRequest
	}{
		{"missing creator", CreateRequest{Name: "x", Subtype: models.OutSubtypeStayOut}},
		{"missing name", CreateRequest{Subtype: models.OutSubtypeStayOut, CreatedBy: creatorID}},
		{"blank name", CreateRequest{Name: "   ", Subtype: models.OutSubtypeStayOut, CreatedBy: creatorID}},
		{"missing subtype", CreateRequest{Name: "x", CreatedBy: creatorID}},
		{"unknown subtype", CreateRequest{Name: "x", Subtype: "weekend", CreatedBy: creatorID}},
		{"attendance type as subtype", CreateRequest{Name: "x", Subtype: "attendance", CreatedBy: creatorID}},
		{"return time in the past", CreateRequest{
			Name: "x", Subtype: models.OutSubtypeStayOut,
			ExpectedReturnAt: &past, CreatedBy: creatorID,
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := svc.Create(context.Background(), tc.req); !errors.Is(err, ErrInvalidRequest) {
				t.Fatalf("Create(%+v) error = %v; want ErrInvalidRequest", tc.req, err)
			}
		})
	}
}

// The database, not just the service, must refuse an Out session that carries
// an end time — that constraint is what guarantees a Stay Out session cannot
// be expired overnight by a future code path.
func TestSchemaRejectsOutSessionWithEndTime(t *testing.T) {
	db, prefix := openOutServiceDB(t)
	creatorID := prefix + "-creator"
	seedUser(t, db, creatorID, "SSG", models.BatteryAlpha, true)

	_, err := db.Pool.Exec(context.Background(), `
		INSERT INTO attendance_session (
			id, name, qr_code, qr_code_secret, scope, batteries, status,
			created_by, start_time, end_time, session_type, out_subtype,
			expected_return_at, "createdAt", "updatedAt"
		) VALUES ($1, 'bad', $2, 'secret', 'unit_wide', '{}', 'active',
		          $3, NOW(), NOW() + interval '1 hour', 'out', 'stay_out',
		          NOW() + interval '9 hours', NOW(), NOW())
	`, prefix+"-bad", prefix+"-bad-qr", creatorID)
	if err == nil {
		t.Fatal("insert of an Out session with an end_time succeeded; want a constraint violation")
	}
}

// An Out session lives in attendance_session, so every ordinary attendance
// path must continue to ignore it.
func TestOutSessionIsInvisibleToAttendanceFlows(t *testing.T) {
	db, prefix := openOutServiceDB(t)
	creatorID := prefix + "-creator"
	soldierID := prefix + "-soldier"
	seedUser(t, db, creatorID, "SSG", models.BatteryAlpha, true)
	seedUser(t, db, soldierID, "PTE", models.BatteryAlpha, false)

	outSession, err := NewService(db).Create(context.Background(), CreateRequest{
		Name:      "Stay Out 21 Aug",
		Subtype:   models.OutSubtypeStayOut,
		CreatedBy: creatorID,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	t.Run("absent from the active session list", func(t *testing.T) {
		actor := &models.User{ID: creatorID, IsSuperadmin: true}
		active, err := sessionservice.NewService(db, "@bot").ListActive(context.Background(), actor)
		if err != nil {
			t.Fatalf("ListActive() error = %v", err)
		}
		for _, s := range active {
			if s.ID == outSession.ID {
				t.Fatal("Out session appeared in the attendance active-session list")
			}
		}
	})

	t.Run("refuses an attendance mark", func(t *testing.T) {
		ctx := context.Background()
		tx, err := db.Pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer func() { _ = tx.Rollback(ctx) }()

		outcome, err := attendance.Mark(ctx, tx, attendance.MarkRequest{
			SessionID: outSession.ID,
			UserID:    soldierID,
			Method:    models.MarkingMethodQRScan,
		})
		if err != nil {
			t.Fatalf("Mark() error = %v", err)
		}
		if outcome != attendance.SessionClosed {
			t.Fatalf("Mark() outcome = %v; want SessionClosed for an Out session", outcome)
		}

		var records int
		if err := tx.QueryRow(ctx,
			`SELECT COUNT(*) FROM attendance_record WHERE session_id = $1`,
			outSession.ID).Scan(&records); err != nil {
			t.Fatalf("count attendance records: %v", err)
		}
		if records != 0 {
			t.Fatalf("attendance records against an Out session = %d; want 0", records)
		}
	})
}

// The reverse direction: an ordinary session ID must not resolve through the
// Out endpoints.
func TestGetRejectsAttendanceSession(t *testing.T) {
	db, prefix := openOutServiceDB(t)
	creatorID := prefix + "-creator"
	seedUser(t, db, creatorID, "SSG", models.BatteryAlpha, true)

	ordinary, err := sessionservice.NewService(db, "@bot").Create(context.Background(),
		sessionservice.CreateRequest{
			Name:      "Morning parade",
			Scope:     models.SessionScopeUnitWide,
			CreatedBy: creatorID,
		})
	if err != nil {
		t.Fatalf("create attendance session: %v", err)
	}

	if _, err := NewService(db).Get(context.Background(), ordinary.ID); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("Get(attendance session) error = %v; want ErrSessionNotFound", err)
	}
}

func openOutServiceDB(t *testing.T) (*database.DB, string) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to run the out service integration test")
	}
	db, err := database.NewPostgresDB(url)
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	prefix := fmt.Sprintf("out-service-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = db.Pool.Exec(ctx, `DELETE FROM out_movement WHERE session_id IN (
			SELECT id FROM attendance_session WHERE created_by LIKE $1)`, prefix+"-%")
		_, _ = db.Pool.Exec(ctx, `DELETE FROM attendance_record WHERE user_id LIKE $1`, prefix+"-%")
		_, _ = db.Pool.Exec(ctx, `DELETE FROM attendance_session WHERE created_by LIKE $1`, prefix+"-%")
		_, _ = db.Pool.Exec(ctx, `DELETE FROM "user" WHERE id LIKE $1`, prefix+"-%")
		db.Close()
	})
	return db, prefix
}

func seedUser(t *testing.T, db *database.DB, id, rank, battery string, superadmin bool) {
	t.Helper()
	_, err := db.Pool.Exec(context.Background(), `
		INSERT INTO "user" (id, "full_name", rank, battery, password, extras,
		                    "is_superadmin", verified, "createdAt", "updatedAt")
		VALUES ($1, $2, $3, $4, 'test', '{}'::jsonb, $5, true, NOW(), NOW())
	`, id, id+" name", rank, battery, superadmin)
	if err != nil {
		t.Fatalf("insert user %s: %v", id, err)
	}
}

// --- helpers shared with the movement tests ---

func seedUserVerified(t *testing.T, db *database.DB, id, rank, battery string, superadmin, verified bool) {
	t.Helper()
	_, err := db.Pool.Exec(context.Background(), `
		INSERT INTO "user" (id, "full_name", rank, battery, password, extras,
		                    "is_superadmin", verified, "createdAt", "updatedAt")
		VALUES ($1, $2, $3, $4, 'test', '{}'::jsonb, $5, $6, NOW(), NOW())
	`, id, id+" name", rank, battery, superadmin, verified)
	if err != nil {
		t.Fatalf("insert user %s: %v", id, err)
	}
}

// seedOutSessionRow inserts an Out session directly so a test can pin its
// expected return time, including one already in the past.
func seedOutSessionRow(t *testing.T, db *database.DB, id, creatorID, subtype string, expectedReturn time.Time) string {
	t.Helper()
	_, err := db.Pool.Exec(context.Background(), `
		INSERT INTO attendance_session (
			id, name, qr_code, qr_code_secret, scope, batteries, status,
			created_by, start_time, session_type, out_subtype,
			expected_return_at, "createdAt", "updatedAt"
		) VALUES ($1, $2, $3, 'secret', 'unit_wide', '{}', 'active',
		          $4, NOW(), 'out', $5, $6, NOW(), NOW())
	`, id, "Out "+subtype, id+":secret", creatorID, subtype, expectedReturn)
	if err != nil {
		t.Fatalf("insert out session %s: %v", id, err)
	}
	return id
}

// seedSessionAndSoldier creates an active Out session plus one verified soldier.
func seedSessionAndSoldier(t *testing.T, db *database.DB, prefix string) (string, string) {
	t.Helper()
	creator := prefix + "-creator"
	soldier := prefix + "-soldier"
	seedUser(t, db, creator, "SSG", models.BatteryAlpha, true)
	seedUser(t, db, soldier, "PTE", models.BatteryAlpha, false)
	sess := seedOutSessionRow(t, db, prefix+"-sess", creator,
		models.OutSubtypeNightsOut, time.Now().Add(6*time.Hour))
	return sess, soldier
}

// writeMovement inserts a movement at a chosen time, bypassing the duplicate
// window so tests can build a history.
func writeMovement(t *testing.T, db *database.DB, sessionID, userID, direction string, at time.Time) {
	t.Helper()
	id := fmt.Sprintf("%s-%s-%d", userID, direction, at.UnixNano())
	_, err := db.Pool.Exec(context.Background(), `
		INSERT INTO out_movement (id, session_id, user_id, direction, occurred_at, method)
		VALUES ($1, $2, $3, $4, $5, 'qr_scan')
	`, id, sessionID, userID, direction, at)
	if err != nil {
		t.Fatalf("insert movement: %v", err)
	}
}

func mustRecord(t *testing.T, db *database.DB, req RecordRequest) RecordResult {
	t.Helper()
	res, err := NewService(db).RecordMovement(context.Background(), req)
	if err != nil {
		t.Fatalf("RecordMovement: %v", err)
	}
	return res
}
