-- +goose Up
-- +goose StatementBegin
ALTER TABLE "user" ADD COLUMN IF NOT EXISTS dob TEXT;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE "user" DROP COLUMN IF EXISTS dob;
-- +goose StatementEnd
