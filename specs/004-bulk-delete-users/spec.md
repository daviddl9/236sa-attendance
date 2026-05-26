# Feature Specification: Selection-Based Bulk Deletion Of Users From The Users Dashboard

**Feature Branch**: `004-bulk-delete-users`

**Created**: 2026-05-26

**Status**: Draft

**Input**: User description: "Improve bulk deletion of users. Today the dashboard's bulk delete button deletes every row matching the current search/battery/rank filters — there is no way to delete a chosen subset, and there is no per-row visibility into what is about to disappear. Replace it with a checkbox-driven selection model so a superadmin picks the users to remove, sees them in the confirm step, and removes only those."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Superadmin Selects A Subset Of Users And Deletes Just Them (Priority: P1) 🎯 MVP

As a superadmin, I want to tick checkboxes against the user rows I want to remove, then click "Delete selected" and confirm, so that I can clean up an exact handful of users without sweeping every row that happens to share my current filters.

**Why this priority**: This is the whole point of the feature. The existing bulk delete is filter-based and silently extends to every matching row, which makes admins reluctant to use it for fear of removing more than they meant to. A selection model collapses that risk because the set of users being deleted is exactly the set the admin ticked.

**Independent Test**: A tester signs in as superadmin, opens `/dashboard/users`, ticks three rows, clicks "Delete selected", confirms in the dialog, and confirms that exactly those three rows disappear from the list (and from the database) while every other row remains untouched.

**Acceptance Scenarios**:

1. **Given** a signed-in superadmin is on `/dashboard/users` viewing the users list, **When** the page renders, **Then** each user row displays a checkbox in a leading column and a header checkbox is present that selects every visible row on the current page.
2. **Given** no rows are ticked, **When** the superadmin looks at the bulk-delete action, **Then** the action is visible but disabled, with a tooltip or inline hint explaining that at least one user must be selected.
3. **Given** one or more rows are ticked, **When** the superadmin clicks the bulk-delete action, **Then** the system opens a confirmation dialog that lists the selected users (Full Name, Rank, Battery) and shows the total count.
4. **Given** the confirmation dialog is open, **When** the superadmin confirms, **Then** the system deletes exactly the selected users, refreshes the users list, clears the selection, and shows a success toast naming the count deleted.
5. **Given** the confirmation dialog is open, **When** the superadmin cancels, **Then** no rows are deleted and the selection is preserved so the admin can adjust and retry.

---

### User Story 2 - Selection Survives Pagination, Search, And Filter Changes (Priority: P1)

As a superadmin, I want my selection to persist by user identity as I move between pages, type in the search box, or change the rank / battery filter, so that I can pick people from different filtered slices and still delete them in one confirmation step.

**Why this priority**: Without persistence, the admin's only way to delete a mixed group is to either filter narrowly enough that everything fits on one page (often impossible) or fall back to the filter-based "delete everything matching" model that this feature replaces. Persistence is what makes selection genuinely better than the old approach.

**Independent Test**: A tester selects two users on page 1 of the list, navigates to page 2 and selects one more, returns to page 1 and confirms the original two are still ticked, then opens the confirmation dialog and confirms it lists all three.

**Acceptance Scenarios**:

1. **Given** the superadmin has selected one or more users on the current page, **When** they navigate to a different page via the pagination control, **Then** the selection state for the original rows is preserved and the new page's rows show their own (independent) checkboxes.
2. **Given** the superadmin has selected one or more users, **When** they change the search, battery, or rank filter so the selected rows are no longer in the visible list, **Then** the selection is preserved by user id and the "X selected" indicator continues to reflect the full count.
3. **Given** the superadmin has rows selected that are no longer visible under the current filter, **When** they open the confirmation dialog, **Then** the dialog still lists those users so the admin can see exactly what will be deleted.
4. **Given** the superadmin reloads the page or navigates away and back, **When** the dashboard re-renders, **Then** the selection MAY be cleared (selection state is session-only and not persisted across navigations), and the dashboard MUST surface an "X selected" badge while the selection is non-empty so the admin never loses sight of it within a session.

---

### User Story 3 - Header Checkbox Selects Only Visible Rows And Makes That Clear (Priority: P1)

As a superadmin, I want the header checkbox in the list to select only the rows that are currently visible on the current page, with clear language so I don't accidentally believe I selected every matching row, so that the "select-all" affordance never silently extends to invisible data.

**Why this priority**: Header-checkbox semantics are the most common source of bulk-delete accidents in CRUD tables: admins assume it spans the entire filtered set when in reality it spans the current page (or vice versa). Pinning the semantics to "current page only" with explicit copy is what keeps the selection model honest.

**Independent Test**: A tester filters the list so that 30 users match (across two pages of 20), ticks the header checkbox on page 1, opens the confirmation dialog, and confirms it shows 20 users (not 30); then navigates to page 2, ticks the header checkbox there, opens the confirmation dialog, and confirms it now shows 30.

**Acceptance Scenarios**:

1. **Given** the users list is on page 1 with N rows visible (where N is the page size or fewer), **When** the superadmin ticks the header checkbox, **Then** only those N rows become selected — never rows on other pages of the same filter.
2. **Given** the header checkbox is ticked and every visible row is already selected, **When** the superadmin un-ticks the header checkbox, **Then** the selection for those visible rows is cleared but the selection for rows on other pages is preserved.
3. **Given** some but not all visible rows are selected, **When** the page renders, **Then** the header checkbox shows an indeterminate ("partial") state and clicking it selects every remaining visible row.
4. **Given** the header checkbox label is shown to the user, **When** the admin reads it, **Then** the copy makes clear it applies to "this page" and not "all matching" — for example, hover/tooltip text reads "Select all on this page".

---

### User Story 4 - The Confirmation Dialog Names Every User About To Be Deleted (Priority: P1)

As a superadmin, I want the confirmation dialog to show me the actual list of users I am about to delete — not just a number — so I can catch any mistakes in my selection before the action is committed.

**Why this priority**: A count alone is a weak safety net. Showing the names (and rank/battery) lets the admin recognise mistakes that a count cannot: "wait, that's the wrong John". This is the safety affordance that makes a destructive permanent action acceptable.

**Independent Test**: A tester selects 5 users and opens the confirmation dialog. They confirm that all 5 names, ranks, and batteries appear in the dialog body, that the count "5 users" matches, and that no other names appear.

**Acceptance Scenarios**:

1. **Given** the superadmin opens the confirmation dialog with N users selected, **When** the dialog renders, **Then** it shows a scrollable list of N rows displaying Full Name, Rank, and Battery for each selected user, plus a heading-level total count.
2. **Given** the selected set is large (e.g. more than 50 users), **When** the dialog renders, **Then** the list is virtualised or paginated within the dialog so it remains responsive, but every selected user remains accessible in the dialog (no silent truncation).
3. **Given** the dialog is open, **When** the admin clicks the destructive "Delete" button, **Then** the button is disabled while the request is in flight and the dialog cannot be closed mid-request; on success it closes, on failure it stays open and surfaces the error.
4. **Given** the dialog warns about cascading impact (sessions and attendance records linked to the selected users), **When** the admin reads it, **Then** the warning is explicit about what else gets removed (attendance records, statuses, custom-session participation) so the deletion is not surprising in its breadth.

---

### User Story 5 - Self-Protection And System-Account Protection (Priority: P2)

As a superadmin, I should not be able to delete my own account or the seeded system administrator row via this flow, so an accidental tick cannot lock the team out of the system or break invariants.

**Why this priority**: Deleting your own row mid-session destroys your session token (because sessions cascade-delete on user delete) and leaves the system without a clean recovery path; deleting the seeded admin row (id `00000000000000000000000000000000`) similarly breaks the auth model. The current filter-based bulk delete already excludes the seeded admin; the selection model must continue to.

**Independent Test**: A tester signs in as a superadmin, attempts to tick their own row and the seeded admin row, and confirms either (a) those checkboxes are disabled with a tooltip explaining why, or (b) the bulk-delete request rejects them at the backend.

**Acceptance Scenarios**:

1. **Given** the superadmin's own user row appears in the list, **When** the page renders, **Then** the checkbox for that row is disabled with a tooltip ("You cannot delete your own account from here").
2. **Given** the seeded admin row (id `00000000000000000000000000000000`) is excluded from the list today, **When** the page renders, **Then** it continues to be excluded; if a bulk-delete request ever references that id directly, the backend MUST reject it.
3. **Given** the bulk-delete request includes the requester's own user id (e.g. via a client bypass), **When** the backend processes the request, **Then** it MUST exclude that id from the deletion and return the per-id result so the client surfaces the skip.

---

### User Story 6 - Partial Success Is Reported Per User, Not As "All Or Nothing" (Priority: P2)

As a superadmin, I want the backend to report per-user success/failure for a bulk-delete request, so that if a few users cannot be deleted (because they have already been removed, or because of a database error on one row), the rest still proceed and I see exactly what happened.

**Why this priority**: The existing filter-based path deletes in one statement and reports a single count. With a selection model the admin has named individuals, so a per-id outcome is far more useful — and it allows recovery: the admin can read the failures, fix the underlying issue, and re-issue a smaller request.

**Independent Test**: A tester selects 5 users where one of them has already been deleted in another browser tab. They confirm the bulk delete; the result toast/dialog reports `4 deleted, 1 skipped (already deleted)` and the four remaining target users are gone.

**Acceptance Scenarios**:

1. **Given** the bulk-delete request includes a user id that no longer exists, **When** the backend processes it, **Then** that id is reported in the response as "not_found" and the rest of the request continues.
2. **Given** the bulk-delete request includes a user id whose deletion fails for an unexpected reason, **When** the backend processes it, **Then** the failure is captured per id in the response and the rest of the request continues.
3. **Given** the backend response contains per-id outcomes, **When** the frontend receives it, **Then** the dashboard shows a summary toast with deleted / skipped / failed counts and (if there were failures) an expandable list of the affected names.

---

### User Story 7 - Existing Filter-Based "Delete All Matching" Path Is Retired (Priority: P3)

As a superadmin, after the selection model ships, I should not have a separate "delete every user matching my filters" button anymore, so that the dashboard offers a single, predictable bulk-delete affordance.

**Why this priority**: Keeping both paths in parallel re-introduces the very confusion this feature is meant to remove, and doubles the surface area we have to keep safe. The new selection model already supports "select all on page" and (with persistence) lets an admin assemble any custom set; the filter-based path is therefore redundant. Retiring it is a cleanup that should not block the MVP but should happen as part of this feature.

**Independent Test**: After deployment, a tester confirms that the only bulk-delete affordance on `/dashboard/users` is the selection-driven one, that the filter-only "Delete all matching" button is gone, and that the previous API entry point either is removed or now requires an explicit id list.

**Acceptance Scenarios**:

1. **Given** the new selection-driven bulk-delete is in place, **When** the dashboard renders, **Then** there is no separate "Delete all matching filters" button visible to the superadmin.
2. **Given** the previous filter-based bulk-delete endpoint exists, **When** the cleanup ships, **Then** either the endpoint is removed entirely or it is changed to require an explicit list of user ids (matching the new contract).
3. **Given** the previous `GET /api/users/bulk/count` helper supports the old filter-based path, **When** the cleanup ships, **Then** that endpoint is removed if it has no remaining call sites, and otherwise documented as unused.

---

### Edge Cases

- The list is empty (no users match the current filters) — the header checkbox MUST be disabled and the bulk-delete action MUST remain disabled.
- All visible rows on a page are the admin's own row plus the seeded admin row (i.e. nothing the admin is allowed to delete) — the header checkbox MUST be disabled rather than selecting nothing silently.
- The admin selects 200+ users — the confirmation dialog MUST handle the list without freezing, and the backend request payload MUST stay within reasonable size (≤ a few KB of ids).
- Two superadmins delete overlapping selections concurrently — each request MUST be processed independently and the per-id "not_found" outcome MUST cover the second request's losers.
- The admin selects users, then their session expires before they confirm — confirming MUST surface an authentication error and the selection MUST be preserved so the admin can re-authenticate and retry without losing their work.
- The admin selects users, then refreshes the page — selection MAY be cleared (selection state is in-memory and not persisted across reloads); the dashboard MUST NOT silently re-fire a previous request.
- The admin's filters narrow the list so the selected rows are no longer visible — the "X selected" indicator MUST continue to reflect the true selection count and the bulk-delete affordance MUST remain enabled.
- The bulk-delete endpoint receives an empty `userIds` array — the backend MUST treat this as a 400 Bad Request rather than deleting nothing silently.
- A selected user has cascading data (attendance records, statuses, attendance-session participation, sign-in sessions) — the deletion MUST proceed and remove the linked rows via the existing cascade rules; the confirmation dialog MUST warn about this so the admin understands the breadth.
- A non-superadmin somehow reaches the new bulk-delete endpoint with a session cookie — the backend MUST return 403 and write no rows.

## Requirements *(mandatory)*

### Functional Requirements

#### Selection model

- **FR-001**: Each user row on `/dashboard/users` MUST render a leading checkbox that toggles that row's membership in the selection set.
- **FR-002**: The users list MUST render a header-row checkbox that selects (or deselects) only the rows currently visible on the current page; it MUST display an indeterminate state when some-but-not-all visible rows are selected.
- **FR-003**: The header checkbox's affordance copy (label, tooltip, or accessible name) MUST make clear that it selects "this page" only, not the full filtered set.
- **FR-004**: The selection set MUST persist across pagination changes and across changes to the search, battery, and rank filters within a single session; it MAY be cleared on full page reload.
- **FR-005**: The dashboard MUST surface a persistent "X selected" indicator whenever the selection set is non-empty, including a "Clear selection" affordance.
- **FR-006**: The bulk-delete action button MUST be enabled exactly when the selection set is non-empty and the signed-in user is a superadmin; otherwise it MUST be disabled.

#### Self- and system-protection

- **FR-007**: The checkbox for the signed-in superadmin's own row MUST be disabled, with a tooltip explaining why.
- **FR-008**: The seeded admin row (id `00000000000000000000000000000000`) MUST continue to be excluded from the list and MUST never be a valid target of bulk delete.
- **FR-009**: The backend MUST defensively exclude both the requester's own id and the seeded admin id from any bulk-delete request, reporting them as "skipped" in the per-id outcome.

#### Confirmation dialog

- **FR-010**: Activating the bulk-delete action with N users selected MUST open a confirmation dialog before any backend call is made.
- **FR-011**: The confirmation dialog MUST display a heading-level total count (e.g. "Delete 7 users") and a per-user list showing Full Name, Rank, and Battery for every selected user.
- **FR-012**: The confirmation dialog MUST surface a clear warning that deletion also removes associated attendance records, statuses, and session participation entries (the existing cascade behaviour), so the admin is not surprised by the breadth.
- **FR-013**: The destructive button MUST be disabled while the request is in flight; the dialog MUST NOT auto-close on failure so the admin can read the error.

#### Backend contract

- **FR-014**: The system MUST expose a bulk-delete endpoint that accepts an explicit array of user ids and is gated by the existing `RequireSuperadmin` middleware.
- **FR-015**: The endpoint MUST reject a request with an empty `userIds` array as 400 Bad Request.
- **FR-016**: The endpoint MUST process the request as a best-effort batch, returning a per-id outcome of "deleted", "skipped" (with reason: `self`, `system_admin`, or `not_found`), or "failed" (with a short error code).
- **FR-017**: The endpoint MUST be idempotent: re-issuing a bulk-delete with the same id list MUST not error on already-deleted ids; they MUST be reported as "skipped: not_found".
- **FR-018**: The endpoint MUST rely on existing cascade rules to remove dependent rows (attendance records, statuses, custom-session participants, sessions) — it MUST NOT introduce a new cascade path of its own.
- **FR-019**: The endpoint MUST log a single audit-style line per request capturing the requester id, the count requested, and the resulting deleted / skipped / failed counts.

#### Outcome reporting

- **FR-020**: On a fully successful bulk-delete, the dashboard MUST show a success toast with the deleted count and refresh the users list; the selection set MUST be cleared.
- **FR-021**: On a partial-success bulk-delete, the dashboard MUST show a mixed-outcome toast with deleted / skipped / failed counts and a way to expand and view the list of skipped/failed names; the users list MUST refresh; the selection set MUST be cleared.
- **FR-022**: On a fully failed bulk-delete (e.g. 500), the dashboard MUST show an error toast naming the backend message and MUST preserve the selection set so the admin can retry.

#### Retirement of the old filter-based path

- **FR-023**: The previous "delete every user matching the current filters" UI button MUST be removed from `/dashboard/users` as part of this feature; the selection-driven action is the only bulk-delete affordance.
- **FR-024**: The previous filter-based bulk-delete endpoint (`DELETE /api/users/bulk` accepting search/battery/rank in the body) MUST either be removed or repurposed to require an explicit id list; in both cases the frontend MUST no longer call the filter-based shape.
- **FR-025**: The previous filter-based count helper endpoint (`GET /api/users/bulk/count`) MUST be removed if it has no remaining call sites after this feature ships.

### Key Entities

- **Selection Set**: A session-scoped set of user ids that the admin has ticked. It persists across pagination and filter changes within the session and is cleared on a successful bulk delete or by an explicit "Clear selection" affordance.
- **Bulk Delete Request**: The payload sent to the backend, carrying an explicit `userIds` array and nothing else. The request is gated by the `RequireSuperadmin` middleware.
- **Bulk Delete Outcome**: The per-id result returned to the client — `deleted` for successful deletions, `skipped` (with reason: `self`, `system_admin`, or `not_found`) for safely-ignored ids, and `failed` (with a short error code) for unexpected per-id errors.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A superadmin can select up to 20 users on a single page and delete them end-to-end (tick, click, confirm, list refresh) in under 15 seconds.
- **SC-002**: 0% of bulk-delete requests result in the deletion of users the admin did not explicitly select; the deleted set MUST equal the confirmed set minus any skipped/failed ids reported in the response.
- **SC-003**: 100% of attempts to bulk-delete the requester's own id or the seeded admin id are reported as `skipped` and result in no rows being removed for those ids.
- **SC-004**: 100% of non-superadmin requests to the bulk-delete endpoint return 403; 0% delete any rows.
- **SC-005**: Selection state survives pagination changes and filter changes in 100% of cases within a session; the "X selected" indicator MUST reflect the true count after each such change.
- **SC-006**: For requests up to 200 user ids, the backend MUST return a response within 5 seconds under a normal production load.
- **SC-007**: After this feature ships, the only bulk-delete entry point visible on `/dashboard/users` is the selection-driven one; the filter-based UI button is no longer present.

## Assumptions

- The existing `Checkbox` UI component (`frontend/src/components/ui/checkbox.tsx`) is suitable for both row-level and header-level selection and does not need redesign for this feature.
- The existing user tier model from feature 003 remains authoritative for who may issue bulk deletes (Superadmin only); this feature does not change the access gate, only the request shape it accepts.
- The existing cascade rules on the `user` table (sessions, attendance records, statuses, custom-session participants) are sufficient to clean up dependent rows on delete; this feature does not introduce or alter those cascades.
- The dashboard's existing search / battery / rank filter behaviour from `/dashboard/users` is unchanged by this feature; only the bulk-delete affordance changes.
- Selection state lives in component state (React) and is intentionally session-only — persisting selection across reloads would risk re-executing a stale destructive intent and is out of scope.
- "Page size" stays at the current 20-rows-per-page value used by `apiClient.listUsers`; this feature does not change pagination.
- The previous filter-based bulk-delete API endpoint (`DELETE /api/users/bulk`) and count endpoint (`GET /api/users/bulk/count`) have no callers outside this dashboard and may therefore be removed or repurposed in the same feature; if any external caller exists, the change must be coordinated separately and is out of scope for this spec.
- Undo/restore of deleted users is out of scope; the existing system has no soft-delete model and adding one is a much larger change.
- Audit logging of bulk-delete requests reuses the existing logging conventions in the backend handler layer; this feature does not introduce a separate audit-log table.
