# Tasks: Group Member Management & Ops-Group Seeding

**Input**: Design documents from `specs/010-ops-groups/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/

**Tests**: Backend unit/integration tests requested in plan.md Validation Plan. Written as Go table tests (repo convention) — no separate test-task phase; each implementation task includes a co-located test.

**Organization**: Tasks grouped by user story. Backend service layer is foundational (shared by US1 + US2).

## Format: `[ID] [P?] [Story] Description`

## Phase 1: Setup

**Purpose**: No project init needed (existing backend/frontend). Confirm branch & baseline.

- [ ] T001 Confirm working tree baseline builds before changes: `go build ./...` (backend) and `npm run build` (frontend) pass on branch `020-group-member-management`

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Models + service-layer member operations shared by US1 (UI) and US2 (seed).

**⚠️ CRITICAL**: No user story work until this phase completes.

- [ ] T002 [P] Add `GroupMember` struct (UserID, FullName, Rank, Battery) to `backend/internal/models/group.go`
- [ ] T003 [P] Add `SetMembers(ctx, groupID string, userIDs []string) (int, error)` to `backend/internal/services/groups/service.go` — one tx: verify group exists (else `ErrNotFound`), `DELETE` existing members, `INSERT` deduped list with `ON CONFLICT DO NOTHING`, return new count
- [ ] T004 [P] Add `RemoveMember(ctx, groupID, userID string) error` to `backend/internal/services/groups/service.go` — verify group exists (else `ErrNotFound`), `DELETE ... WHERE group_id AND user_id` (no-op if absent)
- [ ] T005 [P] Add `MembersWithDetails(ctx, groupID string) ([]models.GroupMember, error)` to `backend/internal/services/groups/service.go` — JOIN `participant_group_member` with `"user"` on full_name/rank/battery, ordered by `lower(full_name)`
- [ ] T006 Service tests for T003/T004/T005 in `backend/internal/services/groups/` — TDD: write first, assert failures (replace clears+adds; dedup; missing group → ErrNotFound; remove absent → no-op; detail ordering)

**Checkpoint**: Service layer ready — US1 and US2 can be implemented independently.

---

## Phase 3: User Story 1 - Commander builds and adjusts a group by hand (P1) 🎯 MVP

**Goal**: Tier 3+ users can view, add (search or create-new), and remove group members via the UI — no Excel needed.

**Independent Test**: Open the Groups page, open a group's member dialog, remove a member, search-and-add an existing user, create a new person inline (auto-added to group); counts update.

### Implementation for User Story 1

- [ ] T007 [US1] Extend `GET /api/groups/{id}` handler in `backend/internal/handlers/groups.go` to return `members: []GroupMember` (from `MembersWithDetails`) alongside IDs — update `GroupResponse` to include `Members []models.GroupMember`
- [ ] T008 [US1] Add `SetMembers` handler (`PUT /api/groups/{id}/members`, body `{memberIds}`, 200 + group/count, 404/400 maps) to `backend/internal/handlers/groups.go`
- [ ] T009 [US1] Add `RemoveMember` handler (`DELETE /api/groups/{id}/members/{userId}`, 204, 404 maps) to `backend/internal/handlers/groups.go`
- [ ] T010 [US1] Register the two new routes under the existing `/api/groups` Tier 3 router in `backend/cmd/api/main.go` (`r.With(middleware.RequireUnitCommander(db))`)
- [ ] T011 [US1] Handler tests for T007–T010 in `backend/internal/handlers/groups_test.go` (auth 401/403 paths, 404 unknown group, replace semantics, remove semantics)
- [ ] T012 [US1] Add `setGroupMembers(id, memberIds)` and `deleteGroupMember(id, userId)` + `GroupMember` type + `members` field on `GroupResponse` in `frontend/src/lib/api-client.ts`
- [ ] T013 [US1] Member list dialog on the Groups page: click a group card → `getGroup(id)` → list members (name · rank · battery) with per-row delete in `frontend/src/routes/dashboard/groups/index.tsx`
- [ ] T014 [US1] "Add people" dialog: debounced search against `listUsers({search})`, multi-select checkboxes, "Add selected" → `setGroupMembers(id, existingIds + newIds)` in `frontend/src/routes/dashboard/groups/index.tsx`
- [ ] T015 [US1] "+ New person" affordance reusing `AddUserDialog` (`frontend/src/components/users/add-user-dialog.tsx` — add optional `onCreated` prop so the created user is auto-added to the current group)
- [ ] T016 [US1] Frontend build/lint pass: `npm run build` + `npm run lint`

**Checkpoint**: US1 fully functional — hand-built groups work end to end.

---

## Phase 4: User Story 2 - Seed ops groups from the roster (P1)

**Goal**: CLI creates the 8 ops groups from the roster Excel, idempotently.

**Independent Test**: Run `seed-groups` twice against a DB with imported roster; first run creates 8 groups with expected counts, second run changes nothing; a manually added member survives.

### Implementation for User Story 2

- [ ] T017 [US2] Extract shared roster-parsing helpers (`findDataSheet`, `cellValue`, `normalizeNRICLast5`) to a shared spot usable by `backend/cmd/seed-groups` (keep `groups.go` imports working) — `backend/internal/` helper package or unexported fns promoted
- [ ] T018 [US2] `backend/cmd/seed-groups/main.go`: flags `-file -password -created-by -db-url`; open Excel with excelize; iterate the `236 SA` sheet
- [ ] T019 [US2] Implement the 8 rule predicates (RnS, FDC/BOC, BCS, CSS, PSO rank≥1SG, A/B/HQ Bty) from plan.md/research.md in `backend/cmd/seed-groups/main.go`
- [ ] T020 [US2] User matching: by `nric_last5` (normalized) with exact-name tiebreak among `verified=true`; ambiguous/unmatched rows collected per group
- [ ] T021 [US2] Group create/reuse (`created_by` + `lower(name)`) + member inserts `ON CONFLICT DO NOTHING`; summary output per plan.md contract (created/reused, members added, unmatched list)
- [ ] T022 [US2] Idempotence regression: run twice against dev DB → no dup groups, no lost members incl. a manually-added member
- [ ] T023 [US2] Integration run with the real roster file → assert counts RnS 8 · FDC/BOC 11 · BCS 16 · CSS 76 · PSO 40 · A Bty 90 · B Bty 89 · HQ Bty 139

**Checkpoint**: Seeded ops groups exist and match the confirmed rules.

---

## Phase 5: User Story 3 - Start a session from a seeded group (P2)

**Goal**: Existing session-from-group flow works with the newly seeded ops groups.

**Independent Test**: From the Groups page, "Start Session" on a seeded group → session participants equal group members.

### Implementation for User Story 3

- [ ] T024 [US3] Verification only: create a session from a seeded group (e.g. CSS) on the dev DB and confirm participants match — no code change expected (existing `POST /api/groups/{id}/sessions`)

**Checkpoint**: US3 verified.

---

## Phase 6: Polish & Cross-Cutting Concerns

- [ ] T025 [P] Update `docs/` or QUICKSTART with the seeding instructions (from `specs/010-ops-groups/quickstart.md`) if docs reference groups
- [ ] T026 Run full backend suite `go test ./...` and frontend `npm run build` + `npm run lint`
- [ ] T027 Self-review diff: tight (<500 lines), no unrelated changes; group/member data flow verified end to end on preview

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (T001)**: baseline — no deps
- **Foundational (T002–T006)**: after T001 — blocks US1 + US2
- **US1 (T007–T016)**: after Foundational
- **US2 (T017–T023)**: after Foundational
- **US3 (T024)**: after US1 + US2 (needs a seeded group)
- **Polish (T025–T027)**: after US1 + US2

### User Story Dependencies

- **US1 (P1)**: Foundational only — independent of US2
- **US2 (P1)**: Foundational only — independent of US1 (can run in parallel)
- **US3 (P2)**: depends on US1 + US2 (needs a seeded group to start a session from)

### Parallel Opportunities

- T002–T005 (foundational service methods): all different methods/file regions — [P]
- US1 and US2: fully parallel after Foundational
- T007–T011 (backend US1) vs T012–T016 (frontend US1): backend then frontend, but handler tests independent
- T018–T021 (seed CLI): one-file sequence, not [P]

---

## Parallel Example: after Foundational

```bash
# Worker A — US1 backend endpoints + routes + tests:
Task: T007-T011 (groups.go handlers, main.go routes, groups_test.go)

# Worker B — US2 seed CLI:
Task: T018-T021 (backend/cmd/seed-groups/main.go)

# (US1 frontend T012-T016 waits for T007-T010 API contract in place)
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. T001 baseline
2. T002–T006 foundational service layer
3. T007–T016 US1 (member UI + backend)
4. STOP, validate US1 on preview, deploy if ready

### Incremental Delivery

1. Foundation → US1 (member UI) → US2 (seed CLI) → US3 verify → polish
2. US1 doesn't block US2 (CLI is standalone); US2 adds real group data that US1's UI manages

### Notes

- TDD applies to backend service/handler tasks (write failing tests first, per plan.md Validation Plan & AGENTS.md).
- Commit per logical task group with the repo convention (sign as David, no co-author trailers, never `git add .` for PR work).
- The roster file is at `~/Downloads/236SA 2026 ICT 6 callup (test) 2 (1).xlsx` (password `yeoec`) for the US2 integration run.
