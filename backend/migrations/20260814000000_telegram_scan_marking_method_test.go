package migrations

import (
	"context"
	"database/sql"
	"embed"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

// Keep the migration test next to the migration and run the exact checked-in
// SQL through Goose. TEST_DATABASE_URL is expected to point at a disposable
// PostgreSQL database, never a production database.
var (
	//go:embed 20260814000000_telegram_scan_marking_method.sql
	telegramScanMigration embed.FS
)

func TestTelegramScanMarkingMigrationDownGuardsExistingRows(t *testing.T) {
	db, migrationDir := openTelegramScanMigrationDB(t)
	createOldAttendanceRecordTable(t, db)
	applyTelegramScanMigration(t, db, migrationDir)

	if _, err := db.ExecContext(context.Background(), `
		INSERT INTO attendance_record (id, marking_method) VALUES ('synthetic-telegram-record', 'telegram_scan')
	`); err != nil {
		t.Fatalf("insert synthetic Telegram record: %v", err)
	}

	err := goose.DownContext(context.Background(), db, migrationDir)
	if err == nil || !strings.Contains(err.Error(), "telegram_scan attendance records exist") {
		t.Fatalf("guarded rollback error = %v, want a clear telegram_scan warning", err)
	}

	var count int
	if err := db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM attendance_record WHERE marking_method = 'telegram_scan'`).Scan(&count); err != nil {
		t.Fatalf("count Telegram records after guarded rollback: %v", err)
	}
	if count != 1 {
		t.Fatalf("Telegram records after guarded rollback = %d, want 1", count)
	}

	var applied bool
	if err := db.QueryRowContext(context.Background(), `
		SELECT is_applied FROM tg_migration_guard.goose_db_version WHERE version_id = 20260814000000
	`).Scan(&applied); err != nil {
		t.Fatalf("read migration version after guarded rollback: %v", err)
	}
	if !applied {
		t.Fatal("guarded rollback changed migration history")
	}
}

func TestTelegramScanMarkingMigrationDownWithoutRowsRestoresOldConstraint(t *testing.T) {
	db, migrationDir := openTelegramScanMigrationDB(t)
	createOldAttendanceRecordTable(t, db)
	applyTelegramScanMigration(t, db, migrationDir)

	if err := goose.DownContext(context.Background(), db, migrationDir); err != nil {
		t.Fatalf("clean rollback: %v", err)
	}
	if _, err := db.ExecContext(context.Background(), `
		INSERT INTO attendance_record (id, marking_method) VALUES ('synthetic-telegram-record', 'telegram_scan')
	`); err == nil {
		t.Fatal("old constraint accepted telegram_scan after clean rollback")
	}
	if _, err := db.ExecContext(context.Background(), `
		INSERT INTO attendance_record (id, marking_method) VALUES ('synthetic-qr-record', 'qr_scan')
	`); err != nil {
		t.Fatalf("old constraint rejected qr_scan after clean rollback: %v", err)
	}
}

func openTelegramScanMigrationDB(t *testing.T) (*sql.DB, string) {
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

	// The one-connection pool keeps this schema search_path in force for Goose
	// and all statements. A schema-qualified version table keeps the harness
	// isolated even when TEST_DATABASE_URL already has other migrations.
	if _, err := db.ExecContext(context.Background(), `CREATE SCHEMA tg_migration_guard`); err != nil {
		db.Close()
		t.Fatalf("create migration test schema: %v", err)
	}
	if _, err := db.ExecContext(context.Background(), `SET search_path TO tg_migration_guard`); err != nil {
		db.Close()
		t.Fatalf("set migration test search path: %v", err)
	}
	goose.SetTableName("tg_migration_guard.goose_db_version")
	goose.SetDialect("postgres")
	migrationDir := writeTelegramScanMigration(t)
	t.Cleanup(func() {
		goose.SetTableName("goose_db_version")
		_, _ = db.ExecContext(context.Background(), `DROP SCHEMA tg_migration_guard CASCADE`)
		_ = db.Close()
	})
	return db, migrationDir
}

func writeTelegramScanMigration(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	contents, err := telegramScanMigration.ReadFile("20260814000000_telegram_scan_marking_method.sql")
	if err != nil {
		t.Fatalf("read embedded marking migration: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "20260814000000_telegram_scan_marking_method.sql"), contents, 0o600); err != nil {
		t.Fatalf("write temporary marking migration: %v", err)
	}
	return dir
}

func createOldAttendanceRecordTable(t *testing.T, db *sql.DB) {
	t.Helper()
	_, err := db.ExecContext(context.Background(), `
		CREATE TABLE attendance_record (
			id TEXT PRIMARY KEY,
			marking_method TEXT NOT NULL CHECK (marking_method IN ('qr_scan', 'manual'))
		)
	`)
	if err != nil {
		t.Fatalf("create old attendance_record table: %v", err)
	}
}

func applyTelegramScanMigration(t *testing.T, db *sql.DB, migrationDir string) {
	t.Helper()
	if err := goose.UpContext(context.Background(), db, migrationDir); err != nil {
		t.Fatalf("apply Telegram marking migration: %v", err)
	}
	var version int64
	if err := db.QueryRowContext(context.Background(), `
		SELECT version_id FROM tg_migration_guard.goose_db_version WHERE is_applied ORDER BY version_id DESC LIMIT 1
	`).Scan(&version); err != nil {
		t.Fatalf("read applied marking migration: %v", err)
	}
	if version != 20260814000000 {
		t.Fatalf("applied migration version = %d, want 20260814000000", version)
	}
}
