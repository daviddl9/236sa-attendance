-- +goose Up
-- +goose StatementBegin
-- Audit tables for the agent-assisted personnel import flow (feature 002).
-- The `user` table itself is unchanged. Every parsed-row outcome is recorded
-- here so admins can answer "what did this import change?" without
-- consulting the database directly (spec SC-007).

CREATE TABLE IF NOT EXISTS personnel_imports (
    id TEXT PRIMARY KEY,
    admin_user_id TEXT NOT NULL REFERENCES "user"(id) ON DELETE RESTRICT,
    document_filename TEXT NOT NULL,
    merge_mode TEXT CHECK (merge_mode IN ('fill_gaps', 'override')),
    row_counts JSONB NOT NULL DEFAULT '{}'::jsonb,
    status TEXT NOT NULL CHECK (status IN ('pending', 'previewed', 'committed', 'failed')),
    error_message TEXT,
    started_at TIMESTAMP NOT NULL DEFAULT NOW(),
    finished_at TIMESTAMP
);

CREATE TABLE IF NOT EXISTS personnel_import_changes (
    id TEXT PRIMARY KEY,
    import_id TEXT NOT NULL REFERENCES personnel_imports(id) ON DELETE CASCADE,
    user_id TEXT REFERENCES "user"(id) ON DELETE SET NULL,
    action TEXT NOT NULL CHECK (action IN ('created', 'updated', 'skipped', 'failed')),
    field_name TEXT,
    before_value TEXT,
    after_value TEXT,
    reason TEXT,
    "createdAt" TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_personnel_imports_admin_user_id ON personnel_imports(admin_user_id);
CREATE INDEX IF NOT EXISTS idx_personnel_imports_status ON personnel_imports(status);
CREATE INDEX IF NOT EXISTS idx_personnel_import_changes_import_id ON personnel_import_changes(import_id);
CREATE INDEX IF NOT EXISTS idx_personnel_import_changes_user_id ON personnel_import_changes(user_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_personnel_import_changes_user_id;
DROP INDEX IF EXISTS idx_personnel_import_changes_import_id;
DROP INDEX IF EXISTS idx_personnel_imports_status;
DROP INDEX IF EXISTS idx_personnel_imports_admin_user_id;

DROP TABLE IF EXISTS personnel_import_changes;
DROP TABLE IF EXISTS personnel_imports;
-- +goose StatementEnd
