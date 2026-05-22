---
description: "Task list for feature 002-agent-onboarding-flow"
---

# Tasks: Agent-Assisted Onboarding And Explicit Signup For New Users

**Input**: Design documents from `/specs/002-agent-onboarding-flow/`

**Prerequisites**: [spec.md](./spec.md) (required), [plan.md](./plan.md) (required)

**Tests**: Test tasks are INCLUDED. The plan calls for handler/service unit tests in Go (package-mode) and Vitest/RTL tests for the new sign-in branches and `NoPasteInput`. Manual end-to-end verification of the Anthropic-backed parse path against a real API key in dev is also required.

**Organization**: Tasks are grouped by user story so each story can be implemented, tested, and demoed independently.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on uncompleted tasks)
- **[Story]**: User story this task belongs to (US1–US5); shared work has no story tag

---

## Phase 1: Setup

**Purpose**: Wire the new server-side secret and create the empty package skeleton so later tasks can import without circular churn.

- [ ] T001 Add `ANTHROPIC_API_KEY` to `backend/.env.example` with a short comment pointing at the doc-import feature
- [ ] T002 Add `ANTHROPIC_API_KEY` to the `environment:` block of `docker-compose.yml` (passthrough only; never inline the value)
- [ ] T003 [P] Document the env var and `redcon.236sa.one` rollout step in `specs/002-agent-onboarding-flow/quickstart.md` (create file)
- [ ] T004 [P] Create empty package files: `backend/internal/services/agent/client.go`, `backend/internal/services/agent/parser.go`, `backend/internal/services/agent/doc.go` (package comment only)
- [ ] T005 [P] Create empty handler stub `backend/internal/handlers/import.go` and empty test file `backend/internal/handlers/import_test.go` (package decl only)
- [ ] T006 [P] Create empty `frontend/src/components/no-paste-input.tsx` with a typed component shell that renders a regular `<input>` (no behaviour yet)

**Checkpoint**: Repo builds (`go build ./...` and `npm run build`) with the new empty files in place.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Schema, models, and the `services/agent` contract that every user story depends on. **No user story work begins until this phase is complete.**

- [ ] T007 Add migration `backend/migrations/00X_personnel_imports.sql` creating tables `personnel_imports` (id, admin_user_id, document_filename, merge_mode nullable until commit, row_counts JSONB, started_at, finished_at nullable, status enum [`pending`,`previewed`,`committed`,`failed`], error_message nullable) and `personnel_import_changes` (id, import_id FK, user_id FK nullable for `failed`, action enum [`created`,`updated`,`skipped`,`failed`], field_name nullable, before_value nullable, after_value nullable, reason nullable) with appropriate indexes (`personnel_imports.admin_user_id`, `personnel_import_changes.import_id`)
- [ ] T008 [P] Create `backend/internal/models/import.go` with `PersonnelImport`, `PersonnelImportChange`, `MergeMode`, and `ImportStatus` Go types matching the migration; no logic, only types and JSON tags
- [ ] T009 [P] Define the `agent.ParsedPersonnelRow` struct and the `agent.Parser` interface signature (`ParseDocument(ctx, filename, contents, mediaType) ([]ParsedPersonnelRow, error)`) in `backend/internal/services/agent/doc.go` — interface only, no implementation
- [ ] T010 [P] Add typed errors `agent.ErrAPITimeout`, `agent.ErrInvalidAPIKey`, `agent.ErrNoRowsDetected`, `agent.ErrDocumentTooLarge` in `backend/internal/services/agent/doc.go` so handlers and tests can match without sniffing strings
- [ ] T011 Verify `RequireSuperadmin` middleware in `backend/internal/middleware/rbac.go` is sufficient for the new admin routes (read-only audit, no code change expected); record finding in `quickstart.md`

**Checkpoint**: Migration applies cleanly against the dev DB; `go build ./...` passes; `agent.Parser` interface is callable but not yet implemented.

---

## Phase 3: User Story 1 — Administrator Imports Personnel From A Source Document (Priority: P1) MVP

**Goal**: Admin can upload a document, the backend calls the Claude agent, and a structured preview comes back with match status — **without any DB write to the `user` table yet**.

**Independent Test**: Admin uploads a sample PDF / DOCX / XLSX on the new import page, sees a per-row preview with each row labelled `New` or `Existing match`, and confirms that the `user` table is unchanged afterward.

### Tests for User Story 1

- [ ] T012 [P] [US1] `backend/internal/services/agent/parser_test.go` — golden-fixture test: a small captured Anthropic response is parsed into the expected `[]ParsedPersonnelRow`
- [ ] T013 [P] [US1] `backend/internal/services/agent/parser_test.go` — error mapping: timeout → `ErrAPITimeout`, 401 → `ErrInvalidAPIKey`, empty rows → `ErrNoRowsDetected`
- [ ] T014 [P] [US1] `backend/internal/handlers/import_test.go` — `POST /api/admin/users/import-document/preview` requires superadmin (403 otherwise), rejects missing/oversize upload, and never writes to `user`
- [ ] T015 [P] [US1] `backend/internal/handlers/import_test.go` — preview labels each parsed row as `new` or `existing_match` based on `(full_name, nric_last5)` lookup against fixture users

### Implementation for User Story 1

- [ ] T016 [US1] Implement `backend/internal/services/agent/client.go`: HTTPS client reading `ANTHROPIC_API_KEY` once at startup, with `Files.Upload` and `Messages.Create` methods; timeouts, no retries in v1, never logs the key
- [ ] T017 [US1] Implement `backend/internal/services/agent/parser.go`: builds the prompt + structured-output tool schema, dispatches to client, validates rows against feature 001's NRIC Last 5 regex, marks invalid rows with a validity flag, returns `[]ParsedPersonnelRow`
- [ ] T018 [US1] Implement `PreviewImport` in `backend/internal/handlers/import.go`: validate upload (size ≤ 10 MB, accepted MIME types), persist a `personnel_imports` row in status `pending`, call `agent.Parser.ParseDocument`, look up `(full_name, nric_last5)` matches for each row, update the row to `previewed`, return `{ preview_id, rows: [...with match_status] }`
- [ ] T019 [US1] Register route `POST /api/admin/users/import-document/preview` under the admin group in `backend/cmd/api/main.go`
- [ ] T020 [P] [US1] Add API types and `previewImport(file)` function in `frontend/src/lib/api-client.ts`
- [ ] T021 [US1] Create `frontend/src/routes/dashboard/users/import-document.tsx`: drag-drop file picker (reuse the pattern from `bulk-upload.tsx`), submit → preview table with columns Full Name / Rank / Battery / NRIC Last 5 / Match Status / Source-snippet column for low-confidence rows; no commit button yet (US2)
- [ ] T022 [US1] Add a "Import via document" link from `frontend/src/routes/dashboard/users/bulk-upload.tsx` so admins can find the new flow next to the existing bulk upload

**Checkpoint**: Admin can upload a document, see a structured preview labelled with match status. The `user` table is provably untouched. US1 demoable on its own.

---

## Phase 4: User Story 2 — Administrator Chooses Merge Behaviour Before Import (Priority: P1)

**Goal**: After the preview, the admin picks `fill_gaps` or `override`, commits, and the user table is updated according to the chosen mode. **NRIC Last 5 / password is never overwritten on existing users.**

**Independent Test**: Run the same document import twice with each merge mode and confirm: in `fill_gaps`, only previously-empty fields on existing users are populated; in `override`, all non-password fields are replaced; in both modes, existing NRIC Last 5 / password is unchanged.

### Tests for User Story 2

- [ ] T023 [P] [US2] `backend/internal/handlers/import_test.go` — commit endpoint requires superadmin and a valid `preview_id` in status `previewed`
- [ ] T024 [P] [US2] `backend/internal/handlers/import_test.go` — `fill_gaps` mode populates only empty fields on existing users; pre-filled fields are byte-equal after import
- [ ] T025 [P] [US2] `backend/internal/handlers/import_test.go` — `override` mode replaces non-password fields on existing users
- [ ] T026 [P] [US2] `backend/internal/handlers/import_test.go` — NRIC Last 5 / password column is unchanged for existing users in BOTH modes
- [ ] T027 [P] [US2] `backend/internal/handlers/import_test.go` — transactionality: when the parser succeeds but a single-row write fails, the whole import rolls back and the `user` table is unchanged (FR-011, SC-006)
- [ ] T028 [P] [US2] `backend/internal/handlers/import_test.go` — rows that fail the feature 001 NRIC Last 5 format check are rejected even if the admin confirms

### Implementation for User Story 2

- [ ] T029 [US2] Implement `CommitImport` in `backend/internal/handlers/import.go`: takes `{ preview_id, merge_mode }`, opens a single pgx transaction, iterates rows: `create` for new rows, `fill_gaps`/`override` update for existing matches, skips invalid format rows, never writes the password column on updates, persists per-row entries to `personnel_import_changes`, commits and updates `personnel_imports.status = "committed"`; on any error the transaction is rolled back and status set to `failed` with `error_message`
- [ ] T030 [US2] Reuse the existing parallel bcrypt-hash pattern from `BulkCreateUsers` in `backend/internal/handlers/admin.go` for password hashing on the `create` path (extract to a small helper in `backend/internal/handlers/admin.go` if needed)
- [ ] T031 [US2] Register route `POST /api/admin/users/import-document/commit` in `backend/cmd/api/main.go`
- [ ] T032 [P] [US2] Add `commitImport(previewId, mergeMode)` function and types in `frontend/src/lib/api-client.ts`
- [ ] T033 [US2] Extend `frontend/src/routes/dashboard/users/import-document.tsx` with a merge-mode picker (radio: "Fill gaps only" / "Override fields from document") and a Commit button that calls `commitImport`; show a loading state and toast on success/failure

**Checkpoint**: Admin can run a full document → preview → choose merge mode → commit cycle. Re-imports behave correctly under both modes. NRIC Last 5 is invariant across imports.

---

## Phase 5: User Story 3 — Returning User Signs In Against The Imported Database (Priority: P1)

**Goal**: A personnel user whose record was imported can sign in with matching Full Name + NRIC Last 5 and enter the app directly — no signup prompt, no duplicate account.

**Independent Test**: Import a roster containing a known user, then sign in as that user with the matching credentials and confirm they enter the app directly with rank/battery already populated.

### Tests for User Story 3

- [ ] T034 [P] [US3] `backend/internal/handlers/auth_test.go` — known user (full_name + nric_last5 match) → 200 success with session cookie, no new user row created
- [ ] T035 [P] [US3] `backend/internal/handlers/auth_test.go` — admin sign-in with admin credentials still works and is NOT routed through the new branch

### Implementation for User Story 3

- [ ] T036 [US3] Modify `SignIn` in `backend/internal/handlers/auth.go`: replace the silent auto-create with an explicit lookup by `(full_name, nric_last5)`; on match, authenticate and create the session as today; on no match, return a placeholder generic error response (US4 will refine this branch). Keep the admin path untouched.
- [ ] T037 [US3] Confirm the existing `frontend/src/routes/sign-in.tsx` happy path still works against the modified backend; no UI change required for US3

**Checkpoint**: Imported users sign in directly; admin sign-in unchanged. Unknown names currently see a generic error (refined in US4).

---

## Phase 6: User Story 4 — New User Goes Through Explicit Signup With NRIC Confirmation (Priority: P1)

**Goal**: Unknown Full Names no longer silently create accounts. The sign-in page presents an explicit "Create your account" block with a separate confirmation NRIC Last 5 field where paste, drop, context-menu paste, and browser autofill are blocked. An account is created only after both NRIC values match and the format check passes.

**Independent Test**: Submit the sign-in form with a name not in the DB → page reveals "Create your account" block naming the user; type matching values into both NRIC fields → account created and signed in. Attempt to paste into the confirmation field → blocked. Mistype the confirmation → submission rejected without creating an account. Same name with wrong NRIC for an existing user → generic invalid-credentials, no signup block.

### Tests for User Story 4

- [ ] T038 [P] [US4] `backend/internal/handlers/auth_test.go` — unknown full_name returns `signup_required` payload with the submitted name echoed back, no user row created
- [ ] T039 [P] [US4] `backend/internal/handlers/auth_test.go` — full_name matches existing user but nric_last5 differs → generic `invalid_credentials`, never `signup_required` (FR-021 leak prevention)
- [ ] T040 [P] [US4] `backend/internal/handlers/auth_test.go` — `POST /api/auth/sign-up` happy path with matching NRIC values and valid format creates user and returns session
- [ ] T041 [P] [US4] `backend/internal/handlers/auth_test.go` — `POST /api/auth/sign-up` rejects mismatched NRIC values
- [ ] T042 [P] [US4] `backend/internal/handlers/auth_test.go` — `POST /api/auth/sign-up` rejects values that fail feature 001's four-digits-plus-letter format
- [ ] T043 [P] [US4] `frontend/src/components/__tests__/no-paste-input.test.tsx` — paste event is prevented; drop event is prevented; context-menu event is prevented; `autoComplete="off"` is rendered
- [ ] T044 [P] [US4] `frontend/src/routes/__tests__/sign-in.test.tsx` — when backend returns `signup_required`, the explicit signup block renders with the submitted name; mismatched NRIC + confirm-NRIC values display the mismatch message and prevent submit

### Implementation for User Story 4

- [ ] T045 [US4] Refine `SignIn` in `backend/internal/handlers/auth.go`: when no user matches `full_name`, return `{ outcome: "signup_required", full_name }`; when `full_name` matches but `nric_last5` does not, return `{ outcome: "invalid_credentials" }`; admin path still bypasses this branching
- [ ] T046 [US4] Add `SignUp` handler in `backend/internal/handlers/auth.go`: accepts `{ full_name, nric_last5, confirm_nric_last5 }`, rejects mismatch, applies feature 001 format check, hashes via bcrypt, inserts new `user` row, creates session
- [ ] T047 [US4] Register route `POST /api/auth/sign-up` in `backend/cmd/api/main.go`
- [ ] T048 [P] [US4] Add `signUp({ full_name, nric_last5, confirm_nric_last5 })` and `SignInOutcome` types in `frontend/src/lib/api-client.ts`
- [ ] T049 [US4] Implement `frontend/src/components/no-paste-input.tsx`: `<input>` that calls `preventDefault` on `onPaste`, `onDrop`, `onContextMenu`; sets `autoComplete="off"`, `autoCorrect="off"`, `autoCapitalize="off"`, `spellCheck={false}`; forwards refs; otherwise typed-equivalent to the regular `Input` component
- [ ] T050 [US4] Extend `frontend/src/routes/sign-in.tsx`: when the sign-in response is `signup_required`, render a "Create your account for {name}" block on the same page with the original NRIC Last 5 field (regular `Input`) and a confirmation field using `NoPasteInput`; show mismatch error inline; call `signUp` on submit; show the standard generic error message for `invalid_credentials`

**Checkpoint**: Silent auto-create is gone. New users always pass through the explicit signup with NRIC confirmation. Paste / drop / context-menu / autofill are blocked on the confirmation field only. Name-existence leak is closed (FR-021).

---

## Phase 7: User Story 5 — Administrator Reviews What The Agent Changed (Priority: P2)

**Goal**: After every import, the admin can open a summary showing counts and per-row detail (before/after values for updated rows, reasons for skipped/failed rows).

**Independent Test**: Run an import that creates some users, updates some, and skips some. Open the summary and confirm every action is listed with enough detail to identify the user and the affected field(s).

### Tests for User Story 5

- [ ] T051 [P] [US5] `backend/internal/handlers/import_test.go` — committing an import writes one `personnel_import_changes` row per change with correct `action`, `field_name`, `before_value`, `after_value`
- [ ] T052 [P] [US5] `backend/internal/handlers/import_test.go` — skipped rows record a human-readable `reason` (e.g., `"invalid_nric_format"`, `"duplicate_in_document"`)
- [ ] T053 [P] [US5] `backend/internal/handlers/import_test.go` — `GET /api/admin/users/import-document/:id` returns counts + grouped per-row changes for the given import; admin-only

### Implementation for User Story 5

- [ ] T054 [US5] Add `GetImportSummary` handler in `backend/internal/handlers/import.go` returning `{ counts: {created, updated, skipped, failed}, changes: [...] }`
- [ ] T055 [US5] Register route `GET /api/admin/users/import-document/:id` in `backend/cmd/api/main.go`
- [ ] T056 [P] [US5] Add `getImportSummary(id)` to `frontend/src/lib/api-client.ts`
- [ ] T057 [US5] Extend `frontend/src/routes/dashboard/users/import-document.tsx` with a post-commit summary view: counts row + a collapsible per-user list showing the changed fields with before/after values for `updated` rows and the reason for `skipped`/`failed` rows; keep the summary URL bookmarkable via the import id

**Checkpoint**: Admin can audit any past import end-to-end.

---

## Phase 8: Polish & Cross-Cutting

- [ ] T058 [P] Run `npm run build` and `npm run lint` in `frontend/`; fix any warnings introduced by the new files
- [ ] T059 [P] Run `go test ./internal/...` from the repo root (package-mode — see project memory about the smart-test hook false positive) and confirm all new tests pass
- [ ] T060 Manual end-to-end run from `specs/002-agent-onboarding-flow/quickstart.md`: set `ANTHROPIC_API_KEY` in `backend/.env`, run `docker compose up`, import one PDF / DOCX / XLSX sample each, verify preview + commit + summary for both merge modes, plus an explicit signup for an unknown user, plus a returning sign-in for an imported user
- [ ] T061 [P] Add a short "Onboarding via document import" section to `README.md` pointing at the admin route and the env var
- [ ] T062 Deploy gate: push to `main`, monitor `gh run watch <id>` per CLAUDE.md, then SSH and confirm `sudo docker logs apps-attendance-api-1 --tail 50` shows the migration applied and no startup errors

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately.
- **Foundational (Phase 2)**: Depends on Setup completion. **Blocks all user stories.**
- **US1 (Phase 3)**: Depends on Foundational. Delivers preview-only MVP.
- **US2 (Phase 4)**: Depends on US1 (uses the same preview pipeline + DB schema).
- **US3 (Phase 5)**: Depends on Foundational only. **Can run in parallel with US1/US2.**
- **US4 (Phase 6)**: Depends on US3 (extends the modified `SignIn` handler).
- **US5 (Phase 7)**: Depends on US2 (reads the audit rows written during commit).
- **Polish (Phase 8)**: Depends on whichever user stories are in scope for the release.

### Within Each User Story

- Tests are written before or alongside implementation; the package-mode test command (`go test ./internal/handlers/...`) is the authoritative pass/fail signal — single-file `go test file.go` is a known false positive in this repo.
- Backend models / migrations come before handlers; handlers come before frontend wiring.
- Each user story ends at a checkpoint that can be independently demoed.

### Parallel Opportunities

- All Setup tasks marked [P] (T003, T004, T005, T006) can run in parallel after T001/T002.
- All Foundational [P] tasks (T008, T009, T010) can run in parallel after T007.
- US1 tests (T012–T015) are all [P]; US1 implementation tasks T017 / T020 / T021 can run in parallel once T016 is in place.
- US3 (Phase 5) and US1/US2 (Phases 3–4) can be developed in parallel by different people.
- All US4 backend tests (T038–T042) are [P]; the `NoPasteInput` component (T049) and `api-client.ts` types (T048) can be built in parallel with the handler work.

---

## Implementation Strategy

### MVP First (US1 only)

1. Phase 1 → Phase 2 → Phase 3.
2. Stop at the US1 checkpoint. Demo: admin pastes a roster PDF, sees a structured preview with match status, no DB write. This alone is a meaningful early win — admins can already validate the agent's accuracy before any commit code exists.

### Incremental Delivery

1. **MVP** — US1: structured preview from any uploaded document.
2. **+US2** — commit with explicit merge mode; full import cycle works.
3. **+US3** — imported users sign in directly against the populated DB.
4. **+US4** — explicit signup with NRIC confirmation and paste block closes the silent auto-create gap.
5. **+US5** — post-import summary turns the import into an auditable record.

Ship after any of the above; each delivers user-visible value without breaking earlier slices.

### Parallel Team Strategy

After Phase 2:
- Developer A: US1 + US2 (admin import pipeline, end to end).
- Developer B: US3 + US4 (auth changes + explicit signup UI).
- Developer C (when A's commit is in): US5 (summary view + audit reads).

---

## Notes

- `[P]` means the task touches a different file from its phase-mates with no in-flight dependency — safe to take a different developer.
- The `agent` package boundary is deliberate: nothing outside `backend/internal/services/agent/` should import `net/http` for Anthropic calls. If Phase 0 research shows we need a Claude Agent SDK sidecar instead, the swap is contained.
- The smart-test hook's "test failed" output for `go test <file>` is a false positive in this repo; trust `go test ./internal/...` (package mode) as the source of truth.
- Commit after each task or logical group; keep PRs scoped by user story so each can land independently per the incremental strategy.
