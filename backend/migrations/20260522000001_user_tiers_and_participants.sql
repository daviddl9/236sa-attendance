-- Feature 003: user tiers (tier_override + verified)
ALTER TABLE "user"
  ADD COLUMN IF NOT EXISTS tier_override SMALLINT DEFAULT NULL
  CHECK (tier_override IN (2, 3));

ALTER TABLE "user"
  ADD COLUMN IF NOT EXISTS verified BOOLEAN NOT NULL DEFAULT true;

-- Feature 004: custom-list session participants
CREATE TABLE IF NOT EXISTS session_participants (
    session_id TEXT NOT NULL REFERENCES attendance_session(id) ON DELETE CASCADE,
    user_id    TEXT NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
    PRIMARY KEY (session_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_session_participants_session
    ON session_participants(session_id);

-- Widen scope constraint to allow 'custom_list'
ALTER TABLE attendance_session
  DROP CONSTRAINT IF EXISTS attendance_session_scope_check;

ALTER TABLE attendance_session
  ADD CONSTRAINT attendance_session_scope_check
  CHECK (scope IN ('unit_wide', 'battery_specific', 'custom_list'));
