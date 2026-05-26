---
description: "Task list for feature 003-add-new-user"
---

# Tasks: Superadmin Adds A Single New User From The Users Dashboard

**Input**: Design documents from `/specs/003-add-new-user/`

**Prerequisites**: [spec.md](./spec.md) (required), [plan.md](./plan.md) (required)

**Tests**: Test tasks are INCLUDED. The plan calls for Go handler tests in package mode and a small Vitest/RTL test for the dialog's duplicate-conflict branch. Manual end-to-end verification of the create flow per `quickstart.md` is also required.

**Organization**: Tasks are grouped by user story so each story can be implemented, tested, and demoed independently.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no in-flight dependency)
- **[Story]**: User story this task belongs to (US1–US6); shared work has no story tag

---

## Phase 1: Setup

**Purpose**: Stub the new files so the rest of the work can compile without circular churn.

- [ ] T001 [P] Add an empty `CreateUser` handler stub plus a `CreateUserRequest` struct to `backend/internal/handlers/user.go` (no body yet, returns 501) so route wiring in T002 compiles
- [ ] T002 Wire the new route in `backend/cmd/api/main.go`: under the existing `r.Route("/users", ...)` group, add `r.With(middleware.RequireSuperadmin(db)).Post("/", userHandler.CreateUser)` directly above the existing `PUT /{id}` line
- [ ] T003 [P] Create empty component file `frontend/src/components/users/add-user-dialog.tsx` exporting a typed `AddUserDialog` shell that renders nothing — keeps the import in T013 valid
- [ ] T004 [P] Add `createUser(payload)` method stub and `CreateUserConflict` response type to `frontend/src/lib/api-client.ts` returning a `Promise<never>` so the dialog can import it

**Checkpoint**: `go build ./...` and `npm run build` both pass with the new empty files in place.

---

## Phase 2: Foundational

**Purpose**: Shared validation and shape used by every user story.

- [ ] T005 Confirm `models.IsSuperadminByRank` (in `backend/internal/models/user.go`) is reusable as-is for the create handler; record finding in `quickstart.md` if any gap is found
- [ ] T006 [P] Verify `normalizeNRICLast5` and `nricLast5FormatMessage` helpers in `backend/internal/handlers/credential_validation.go` are reusable from `CreateUser` (read-only audit; no change expected)
- [ ] T007 [P] Verify `middleware.RequireSuperadmin` covers the new route signature — single read of `backend/internal/middleware/` is sufficient; no code change expected

**Checkpoint**: No foundational work needs new code; the helpers and middleware required by the create handler exist already.

---

## Phase 3: User Story 1 — Superadmin Creates A Single User (Priority: P1) MVP

**Goal**: Superadmin opens the dialog, fills the four required fields, and a new user with `verified = true` appears in the list and can sign in immediately.

**Independent Test**: Sign in as superadmin, click **Add User**, fill Full Name + Rank + Battery + NRIC Last 5, submit, see the user in the list, sign out, sign in as the new user — no signup flow, no pending-approval screen.

### Tests for User Story 1

- [ ] T008 [P] [US1] `backend/internal/handlers/user_test.go` — happy path: `POST /api/users` with valid payload as superadmin returns 201 and the `UserProfile` shape; a `SELECT` afterwards finds the row with `verified = true`, `is_superadmin = false`, hashed `password`
- [ ] T009 [P] [US1] `backend/internal/handlers/user_test.go` — non-superadmin (Tier 1/2/3) returns 403; no row is inserted
- [ ] T010 [P] [US1] `backend/internal/handlers/user_test.go` — missing any of `fullName`, `rank`, `battery`, `nricLast5` returns 400 with the field name in the message; no row is inserted

### Implementation for User Story 1

- [ ] T011 [US1] Implement the `CreateUser` handler body in `backend/internal/handlers/user.go`: decode the request, trim Full Name (and collapse internal whitespace), validate rank against `models.ValidRanks`, validate battery against `BatteryHQ/Alpha/Bravo`, validate + normalise `nricLast5` via `normalizeNRICLast5`, bcrypt-hash, `INSERT` into `"user"` with `verified = true` and `is_superadmin` per `models.IsSuperadminByRank`, return 201 with the `UserProfile` shape matching `GetUser`
- [ ] T012 [US1] Implement `apiClient.createUser` in `frontend/src/lib/api-client.ts`: `POST /api/users` with the payload, return the created `UserProfile` on 201, map 409 to a typed `CreateUserConflict` error so the dialog can branch on it
- [ ] T013 [US1] Implement `frontend/src/components/users/add-user-dialog.tsx`: shadcn `Dialog` with controlled form state for Full Name (`Input`), Rank (`Select` populated from the same list used in `$userId.tsx`), Battery (`Select`: HQ/Alpha/Bravo), NRIC Last 5 (`Input` with the same `isValidNricLast5` validation + uppercase normalisation used in `$userId.tsx`); a submit button that disables while in flight; on success closes the dialog, fires a `toast.success`, and calls `queryClient.invalidateQueries({ queryKey: ['users'] })`
- [ ] T014 [US1] Mount the dialog in `frontend/src/routes/dashboard/users/index.tsx`: add an **Add User** button next to **Bulk Upload** that is rendered only when `isSuperadmin`, open the dialog from local state, pass `queryClient` into the dialog via props

**Checkpoint**: Superadmin can create a single user end-to-end, the row appears in the list immediately, and that user can sign in.

---

## Phase 4: User Story 2 — NRIC Last 5 Format Is Enforced At Create Time (Priority: P1)

**Goal**: Both the client and the server reject NRIC last 5 values that do not match `^\d{4}[A-Za-z]$`. Lowercase input is normalised to uppercase before any DB write.

**Independent Test**: Submit `1234a` → succeeds, stored as `1234A`. Submit `12345` (no letter) → form blocks; submit via API directly → backend returns 400.

### Tests for User Story 2

- [ ] T015 [P] [US2] `backend/internal/handlers/user_test.go` — `nricLast5 = "12345"` (no letter) returns 400 with the standard NRIC message; no row inserted
- [ ] T016 [P] [US2] `backend/internal/handlers/user_test.go` — `nricLast5 = "1234a"` succeeds; the persisted `nric_last5` column is `"1234A"`; the user can sign in with both `1234A` and `1234a` (the auth layer already normalises on read)
- [ ] T017 [P] [US2] `backend/internal/handlers/user_test.go` — body that omits `nricLast5` entirely returns 400; never reaches the bcrypt step

### Implementation for User Story 2

- [ ] T018 [US2] In `CreateUser` (T011), call `normalizeNRICLast5` before duplicate detection and before the `INSERT`; return 400 with `nricLast5FormatMessage` when normalisation fails — same path used by `RegisterUser`
- [ ] T019 [US2] In `add-user-dialog.tsx` (T013), wire the NRIC Last 5 input to `isValidNricLast5` for inline validation: show `NRIC_LAST5_FIELD_MESSAGE` below the field when invalid, disable the submit button until valid, and normalise via `normalizeNricLast5` before sending

**Checkpoint**: Invalid NRIC last 5 values are rejected at both layers; lowercase input round-trips as uppercase.

---

## Phase 5: User Story 3 — CPT+ Auto-Superadmin (Priority: P1)

**Goal**: A user created with rank ≥ CPT has `is_superadmin = true` automatically and reports `tier = 4` on `/api/auth/session`.

**Independent Test**: Create a user with rank CPT, sign in as them, inspect `/api/auth/session` — `isSuperadmin: true, tier: 4`.

### Tests for User Story 3

- [ ] T020 [P] [US3] `backend/internal/handlers/user_test.go` — rank `CPT` → row inserted with `is_superadmin = true`
- [ ] T021 [P] [US3] `backend/internal/handlers/user_test.go` — rank `LTC` → row inserted with `is_superadmin = true`
- [ ] T022 [P] [US3] `backend/internal/handlers/user_test.go` — rank `LTA` → row inserted with `is_superadmin = false`

### Implementation for User Story 3

- [ ] T023 [US3] In `CreateUser` (T011), compute `isSuperadmin = models.IsSuperadminByRank(req.Rank)` and pass it to the `INSERT`. No frontend change.

**Checkpoint**: CPT+ users created via the dialog gain superadmin access on first sign-in.

---

## Phase 6: User Story 4 — Duplicate Detection (Priority: P1)

**Goal**: A second create attempt for the same `(LOWER(full_name), nric_last5)` returns 409 with enough information for the frontend to link to the existing record. No second row is ever inserted.

**Independent Test**: Create `John Doe / 1234A`; submit again — dialog shows "already exists" with a link to the existing detail page. Confirm the DB still has exactly one row.

### Tests for User Story 4

- [ ] T024 [P] [US4] `backend/internal/handlers/user_test.go` — pre-seed a verified user `John Doe / 1234A`; second `POST /api/users` returns 409 with `existingUserId` matching the seed and `verified = true`; only one row exists afterwards
- [ ] T025 [P] [US4] `backend/internal/handlers/user_test.go` — pre-seed an unverified user `John Doe / 1234A` (verified=false, e.g. via `RegisterUser`); second `POST /api/users` returns 409 with `verified = false`; no second row is inserted
- [ ] T026 [P] [US4] `backend/internal/handlers/user_test.go` — duplicate detection is case-insensitive on Full Name: a seed with `"John Doe"` collides with a create attempt for `"john doe"` and `"JOHN DOE"`
- [ ] T027 [P] [US4] `frontend/src/components/users/__tests__/add-user-dialog.test.tsx` — when the API returns a 409 with `verified: true`, the dialog renders the "View existing user" link pointing at `/dashboard/users/{id}`; with `verified: false`, the link points at `/dashboard/admin/registrations`

### Implementation for User Story 4

- [ ] T028 [US4] Add the pre-insert duplicate check in `CreateUser` (T011): `SELECT id, verified FROM "user" WHERE LOWER(full_name) = LOWER($1) AND nric_last5 = $2 LIMIT 1` after normalisation; on hit, return 409 with `{ error: "user_exists", existingUserId, verified, fullName }`
- [ ] T029 [US4] In `add-user-dialog.tsx`, catch the typed `CreateUserConflict` thrown by `apiClient.createUser`: render an inline conflict block above the form with the existing user's full name and a link routed by `verified` (detail page vs registrations); keep the form values intact so the admin can adjust and retry
- [ ] T030 [US4] Verify trim + whitespace-collapse on Full Name happens before the duplicate check, so `"  John  Doe  "` is treated as a duplicate of `"John Doe"` (FR-008); add a backend test case for this in `user_test.go`

**Checkpoint**: Duplicate Full Name + NRIC Last 5 combinations are caught; admins are guided to the existing record.

---

## Phase 7: User Story 5 — Access Control On The Dialog Action And Endpoint (Priority: P2)

**Goal**: Non-superadmins cannot see the **Add User** button and cannot hit the endpoint directly.

**Independent Test**: As a Tier 3 user, confirm the button is invisible on `/dashboard/users`; call `POST /api/users` directly with the session cookie — receive 403.

### Tests for User Story 5

- [ ] T031 [P] [US5] `backend/internal/handlers/user_test.go` — Tier 1, Tier 2, Tier 3, and unauthenticated requests to `POST /api/users` all return 401 or 403 as appropriate; no row is inserted

### Implementation for User Story 5

- [ ] T032 [US5] In `frontend/src/routes/dashboard/users/index.tsx`, wrap the **Add User** button in the existing `isSuperadmin` conditional (the same one already guarding **Bulk Delete** and **Bulk Upload**); confirm the dialog mount is also conditional so a non-superadmin who somehow lands on the page does not hold dialog state
- [ ] T033 [US5] No backend change beyond the route's `RequireSuperadmin` middleware — confirm via T031

**Checkpoint**: The create action is invisible and unreachable to non-superadmins.

---

## Phase 8: User Story 6 — Optional Fields (Priority: P3)

**Goal**: DOB (DDMMYY) and free-text extras key-value rows can be filled in optionally; empty values do not block submission.

**Independent Test**: Create a user with only required fields → success. Create a second user with DOB `150393` and one extras row → both values appear on the user detail page.

### Tests for User Story 6

- [ ] T034 [P] [US6] `backend/internal/handlers/user_test.go` — body with no `dob` and no `extras` succeeds; the persisted row has `dob = NULL` and `extras = '{}'`
- [ ] T035 [P] [US6] `backend/internal/handlers/user_test.go` — body with `dob = "150393"` succeeds; `dob = "150393"` on the row
- [ ] T036 [P] [US6] `backend/internal/handlers/user_test.go` — body with `dob = "15-03-93"` (8 chars) returns 400 with a DDMMYY message; no row inserted
- [ ] T037 [P] [US6] `backend/internal/handlers/user_test.go` — body with `extras = { "Section": "Recon" }` round-trips correctly on `GET /api/users/{id}`

### Implementation for User Story 6

- [ ] T038 [US6] Extend `CreateUser` to accept and persist optional `dob` (6-char DDMMYY validation matching `RegisterUser`) and `extras` (JSON object → JSONB column). Default `extras` to `'{}'::jsonb` when absent.
- [ ] T039 [US6] In `add-user-dialog.tsx`, add a collapsible "Optional fields" section with a 6-char DOB input and a minimal key/value extras editor (an "Add row" button appends a `{ key: '', value: '' }` pair; rows with empty keys are dropped before submit)

**Checkpoint**: DOB and extras can be captured at create time but never block submission.

---

## Phase 9: Polish & Cross-Cutting

- [ ] T040 [P] Run `npm run build` and `npm run lint` in `frontend/`; fix any warnings introduced by the new files
- [ ] T041 [P] Run `go test ./backend/internal/handlers/...` (package mode — see project memory about the smart-test hook false positive) and confirm all new tests pass
- [ ] T042 Manual end-to-end run from `specs/003-add-new-user/quickstart.md`: walk every US verification step
- [ ] T043 [P] Add a one-line note to `README.md` under user management mentioning the new admin-side single-user create action
- [ ] T044 Deploy gate: push to `main`, monitor `gh run watch <id>` per CLAUDE.md, then SSH and confirm `sudo docker logs apps-attendance-api-1 --tail 50` shows clean startup

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately.
- **Foundational (Phase 2)**: Read-only audit; no blocking work.
- **US1 (Phase 3)**: Depends on Setup. Delivers the MVP.
- **US2 (Phase 4)**: Depends on US1 (extends the handler and the dialog).
- **US3 (Phase 5)**: Depends on US1 (one extra line in the `INSERT` path).
- **US4 (Phase 6)**: Depends on US1 (pre-insert query + conflict response shape).
- **US5 (Phase 7)**: Depends on US1 (only conditional rendering + a regression test).
- **US6 (Phase 8)**: Depends on US1 (extends the request body and the form).
- **Polish (Phase 9)**: Depends on whichever user stories are in scope for the release.

### Within Each User Story

- Tests are written alongside implementation; package-mode `go test ./internal/handlers/...` is the authoritative pass/fail signal.
- Backend handler changes come before frontend wiring within a story.
- Each user story ends at a checkpoint that can be independently demoed.

### Parallel Opportunities

- All Phase 1 setup tasks marked [P] (T001, T003, T004) can run in parallel.
- US2, US3, US4, US5, and US6 all build on top of US1 but touch largely disjoint code paths — once US1 lands, US2/US3/US4/US5/US6 can proceed in parallel.

---

## Implementation Strategy

### MVP First (US1 only)

1. Phase 1 → Phase 2 → Phase 3. Stop at the US1 checkpoint.
2. Demo: superadmin creates a user, the user signs in. This alone is a meaningful win — admins no longer need a one-row Excel file.

### Incremental Delivery

1. **MVP** — US1: create a user end-to-end.
2. **+US2** — NRIC format enforcement on both layers (small but important safety net).
3. **+US3** — CPT+ auto-superadmin (one-line backend change).
4. **+US4** — duplicate detection (the most user-visible polish step).
5. **+US5** — access-control hardening (one conditional + one test).
6. **+US6** — DOB and extras (nice-to-have, ships when there's appetite).

Ship after any of the above; each delivers user-visible value without breaking earlier slices.

---

## Notes

- The `agent` package is irrelevant to this feature — `CreateUser` does not invoke Anthropic or any external service.
- Tier override and explicit `is_superadmin` are deliberately not accepted by the create endpoint; admins promote via the existing user detail page.
- The smart-test hook's "test failed" output for `go test <file>` is a known false positive in this repo; trust `go test ./internal/handlers/...` (package mode) as the source of truth.
- Commit after each task or logical group; keep PRs scoped per user story so each can land independently per the incremental strategy.
