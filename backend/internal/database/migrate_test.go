package database

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/davidlivingston/go-nextjs-starter/backend/internal/services/deeplink"
)

// TEST_DATABASE_URL must point to a disposable PostgreSQL database, never a
// production database. The test uses a temporary schema and migration files.
func TestRunMigrationsAllowsMissingVersions(t *testing.T) {
	baseURL := os.Getenv("TEST_DATABASE_URL")
	if baseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run PostgreSQL migration tests")
	}

	runnerURL := createMigrationOrderSchema(t, baseURL)
	allMigrations, laterMigration := writeMigrationOrderFixture(t)

	previousTableName := goose.TableName()
	goose.SetTableName("goose_db_version")
	t.Cleanup(func() { goose.SetTableName(previousTableName) })

	setupDB := openMigrationOrderDB(t, runnerURL)
	if err := goose.SetDialect("postgres"); err != nil {
		setupDB.Close()
		t.Fatalf("set Goose dialect: %v", err)
	}
	if err := goose.Up(setupDB, laterMigration); err != nil {
		setupDB.Close()
		t.Fatalf("apply already-released later migration: %v", err)
	}
	if err := setupDB.Close(); err != nil {
		t.Fatalf("close migration setup database: %v", err)
	}

	if err := RunMigrations(runnerURL, allMigrations); err != nil {
		t.Fatalf("run migrations with an already-applied later version: %v", err)
	}

	verifyDB := openMigrationOrderDB(t, runnerURL)
	defer verifyDB.Close()

	var gotSteps []string
	rows, err := verifyDB.QueryContext(context.Background(), `
		SELECT step FROM migration_order_events ORDER BY id
	`)
	if err != nil {
		t.Fatalf("read migration execution order: %v", err)
	}
	for rows.Next() {
		var step string
		if err := rows.Scan(&step); err != nil {
			rows.Close()
			t.Fatalf("scan migration execution step: %v", err)
		}
		gotSteps = append(gotSteps, step)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		t.Fatalf("iterate migration execution order: %v", err)
	}
	if err := rows.Close(); err != nil {
		t.Fatalf("close migration execution order: %v", err)
	}
	wantSteps := []string{"later", "lower-1", "lower-2", "newer"}
	if !reflect.DeepEqual(gotSteps, wantSteps) {
		t.Fatalf("migration execution order = %v, want %v", gotSteps, wantSteps)
	}

	var gotVersions []int64
	rows, err = verifyDB.QueryContext(context.Background(), `
		SELECT version_id
		FROM goose_db_version
		WHERE is_applied AND version_id > 0
		ORDER BY version_id
	`)
	if err != nil {
		t.Fatalf("read applied migration versions: %v", err)
	}
	for rows.Next() {
		var version int64
		if err := rows.Scan(&version); err != nil {
			rows.Close()
			t.Fatalf("scan applied migration version: %v", err)
		}
		gotVersions = append(gotVersions, version)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		t.Fatalf("iterate applied migration versions: %v", err)
	}
	if err := rows.Close(); err != nil {
		t.Fatalf("close applied migration versions: %v", err)
	}
	wantVersions := []int64{20260812000000, 20260812000001, 20260813000000, 20260814000000}
	if !reflect.DeepEqual(gotVersions, wantVersions) {
		t.Fatalf("applied migration versions = %v, want %v", gotVersions, wantVersions)
	}

	var deeplinkCode string
	if err := verifyDB.QueryRowContext(context.Background(), `
		SELECT deeplink_code FROM attendance_session WHERE id = 'active-session'
	`).Scan(&deeplinkCode); err != nil {
		t.Fatalf("read backfilled deeplink code: %v", err)
	}
	if !deeplink.IsValidCode(deeplinkCode) {
		t.Fatalf("backfilled deeplink code %q is invalid", deeplinkCode)
	}
}

func createMigrationOrderSchema(t *testing.T, baseURL string) string {
	t.Helper()

	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		t.Fatalf("parse TEST_DATABASE_URL: %v", err)
	}
	if parsedURL.Scheme == "" || parsedURL.Host == "" {
		t.Fatalf("TEST_DATABASE_URL must be a PostgreSQL URL")
	}

	schema := fmt.Sprintf("migration_order_%d", time.Now().UnixNano())
	adminDB := openMigrationOrderDB(t, baseURL)
	if _, err := adminDB.ExecContext(context.Background(), "CREATE SCHEMA "+schema); err != nil {
		adminDB.Close()
		t.Fatalf("create migration test schema: %v", err)
	}
	t.Cleanup(func() {
		_, _ = adminDB.ExecContext(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
		_ = adminDB.Close()
	})

	query := parsedURL.Query()
	options := strings.TrimSpace(query.Get("options"))
	if options != "" {
		options += " "
	}
	query.Set("options", options+"-csearch_path="+schema)
	parsedURL.RawQuery = query.Encode()
	return parsedURL.String()
}

func openMigrationOrderDB(t *testing.T, databaseURL string) *sql.DB {
	t.Helper()
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatalf("open migration test database: %v", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err := db.PingContext(context.Background()); err != nil {
		db.Close()
		t.Fatalf("ping migration test database: %v", err)
	}
	return db
}

func writeMigrationOrderFixture(t *testing.T) (string, string) {
	t.Helper()
	allDir := t.TempDir()
	laterDir := t.TempDir()

	migrations := map[string]string{
		"20260812000000_lower_one.sql": `-- +goose Up
-- +goose StatementBegin
INSERT INTO migration_order_events (step) VALUES ('lower-1');
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM migration_order_events WHERE step = 'lower-1';
-- +goose StatementEnd
`,
		"20260812000001_lower_two.sql": `-- +goose Up
-- +goose StatementBegin
INSERT INTO migration_order_events (step) VALUES ('lower-2');
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM migration_order_events WHERE step = 'lower-2';
-- +goose StatementEnd
`,
		"20260813000000_later.sql": `-- +goose Up
-- +goose StatementBegin
CREATE TABLE migration_order_events (
    id INTEGER GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY,
    step TEXT NOT NULL
);
CREATE TABLE attendance_session (
    id TEXT PRIMARY KEY,
    status TEXT NOT NULL,
    deeplink_code TEXT
);
INSERT INTO migration_order_events (step) VALUES ('later');
INSERT INTO attendance_session (id, status) VALUES ('active-session', 'active');
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE attendance_session;
DROP TABLE migration_order_events;
-- +goose StatementEnd
`,
		"20260814000000_newer.sql": `-- +goose Up
-- +goose StatementBegin
INSERT INTO migration_order_events (step) VALUES ('newer');
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM migration_order_events WHERE step = 'newer';
-- +goose StatementEnd
`,
	}

	for name, contents := range migrations {
		if err := os.WriteFile(filepath.Join(allDir, name), []byte(contents), 0o600); err != nil {
			t.Fatalf("write migration fixture %s: %v", name, err)
		}
	}
	laterName := "20260813000000_later.sql"
	if err := os.WriteFile(filepath.Join(laterDir, laterName), []byte(migrations[laterName]), 0o600); err != nil {
		t.Fatalf("write later migration fixture: %v", err)
	}
	return allDir, laterDir
}
