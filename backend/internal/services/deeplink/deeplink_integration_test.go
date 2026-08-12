package deeplink

import (
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestBackfillActiveSessionCodes(t *testing.T) {
	db, prefix := openTestDB(t)
	userID := prefix + "-user"
	seedUser(t, db, userID)

	activeOne := prefix + "-active-one"
	activeTwo := prefix + "-active-two"
	closed := prefix + "-closed"
	preexisting := prefix + "-preexisting"
	seedSession(t, db, activeOne, "same name", "active", userID, "")
	seedSession(t, db, activeTwo, "same name", "active", userID, "")
	seedSession(t, db, closed, "same name", "closed", userID, "")
	seedSession(t, db, preexisting, "same name", "active", userID, "existing-code")
	seedAttendanceRecord(t, db, activeOne, userID)

	if err := BackfillActiveSessionCodes(context.Background(), db); err != nil {
		t.Fatalf("backfill active session codes: %v", err)
	}

	oneCode := sessionCode(t, db, activeOne)
	twoCode := sessionCode(t, db, activeTwo)
	if !validCode(oneCode) || !validCode(twoCode) {
		t.Fatalf("backfilled codes = (%q, %q), want 22-character base64url values", oneCode, twoCode)
	}
	if oneCode == twoCode {
		t.Fatalf("identical session names received the same code: %q", oneCode)
	}
	if got := sessionCode(t, db, closed); got != "" {
		t.Fatalf("closed session code = %q, want null", got)
	}
	if got := sessionCode(t, db, preexisting); got != "existing-code" {
		t.Fatalf("pre-existing session code = %q, want existing-code", got)
	}
	assertAttendanceCount(t, db, activeOne, 1)

	if err := BackfillActiveSessionCodes(context.Background(), db); err != nil {
		t.Fatalf("repeat backfill active session codes: %v", err)
	}
	if got := sessionCode(t, db, activeOne); got != oneCode {
		t.Fatalf("repeat backfill changed code from %q to %q", oneCode, got)
	}
}

func TestTelegramPairingConstraintsAndDeletes(t *testing.T) {
	db, prefix := openTestDB(t)
	soldierOne := prefix + "-soldier-one"
	soldierTwo := prefix + "-soldier-two"
	commander := prefix + "-commander"
	seedUser(t, db, soldierOne)
	seedUser(t, db, soldierTwo)
	seedUser(t, db, commander)

	insertPairing(t, db, prefix+"-pairing", 900000000000001, soldierOne, commander)
	if _, err := db.ExecContext(context.Background(), `
		INSERT INTO telegram_pairing (id, telegram_id, user_id)
		VALUES ($1, $2, $3)
	`, prefix+"-duplicate-telegram", int64(900000000000001), soldierTwo); err == nil {
		t.Fatal("second pairing for the same telegram_id succeeded")
	}
	if _, err := db.ExecContext(context.Background(), `
		INSERT INTO telegram_pairing (id, telegram_id, user_id)
		VALUES ($1, $2, $3)
	`, prefix+"-duplicate-user", int64(900000000000002), soldierOne); err == nil {
		t.Fatal("second pairing for the same user_id succeeded")
	}

	if _, err := db.ExecContext(context.Background(), `DELETE FROM "user" WHERE id = $1`, commander); err != nil {
		t.Fatalf("delete confirming commander: %v", err)
	}
	var confirmedBy *string
	if err := db.QueryRowContext(context.Background(), `
		SELECT confirmed_by FROM telegram_pairing WHERE id = $1
	`, prefix+"-pairing").Scan(&confirmedBy); err != nil {
		t.Fatalf("read pairing after commander deletion: %v", err)
	}
	if confirmedBy != nil {
		t.Fatalf("confirmed_by = %q, want null", *confirmedBy)
	}

	if _, err := db.ExecContext(context.Background(), `DELETE FROM "user" WHERE id = $1`, soldierOne); err != nil {
		t.Fatalf("delete paired user: %v", err)
	}
	var pairingCount int
	if err := db.QueryRowContext(context.Background(), `
		SELECT COUNT(*) FROM telegram_pairing WHERE id = $1
	`, prefix+"-pairing").Scan(&pairingCount); err != nil {
		t.Fatalf("count pairing after user deletion: %v", err)
	}
	if pairingCount != 0 {
		t.Fatalf("pairing count after user deletion = %d, want 0", pairingCount)
	}
}

func openTestDB(t *testing.T) (*sql.DB, string) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to run deeplink integration tests")
	}
	db, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	if err := db.PingContext(context.Background()); err != nil {
		db.Close()
		t.Fatalf("ping test database: %v", err)
	}
	prefix := fmt.Sprintf("deeplink-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = db.ExecContext(ctx, `DELETE FROM attendance_session WHERE id LIKE $1`, prefix+"-%")
		_, _ = db.ExecContext(ctx, `DELETE FROM telegram_pairing_attempt WHERE id LIKE $1`, prefix+"-%")
		_, _ = db.ExecContext(ctx, `DELETE FROM telegram_pairing_request WHERE id LIKE $1`, prefix+"-%")
		_, _ = db.ExecContext(ctx, `DELETE FROM telegram_pairing WHERE id LIKE $1`, prefix+"-%")
		_, _ = db.ExecContext(ctx, `DELETE FROM "user" WHERE id LIKE $1`, prefix+"-%")
		_ = db.Close()
	})
	return db, prefix
}

func seedUser(t *testing.T, db *sql.DB, id string) {
	t.Helper()
	_, err := db.ExecContext(context.Background(), `
		INSERT INTO "user" (id, "full_name", rank, battery, password, extras, "is_superadmin", verified, "createdAt", "updatedAt")
		VALUES ($1, $2, 'PTE', 'Alpha', 'test', '{}'::jsonb, false, true, NOW(), NOW())
	`, id, strings.ToUpper(id))
	if err != nil {
		t.Fatalf("insert user %s: %v", id, err)
	}
}

func seedSession(t *testing.T, db *sql.DB, id, name, status, creatorID, code string) {
	t.Helper()
	_, err := db.ExecContext(context.Background(), `
		INSERT INTO attendance_session (id, name, qr_code, qr_code_secret, scope, batteries, status, created_by, start_time, deeplink_code)
		VALUES ($1, $2, $3, $4, 'unit_wide', '{}', $5, $6, NOW(), NULLIF($7, ''))
	`, id, name, id+" qr", id+" secret", status, creatorID, code)
	if err != nil {
		t.Fatalf("insert session %s: %v", id, err)
	}
}

func seedAttendanceRecord(t *testing.T, db *sql.DB, sessionID, userID string) {
	t.Helper()
	_, err := db.ExecContext(context.Background(), `
		INSERT INTO attendance_record (id, session_id, user_id, marking_method)
		VALUES ($1, $2, $3, 'manual')
	`, sessionID+"-record", sessionID, userID)
	if err != nil {
		t.Fatalf("insert attendance record: %v", err)
	}
}

func insertPairing(t *testing.T, db *sql.DB, id string, telegramID int64, userID, confirmedBy string) {
	t.Helper()
	_, err := db.ExecContext(context.Background(), `
		INSERT INTO telegram_pairing (id, telegram_id, user_id, confirmed_by)
		VALUES ($1, $2, $3, $4)
	`, id, telegramID, userID, confirmedBy)
	if err != nil {
		t.Fatalf("insert pairing: %v", err)
	}
}

func sessionCode(t *testing.T, db *sql.DB, sessionID string) string {
	t.Helper()
	var code *string
	if err := db.QueryRowContext(context.Background(), `
		SELECT deeplink_code FROM attendance_session WHERE id = $1
	`, sessionID).Scan(&code); err != nil {
		t.Fatalf("read session code: %v", err)
	}
	if code == nil {
		return ""
	}
	return *code
}

func validCode(code string) bool {
	if len(code) != 22 {
		return false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(code)
	return err == nil && len(decoded) == codeBytes
}

func assertAttendanceCount(t *testing.T, db *sql.DB, sessionID string, want int) {
	t.Helper()
	var got int
	if err := db.QueryRowContext(context.Background(), `
		SELECT COUNT(*) FROM attendance_record WHERE session_id = $1
	`, sessionID).Scan(&got); err != nil {
		t.Fatalf("count attendance records: %v", err)
	}
	if got != want {
		t.Fatalf("attendance record count = %d, want %d", got, want)
	}
}
