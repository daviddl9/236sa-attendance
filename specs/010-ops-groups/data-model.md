# Phase 1 Data Model: Group Membership

**Date**: 2026-08-17 · **Feature**: [spec.md](./spec.md)

No schema changes. The existing `participant_group` / `participant_group_member` tables (migration `20260817000000_participant_groups.sql`) are sufficient.

## Entities

### ParticipantGroup (existing — unchanged)

| Field | Type | Notes |
|---|---|---|
| id | TEXT PK | generated |
| name | TEXT | unique per created_by (lower) |
| created_by | TEXT FK → user.id | seeded groups owned by a real admin user |
| createdAt / updatedAt | TIMESTAMP | |

### ParticipantGroupMember (existing — now mutable)

| Field | Type | Notes |
|---|---|---|
| group_id | TEXT FK → participant_group (CASCADE) | |
| user_id | TEXT FK → user (CASCADE) | PK (group_id, user_id) |

### GroupMemberDetail (new read model for the UI)

| Field | Type | Source |
|---|---|---|
| UserID | string | participant_group_member.user_id |
| FullName | string | "user".full_name |
| Rank | *string | "user".rank |
| Battery | *string | "user".battery |

## Operations

| Operation | SQL shape | Semantics |
|---|---|---|
| Replace members | tx: `DELETE FROM participant_group_member WHERE group_id=$1` then `INSERT … ON CONFLICT DO NOTHING` per id | atomic replace; returns new count |
| Remove one member | `DELETE FROM participant_group_member WHERE group_id=$1 AND user_id=$2` | no-op if absent; group must exist (checked) |
| List members w/ details | `SELECT m.user_id, u.full_name, u.rank, u.battery FROM participant_group_member m JOIN "user" u ON u.id = m.user_id WHERE m.group_id=$1 ORDER BY lower(u.full_name)` | used by GET /groups/{id} |
| Seed group create | `INSERT INTO participant_group … ON CONFLICT DO NOTHING` (after reuse-check by name+created_by) | idempotent |
| Seed member add | `INSERT INTO participant_group_member … ON CONFLICT DO NOTHING` | additive only |

## Validation rules

- New members must be existing user IDs (FK enforces; handler returns 400 on invalid IDs → reported via constraint failure mapped to a clear error).
- `memberIds` array may be empty (clears a group) — allowed.
- Duplicate IDs in a replace payload are deduped.
- Roster NRIC normalized uppercase before match; only `verified = true` users are match targets.
