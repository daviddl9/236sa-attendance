-- +goose Up
-- +goose StatementBegin
-- Rename nric_last4 to nric_last5 for 5-character NRIC storage
-- Password is now just NRIC Last 5 instead of NRIC Last 4 + DOB

ALTER TABLE "user" RENAME COLUMN "nric_last4" TO "nric_last5";

-- Recreate index with new column name
DROP INDEX IF EXISTS idx_user_nric_last4;
CREATE INDEX IF NOT EXISTS idx_user_nric_last5 ON "user"("nric_last5");

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE "user" RENAME COLUMN "nric_last5" TO "nric_last4";

DROP INDEX IF EXISTS idx_user_nric_last5;
CREATE INDEX IF NOT EXISTS idx_user_nric_last4 ON "user"("nric_last4");

-- +goose StatementEnd
