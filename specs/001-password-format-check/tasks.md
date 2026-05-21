# Tasks: Password Format Check

**Input**: Design documents from `specs/001-password-format-check/`

**Prerequisites**: [plan.md](./plan.md), [spec.md](./spec.md), [research.md](./research.md), [data-model.md](./data-model.md), [contracts/password-format-api.md](./contracts/password-format-api.md), [quickstart.md](./quickstart.md)

**Tests**: Focused backend validation tests are included because the plan calls for backend validation as the source of truth and targeted handler/unit coverage. Frontend verification uses lint/build plus manual UI checks from quickstart.

**Organization**: Tasks are grouped by user story so valid sign-in, invalid-format feedback, and admin personnel creation/import validation can be delivered and tested independently.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel because it touches different files and does not depend on incomplete tasks
- **[Story]**: Maps to the user story from `spec.md`
- Every task includes an exact repository path

## Phase 1: Setup (Shared Context)

**Purpose**: Confirm current behavior and avoid disturbing unrelated work already in the branch.

- [x] T001 Review existing uncommitted changes in `backend/internal/handlers/admin.go`, `backend/internal/handlers/user.go`, `backend/internal/models/user.go`, `frontend/src/components/users/user-table.tsx`, `frontend/src/lib/api-client.ts`, and `frontend/src/routes/dashboard/users/bulk-upload.tsx` before editing overlapping files
- [x] T002 [P] Confirm current sign-in password behavior in `backend/internal/handlers/auth.go`, `frontend/src/routes/sign-in.tsx`, and `frontend/src/routes/index.tsx`
- [x] T003 [P] Confirm current registration and admin user edit NRIC field behavior in `backend/internal/handlers/user.go`, `frontend/src/routes/attendance/register.tsx`, and `frontend/src/routes/dashboard/users/$userId.tsx`
- [x] T004 [P] Confirm current bulk personnel validation and upload copy in `backend/internal/handlers/admin.go`, `frontend/src/lib/parse-excel.ts`, and `frontend/src/routes/dashboard/users/bulk-upload.tsx`

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Add shared validation primitives used by all user stories.

**CRITICAL**: No user story implementation should begin until these tasks are complete.

- [x] T005 Create backend NRIC Last 5 validation helper in `backend/internal/handlers/credential_validation.go`
- [x] T006 [P] Create backend validation unit tests for valid values, invalid values, lowercase final letters, and whitespace cases in `backend/internal/handlers/credential_validation_test.go`
- [x] T007 Create frontend NRIC Last 5 validation helper and message constants in `frontend/src/lib/nric-password.ts`
- [x] T008 Run focused backend validation tests for `backend/internal/handlers/credential_validation_test.go` with the package-equivalent `go test` command from repository root

**Checkpoint**: Shared validation is ready and all user stories can use the same rule.

---

## Phase 3: User Story 1 - Personnel Sign In With Valid NRIC Last 5 (Priority: P1) MVP

**Goal**: Regular personnel can enter a valid four-digits-plus-letter password and continue through normal sign-in, QR redirect, and attendance behavior.

**Independent Test**: Enter a regular personnel identifier with `1234A` or `1234a`; the format check passes and the existing authentication flow continues.

### Tests for User Story 1

- [x] T009 [P] [US1] Add backend sign-in tests for valid uppercase/lowercase personnel passwords and administrator exemption in `backend/internal/handlers/auth_test.go`

### Implementation for User Story 1

- [x] T010 [US1] Apply backend personnel password format validation without blocking administrator sign-in in `backend/internal/handlers/auth.go`
- [x] T011 [P] [US1] Replace stale 10-character sign-in validation with NRIC Last 5 validation in `frontend/src/routes/sign-in.tsx`
- [x] T012 [P] [US1] Add matching NRIC Last 5 sign-in validation to the root sign-in form in `frontend/src/routes/index.tsx`
- [x] T013 [US1] Preserve QR token and redirect behavior after valid sign-in in `frontend/src/routes/sign-in.tsx`
- [x] T014 [US1] Run `go test ./...` for `backend/` and confirm valid personnel/admin sign-in behavior in `backend/internal/handlers/auth.go` is not regressed

**Checkpoint**: User Story 1 is fully functional and testable independently.

---

## Phase 4: User Story 2 - Personnel Get Clear Feedback For Invalid Password Format (Priority: P1)

**Goal**: Regular personnel receive clear immediate feedback when the password is not exactly four digits followed by one letter.

**Independent Test**: Try `12345`, `123A4`, `1234@`, `1234AB`, and `123A` on each sign-in form; each is rejected with the expected format and example.

### Tests for User Story 2

- [x] T015 [P] [US2] Add backend sign-in tests that reject invalid personnel password formats without creating sessions or incomplete users in `backend/internal/handlers/auth_test.go`

### Implementation for User Story 2

- [x] T016 [US2] Return a clear client error for invalid regular-personnel password format in `backend/internal/handlers/auth.go`
- [x] T017 [P] [US2] Show the shared invalid-format message before submission in `frontend/src/routes/sign-in.tsx`
- [x] T018 [P] [US2] Show the shared invalid-format message before submission in `frontend/src/routes/index.tsx`
- [x] T019 [US2] Ensure password tooltip/copy names NRIC Last 5 and example `1234A` consistently in `frontend/src/routes/sign-in.tsx` and `frontend/src/routes/index.tsx`
- [x] T020 [US2] Run frontend validation checks for `frontend/src/routes/sign-in.tsx`, `frontend/src/routes/index.tsx`, and `frontend/src/lib/nric-password.ts` with `cd frontend && npm run lint && npm run build`

**Checkpoint**: User Story 2 is fully functional and testable independently.

---

## Phase 5: User Story 3 - Administrators Add Personnel With Valid NRIC Last 5 Values (Priority: P2)

**Goal**: Administrator creation, update, and import flows enforce the same NRIC Last 5 format so bad personnel records are rejected before they are stored.

**Independent Test**: Submit valid and invalid NRIC Last 5 values through registration, user update, and bulk create/upload flows; invalid rows are rejected with actionable feedback while valid rows remain eligible.

### Tests for User Story 3

- [x] T021 [P] [US3] Add backend bulk validation tests for valid and invalid NRIC Last 5 rows in `backend/internal/handlers/admin_test.go`
- [x] T022 [P] [US3] Add backend registration/update validation tests for valid and invalid NRIC Last 5 values in `backend/internal/handlers/user_test.go`

### Implementation for User Story 3

- [x] T023 [US3] Replace length-only NRIC Last 5 checks in bulk upload and bulk create with shared format validation in `backend/internal/handlers/admin.go`
- [x] T024 [US3] Apply shared NRIC Last 5 validation to registration and superadmin user update paths in `backend/internal/handlers/user.go`
- [x] T025 [US3] Align registration request/response type names from NRIC Last 4 to NRIC Last 5 in `frontend/src/lib/api-client.ts`
- [x] T026 [US3] Update registration field label, placeholder, max length, input normalization, and validation message for NRIC Last 5 in `frontend/src/routes/attendance/register.tsx`
- [x] T027 [US3] Ensure admin user edit UI sends and labels NRIC Last 5 consistently in `frontend/src/routes/dashboard/users/$userId.tsx`
- [x] T028 [US3] Add or preserve frontend bulk-upload pre-submit guidance for four digits plus one letter in `frontend/src/routes/dashboard/users/bulk-upload.tsx`
- [x] T029 [US3] Confirm Excel parsing keeps NRIC Last 5 values intact and does not truncate the final letter in `frontend/src/lib/parse-excel.ts`
- [x] T030 [US3] Run `go test ./...` for `backend/` and `cd frontend && npm run lint && npm run build` for `frontend/`

**Checkpoint**: User Story 3 is fully functional and testable independently.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Final consistency checks and documentation validation across the feature.

- [x] T031 [P] Verify quickstart manual checks in `specs/001-password-format-check/quickstart.md`
- [x] T032 [P] Search for stale `NRIC Last 4`, `10 characters`, and `DOB` password copy in `backend/` and `frontend/src/`
- [x] T033 Update any remaining stale password-format copy found by T032 in `backend/` and `frontend/src/`
- [x] T034 Run final repository checks with `go test ./...` for `backend/` and `cd frontend && npm run lint && npm run build` for `frontend/`
- [x] T035 Review `specs/001-password-format-check/tasks.md` and mark completed tasks accurately after implementation

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies; can start immediately
- **Foundational (Phase 2)**: Depends on Setup completion; blocks all user stories
- **User Story 1 (Phase 3)**: Depends on Foundational; recommended MVP
- **User Story 2 (Phase 4)**: Depends on Foundational and can be implemented alongside US1, but final user-facing copy should remain consistent with US1
- **User Story 3 (Phase 5)**: Depends on Foundational and can be implemented after or alongside US1/US2
- **Polish (Phase 6)**: Depends on selected user stories being complete

### User Story Dependencies

- **US1 (P1)**: No dependency on other stories after Foundational
- **US2 (P1)**: No dependency on other stories after Foundational; shares frontend sign-in files with US1, so coordinate edits if parallelized
- **US3 (P2)**: No dependency on US1/US2 after Foundational; shares the validation helper only

### Within Each User Story

- Write or update tests before behavior changes where a backend test file is listed
- Implement backend validation before relying on frontend validation
- Update frontend copy and input behavior after shared validation messages exist
- Run story-specific checks before moving to the next story

---

## Parallel Opportunities

- T002, T003, and T004 can run in parallel during setup
- T006 can be written while T005 is being drafted, then finalized once the helper signature is known
- T011 and T012 can run in parallel after T007
- T017 and T018 can run in parallel after T007
- T021 and T022 can run in parallel after T005
- T026, T027, T028, and T029 touch different frontend files and can run in parallel after T007 and T025
- T031 and T032 can run in parallel during polish

## Parallel Example: User Story 1

```bash
Task: "Add backend sign-in tests for valid uppercase/lowercase personnel passwords and administrator exemption in backend/internal/handlers/auth_test.go"
Task: "Replace stale 10-character sign-in validation with NRIC Last 5 validation in frontend/src/routes/sign-in.tsx"
Task: "Add matching NRIC Last 5 sign-in validation to the root sign-in form in frontend/src/routes/index.tsx"
```

## Parallel Example: User Story 2

```bash
Task: "Show the shared invalid-format message before submission in frontend/src/routes/sign-in.tsx"
Task: "Show the shared invalid-format message before submission in frontend/src/routes/index.tsx"
```

## Parallel Example: User Story 3

```bash
Task: "Add backend bulk validation tests for valid and invalid NRIC Last 5 rows in backend/internal/handlers/admin_test.go"
Task: "Add backend registration/update validation tests for valid and invalid NRIC Last 5 values in backend/internal/handlers/user_test.go"
Task: "Update registration field label, placeholder, max length, input normalization, and validation message for NRIC Last 5 in frontend/src/routes/attendance/register.tsx"
Task: "Add or preserve frontend bulk-upload pre-submit guidance for four digits plus one letter in frontend/src/routes/dashboard/users/bulk-upload.tsx"
```

---

## Implementation Strategy

### MVP First (US1 Only)

1. Complete Phase 1 setup checks.
2. Complete Phase 2 shared validation helpers.
3. Complete Phase 3 valid personnel sign-in behavior.
4. Stop and validate US1 independently with `1234A`, `1234a`, and administrator sign-in.

### Incremental Delivery

1. Deliver US1 so valid personnel sign-in works with the new format.
2. Deliver US2 so invalid personnel passwords are rejected with clear feedback.
3. Deliver US3 so admin/import/registration paths cannot create bad personnel records.
4. Complete polish checks and final validation commands.

### Notes

- Preserve existing uncommitted user work; do not revert unrelated changes in files that overlap this feature.
- Backend validation is the source of truth; frontend validation is for faster feedback.
- Administrator credentials remain exempt from the regular personnel NRIC Last 5 format.
- No database migration is expected for this feature.
