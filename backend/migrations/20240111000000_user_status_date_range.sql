-- +goose Up
-- +goose StatementBegin
-- Alter user_status_date to support date ranges (start_date/time to end_date/time)

-- Add new columns
ALTER TABLE user_status_date ADD COLUMN start_date DATE;
ALTER TABLE user_status_date ADD COLUMN end_date DATE;

-- Migrate existing data: copy date to both start_date and end_date
UPDATE user_status_date SET start_date = date, end_date = date WHERE start_date IS NULL;

-- Make new columns NOT NULL after migration
ALTER TABLE user_status_date ALTER COLUMN start_date SET NOT NULL;
ALTER TABLE user_status_date ALTER COLUMN end_date SET NOT NULL;

-- Drop old constraint and column
ALTER TABLE user_status_date DROP CONSTRAINT IF EXISTS valid_time_range;
ALTER TABLE user_status_date DROP COLUMN date;

-- Add new constraint: end must be after start (comparing date+time)
ALTER TABLE user_status_date ADD CONSTRAINT valid_date_time_range
    CHECK ((end_date > start_date) OR (end_date = start_date AND end_time > start_time));

-- Update index
DROP INDEX IF EXISTS idx_user_status_date_date;
CREATE INDEX IF NOT EXISTS idx_user_status_date_start_date ON user_status_date(start_date);
CREATE INDEX IF NOT EXISTS idx_user_status_date_end_date ON user_status_date(end_date);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Revert changes

-- Add back old column
ALTER TABLE user_status_date ADD COLUMN date DATE;

-- Migrate data back
UPDATE user_status_date SET date = start_date WHERE date IS NULL;

-- Make it NOT NULL
ALTER TABLE user_status_date ALTER COLUMN date SET NOT NULL;

-- Drop new constraint and columns
ALTER TABLE user_status_date DROP CONSTRAINT IF EXISTS valid_date_time_range;
ALTER TABLE user_status_date DROP COLUMN start_date;
ALTER TABLE user_status_date DROP COLUMN end_date;

-- Add back old constraint
ALTER TABLE user_status_date ADD CONSTRAINT valid_time_range CHECK (end_time > start_time);

-- Update indexes
DROP INDEX IF EXISTS idx_user_status_date_start_date;
DROP INDEX IF EXISTS idx_user_status_date_end_date;
CREATE INDEX IF NOT EXISTS idx_user_status_date_date ON user_status_date(date);

-- +goose StatementEnd
