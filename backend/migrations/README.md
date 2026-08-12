# Database Migrations

This project uses [Goose](https://github.com/pressly/goose) for database migrations.

## Migration Files

Migration files follow the naming convention:
- `YYYYMMDDHHMMSS_description.sql` - Single file containing both up and down migrations

## Running Migrations

### Using Docker Compose (Recommended)

Migrations run automatically when the backend container starts.

### Using Make Commands

```bash
# Run migrations up
make migrate-up

# Rollback migrations
make migrate-down

# Check migration status
make migrate-status
```

### Using Go Directly

```bash
# Run migrations up
go run ./cmd/migrate up ./migrations

# Rollback last migration
go run ./cmd/migrate down ./migrations

# Check status
go run ./cmd/migrate status ./migrations

# Rollback to specific version
go run ./cmd/migrate down-to VERSION ./migrations
```

## Telegram marking-method rollback safety

The `20260814000000_telegram_scan_marking_method.sql` Down migration is
non-destructive. It checks for existing `telegram_scan` attendance rows and
raises a clear error before changing the constraint when any are present. A
clean rollback is allowed only when no Telegram rows exist; operators must
retain or explicitly migrate those rows before retrying. The migration never
deletes or relabels attendance history.

## Creating New Migrations

To create a new migration, add a single file following the naming pattern:

1. Create `YYYYMMDDHHMMSS_description.sql` with both up and down migrations

Example:
- `20240102000000_add_user_preferences.sql` (contains both `-- +goose Up` and `-- +goose Down` sections)

## Migration Format

Each migration file should start with Goose directives:

```sql
-- +goose Up
-- +goose StatementBegin
-- Your SQL here
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Rollback SQL here
-- +goose StatementEnd
```

