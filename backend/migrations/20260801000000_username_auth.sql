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

-- Existing unapproved rows are COPIED, never deleted.
--
-- Deleting them is unsafe: attendance_record.user_id is ON DELETE CASCADE, and
-- ManualMarkAttendance (attendance.go:252) does not check `verified`, so an
-- unapproved user can legitimately hold attendance records. Deleting would
-- silently destroy parade history. (attendance_session.created_by is ON DELETE
-- RESTRICT, so the alternative outcome is a hard migration failure.)
--
-- The Missing-list bug that motivated moving these rows is fixed independently
-- by the explicit `verified = true` filters in reports.go, so the delete buys
-- no correctness and only risks data loss.
--
-- Result: an unapproved person appears in both tables. Reports exclude them via
-- `verified = true`, and PR3's approval flow links the pending row to the
-- existing user row, which is exactly the fuzzy-match path it already needs.
-- The reserved username cannot collide with a user-chosen one and is rejected
-- by the sign-in path.
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
WHERE verified = false
ON CONFLICT (id) DO NOTHING;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Up only ever copied rows out of "user", never deleted them, so rolling back
-- is a plain teardown. Re-inserting here would violate the "user" primary key.
-- Self-registrations created after this migration exist only in
-- pending_registration and are intentionally discarded on rollback; they were
-- never roster members and hold no attendance history.
DROP INDEX IF EXISTS idx_pending_registration_username;
DROP TABLE IF EXISTS pending_registration;
DROP INDEX IF EXISTS idx_user_username_normalized;
ALTER TABLE "user"
  DROP COLUMN IF EXISTS password_change_required,
  DROP COLUMN IF EXISTS username;
-- +goose StatementEnd
