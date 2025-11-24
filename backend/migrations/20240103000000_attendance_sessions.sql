-- +goose Up
-- +goose StatementBegin
-- Attendance session management schema

-- Attendance sessions table
CREATE TABLE IF NOT EXISTS attendance_session (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    session_type TEXT NOT NULL CHECK (session_type IN ('first_parade', 'morning_formation', 'custom')),
    qr_code TEXT NOT NULL UNIQUE,
    qr_code_secret TEXT NOT NULL,
    scope TEXT NOT NULL CHECK (scope IN ('unit_wide', 'battery_specific')),
    batteries TEXT[] DEFAULT '{}',
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'closed')),
    created_by TEXT NOT NULL REFERENCES "user"(id) ON DELETE RESTRICT,
    start_time TIMESTAMP NOT NULL,
    end_time TIMESTAMP,
    closed_at TIMESTAMP,
    "createdAt" TIMESTAMP NOT NULL DEFAULT NOW(),
    "updatedAt" TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Indexes for attendance sessions
CREATE INDEX IF NOT EXISTS idx_attendance_session_status ON attendance_session(status);
CREATE INDEX IF NOT EXISTS idx_attendance_session_created_by ON attendance_session(created_by);
CREATE INDEX IF NOT EXISTS idx_attendance_session_start_time ON attendance_session(start_time);
CREATE INDEX IF NOT EXISTS idx_attendance_session_qr_code ON attendance_session(qr_code);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Drop indexes
DROP INDEX IF EXISTS idx_attendance_session_qr_code;
DROP INDEX IF EXISTS idx_attendance_session_start_time;
DROP INDEX IF EXISTS idx_attendance_session_created_by;
DROP INDEX IF EXISTS idx_attendance_session_status;

-- Drop table
DROP TABLE IF EXISTS attendance_session;
-- +goose StatementEnd

