-- +goose Up
-- +goose StatementBegin

-- "Out" sessions track soldiers leaving camp for the evening (Nights Out),
-- overnight (Stay Out) or for a period (Off Pass). They carry no roster:
-- soldiers enrol themselves by scanning at the gate.
--
-- session_type existed in the original schema and was dropped in
-- 20240106000000. It returns with a narrower meaning; the default classifies
-- every existing row as an ordinary attendance session.
ALTER TABLE attendance_session
  ADD COLUMN IF NOT EXISTS session_type TEXT NOT NULL DEFAULT 'attendance',
  ADD COLUMN IF NOT EXISTS out_subtype TEXT,
  ADD COLUMN IF NOT EXISTS expected_return_at TIMESTAMPTZ;

ALTER TABLE attendance_session
  DROP CONSTRAINT IF EXISTS attendance_session_session_type_check;
ALTER TABLE attendance_session
  ADD CONSTRAINT attendance_session_session_type_check
  CHECK (session_type IN ('attendance', 'out'));

ALTER TABLE attendance_session
  DROP CONSTRAINT IF EXISTS attendance_session_out_subtype_check;
ALTER TABLE attendance_session
  ADD CONSTRAINT attendance_session_out_subtype_check
  CHECK (out_subtype IS NULL OR out_subtype IN ('nights_out', 'stay_out', 'off_pass'));

-- An Out session always carries a sub-type and an expected return time, and
-- never an end_time. Out sessions must survive overnight and close only when a
-- commander closes them, so they must stay unreachable by the end-time expiry
-- path in services/attendance. Every other session carries none of the three.
ALTER TABLE attendance_session
  DROP CONSTRAINT IF EXISTS attendance_session_out_fields_check;
ALTER TABLE attendance_session
  ADD CONSTRAINT attendance_session_out_fields_check CHECK (
    (session_type = 'out'
       AND out_subtype IS NOT NULL
       AND expected_return_at IS NOT NULL
       AND end_time IS NULL)
    OR
    (session_type <> 'out'
       AND out_subtype IS NULL
       AND expected_return_at IS NULL)
  );

CREATE INDEX IF NOT EXISTS idx_attendance_session_type
  ON attendance_session (session_type);

-- Append-only movement log. A person's current direction is the latest
-- non-voided row. Corrections void rows rather than deleting them, so the gate
-- timeline survives a commander fixing someone's mistake.
CREATE TABLE IF NOT EXISTS out_movement (
    id           TEXT PRIMARY KEY,
    session_id   TEXT NOT NULL REFERENCES attendance_session(id) ON DELETE CASCADE,
    user_id      TEXT NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
    direction    TEXT NOT NULL CHECK (direction IN ('out', 'in')),
    occurred_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    method       TEXT NOT NULL CHECK (method IN ('qr_scan', 'telegram_scan', 'manual')),
    recorded_by  TEXT REFERENCES "user"(id) ON DELETE SET NULL,
    voided_at    TIMESTAMPTZ,
    voided_by    TEXT REFERENCES "user"(id) ON DELETE SET NULL,
    void_reason  TEXT,
    "createdAt"  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT out_movement_manual_has_recorder
      CHECK (method <> 'manual' OR recorded_by IS NOT NULL),
    CONSTRAINT out_movement_void_is_complete
      CHECK ((voided_at IS NULL) = (voided_by IS NULL))
);

-- Partial index matching the current-state lookup: latest live row per person.
CREATE INDEX IF NOT EXISTS idx_out_movement_state
  ON out_movement (session_id, user_id, occurred_at DESC, id DESC)
  WHERE voided_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_out_movement_session
  ON out_movement (session_id);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS out_movement;

DROP INDEX IF EXISTS idx_attendance_session_type;

ALTER TABLE attendance_session
  DROP CONSTRAINT IF EXISTS attendance_session_out_fields_check,
  DROP CONSTRAINT IF EXISTS attendance_session_out_subtype_check,
  DROP CONSTRAINT IF EXISTS attendance_session_session_type_check;

ALTER TABLE attendance_session
  DROP COLUMN IF EXISTS expected_return_at,
  DROP COLUMN IF EXISTS out_subtype,
  DROP COLUMN IF EXISTS session_type;
-- +goose StatementEnd
