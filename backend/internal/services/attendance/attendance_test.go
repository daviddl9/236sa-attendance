package attendance

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/davidlivingston/go-nextjs-starter/backend/internal/database"
)

func TestMarkActiveUnitWideSession(t *testing.T) {
	db, prefix := openTestDB(t)
	userID := prefix + "-user"
	sessionID := prefix + "-session"
	seedUser(t, db, userID, "UNIT USER", "Alpha")
	seedSession(t, db, sessionID, "unit_wide", nil, "active", userID)

	if outcome := mark(t, db, MarkRequest{SessionID: sessionID, UserID: userID, Method: "qr_scan"}); outcome != Marked {
		t.Fatalf("outcome = %v, want Marked", outcome)
	}
	assertRecordCount(t, db, sessionID, 1)
}

func TestMarkAlreadyMarkedIsIdempotent(t *testing.T) {
	db, prefix := openTestDB(t)
	userID := prefix + "-user"
	sessionID := prefix + "-session"
	seedUser(t, db, userID, "DUPLICATE USER", "Alpha")
	seedSession(t, db, sessionID, "unit_wide", nil, "active", userID)

	first := mark(t, db, MarkRequest{SessionID: sessionID, UserID: userID, Method: "qr_scan"})
	second := mark(t, db, MarkRequest{SessionID: sessionID, UserID: userID, Method: "qr_scan"})
	if first != Marked || second != AlreadyMarked {
		t.Fatalf("outcomes = (%v, %v), want (Marked, AlreadyMarked)", first, second)
	}
	assertRecordCount(t, db, sessionID, 1)
}

func TestMarkClosedSessionDoesNotInsert(t *testing.T) {
	db, prefix := openTestDB(t)
	userID := prefix + "-user"
	sessionID := prefix + "-session"
	seedUser(t, db, userID, "CLOSED USER", "Alpha")
	seedSession(t, db, sessionID, "unit_wide", nil, "closed", userID)

	if outcome := mark(t, db, MarkRequest{SessionID: sessionID, UserID: userID, Method: "qr_scan"}); outcome != SessionClosed {
		t.Fatalf("outcome = %v, want SessionClosed", outcome)
	}
	assertRecordCount(t, db, sessionID, 0)
}

func TestMarkBatterySpecificScope(t *testing.T) {
	t.Run("in scope", func(t *testing.T) {
		db, prefix := openTestDB(t)
		userID := prefix + "-user"
		sessionID := prefix + "-session"
		seedUser(t, db, userID, "ALPHA USER", "Alpha")
		seedSession(t, db, sessionID, "battery_specific", []string{"Alpha"}, "active", userID)

		if outcome := mark(t, db, MarkRequest{SessionID: sessionID, UserID: userID, Method: "qr_scan"}); outcome != Marked {
			t.Fatalf("outcome = %v, want Marked", outcome)
		}
		assertRecordCount(t, db, sessionID, 1)
	})

	t.Run("out of scope", func(t *testing.T) {
		db, prefix := openTestDB(t)
		userID := prefix + "-user"
		sessionID := prefix + "-session"
		seedUser(t, db, userID, "BRAVO USER", "Bravo")
		seedSession(t, db, sessionID, "battery_specific", []string{"Alpha"}, "active", userID)

		if outcome := mark(t, db, MarkRequest{SessionID: sessionID, UserID: userID, Method: "qr_scan"}); outcome != OutOfScope {
			t.Fatalf("outcome = %v, want OutOfScope", outcome)
		}
		assertRecordCount(t, db, sessionID, 0)
	})
}

func TestMarkCustomListScope(t *testing.T) {
	t.Run("participant is in scope", func(t *testing.T) {
		db, prefix := openTestDB(t)
		userID := prefix + "-user"
		sessionID := prefix + "-session"
		seedUser(t, db, userID, "PARTICIPANT", "Bravo")
		seedSession(t, db, sessionID, "custom_list", nil, "active", userID)
		seedParticipant(t, db, sessionID, userID)

		if outcome := mark(t, db, MarkRequest{SessionID: sessionID, UserID: userID, Method: "qr_scan"}); outcome != Marked {
			t.Fatalf("outcome = %v, want Marked", outcome)
		}
		assertRecordCount(t, db, sessionID, 1)
	})

	t.Run("non-participant is out of scope", func(t *testing.T) {
		db, prefix := openTestDB(t)
		userID := prefix + "-user"
		sessionID := prefix + "-session"
		seedUser(t, db, userID, "NON PARTICIPANT", "Bravo")
		seedSession(t, db, sessionID, "custom_list", nil, "active", userID)

		if outcome := mark(t, db, MarkRequest{SessionID: sessionID, UserID: userID, Method: "qr_scan"}); outcome != OutOfScope {
			t.Fatalf("outcome = %v, want OutOfScope", outcome)
		}
		assertRecordCount(t, db, sessionID, 0)
	})
}

func TestMarkManualRecordsMarkedBy(t *testing.T) {
	db, prefix := openTestDB(t)
	userID := prefix + "-user"
	commanderID := prefix + "-commander"
	sessionID := prefix + "-session"
	seedUser(t, db, userID, "MANUALLY MARKED", "Alpha")
	seedUser(t, db, commanderID, "COMMANDER", "Alpha")
	seedSession(t, db, sessionID, "unit_wide", nil, "active", commanderID)

	outcome := mark(t, db, MarkRequest{
		SessionID: sessionID,
		UserID:    userID,
		Method:    "manual",
		MarkedBy:  &commanderID,
	})
	if outcome != Marked {
		t.Fatalf("outcome = %v, want Marked", outcome)
	}

	var markedBy *string
	if err := db.Pool.QueryRow(context.Background(), `
		SELECT marked_by FROM attendance_record WHERE session_id = $1 AND user_id = $2
	`, sessionID, userID).Scan(&markedBy); err != nil {
		t.Fatalf("read marked_by: %v", err)
	}
	if markedBy == nil || *markedBy != commanderID {
		t.Fatalf("marked_by = %v, want %q", markedBy, commanderID)
	}
}

func TestMarkConcurrentDuplicateIsIdempotent(t *testing.T) {
	db, prefix := openTestDB(t)
	userID := prefix + "-user"
	sessionID := prefix + "-session"
	seedUser(t, db, userID, "CONCURRENT USER", "Alpha")
	seedSession(t, db, sessionID, "unit_wide", nil, "active", userID)

	const workers = 2
	start := make(chan struct{})
	outcomes := make(chan MarkOutcome, workers)
	errors := make(chan error, workers)
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			<-start
			tx, err := db.Pool.Begin(context.Background())
			if err != nil {
				errors <- fmt.Errorf("begin transaction: %w", err)
				return
			}
			defer func() { _ = tx.Rollback(context.Background()) }()

			outcome, err := Mark(context.Background(), tx, MarkRequest{
				SessionID: sessionID,
				UserID:    userID,
				Method:    "qr_scan",
			})
			if err != nil {
				errors <- err
				return
			}
			if err := tx.Commit(context.Background()); err != nil {
				errors <- fmt.Errorf("commit transaction: %w", err)
				return
			}
			outcomes <- outcome
		}()
	}
	close(start)
	wg.Wait()
	close(outcomes)
	close(errors)

	for err := range errors {
		if err != nil {
			t.Fatalf("concurrent mark: %v", err)
		}
	}
	var got []MarkOutcome
	for outcome := range outcomes {
		got = append(got, outcome)
	}
	if len(got) != workers {
		t.Fatalf("outcomes = %d, want %d", len(got), workers)
	}
	marked, alreadyMarked := 0, 0
	for _, outcome := range got {
		switch outcome {
		case Marked:
			marked++
		case AlreadyMarked:
			alreadyMarked++
		default:
			t.Fatalf("unexpected outcome = %v", outcome)
		}
	}
	if marked != 1 || alreadyMarked != 1 {
		t.Fatalf("outcomes = (marked %d, already marked %d), want (1, 1)", marked, alreadyMarked)
	}
	assertRecordCount(t, db, sessionID, 1)
}

func openTestDB(t *testing.T) (*database.DB, string) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to run the attendance service integration test")
	}
	db, err := database.NewPostgresDB(url)
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	prefix := fmt.Sprintf("attendance-service-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = db.Pool.Exec(ctx, `DELETE FROM attendance_session WHERE id LIKE $1`, prefix+"-%")
		_, _ = db.Pool.Exec(ctx, `DELETE FROM "user" WHERE id LIKE $1`, prefix+"-%")
		db.Close()
	})
	return db, prefix
}

func seedUser(t *testing.T, db *database.DB, id, name, battery string) {
	t.Helper()
	_, err := db.Pool.Exec(context.Background(), `
		INSERT INTO "user" (id, name, email, "full_name", rank, battery, password, extras, "is_superadmin", verified, "createdAt", "updatedAt")
		VALUES ($1, $2, $3, $2, 'PTE', $4, 'test', '{}'::jsonb, false, true, NOW(), NOW())
	`, id, name, id+"@example.test", battery)
	if err != nil {
		t.Fatalf("insert user %s: %v", id, err)
	}
}

func seedSession(t *testing.T, db *database.DB, id, scope string, batteries []string, status, creatorID string) {
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

func seedParticipant(t *testing.T, db *database.DB, sessionID, userID string) {
	t.Helper()
	if _, err := db.Pool.Exec(context.Background(), `
		INSERT INTO session_participants (session_id, user_id) VALUES ($1, $2)
	`, sessionID, userID); err != nil {
		t.Fatalf("insert participant: %v", err)
	}
}

func mark(t *testing.T, db *database.DB, req MarkRequest) MarkOutcome {
	t.Helper()
	tx, err := db.Pool.Begin(context.Background())
	if err != nil {
		t.Fatalf("begin transaction: %v", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	outcome, err := Mark(context.Background(), tx, req)
	if err != nil {
		t.Fatalf("mark: %v", err)
	}
	if err := tx.Commit(context.Background()); err != nil {
		t.Fatalf("commit transaction: %v", err)
	}
	return outcome
}

func assertRecordCount(t *testing.T, db *database.DB, sessionID string, want int) {
	t.Helper()
	var got int
	if err := db.Pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM attendance_record WHERE session_id = $1
	`, sessionID).Scan(&got); err != nil {
		t.Fatalf("count records: %v", err)
	}
	if got != want {
		t.Fatalf("record count = %d, want %d", got, want)
	}
}
