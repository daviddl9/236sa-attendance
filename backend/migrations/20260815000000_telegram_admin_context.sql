-- +goose Up
-- +goose StatementBegin
ALTER TABLE telegram_chat_context
  ALTER COLUMN session_id DROP NOT NULL;

ALTER TABLE telegram_chat_context
  ADD COLUMN state TEXT NOT NULL DEFAULT 'idle',
  ADD COLUMN draft_name TEXT,
  ADD COLUMN draft_scope TEXT,
  ADD COLUMN draft_battery TEXT,
  ADD COLUMN draft_end_time TIMESTAMP,
  ADD COLUMN expires_at TIMESTAMP,
  ADD COLUMN version BIGINT NOT NULL DEFAULT 0;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM telegram_chat_context
WHERE session_id IS NULL
  AND state IN ('idle', 'draft');

ALTER TABLE telegram_chat_context
  ALTER COLUMN session_id SET NOT NULL;

ALTER TABLE telegram_chat_context
  DROP COLUMN state,
  DROP COLUMN draft_name,
  DROP COLUMN draft_scope,
  DROP COLUMN draft_battery,
  DROP COLUMN draft_end_time,
  DROP COLUMN expires_at,
  DROP COLUMN version;
-- +goose StatementEnd
