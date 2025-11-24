-- +goose Up
-- +goose StatementBegin
-- Remove session_type column and make start_time default to NOW()

-- Drop the check constraint first
ALTER TABLE attendance_session DROP CONSTRAINT IF EXISTS attendance_session_session_type_check;

-- Remove session_type column
ALTER TABLE attendance_session DROP COLUMN IF EXISTS session_type;

-- Make start_time default to NOW() and allow NULL temporarily for existing records
ALTER TABLE attendance_session ALTER COLUMN start_time SET DEFAULT NOW();

-- Update any NULL start_time values to NOW()
UPDATE attendance_session SET start_time = NOW() WHERE start_time IS NULL;

-- Make start_time NOT NULL again (if it wasn't already)
ALTER TABLE attendance_session ALTER COLUMN start_time SET NOT NULL;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Restore session_type column and remove default from start_time

-- Add session_type column back
ALTER TABLE attendance_session ADD COLUMN session_type TEXT;

-- Set default value for existing records
UPDATE attendance_session SET session_type = 'custom' WHERE session_type IS NULL;

-- Make it NOT NULL
ALTER TABLE attendance_session ALTER COLUMN session_type SET NOT NULL;

-- Add check constraint back
ALTER TABLE attendance_session ADD CONSTRAINT attendance_session_session_type_check 
    CHECK (session_type IN ('first_parade', 'morning_formation', 'custom'));

-- Remove default from start_time
ALTER TABLE attendance_session ALTER COLUMN start_time DROP DEFAULT;

-- +goose StatementEnd

