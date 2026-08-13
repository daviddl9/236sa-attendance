package migrations

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

// Keep this regression test next to the migration and execute the exact
// checked-in SQL against an isolated schema. TEST_DATABASE_URL must point to a
// disposable PostgreSQL database, never a production database.
var (
	//go:embed 20260815000000_telegram_admin_context.sql
	telegramAdminContextMigration embed.FS
)

func TestTelegramAdminContextMigrationDownRemovesTransientNullRows(t *testing.T) {
	db, migrationDir, schema := openTelegramAdminContextMigrationDB(t)
	createOldTelegramContextTable(t, db)
	if err := goose.UpContext(context.Background(), db, migrationDir); err != nil {
		t.Fatalf("apply Telegram admin context migration: %v", err)
	}

	if _, err := db.ExecContext(context.Background(), `
		INSERT INTO attendance_session (id) VALUES ('selected-session')
	`); err != nil {
		t.Fatalf("seed selected session: %v", err)
	}
	states := []string{
		"creating_name", "choosing_scope", "choosing_battery", "choosing_duration",
		"confirming_creation", "viewing_status", "searching_name", "confirming_mark",
		"reviewing_own_marks",
	}
	for index, state := range states {
		if _, err := db.ExecContext(context.Background(), `
			INSERT INTO telegram_chat_context (telegram_id, session_id, state)
			VALUES ($1, NULL, $2)
		`, int64(index+1), state); err != nil {
			t.Fatalf("seed transient %s context: %v", state, err)
		}
	}
	if _, err := db.ExecContext(context.Background(), `
		INSERT INTO telegram_chat_context (telegram_id, session_id, state)
		VALUES (100, 'selected-session', 'viewing_status')
	`); err != nil {
		t.Fatalf("seed selected context: %v", err)
	}

	if err := goose.DownContext(context.Background(), db, migrationDir); err != nil {
		t.Fatalf("rollback Telegram admin context migration: %v", err)
	}

	var remaining int
	if err := db.QueryRowContext(context.Background(), `
		SELECT COUNT(*) FROM telegram_chat_context WHERE session_id IS NULL
	`).Scan(&remaining); err != nil {
		t.Fatalf("count transient contexts after rollback: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("transient null-session contexts after rollback = %d, want 0", remaining)
	}

	var selectedSession string
	if err := db.QueryRowContext(context.Background(), `
		SELECT session_id FROM telegram_chat_context WHERE telegram_id = 100
	`).Scan(&selectedSession); err != nil {
		t.Fatalf("read selected context after rollback: %v", err)
	}
	if selectedSession != "selected-session" {
		t.Fatalf("selected context session_id = %q, want selected-session", selectedSession)
	}

	var nullable string
	if err := db.QueryRowContext(context.Background(), `
		SELECT is_nullable
		FROM information_schema.columns
		WHERE table_schema = $1 AND table_name = 'telegram_chat_context'
		  AND column_name = 'session_id'
	`, schema).Scan(&nullable); err != nil {
		t.Fatalf("read rolled-back session_id constraint: %v", err)
	}
	if nullable != "NO" {
		t.Fatalf("rolled-back session_id is_nullable = %q, want NO", nullable)
	}
}

func openTelegramAdminContextMigrationDB(t *testing.T) (*sql.DB, string, string) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to run PostgreSQL migration tests")
	}
	db, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatalf("open migration test database: %v", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err := db.PingContext(context.Background()); err != nil {
		db.Close()
		t.Fatalf("ping migration test database: %v", err)
	}

	schema := fmt.Sprintf("tg_admin_context_guard_%d", time.Now().UnixNano())
	if _, err := db.ExecContext(context.Background(), `CREATE SCHEMA "`+schema+`"`); err != nil {
		db.Close()
		t.Fatalf("create migration test schema: %v", err)
	}
	if _, err := db.ExecContext(context.Background(), `SET search_path TO "`+schema+`"`); err != nil {
		db.Close()
		t.Fatalf("set migration test search path: %v", err)
	}
	goose.SetTableName(schema + ".goose_db_version")
	goose.SetDialect("postgres")
	migrationDir := writeTelegramAdminContextMigration(t)
	t.Cleanup(func() {
		goose.SetTableName("goose_db_version")
		_, _ = db.ExecContext(context.Background(), `DROP SCHEMA "`+schema+`" CASCADE`)
		_ = db.Close()
	})
	return db, migrationDir, schema
}

func writeTelegramAdminContextMigration(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	contents, err := telegramAdminContextMigration.ReadFile("20260815000000_telegram_admin_context.sql")
	if err != nil {
		t.Fatalf("read embedded Telegram admin context migration: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "20260815000000_telegram_admin_context.sql"), contents, 0o600); err != nil {
		t.Fatalf("write temporary Telegram admin context migration: %v", err)
	}
	return dir
}

func createOldTelegramContextTable(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(), `
		CREATE TABLE attendance_session (id TEXT PRIMARY KEY);
		CREATE TABLE telegram_chat_context (
			telegram_id BIGINT PRIMARY KEY,
			session_id TEXT NOT NULL REFERENCES attendance_session(id) ON DELETE CASCADE,
			"updatedAt" TIMESTAMP NOT NULL DEFAULT NOW()
		)
	`); err != nil {
		t.Fatalf("create old Telegram context tables: %v", err)
	}
}
