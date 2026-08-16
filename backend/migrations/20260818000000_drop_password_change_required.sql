-- +goose Up
-- +goose StatementBegin
-- Login moved to DOB-based authentication, so the forced password-change flag
-- is obsolete. Production already has it false for every user (0/361), so
-- dropping the column is a safe, lossless cleanup.
ALTER TABLE "user" DROP COLUMN IF EXISTS password_change_required;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE "user"
  ADD COLUMN IF NOT EXISTS password_change_required BOOLEAN NOT NULL DEFAULT false;
-- +goose StatementEnd
