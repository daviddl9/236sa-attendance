-- +goose Up
-- +goose StatementBegin
ALTER TABLE attendance_record
  DROP CONSTRAINT IF EXISTS attendance_record_marking_method_check;

ALTER TABLE attendance_record
  ADD CONSTRAINT attendance_record_marking_method_check
  CHECK (marking_method IN ('qr_scan', 'telegram_scan', 'manual'));
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Rollback is intentionally non-destructive. The old constraint cannot
-- represent a committed telegram_scan row, so refuse before changing the
-- constraint rather than deleting or relabelling attendance history.
DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM attendance_record WHERE marking_method = 'telegram_scan'
  ) THEN
    RAISE EXCEPTION USING MESSAGE =
      'cannot roll back telegram_scan marking method constraint while telegram_scan attendance records exist; remove or migrate those records before retrying';
  END IF;
END
$$;

ALTER TABLE attendance_record
  DROP CONSTRAINT IF EXISTS attendance_record_marking_method_check;

ALTER TABLE attendance_record
  ADD CONSTRAINT attendance_record_marking_method_check
  CHECK (marking_method IN ('qr_scan', 'manual'));
-- +goose StatementEnd
