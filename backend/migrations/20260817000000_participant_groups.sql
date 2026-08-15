-- +goose Up
-- +goose StatementBegin
-- A named, reusable participant list, decoupled from any single session so a
-- recurring group (e.g. the advance party) is created once and reused for every
-- callup instead of re-uploading and re-matching an Excel each time.
CREATE TABLE participant_group (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    created_by  TEXT NOT NULL REFERENCES "user"(id),
    "createdAt" TIMESTAMP NOT NULL DEFAULT NOW(),
    "updatedAt" TIMESTAMP NOT NULL DEFAULT NOW()
);

-- One creator should not end up with duplicate groups under the same name.
CREATE UNIQUE INDEX idx_participant_group_creator_name
  ON participant_group (created_by, lower(name));

CREATE TABLE participant_group_member (
    group_id TEXT NOT NULL REFERENCES participant_group(id) ON DELETE CASCADE,
    user_id  TEXT NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
    PRIMARY KEY (group_id, user_id)
);

CREATE INDEX idx_participant_group_member_user ON participant_group_member (user_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS participant_group_member;
DROP TABLE IF EXISTS participant_group;
-- +goose StatementEnd
