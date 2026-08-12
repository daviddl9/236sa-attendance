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
ALTER TABLE attendance_record
  DROP CONSTRAINT IF EXISTS attendance_record_marking_method_check;

ALTER TABLE attendance_record
  ADD CONSTRAINT attendance_record_marking_method_check
  CHECK (marking_method IN ('qr_scan', 'manual'));
-- +goose StatementEnd
