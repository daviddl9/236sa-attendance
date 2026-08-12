-- +goose Up
-- +goose StatementBegin
ALTER TABLE telegram_pairing_request
  ADD COLUMN IF NOT EXISTS attempt_id TEXT REFERENCES telegram_pairing_attempt(id) ON DELETE SET NULL;

-- Link requests created before this column existed to the no-match attempt
-- that preceded their current request timestamp. New writes always set this
-- relationship directly.
WITH request_attempts AS (
    SELECT r.id AS request_id,
           (
               SELECT a.id
               FROM telegram_pairing_attempt a
               WHERE a.telegram_id = r.telegram_id
                 AND a.outcome = 'no_match'
                 AND a."createdAt" <= r."createdAt"
               ORDER BY a."createdAt" DESC
               LIMIT 1
           ) AS attempt_id
    FROM telegram_pairing_request r
)
UPDATE telegram_pairing_request r
SET attempt_id = request_attempts.attempt_id
FROM request_attempts
WHERE r.id = request_attempts.request_id
  AND r.attempt_id IS NULL
  AND request_attempts.attempt_id IS NOT NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE telegram_pairing_request DROP COLUMN IF EXISTS attempt_id;
-- +goose StatementEnd
