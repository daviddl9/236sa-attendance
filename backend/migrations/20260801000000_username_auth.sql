-- +goose Up
-- +goose StatementBegin
-- PR2: username/password authentication.
-- Pending registrations intentionally leave the roster. Rows moved below use a
-- reserved placeholder username; PR3's approval flow resolves them before a
-- real username can be used to sign in.
ALTER TABLE "user"
  ADD COLUMN IF NOT EXISTS username TEXT,
  ADD COLUMN IF NOT EXISTS password_change_required BOOLEAN NOT NULL DEFAULT false;

CREATE UNIQUE INDEX IF NOT EXISTS idx_user_username_normalized
  ON "user" (lower(trim(username)))
  WHERE username IS NOT NULL;

CREATE TABLE IF NOT EXISTS pending_registration (
    id               TEXT PRIMARY KEY,
    username         TEXT NOT NULL,
    password_hash    TEXT NOT NULL,
    claimed_name     TEXT NOT NULL,
    claimed_rank     TEXT NOT NULL,
    claimed_battery  TEXT NOT NULL,
    "createdAt"      TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_pending_registration_username
  ON pending_registration (lower(trim(username)));

-- Existing unapproved rows are not roster members. Keep their submitted
-- details and password hash in the pending table. The reserved username is
-- deliberately not accepted by the sign-in path and cannot collide with a
-- user-chosen username; PR3's approval flow resolves these placeholders.
INSERT INTO pending_registration (
    id, username, password_hash, claimed_name, claimed_rank, claimed_battery, "createdAt"
)
SELECT
    id,
    '__migrated_pending__' || id,
    COALESCE(password, ''),
    COALESCE("full_name", ''),
    COALESCE(rank, ''),
    COALESCE(battery, ''),
    "createdAt"
FROM "user"
WHERE verified = false;

DELETE FROM "user" WHERE verified = false;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Restore pending rows as unverified users before removing the PR2 tables.
-- NRIC/DOB are intentionally not reconstructed; PR5 owns their removal and
-- this rollback cannot recover values that pending_registration never stored.
INSERT INTO "user" (
    id, "full_name", rank, battery, password, extras, "is_superadmin", verified,
    "createdAt", "updatedAt", username, password_change_required
)
SELECT
    id, claimed_name, claimed_rank, claimed_battery, password_hash, '{}'::jsonb,
    false, false, "createdAt", "createdAt", NULL, false
FROM pending_registration;

DROP INDEX IF EXISTS idx_pending_registration_username;
DROP TABLE IF EXISTS pending_registration;
DROP INDEX IF EXISTS idx_user_username_normalized;
ALTER TABLE "user"
  DROP COLUMN IF EXISTS password_change_required,
  DROP COLUMN IF EXISTS username;
-- +goose StatementEnd
