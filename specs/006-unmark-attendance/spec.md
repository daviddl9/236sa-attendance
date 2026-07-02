# Feature Specification: Present-List View With Unmark

**Feature Branch**: `006-unmark-attendance`

**Created**: 2026-07-02

**Status**: Draft

**Input**: User description: "Create a filtered list to quickly see who has already been marked, and let admins / commanders unmark them if they were marked by accident."

## Context

Commanders occasionally mark the wrong person present — a mistapped row in the manual-mark table, or a soldier who scanned the QR for the wrong session. Today there is no way to see or fix this from the UI:

- The session detail page (`frontend/src/routes/dashboard/sessions/$sessionId.tsx`) shows only **counts** of present users and a filterable **Missing Users** table. The list of who is actually marked (`analytics.presentUsers`) is fetched but only surfaced through the "Copy Text" clipboard export and the CSV/Excel/PDF exports — never rendered on screen.
- There is no unmark control anywhere in the frontend.

The backend already fully supports both capabilities:

- `GET /api/reports/sessions/{id}/analytics` returns `presentUsers` (with name, rank, battery) alongside `missingUsers` — Tier 2+ (`backend/internal/handlers/reports.go`).
- `DELETE /api/sessions/{id}/attendance/{userId}` (`RemoveAttendance`, `backend/internal/handlers/attendance.go:373-423`) deletes an attendance record — battery-scoped for non-superadmins, and broadcasts an `attendance_removed` SSE event so live views update. Was Tier 3+; relaxed to Tier 2+ as part of this feature.
- The frontend API client already has `removeAttendance(sessionId, userId)` (`frontend/src/lib/api-client.ts:714-718`) — currently dead code.

**This is almost entirely a frontend feature.** The only backend change is relaxing the route guards on the manual-mark and remove-attendance endpoints from Tier 3+ to Tier 2+ (decision: battery NCOs are trusted to mark/unmark; the handlers' own battery scoping still applies).

## Roles

| Tier | Label | Can view present list | Can mark / unmark |
|---|---|---|---|
| 1 (Enlisted) | user | No (no session detail access) | No |
| 2 (Battery NCO) | battery_nco | Yes (analytics is Tier 2+) | Yes — own battery scope only |
| 3 (Unit Commander) | unit_commander | Yes | Yes — own battery scope only |
| 4 (Superadmin) | superadmin | Yes | Yes — any user |

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Commander Sees Who Is Already Marked, Filtered (Priority: P1) 🎯 MVP

As a commander running a parade, I want a **Present** list on the session detail page — with the same battery filter and name/rank search the Missing Users list already has — so I can instantly confirm whether a specific soldier has been marked without exporting a file or copying text.

**Why this priority**: This is the visibility half of the feature and is useful on its own even before unmark exists. All the data is already in `analytics.presentUsers`; it just needs rendering.

**Independent Test**: Open a session with some marked attendees as a Tier 2+ user. A "Present" section appears next to (or as a tab alongside) the Missing Users section, listing every marked user with rank, name, and battery. Selecting a battery in the filter narrows the list; typing in the search box narrows by name/rank. Counts shown match the analytics totals.

**Acceptance Scenarios**:

1. **Given** a session where 3 users are marked and 5 are missing, **When** a Tier 2+ user opens the session detail page, **Then** the Present list MUST show exactly those 3 users with rank, name, and battery, and the Missing list MUST continue to show the 5 as it does today.
2. **Given** the Present list is showing users from multiple batteries, **When** the user selects a specific battery in the filter, **Then** only marked users from that battery MUST remain visible, and the visible count MUST update accordingly.
3. **Given** the Present list is visible, **When** the user types part of a name or rank in the search box, **Then** the list MUST narrow to matching rows (same matching behavior as the existing Missing Users search).
4. **Given** the session detail page is open with the SSE stream connected, **When** another commander marks a soldier (or a soldier scans the QR), **Then** the newly marked soldier MUST appear in the Present list without a manual page refresh, consistent with how counts update today.

---

### User Story 2 - Commander Unmarks An Accidental Mark (Priority: P1)

As a unit commander or superadmin, I want an **Unmark** action next to each person in the Present list so that when someone was marked by accident I can remove the record in two taps, and the soldier drops back into the Missing list.

**Why this priority**: This is the correction half — the actual pain point. It rides entirely on the existing `DELETE /api/sessions/{id}/attendance/{userId}` endpoint and the already-written `apiClient.removeAttendance` method.

**Independent Test**: As a Tier 2+ user, mark a test user via the Missing list's Mark button, find them in the Present list, tap Unmark, confirm. The row disappears from Present, the user reappears in Missing, present/missing counts update, and `attendance_record` no longer contains the row (verify via re-fetching analytics or the DB).

**Acceptance Scenarios**:

1. **Given** a Tier 2+ user viewing the Present list, **When** they tap Unmark on a row and confirm, **Then** the client MUST call `DELETE /api/sessions/{id}/attendance/{userId}`, the row MUST leave the Present list, the user MUST appear in the Missing list, and both counts MUST update — without a full page reload.
2. **Given** the unmark action, **When** it is invoked, **Then** the UI MUST require an explicit confirmation step (confirm dialog, matching the existing delete-session confirmation pattern) before sending the request — unmarking is destructive of a real record.
3. **Given** a Tier 1 (enlisted) user, **When** they attempt to reach the session detail page, **Then** they get no access to the Present list or unmark controls (unchanged from today).
4. **Given** a Tier 2/3 commander whose battery scope does not include the target user, **When** the backend rejects the delete with 403, **Then** the UI MUST show the server's error message in a toast and leave the list unchanged (no optimistic removal that then has to be rolled back — or if optimistic, it MUST roll back).
5. **Given** two commanders viewing the same session live, **When** one unmarks a soldier, **Then** the other's Present list MUST update via the existing `attendance_removed` SSE event without a refresh.

---

### User Story 3 - Unmark Is Recoverable From A Mistake In The Other Direction (Priority: P2)

As a commander, if I unmark the wrong person, I want re-marking them to be a single tap away — the soldier should now be in the Missing list right where the existing **Mark** button already lives — so a wrong unmark costs seconds, not a re-scan.

**Why this priority**: No new work beyond Stories 1–2 if the lists update correctly; this story pins down that the round trip (mark → unmark → mark) is lossless and leaves exactly one consistent state.

**Independent Test**: Mark a user, unmark them, then mark them again from the Missing list. Final state: user is in Present list exactly once, `attendance_record` has exactly one row for (session, user), marking method reflects the latest manual mark.

**Acceptance Scenarios**:

1. **Given** a user who was marked then unmarked, **When** a commander marks them again via the Missing list, **Then** the mark MUST succeed (no stale "already marked" error) and the Present list MUST show them exactly once.

---

### Edge Cases

- **Closed sessions**: the Present list MUST still render for closed sessions (visibility is a reporting concern). Whether unmark remains available on closed sessions follows whatever the backend allows today — `RemoveAttendance` does not check session status, so the UI does not need to block it; if that is ever deemed wrong it is a backend change out of scope here.
- **Empty states**: a session with zero marked users shows an empty-state message in the Present list ("No one marked yet"), not a blank area.
- **Concurrent unmark**: if two commanders unmark the same user near-simultaneously, the second DELETE will affect zero rows; the backend response is still treated as success by the UI (the desired end state holds). No special handling needed beyond refreshing from analytics/SSE.
- **Large sessions (~400 present)**: the Present list uses the same `UserTable` rendering as the Missing list, which already handles unit-sized lists; no pagination work is in scope.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The session detail page MUST render a Present (already-marked) list from `analytics.presentUsers`, showing rank, name, and battery for each marked user, visible to Tier 2+.
- **FR-002**: The Present list MUST support the same battery filter and name/rank text search as the existing Missing Users list, ideally sharing the same filter controls so one filter drives both lists.
- **FR-003**: For Tier 2+ users, each Present row MUST expose an Unmark action that, after confirmation, calls the existing `apiClient.removeAttendance(sessionId, userId)`.
- **FR-004**: After a successful unmark, the page MUST reconcile state so the user moves from Present to Missing and all counts update, via re-fetch of analytics and/or the existing SSE `attendance_removed` event.
- **FR-005**: Failed unmark requests (403 battery scope, 404, network) MUST surface the error in a toast and leave the rendered lists consistent with server state.
- **FR-006**: The `UserTable` component (`frontend/src/components/users/user-table.tsx`) gains an optional unmark action prop (parallel to the existing `onMark`/`onDelete` props) rather than a new bespoke table component.

### Non-Functional / Constraints

- **NFR-001**: The only backend change is the route-guard relaxation (Tier 3+ → Tier 2+) on the manual-mark and remove-attendance endpoints; handler logic, schema, and SSE events are untouched.
- **NFR-002**: Match existing UI patterns: shadcn components, the existing Select/Input filter styling, the existing confirm-dialog pattern used for Delete Session, and existing toast usage.

### Out of Scope

- Audit trail / history of unmarks (the record is simply deleted, as the backend does today).
- Bulk unmark.
- Any change to marking flows, QR scanning, or exports.

## Success Criteria

1. A commander can answer "has SGT Tan been marked?" for any session in under 5 seconds using only the session detail page (filter or search, no export).
2. An accidental mark can be fully corrected (unmarked) from the UI in ≤ 3 interactions (locate row → Unmark → confirm), and the correction is reflected live for all connected viewers.
3. Tier 2+ users can mark/unmark within their battery scope; commanders outside the target's battery get a clear error, not a silent failure.
4. `apiClient.removeAttendance` is no longer dead code.
