-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS credential_audit (
    id TEXT PRIMARY KEY,
    actor_user_id TEXT REFERENCES "user"(id) ON DELETE SET NULL,
    target_user_id TEXT NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
    action TEXT NOT NULL CHECK (action IN ('approval', 'credential_provision')),
    "createdAt" TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_credential_audit_target
  ON credential_audit(target_user_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_credential_audit_target;
DROP TABLE IF EXISTS credential_audit;
-- +goose StatementEnd
