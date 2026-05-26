---
description: "Task list for feature 004-bulk-delete-users"
---

# Tasks: Selection-Based Bulk Deletion Of Users

**Input**: Design documents from `/specs/004-bulk-delete-users/`

**Prerequisites**: [spec.md](./spec.md) (required), [plan.md](./plan.md) (required)

**Tests**: Test tasks are INCLUDED. The plan calls for Go handler tests in package mode covering per-id outcomes, self-protection, and the empty-array rejection path, plus a Vitest/RTL test for the selection-state hook and the confirmation dialog. Manual end-to-end verification per `quickstart.md` is also required.

**Organization**: Tasks are grouped by user story so each story can be implemented, tested, and demoed independently.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no in-flight dependency)
- **[Story]**: User story this task belongs to (US1–US7); shared work has no story tag

---

## Phase 1: Setup

**Purpose**: Stub the new files so the rest of the work can compile without circular churn.

- [ ] T001 [P] Add an empty `BulkDeleteUsers` handler stub plus `BulkDeleteRequest` / `BulkDeleteResponse` types to `backend/internal/handlers/user.go` (returns 501 for now)
- [ ] T002 Wire the new route in `backend/cmd/api/main.go`: under the existing `r.Route("/users", ...)` group, add `r.With(middleware.RequireSuperadmin(db)).Post("/bulk-delete", userHandler.BulkDeleteUsers)`
- [ ] T003 [P] Create empty component file `frontend/src/components/users/bulk-delete-confirm-dialog.tsx` exporting a typed `BulkDeleteConfirmDialog` shell that renders nothing
- [ ] T004 [P] Add `bulkDeleteUsers({ userIds })` method stub and `BulkDeleteResponse` type to `frontend/src/lib/api-client.ts`; do NOT remove the old filter-based methods yet (the old dialog still uses them until US1 lands)

**Checkpoint**: `go build ./...` and `npm run build` both pass with the new empty files in place.

---

## Phase 2: Foundational

**Purpose**: Confirm the primitives every user story depends on.

- [ ] T005 Verify the existing `Checkbox` component in `frontend/src/components/ui/checkbox.tsx` supports indeterminate state via a `checked` prop value of `'indeterminate'` (Radix primitive default). If not, add a small wrapper in the same file. Record finding in `quickstart.md`.
- [ ] T006 [P] Verify `middleware.RequireSuperadmin` covers the new `POST /api/users/bulk-delete` route signature; no change expected
- [ ] T007 [P] Verify the existing FK cascade chain on `"user"` (sessions, attendance_records, user_statuses, custom-session participants) by reading `backend/migrations/`; record finding in `plan.md` if a missing cascade is discovered (the warning in FR-012 depends on these cascades being in place)

**Checkpoint**: No foundational code change required; the UI primitive and DB cascades exist already.

---

## Phase 3: User Story 1 — Select A Subset And Delete Just Them (Priority: P1) MVP

**Goal**: Row + header checkboxes, selection state, **Delete selected** button, confirmation dialog, and a working `POST /api/users/bulk-delete` end-to-end.

**Independent Test**: As a superadmin, tick three rows, click **Delete selected**, confirm in the dialog, and confirm those exactly three rows are removed from the list and the database while every other row is intact.

### Tests for User Story 1

- [ ] T008 [P] [US1] `backend/internal/handlers/user_test.go` — happy path: `POST /api/users/bulk-delete` with 3 valid ids as superadmin returns 200; the response's `deleted` array contains all 3; a `SELECT` confirms those rows are gone and other rows remain
- [ ] T009 [P] [US1] `backend/internal/handlers/user_test.go` — non-superadmin (Tier 1/2/3) returns 403; no rows deleted
- [ ] T010 [P] [US1] `backend/internal/handlers/user_test.go` — empty `userIds` array returns 400 with a clear message; no rows deleted
- [ ] T011 [P] [US1] `backend/internal/handlers/user_test.go` — request omitting the `userIds` key entirely returns 400

### Implementation for User Story 1

- [ ] T012 [US1] Implement the `BulkDeleteUsers` handler body in `backend/internal/handlers/user.go`: decode `{ userIds }`, reject empty array as 400, defensively split into `selfId` / `systemAdminId` / `targetIds`, execute `DELETE FROM "user" WHERE id = ANY($1) RETURNING id` in a single statement, build `deleted` from the returned ids and `skipped: not_found` from `targetIds \ deleted`, return the response shape from `plan.md`
- [ ] T013 [US1] Implement `apiClient.bulkDeleteUsers({ userIds })` in `frontend/src/lib/api-client.ts`: `POST /api/users/bulk-delete` with the payload; return the typed `BulkDeleteResponse` on 200
- [ ] T014 [US1] Extend `frontend/src/components/users/user-table.tsx`: add a leading checkbox column, props `selectedIds: Set<string>`, `onToggleRow(id)`, `onTogglePage(visibleIds)`, `disabledIds?: Set<string>` (for self / system-admin protection — used in US5); header checkbox computes `'indeterminate'` when some-but-not-all visible rows are in `selectedIds`
- [ ] T015 [US1] In `frontend/src/routes/dashboard/users/index.tsx`: add `selectedIds: useState<Set<string>>(new Set())`, pass into `UserTable`, render an "X selected · Clear" indicator above the table when non-empty, add the **Delete selected** button (enabled iff `selectedIds.size > 0 && isSuperadmin`)
- [ ] T016 [US1] Implement `frontend/src/components/users/bulk-delete-confirm-dialog.tsx`: receives `selectedUsers: UserProfile[]` (the route resolves these from its current page + a small cache of previously-seen pages), renders a scrollable list (Full Name / Rank / Battery) inside the dialog with max-height ~50vh, a destructive "Delete N users" button that calls `apiClient.bulkDeleteUsers`, a cascade-warning paragraph, and a loading state
- [ ] T017 [US1] Wire the dialog in `index.tsx`: open from **Delete selected**, on success show a `toast.success` with the summary, call `queryClient.invalidateQueries({ queryKey: ['users'] })`, clear `selectedIds`, close the dialog

**Checkpoint**: Selection + confirmation + delete + per-id outcome reporting works for the happy path.

---

## Phase 4: User Story 2 — Selection Survives Pagination, Search, And Filter Changes (Priority: P1)

**Goal**: Selection state persists across pagination, search-box changes, and filter changes within a single session.

**Independent Test**: Select two users on page 1, navigate to page 2 and select one more, return to page 1 — the original two are still ticked. Open the confirm dialog — all three are listed.

### Tests for User Story 2

- [ ] T018 [P] [US2] `frontend/src/routes/dashboard/users/__tests__/users-index.test.tsx` — selecting rows on page 1, changing `page` state, and changing `search` does not mutate `selectedIds` (the `Set` reference may change but its contents are preserved)
- [ ] T019 [P] [US2] `frontend/src/routes/dashboard/users/__tests__/users-index.test.tsx` — when the filtered list no longer contains a selected id, the "X selected" indicator still reflects the true count and the **Delete selected** button stays enabled

### Implementation for User Story 2

- [ ] T020 [US2] In `index.tsx`, maintain a small `Map<string, UserProfile>` (the "selection cache") populated whenever a user is added to `selectedIds`, so the confirmation dialog can render Full Name / Rank / Battery for selected users that are no longer visible under the current filter. Drop entries from the cache only when the user is removed from `selectedIds`.
- [ ] T021 [US2] Ensure pagination / filter changes do NOT call any "reset selection" code path; explicitly assert this with a regression test (T018)

**Checkpoint**: Mixed-page, mixed-filter selections work; the dialog lists every selected user regardless of current filter.

---

## Phase 5: User Story 3 — Header Checkbox Selects Only Visible Rows (Priority: P1)

**Goal**: The header checkbox toggles only the currently visible rows, never spans other pages, and shows an indeterminate state when partially selected.

**Independent Test**: Filter to 30 matching users across two pages. Tick the header on page 1 → 20 selected. Navigate to page 2, tick the header → 30 selected. Untick one row on page 2 → header shows indeterminate.

### Tests for User Story 3

- [ ] T022 [P] [US3] `frontend/src/components/users/__tests__/user-table.test.tsx` — header checkbox ticked when `selectedIds` is a superset of visible row ids; unchecked when disjoint; `'indeterminate'` when partial
- [ ] T023 [P] [US3] `frontend/src/components/users/__tests__/user-table.test.tsx` — clicking the header checkbox toggles only the visible ids; clicking it again deselects only those ids (selection on other pages preserved)
- [ ] T024 [P] [US3] `frontend/src/components/users/__tests__/user-table.test.tsx` — header checkbox accessible name reads "Select all on this page"

### Implementation for User Story 3

- [ ] T025 [US3] In `UserTable` (T014), implement the header checkbox's `checked` and `onCheckedChange` logic per the tests above; ignore `disabledIds` when computing the page-selection set (so self / system-admin rows are never auto-selected by the header)
- [ ] T026 [US3] Add a tooltip or `title` attribute to the header checkbox reading "Select all on this page"

**Checkpoint**: Header semantics are pinned to the visible page; the indeterminate state communicates partial selection.

---

## Phase 6: User Story 4 — Confirmation Dialog Names Every User (Priority: P1)

**Goal**: The dialog renders every selected user (Full Name / Rank / Battery) with no silent truncation, includes a cascade-warning paragraph, and the destructive button is disabled while in flight.

**Independent Test**: Select 60 users. Open the dialog — all 60 are listed in a scrollable container. Confirm the cascade-warning paragraph is present. Click delete — the button disables and the dialog remains open until the request resolves.

### Tests for User Story 4

- [ ] T027 [P] [US4] `frontend/src/components/users/__tests__/bulk-delete-confirm-dialog.test.tsx` — given 60 users, the dialog renders 60 list items; the container has a max-height that triggers vertical scrolling
- [ ] T028 [P] [US4] `frontend/src/components/users/__tests__/bulk-delete-confirm-dialog.test.tsx` — the cascade-warning paragraph is rendered (mentions attendance records, statuses, custom-session participation)
- [ ] T029 [P] [US4] `frontend/src/components/users/__tests__/bulk-delete-confirm-dialog.test.tsx` — the destructive button is disabled while `mutation.isPending`; the dialog does not auto-close on error

### Implementation for User Story 4

- [ ] T030 [US4] Round out `bulk-delete-confirm-dialog.tsx` (started in T016): render the scrollable list (`<div class="max-h-[50vh] overflow-y-auto">`), the cascade-warning paragraph, the typed loading state from a `useMutation` call, and an inline error message when the mutation rejects

**Checkpoint**: The dialog is the safety net the spec demands — names visible, cascade explained, no premature closure.

---

## Phase 7: User Story 5 — Self-Protection And System-Admin Protection (Priority: P2)

**Goal**: A superadmin cannot delete their own row from this flow, and the seeded admin id is always excluded. Both checks are enforced on the client (UI) AND on the server (defensive).

**Independent Test**: As a superadmin, attempt to tick your own row (UI blocks it). Attempt the bulk-delete API with your own id and `00000000000000000000000000000000` — backend returns them in `skipped` with reasons `self` and `system_admin`.

### Tests for User Story 5

- [ ] T031 [P] [US5] `backend/internal/handlers/user_test.go` — request with the requester's own id returns 200; that id appears in `skipped` with reason `self`; the row is NOT deleted
- [ ] T032 [P] [US5] `backend/internal/handlers/user_test.go` — request with `00000000000000000000000000000000` returns 200; that id appears in `skipped` with reason `system_admin`; the row is NOT deleted
- [ ] T033 [P] [US5] `frontend/src/components/users/__tests__/user-table.test.tsx` — when `disabledIds` contains the requester's id, the row's checkbox is rendered with `disabled` and a tooltip; the header-checkbox page-selection logic excludes those rows

### Implementation for User Story 5

- [ ] T034 [US5] In `BulkDeleteUsers` (T012), defensively partition the incoming ids into `targetIds`, `selfId`, and `systemAdminId` BEFORE issuing the `DELETE`; build the `skipped` array accordingly
- [ ] T035 [US5] In `index.tsx`, compute `disabledIds = new Set([currentUser.id, '00000000000000000000000000000000'])` and pass it to `UserTable`; in `UserTable` (T014), render the per-row checkbox `disabled` with a tooltip ("You cannot delete your own account from here") when the row id is in `disabledIds`

**Checkpoint**: Self / system-admin protection is enforced at both layers.

---

## Phase 8: User Story 6 — Partial Success Reporting (Priority: P2)

**Goal**: When some ids cannot be deleted (e.g. already removed in another tab), the rest still proceed and the dashboard surfaces the per-id outcome.

**Independent Test**: In tab A, select 5 users. In tab B, single-delete one of them. In tab A, confirm bulk delete — toast reads "4 deleted, 1 skipped (not_found)" with an expandable list.

### Tests for User Story 6

- [ ] T036 [P] [US6] `backend/internal/handlers/user_test.go` — mixed request (3 valid ids, 1 nonexistent id, 1 already-deleted id between the SELECT and DELETE windows) returns the correct per-id outcome with `deleted` = 3 and `skipped: not_found` = 2
- [ ] T037 [P] [US6] `frontend/src/components/users/__tests__/bulk-delete-confirm-dialog.test.tsx` — when the response has any non-empty `skipped` or `failed`, the success toast/post-dialog summary shows the deleted/skipped/failed counts plus an expandable list of names

### Implementation for User Story 6

- [ ] T038 [US6] In `bulk-delete-confirm-dialog.tsx` (or its surrounding hook), after a successful response branch on whether `skipped.length + failed.length > 0`: if zero, show a single-line success toast; otherwise show a mixed-outcome toast with deleted/skipped/failed counts and an expandable "View details" list
- [ ] T039 [US6] On any backend error (5xx, network), keep the dialog open, preserve `selectedIds`, and surface the error inline so the admin can retry

**Checkpoint**: Partial-success outcomes are surfaced with enough detail for the admin to recover.

---

## Phase 9: User Story 7 — Retire The Old Filter-Based Path (Priority: P3)

**Goal**: Remove the filter-based bulk-delete UI button, the two backend endpoints, and the corresponding api-client methods.

**Independent Test**: After deploy, `/dashboard/users` has only the selection-driven bulk-delete affordance. `DELETE /api/users/bulk` and `GET /api/users/bulk/count` both return 404.

### Tests for User Story 7

- [ ] T040 [P] [US7] `backend/internal/handlers/user_test.go` — remove (or rewrite) the existing tests for `BulkDelete` (filter shape) and `BulkDeleteCount` so they no longer reference the deleted handlers; add a new test confirming `DELETE /api/users/bulk` returns 404
- [ ] T041 [P] [US7] `frontend/src/routes/dashboard/users/__tests__/users-index.test.tsx` — no element with the text "Delete all" / "Delete N users matching" exists; the dashboard's only bulk-delete trigger is **Delete selected**

### Implementation for User Story 7

- [ ] T042 [US7] In `backend/internal/handlers/user.go`, delete the `BulkDelete`, `BulkDeleteCount`, and `BulkDeleteRequest` (filter shape) symbols and their imports
- [ ] T043 [US7] In `backend/cmd/api/main.go`, remove the route registrations `r.With(middleware.RequireSuperadmin(db)).Delete("/bulk", userHandler.BulkDelete)` and `r.With(middleware.RequireBatteryNCO(db)).Get("/bulk/count", userHandler.BulkDeleteCount)`
- [ ] T044 [US7] In `frontend/src/lib/api-client.ts`, remove the filter-based `bulkDeleteUsers` overload and `getBulkDeleteCount`; tighten the type so the new `bulkDeleteUsers({ userIds })` is the only signature
- [ ] T045 [US7] In `frontend/src/routes/dashboard/users/index.tsx`, remove the `bulkDeleteDialogOpen` state, the filter-based `bulkDeleteMutation`, and the old confirmation `<Dialog>` JSX block — leaving only the new selection-based flow
- [ ] T046 [US7] Run `grep -r 'bulkDeleteUsers\|getBulkDeleteCount\|/users/bulk[^/-]' frontend backend` and confirm no stragglers remain

**Checkpoint**: The old filter-based path is gone everywhere; the dashboard exposes exactly one bulk-delete affordance.

---

## Phase 10: Polish & Cross-Cutting

- [ ] T047 [P] Run `npm run build` and `npm run lint` in `frontend/`; fix any warnings introduced by the new files
- [ ] T048 [P] Run `go test ./backend/internal/handlers/...` (package mode — see project memory about the smart-test hook false positive) and confirm all new tests pass
- [ ] T049 Manual end-to-end run from `specs/004-bulk-delete-users/quickstart.md`: walk every US verification step including the API-removal check in US7
- [ ] T050 [P] Add a short note to `README.md` under user management explaining the new selection-based bulk delete and the retirement of the old filter-based path
- [ ] T051 Deploy gate: push to `main`, monitor `gh run watch <id>` per CLAUDE.md, then SSH and confirm `sudo docker logs apps-attendance-api-1 --tail 50` shows clean startup; also verify `curl -X DELETE redcon.236sa.one/api/users/bulk` (without auth) returns 404 (route removed) rather than 401 (route exists but auth missing)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately.
- **Foundational (Phase 2)**: Read-only audit; no blocking work.
- **US1 (Phase 3)**: Depends on Setup. Delivers the MVP — the new flow works end-to-end.
- **US2 (Phase 4)**: Depends on US1 (extends the selection state model).
- **US3 (Phase 5)**: Depends on US1 (extends `UserTable` header logic).
- **US4 (Phase 6)**: Depends on US1 (polishes the dialog implementation started in T016).
- **US5 (Phase 7)**: Depends on US1 (extends the handler + the `UserTable` props).
- **US6 (Phase 8)**: Depends on US1 (extends the response handling).
- **US7 (Phase 9)**: Depends on US1..US6 (removes the old path only after the new path covers every use case).
- **Polish (Phase 10)**: Depends on whichever user stories are in scope for the release.

### Within Each User Story

- Tests are written alongside implementation; package-mode `go test ./internal/handlers/...` is the authoritative pass/fail signal for backend.
- Backend handler changes come before frontend wiring within a story.
- Each user story ends at a checkpoint that can be independently demoed.

### Parallel Opportunities

- All Phase 1 setup tasks marked [P] (T001, T003, T004) can run in parallel.
- US3 (header semantics) and US5 (self-protection) and US6 (partial success reporting) all build on top of US1 but touch largely disjoint code paths — once US1 lands, US3/US5/US6 can proceed in parallel.
- US2 (selection persistence) is mostly a regression test concern after US1, and can run alongside US3/US5/US6.
- US7 (retirement) is intentionally last — it removes code paths that the previous stories may have temporarily kept alive.

---

## Implementation Strategy

### MVP First (US1 only)

1. Phase 1 → Phase 2 → Phase 3. Stop at the US1 checkpoint.
2. Demo: superadmin selects three users, deletes them, the rows disappear. The old filter-based path still exists in parallel at this point; that's intentional — it lets the team validate the new flow before pulling the old one out.

### Incremental Delivery

1. **MVP** — US1: selection + confirm + delete works end-to-end (with self-protection deferred to US5).
2. **+US2** — persistence across pagination and filters (mostly a regression test plus a selection cache).
3. **+US3** — header-checkbox "this page only" semantics with indeterminate state.
4. **+US4** — confirmation dialog renders the full list and the cascade warning.
5. **+US5** — self- and system-admin protection.
6. **+US6** — partial-success reporting (turns the toast into an actionable summary).
7. **+US7** — remove the old filter-based path. Ship this last; do not skip it.

Ship after any of the above; each delivers user-visible value without breaking earlier slices. US7 is the only one that requires the team to be confident no external client depends on the old endpoints.

### Parallel Team Strategy

After Phase 2:
- Developer A: US1 (sets up the new endpoint and dialog).
- Developer B: US3 + US5 (UserTable polish + self/system protection) once US1's `UserTable` API shape is in place.
- Developer C: US6 (partial-success rendering) plus the test suite for the dialog.

US7 (retirement) is a single coordinated PR that should be the last merge of the feature.

---

## Notes

- Selection state lives in `useState` on the route component, not in any global store. A page reload clears it — intentional, to avoid stale destructive intent (FR-004).
- Defensive exclusion of `self` and `systemAdminId` happens in the backend handler; the UI's `disabledIds` prop is the visual reinforcement, not the authoritative gate.
- The smart-test hook's "test failed" output for `go test <file>` is a known false positive in this repo; trust `go test ./internal/handlers/...` (package mode) as the source of truth.
- Commit after each task or logical group; keep PRs scoped per user story so each can land independently per the incremental strategy. US7's removal PR should land last, after US1..US6 are deployed and validated.
