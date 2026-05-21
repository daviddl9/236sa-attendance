-- +goose Up
ALTER TABLE "user" ADD COLUMN extras JSONB NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE "user" DROP COLUMN extras;
