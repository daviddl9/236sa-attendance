-- +goose Up
-- +goose StatementBegin
-- Roster imports matched on (full_name, nric_last5), which forced the NRIC to be
-- stored in readable form. Imports now match on an HMAC over the normalised name
-- and date of birth, so the same person is recognised on re-import without either
-- value being retrievable from the database.
--
-- Nullable on purpose. A row whose key cannot be derived -- missing name or date
-- of birth, or no signing secret configured -- must stay NULL rather than take a
-- placeholder, because a shared placeholder would make unrelated people look like
-- the same person on the next import.
ALTER TABLE "user"
  ADD COLUMN IF NOT EXISTS import_key TEXT;

-- Unique only among rows that actually have a key, so NULLs never collide.
CREATE UNIQUE INDEX IF NOT EXISTS idx_user_import_key
  ON "user" (import_key)
  WHERE import_key IS NOT NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_user_import_key;
ALTER TABLE "user"
  DROP COLUMN IF EXISTS import_key;
-- +goose StatementEnd
