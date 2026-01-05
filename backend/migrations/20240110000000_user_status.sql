-- +goose Up
-- +goose StatementBegin
-- Personnel status tracking schema

-- User status table (main status record)
CREATE TABLE IF NOT EXISTS user_status (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
    status_type TEXT NOT NULL CHECK (status_type IN ('stay_out', 'off_pass', 'mc', 'course', 'other')),
    daily_after_time TIME,
    start_date DATE,
    end_date DATE,
    notes TEXT,
    created_by TEXT NOT NULL REFERENCES "user"(id) ON DELETE RESTRICT,
    "createdAt" TIMESTAMP NOT NULL DEFAULT NOW(),
    "updatedAt" TIMESTAMP NOT NULL DEFAULT NOW()
);

-- User status dates table (for multi-date statuses: off_pass, course, other)
CREATE TABLE IF NOT EXISTS user_status_date (
    id TEXT PRIMARY KEY,
    status_id TEXT NOT NULL REFERENCES user_status(id) ON DELETE CASCADE,
    date DATE NOT NULL,
    start_time TIME NOT NULL,
    end_time TIME NOT NULL,
    "createdAt" TIMESTAMP NOT NULL DEFAULT NOW(),
    CONSTRAINT valid_time_range CHECK (end_time > start_time)
);

-- Indexes for user_status
CREATE INDEX IF NOT EXISTS idx_user_status_user_id ON user_status(user_id);
CREATE INDEX IF NOT EXISTS idx_user_status_status_type ON user_status(status_type);
CREATE INDEX IF NOT EXISTS idx_user_status_created_by ON user_status(created_by);

-- Indexes for user_status_date
CREATE INDEX IF NOT EXISTS idx_user_status_date_status_id ON user_status_date(status_id);
CREATE INDEX IF NOT EXISTS idx_user_status_date_date ON user_status_date(date);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Drop indexes
DROP INDEX IF EXISTS idx_user_status_date_date;
DROP INDEX IF EXISTS idx_user_status_date_status_id;
DROP INDEX IF EXISTS idx_user_status_created_by;
DROP INDEX IF EXISTS idx_user_status_status_type;
DROP INDEX IF EXISTS idx_user_status_user_id;

-- Drop tables (order matters due to FK)
DROP TABLE IF EXISTS user_status_date;
DROP TABLE IF EXISTS user_status;
-- +goose StatementEnd
