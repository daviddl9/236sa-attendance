-- +goose Up
-- +goose StatementBegin
-- Remove unused user fields and add password column

-- Add password column to user table
ALTER TABLE "user" ADD COLUMN IF NOT EXISTS password TEXT;

-- Migrate passwords from account table to user table
UPDATE "user" u
SET password = a.password
FROM account a
WHERE a."userId" = u.id AND a."providerId" = 'credential' AND u.password IS NULL;

-- Remove old columns
ALTER TABLE "user" 
DROP COLUMN IF EXISTS name,
DROP COLUMN IF EXISTS email,
DROP COLUMN IF EXISTS "emailVerified",
DROP COLUMN IF EXISTS image,
DROP COLUMN IF EXISTS username;

-- Drop username index if it exists
DROP INDEX IF EXISTS idx_user_username;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Restore columns (data will be lost)
ALTER TABLE "user" 
ADD COLUMN IF NOT EXISTS name TEXT,
ADD COLUMN IF NOT EXISTS email TEXT,
ADD COLUMN IF NOT EXISTS "emailVerified" BOOLEAN NOT NULL DEFAULT false,
ADD COLUMN IF NOT EXISTS image TEXT,
ADD COLUMN IF NOT EXISTS username TEXT UNIQUE;

-- Migrate passwords back to account table (if needed)
-- Note: This is a simplified rollback - full restoration would require backup

-- Remove password column
ALTER TABLE "user" DROP COLUMN IF EXISTS password;
-- +goose StatementEnd

