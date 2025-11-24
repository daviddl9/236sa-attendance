-- +goose Up
-- +goose StatementBegin
-- Attendance records schema

-- Attendance records table
CREATE TABLE IF NOT EXISTS attendance_record (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL REFERENCES attendance_session(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
    marked_at TIMESTAMP NOT NULL DEFAULT NOW(),
    marking_method TEXT NOT NULL CHECK (marking_method IN ('qr_scan', 'manual')),
    marked_by TEXT REFERENCES "user"(id) ON DELETE SET NULL,
    "createdAt" TIMESTAMP NOT NULL DEFAULT NOW(),
    "updatedAt" TIMESTAMP NOT NULL DEFAULT NOW(),
    UNIQUE (session_id, user_id)
);

-- Indexes for attendance records
CREATE INDEX IF NOT EXISTS idx_attendance_record_session_id ON attendance_record(session_id);
CREATE INDEX IF NOT EXISTS idx_attendance_record_user_id ON attendance_record(user_id);
CREATE INDEX IF NOT EXISTS idx_attendance_record_marked_at ON attendance_record(marked_at);
CREATE INDEX IF NOT EXISTS idx_attendance_record_marking_method ON attendance_record(marking_method);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Drop indexes
DROP INDEX IF EXISTS idx_attendance_record_marking_method;
DROP INDEX IF EXISTS idx_attendance_record_marked_at;
DROP INDEX IF EXISTS idx_attendance_record_user_id;
DROP INDEX IF EXISTS idx_attendance_record_session_id;

-- Drop table
DROP TABLE IF EXISTS attendance_record;
-- +goose StatementEnd

