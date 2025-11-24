-- +goose Up
-- +goose StatementBegin
-- Attendance system schema migration
-- Adds military user fields and rank constraints
-- Note: Admin user is created by SeedAdminUser function, not in this migration

-- Add new columns to user table
ALTER TABLE "user" 
ADD COLUMN IF NOT EXISTS "full_name" TEXT,
ADD COLUMN IF NOT EXISTS "rank" TEXT,
ADD COLUMN IF NOT EXISTS "battery" TEXT,
ADD COLUMN IF NOT EXISTS "nric_last4" TEXT,
ADD COLUMN IF NOT EXISTS "dob" TEXT,
ADD COLUMN IF NOT EXISTS "is_superadmin" BOOLEAN NOT NULL DEFAULT false,
ADD COLUMN IF NOT EXISTS "username" TEXT UNIQUE;

-- Update existing name to full_name if full_name is null
UPDATE "user" SET "full_name" = name WHERE "full_name" IS NULL;

-- Add CHECK constraint for valid Singapore ranks
ALTER TABLE "user" 
ADD CONSTRAINT check_valid_rank CHECK (
  rank IS NULL OR rank IN (
    'REC', 'PTE', 'LCP', 'CPL', 'CFC', 
    '3SG', '2SG', '1SG', 'SSG', 'MSG', 
    '3WO', '2WO', '1WO', 'MWO', 'SWO', 
    '2LT', 'LTA', 'CPT', 'MAJ', 'LTC'
  )
);

-- Add CHECK constraint for valid batteries
ALTER TABLE "user" 
ADD CONSTRAINT check_valid_battery CHECK (
  battery IS NULL OR battery IN ('HQ', 'Alpha', 'Bravo')
);

-- Add indexes for efficient filtering
CREATE INDEX IF NOT EXISTS idx_user_rank ON "user"(rank);
CREATE INDEX IF NOT EXISTS idx_user_battery ON "user"(battery);
CREATE INDEX IF NOT EXISTS idx_user_is_superadmin ON "user"("is_superadmin");
CREATE INDEX IF NOT EXISTS idx_user_username ON "user"(username);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Remove indexes
DROP INDEX IF EXISTS idx_user_rank;
DROP INDEX IF EXISTS idx_user_battery;
DROP INDEX IF EXISTS idx_user_is_superadmin;
DROP INDEX IF EXISTS idx_user_username;

-- Remove constraints
ALTER TABLE "user" DROP CONSTRAINT IF EXISTS check_valid_rank;
ALTER TABLE "user" DROP CONSTRAINT IF EXISTS check_valid_battery;

-- Remove columns
ALTER TABLE "user" 
DROP COLUMN IF EXISTS "full_name",
DROP COLUMN IF EXISTS "rank",
DROP COLUMN IF EXISTS "battery",
DROP COLUMN IF EXISTS "nric_last4",
DROP COLUMN IF EXISTS "dob",
DROP COLUMN IF EXISTS "is_superadmin",
DROP COLUMN IF EXISTS "username";
-- +goose StatementEnd

