-- +goose Up
-- +goose StatementBegin
CREATE TABLE telegram_pairing (
    id            TEXT PRIMARY KEY,
    telegram_id   BIGINT NOT NULL UNIQUE,
    user_id       TEXT NOT NULL UNIQUE REFERENCES "user"(id) ON DELETE CASCADE,
    display_name  TEXT,
    confirmed_by  TEXT REFERENCES "user"(id) ON DELETE SET NULL,
    self_confirmed BOOLEAN NOT NULL DEFAULT false,
    "createdAt"   TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE telegram_pairing_request (
    id            TEXT PRIMARY KEY,
    telegram_id   BIGINT NOT NULL UNIQUE,
    display_name  TEXT NOT NULL,
    "createdAt"   TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE telegram_pairing_attempt (
    id            TEXT PRIMARY KEY,
    telegram_id   BIGINT NOT NULL,
    outcome       TEXT NOT NULL CHECK (outcome IN ('proposed','confirmed','refused_conflict','no_match')),
    user_id       TEXT REFERENCES "user"(id) ON DELETE SET NULL,
    "createdAt"   TIMESTAMP NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_telegram_pairing_attempt_recent
  ON telegram_pairing_attempt (telegram_id, "createdAt" DESC);

CREATE TABLE telegram_chat_context (
    telegram_id  BIGINT PRIMARY KEY,
    session_id   TEXT NOT NULL REFERENCES attendance_session(id) ON DELETE CASCADE,
    "updatedAt"  TIMESTAMP NOT NULL DEFAULT NOW()
);

ALTER TABLE attendance_session
  ADD COLUMN IF NOT EXISTS deeplink_code TEXT;
CREATE UNIQUE INDEX IF NOT EXISTS idx_session_deeplink_code
  ON attendance_session (deeplink_code) WHERE deeplink_code IS NOT NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_session_deeplink_code;
ALTER TABLE attendance_session
  DROP COLUMN IF EXISTS deeplink_code;
DROP TABLE IF EXISTS telegram_chat_context;
DROP TABLE IF EXISTS telegram_pairing_attempt;
DROP TABLE IF EXISTS telegram_pairing_request;
DROP TABLE IF EXISTS telegram_pairing;
-- +goose StatementEnd
