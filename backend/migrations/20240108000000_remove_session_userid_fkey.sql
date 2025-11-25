-- +goose Up
-- +goose StatementBegin
-- Remove foreign key constraint from session.userId to allow superusers
-- Superusers are in a separate table, so we can't use a foreign key constraint
-- Application code will validate userId exists in either user or superuser table

-- Drop the foreign key constraint (quoted name for case sensitivity)
ALTER TABLE session DROP CONSTRAINT IF EXISTS "session_userId_fkey";

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Restore foreign key constraint (only if all userIds exist in user table)
-- Note: This may fail if there are sessions for superusers
ALTER TABLE session 
ADD CONSTRAINT session_userId_fkey 
FOREIGN KEY ("userId") REFERENCES "user"(id) ON DELETE CASCADE;
-- +goose StatementEnd

