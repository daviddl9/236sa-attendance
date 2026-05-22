-- +goose Up
-- +goose StatementBegin

-- Backfill: existing CPT+ users should be superadmin
UPDATE "user"
SET is_superadmin = true
WHERE rank IN ('CPT', 'MAJ', 'LTC')
  AND is_superadmin = false;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Not reversible — we don't know which CPT+ users were manually set vs backfilled
-- No-op down migration

-- +goose StatementEnd
