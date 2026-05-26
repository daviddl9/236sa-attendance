# Implementation Plan: Selection-Based Bulk Deletion Of Users

**Branch**: `004-bulk-delete-users` | **Date**: 2026-05-26 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `specs/004-bulk-delete-users/spec.md`

## Summary

Replace the existing filter-based bulk delete (`DELETE /api/users/bulk` with `{search, battery, rank}` in the body) with a selection-based one. Each row in `UserTable` gets a leading checkbox; a header checkbox selects only the current page; selection state lives in `/dashboard/users/index.tsx` as a `Set<string>` of user ids and persists across pagination and filter changes within a session. Clicking **Delete selected** opens a confirmation dialog that lists every selected user (Full Name, Rank, Battery, scrollable for large sets) and warns about cascading deletions of attendance records, statuses, and custom-session participation. On confirm, the frontend calls a new backend endpoint `POST /api/users/bulk-delete` with an explicit `userIds` array; the backend processes the batch as best-effort, defensively excluding the requester's own id and the seeded admin id, and returns a per-id outcome (`deleted`, `skipped: self|system_admin|not_found`, `failed`). The old filter-based endpoints (`DELETE /api/users/bulk` and `GET /api/users/bulk/count`) and the old filter-only confirmation dialog are retired in the same feature.

## Technical Context

**Language/Version**: Go 1.25.2 backend; TypeScript 5.9.3 with React 19.2 frontend (TanStack Router + Query).

**Primary Dependencies**:
- Backend: go-chi router, pgx/PostgreSQL — already in `go.mod`. No new dependency.
- Frontend: shadcn/ui (`Checkbox`, `Dialog`, `Button` — all already present), `sonner` for toasts, TanStack Query for cache invalidation. No new dependency.

**Storage**: PostgreSQL. The existing `user` table and its FK cascade chain (`session`, `attendance_records`, `user_statuses`, custom-session participants from feature 004's `docs/specs/feature-004-custom-participant-sessions.md`) are unchanged. **No schema change.**

**Testing**: Go `go test ./internal/handlers/...` (package mode — single-file `go test file.go` is a known false positive in this repo). Frontend `npm run build` + `npm run lint` plus targeted Vitest/RTL tests for the selection-state hook and the confirmation dialog. Manual end-to-end verification per `quickstart.md`.

**Target Platform**: Web application — Go API container + Vite React SPA, deployed via the existing GitHub Actions pipeline to `redcon.236sa.one`.

**Project Type**: Full-stack web application; no new runtime, container, or service.

**Performance Goals**:
- Bulk delete of up to 200 ids returns within 5 seconds on the production container (SC-006). A single transaction with `DELETE FROM "user" WHERE id = ANY($1)` is sufficient at this scale; cascade deletes on the existing FKs do not require batching.
- The UI must not freeze when the confirmation dialog renders a 200-item list — virtualise via a max-height scroll container; no react-window dependency required at this scale.
- Selection state operations (toggle row, toggle page, clear) are O(1) per user action; the dashboard does not re-fetch the users list on selection change.

**Constraints**:
- Existing `RequireSuperadmin` middleware is authoritative; no parallel gate is introduced.
- Selection state is session-local: stored in component state via `useState`, not persisted to localStorage. A full page reload clears the selection (FR-004 — intentional, to avoid stale destructive intent).
- The backend MUST defensively exclude the requester's own id and the seeded admin id (`00000000000000000000000000000000`) regardless of what the request body contains (FR-009).
- The old filter-based path is removed in the same PR set as the new selection-based path — no parallel codepaths after this feature ships.

**Scale/Scope**:
- Per-request: up to a few hundred user ids in v1; the backend rejects an empty `userIds` array as 400.
- Feature touches ~6 files: 1 backend handler (extend `user.go`), 1 backend route wire-up (`main.go`), 1 handler test file (`user_test.go`), 1 frontend component (`user-table.tsx`), 1 frontend route (`/dashboard/users/index.tsx`), 1 api-client method. The old `BulkDelete` + `BulkDeleteCount` handler methods are removed in the same pass.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

The repository constitution (`.specify/memory/constitution.md`) holds template placeholders only — no enforceable project-specific gates apply. The plan follows project conventions: backend is the source of truth for validation and access control; database changes go through SQL migrations (none required here); UI changes reuse the existing shadcn/Tailwind vocabulary; tests run in package mode. **PASS.**

**Post-Design Recheck**: PASS. The feature adds no new dependency, no new table, no new middleware. It introduces one new endpoint, removes two old endpoints, and changes UI behaviour on a single dashboard route.

## Project Structure

### Documentation (this feature)

```text
specs/004-bulk-delete-users/
├── spec.md                 # Feature spec (already written)
├── plan.md                 # This file
├── quickstart.md           # Phase 1 — under-30-seconds local verification path
└── tasks.md                # Phase 2 — ordered task list (drafted alongside)
```

No separate `research.md`, `data-model.md`, or `contracts/` directory: there is no schema change, no new external API, and the contract is a single endpoint + its retirement counterparts, fully described in `plan.md` § "Backend Contract" below.

### Source Code (repository root)

```text
backend/
├── cmd/api/main.go                              # add POST /api/users/bulk-delete; remove the two old filter-based routes
└── internal/
    └── handlers/
        ├── user.go                              # add BulkDeleteUsers handler + types; remove BulkDelete + BulkDeleteCount methods
        └── user_test.go                         # add tests for the new selection-based behaviour; remove tests for the retired paths

frontend/
└── src/
    ├── lib/
    │   └── api-client.ts                        # add bulkDeleteUsers({userIds}); remove getBulkDeleteCount + filter-based bulkDeleteUsers
    ├── components/
    │   └── users/
    │       ├── user-table.tsx                   # add leading checkbox column with row + header semantics; props for selection set
    │       └── bulk-delete-confirm-dialog.tsx   # NEW: lists selected users, warns about cascade, calls bulkDeleteUsers
    └── routes/
        └── dashboard/
            └── users/
                └── index.tsx                    # replace filter-based bulk delete dialog with selection model + new confirm dialog
```

**Structure Decision**: Keep selection state in `/dashboard/users/index.tsx` (the route component already orchestrates pagination, filters, and the existing delete dialogs); pass `selectedIds` + setter into `UserTable`. Do not introduce a Context or Zustand store — the state is local to a single route and lives only for the session. The confirmation dialog is extracted into its own component (`bulk-delete-confirm-dialog.tsx`) because it has its own loading state and per-id outcome rendering, and that's enough surface to warrant a separate file.

## Backend Contract

### `POST /api/users/bulk-delete` — Best-effort bulk delete (superadmin only)

**Middleware stack**:

```go
r.With(middleware.RequireSuperadmin(db)).Post("/bulk-delete", userHandler.BulkDeleteUsers)
```

**Request body**:

```json
{ "userIds": ["abc", "def", "ghi"] }
```

- `userIds` is required and MUST be a non-empty array of user ids.
- An empty or missing array returns 400.
- Unknown ids are processed and reported as `skipped: not_found` — they are not a 4xx.

**Response — 200 OK**:

```json
{
  "deleted": ["abc"],
  "skipped": [
    { "id": "def", "reason": "not_found" },
    { "id": "ghi", "reason": "self" }
  ],
  "failed": [],
  "summary": { "requested": 3, "deleted": 1, "skipped": 2, "failed": 0 }
}
```

- Reasons in `skipped`: `self` (requester's own id), `system_admin` (the seeded admin id), `not_found`.
- `failed` carries `{ id, code }` where `code` is a short machine-readable string (e.g. `db_error`); a free-text message is logged server-side but not returned, to avoid leaking DB internals.

**Response — 400 Bad Request**: empty `userIds`, malformed JSON.

**Response — 401 Unauthorized**: missing or expired session.

**Response — 403 Forbidden**: signed in but not superadmin.

### Retired endpoints

`DELETE /api/users/bulk` (with `{search, battery, rank}` body) and `GET /api/users/bulk/count` are removed in this feature. The corresponding handlers (`BulkDelete`, `BulkDeleteCount` in `backend/internal/handlers/user.go`) and the corresponding api-client methods (`bulkDeleteUsers` filter shape, `getBulkDeleteCount`) are removed too. There are no external callers of these endpoints outside `/dashboard/users/index.tsx` — confirmed by grepping the repo.

## Key Design Decisions

### D1 — Selection state is session-only `useState<Set<string>>` in the route

**Decision**: Selection lives in `useState` on `/dashboard/users/index.tsx`. It persists across pagination and filter changes within the React component lifecycle (the state survives a re-render that swaps the list contents). It is intentionally cleared on full page reload.

**Why**: Persisting selection across reloads would risk re-executing a stale destructive intent (the admin selects 50 users, walks away, returns hours later, hits Delete without re-checking). For the same reason, we do NOT mirror selection into localStorage or sessionStorage. The "X selected" indicator is the always-visible safety net within a session.

**Trade-off rejected**: A Context or Zustand store would let other dashboard routes read the selection. There is no demand for cross-route selection today, so the local `useState` is sufficient.

### D2 — Header checkbox selects "this page", with explicit copy

**Decision**: The header checkbox toggles selection of the rows currently rendered on the visible page. It shows an indeterminate state when some-but-not-all visible rows are selected. The accessible name reads "Select all on this page".

**Why**: This is the most common source of bulk-delete accidents in CRUD tables. The opposite semantics — "select every row matching my filters across all pages" — is dangerous and historically how this dashboard worked via the now-retired filter-based endpoint. Pinning the header to "this page" with explicit copy keeps the selection model honest. If admins later need a "select all matching filters" affordance, we'll add it as a separate, clearly labelled action with its own confirmation (out of scope here).

### D3 — Confirmation dialog lists every selected user, not just a count

**Decision**: The dialog renders Full Name / Rank / Battery for every selected user inside a max-height scroll container. There is no truncation. For sets above ~50 the list scrolls; no virtualisation library is added in v1.

**Why**: A count alone is a weak safety net. Showing the names is what lets the admin catch "wait, that's the wrong John" before committing the destructive action. At 200 ids the dialog is still responsive without react-window; we'll revisit if real usage exceeds that.

### D4 — Per-id outcome reporting from the backend

**Decision**: The endpoint returns three parallel arrays (`deleted`, `skipped`, `failed`) plus a `summary` block, processed as best-effort: failing one id does not roll back the others. The handler MUST defensively exclude `self` and the seeded admin id regardless of the request body.

**Why**: With a selection model the admin has named individuals, so a per-id outcome is far more useful than a single deletion count. It also enables natural recovery: the admin reads the failures, fixes the underlying issue, and re-issues a smaller request. Defensive exclusion in the handler (not just the client) is the belt-and-braces guarantee for FR-007 / FR-008 / FR-009 / SC-003.

### D5 — Remove the old filter-based endpoints in the same feature

**Decision**: `DELETE /api/users/bulk`, `GET /api/users/bulk/count`, and their api-client + UI counterparts are removed in this feature, not deprecated. The only bulk-delete affordance after this feature ships is the selection-based one.

**Why**: Keeping the old path alive doubles the surface area we have to keep safe and re-introduces the very confusion the new model is meant to remove. A grep confirms no external caller — the only consumers are inside `/dashboard/users/index.tsx`, which we are rewriting. Removing now is the cleaner path.

**Mitigation**: A short note in the PR description and `quickstart.md` explains the contract change. If a hidden caller surfaces, the request shape `{search, battery, rank}` is small enough to add back as a thin adapter that internally fans out to the new selection endpoint.

## Open Questions For Phase 0 Research

1. Does the existing `Checkbox` UI component (`frontend/src/components/ui/checkbox.tsx`) support indeterminate state out of the box, or does it need a small wrapper? Confirm by reading the component; if not, add a wrapper in the same PR.
2. Should the dashboard show a "X of N selected — including N₂ not visible under current filters" line, or just "X selected"? Default: just "X selected" with a "Clear selection" affordance; revisit if real users find the count alone confusing.
3. Should `POST /api/users/bulk-delete` write a single audit log line per request (requester id + counts), or per id deleted? Default: one line per request (FR-019). Per-id audit is the future home for an actual audit-log table when the project introduces one (separate spec).
4. Are there hidden callers of `DELETE /api/users/bulk` / `GET /api/users/bulk/count` outside the dashboard route? Verified `grep -r` in the codebase — none. If a deploy-time check finds one, add an adapter rather than restoring the old handler.

## Complexity Tracking

No constitution violations. No new dependency, no new schema, no new middleware. The feature adds one endpoint, retires two, and changes UI on a single dashboard route.

| Possible complexity | Why it's accepted | Simpler alternative considered |
|---------------------|-------------------|--------------------------------|
| New `bulk-delete-confirm-dialog.tsx` component | Encapsulates the selected-users list, the cascade-warning copy, the per-id outcome rendering, and the loading state in one place so `/dashboard/users/index.tsx` doesn't grow further | Inline the dialog in `index.tsx` — rejected because `index.tsx` already manages pagination, filters, the existing single-delete dialog, and selection state; adding another inline dialog would push the file past readable size |
| Per-id outcome response shape | Lets the dashboard show "X deleted, Y skipped, Z failed" with the specific names, enabling recovery | Single deletion count — rejected because it loses the per-id resolution that the selection model promised admins |
| Retiring the old endpoints in the same feature | Avoids parallel codepaths and the safety drift they cause | Deprecate and remove in a follow-up — rejected because the dashboard rewrite already removes all callers; an unused endpoint is a liability |
