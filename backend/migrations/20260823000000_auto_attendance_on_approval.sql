-- +goose Up
-- +goose StatementBegin
-- Auto-attendance on approval: a pending registration may carry the session
-- the soldier scanned before signing up. The commander's approval marks that
-- session atomically with account creation, so a first-time scan is not lost.
ALTER TABLE pending_registration
  ADD COLUMN IF NOT EXISTS qr_session_id TEXT,
  ADD COLUMN IF NOT EXISTS qr_secret TEXT;

-- The approval auto-mark is a distinct marking method so reports can tell a
-- real-time scan apart from a mark that happened at approval time.
ALTER TABLE attendance_record
  DROP CONSTRAINT IF EXISTS attendance_record_marking_method_check;

ALTER TABLE attendance_record
  ADD CONSTRAINT attendance_record_marking_method_check
  CHECK (marking_method IN ('qr_scan', 'telegram_scan', 'manual', 'approval_auto'));
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Rollback is intentionally non-destructive: the old constraint cannot
-- represent a committed approval_auto row, so refuse before changing the
-- constraint rather than deleting or relabelling attendance history.
DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM attendance_record WHERE marking_method = 'approval_auto'
  ) THEN
    RAISE EXCEPTION USING MESSAGE =
      'cannot roll back approval_auto marking method constraint while approval_auto attendance records exist; remove or migrate those records before retrying';
  END IF;
END
$$;

ALTER TABLE attendance_record
  DROP CONSTRAINT IF EXISTS attendance_record_marking_method_check;

ALTER TABLE attendance_record
  ADD CONSTRAINT attendance_record_marking_method_check
  CHECK (marking_method IN ('qr_scan', 'telegram_scan', 'manual'));

ALTER TABLE pending_registration
  DROP COLUMN IF EXISTS qr_session_id,
  DROP COLUMN IF EXISTS qr_secret;
-- +goose StatementEnd
