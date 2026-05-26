# Implementation Plan: Superadmin Adds A Single New User From The Users Dashboard

**Branch**: `003-add-new-user` | **Date**: 2026-05-26 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `specs/003-add-new-user/spec.md`

## Summary

Add a single-user create path for superadmins on `/dashboard/users`. The frontend gets an "Add User" button that opens a shadcn `Dialog` containing a small form (Full Name, Rank, Battery, NRIC Last 5, optional DOB, optional extras rows). On submit, the dialog calls a new backend endpoint `POST /api/users` (gated by the existing `RequireSuperadmin` middleware) that validates the payload, normalises the NRIC Last 5, applies the feature-003 CPT+ → superadmin rule, performs an application-level duplicate check on `(LOWER(full_name), nric_last5)`, bcrypt-hashes the password, and inserts the row with `verified = true`. On 409 the dialog surfaces the existing user and links to either the user detail page or the registrations approval page. No schema change, no new dependencies — this feature is a thin overlay on existing primitives (`RegisterUser` for shape, `BulkCreateUsers` for parallel bcrypt patterns, `UserTable` for the list refresh).

## Technical Context

**Language/Version**: Go 1.25.2 backend; TypeScript 5.9.3 with React 19.2 frontend (TanStack Router + Query).

**Primary Dependencies**:
- Backend: go-chi router, pgx/PostgreSQL, `golang.org/x/crypto/bcrypt` — all already in `go.mod`. No new dependency.
- Frontend: shadcn/ui (`Dialog`, `Input`, `Label`, `Select`, `Button`), `sonner` for toasts, TanStack Query for cache invalidation. The shared `nric-password.ts` (`isValidNricLast5`, `normalizeNricLast5`, `NRIC_LAST5_FIELD_MESSAGE`) is the single client-side source of truth for NRIC format. No new dependency.

**Storage**: PostgreSQL. The existing `user` table is the only table touched. **No schema change** — `verified`, `is_superadmin`, `tier_override`, `dob`, `extras` columns are already in place from features 003 and earlier migrations.

**Testing**: Go `go test ./internal/handlers/...` (package mode — single-file `go test file.go` is a known false positive per project memory). Frontend `npm run build` + `npm run lint`. Manual UI verification via the quickstart steps. No new test framework introduced.

**Target Platform**: Web application — Go API container + Vite React SPA, deployed via the existing GitHub Actions pipeline to `redcon.236sa.one` (port 8081).

**Project Type**: Full-stack web application; no new runtime, container, or service.

**Performance Goals**:
- Create round-trip under 500 ms p95 on the dev container (one duplicate-check `SELECT`, one bcrypt hash at default cost, one `INSERT`).
- No measurable impact on the `GET /api/users` list latency — the new endpoint adds one row to the existing dataset, well within the existing pagination model.

**Constraints**:
- Existing `RequireSuperadmin` middleware is authoritative; we do not introduce a parallel access gate.
- NRIC Last 5 validation is the four-digits-plus-letter regex from feature 001 (`^\d{4}[A-Za-z]$`), normalised to uppercase before any DB write or duplicate check. Backend re-validates even if the client lets a bad value through.
- CPT+ → `is_superadmin = true` is enforced server-side via `models.IsSuperadminByRank`, matching `RegisterUser`, `BulkCreateUsers`, and `UpdateUser`.
- Duplicate detection is application-level: there is no DB-level unique constraint on `(full_name, nric_last5)` today, and this feature does not add one (out of scope; a partial unique index would need a separate spec and migration).
- The endpoint MUST NOT accept `tier_override` or `is_superadmin` from the request body even if present — tier overrides remain settable only via `PUT /api/users/{id}`.

**Scale/Scope**:
- One row created per request; one duplicate-check query; one bcrypt hash. No batching, no transactions beyond the single `INSERT`.
- Feature touches ~5 files: 1 backend handler (extend `user.go`), 1 route wire-up (`main.go`), 1 handler test file (extend `user_test.go`), 1 frontend dialog component (new), 1 dashboard route (extend `index.tsx`) + 1 api-client method.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

The repository constitution (`.specify/memory/constitution.md`) holds template placeholders only — no enforceable project-specific gates apply. The plan follows project conventions: backend is the source of truth for validation and access control; secrets stay server-side; database changes go through SQL migrations (none required here); UI changes reuse the existing shadcn/Tailwind vocabulary; tests run in package mode (`go test ./internal/handlers/...`). **PASS.**

**Post-Design Recheck**: PASS. The feature adds no new dependency, no new table, no new service, and no new middleware. It is a thin overlay on existing primitives.

## Project Structure

### Documentation (this feature)

```text
specs/003-add-new-user/
├── spec.md                 # Feature spec (already written)
├── plan.md                 # This file
├── quickstart.md           # Phase 1 — under-30-seconds local verification path
└── tasks.md                # Phase 2 — ordered task list (drafted alongside)
```

No separate `research.md`, `data-model.md`, or `contracts/` directory: there is no schema change, no new external API, and the contract is a single endpoint that is fully described in `plan.md` § "Backend Contract" below.

### Source Code (repository root)

```text
backend/
├── cmd/api/main.go                              # wire POST /api/users under the existing /users RequireSuperadmin block
└── internal/
    └── handlers/
        ├── user.go                              # add CreateUser handler + CreateUserRequest type; reuse helpers from RegisterUser
        └── user_test.go                         # add tests covering FR-001..FR-020 and the duplicate-detection branches

frontend/
└── src/
    ├── lib/
    │   └── api-client.ts                        # add createUser(payload) and the CreateUser conflict response type
    ├── components/
    │   └── users/
    │       └── add-user-dialog.tsx              # NEW: shadcn Dialog wrapping the form, calls apiClient.createUser
    └── routes/
        └── dashboard/
            └── users/
                └── index.tsx                    # add "Add User" button (superadmin-only), mount AddUserDialog, invalidate ['users'] on success
```

**Structure Decision**: Keep the create logic colocated with the rest of the user CRUD in `internal/handlers/user.go`. Do not put it under `internal/handlers/admin.go` even though it is a superadmin-only action — `admin.go` is reserved for routes mounted under `/api/admin/*` (bulk-upload, document import, registration approval). `POST /api/users` is the symmetric counterpart of `PUT /api/users/{id}` and `DELETE /api/users/{id}`, both of which live in `user.go`.

## Backend Contract

### `POST /api/users` — Create a single user (superadmin only)

**Middleware stack** (matches existing user write routes):

```go
r.With(middleware.RequireSuperadmin(db)).Post("/", userHandler.CreateUser)
```

**Request body**:

```json
{
  "fullName": "John Doe",
  "rank": "PTE",
  "battery": "Alpha",
  "nricLast5": "1234A",
  "dob": "010195",
  "extras": { "Section": "Recon" }
}
```

- `fullName`, `rank`, `battery`, `nricLast5` are required.
- `dob` is optional; when present MUST be exactly 6 characters (DDMMYY) — same rule as `RegisterUser`.
- `extras` is optional; when present MUST be a JSON object whose values are strings.
- Any `tierOverride` or `isSuperadmin` keys in the body are **ignored** by the handler — tier overrides remain settable only via `PUT /api/users/{id}`.

**Response — 201 Created**:

Return the full `UserProfile` shape used by `GET /api/users/{id}` so the frontend can drop the new row directly into the cached list. `verified = true`, `is_superadmin` set per the CPT+ rule, `tier_override` null, `extras` defaulting to `{}` if not provided.

**Response — 409 Conflict** (duplicate):

```json
{
  "error": "user_exists",
  "existingUserId": "...",
  "verified": true,
  "fullName": "John Doe"
}
```

The frontend uses `verified` to decide whether to deep-link to `/dashboard/users/{id}` (verified) or `/dashboard/admin/registrations` (unverified).

**Response — 400 Bad Request**: validation failure with the same human-readable messages already returned by `RegisterUser` / `BulkCreateUsers` (invalid rank, invalid battery, invalid NRIC format, invalid DOB length).

**Response — 401 Unauthorized**: missing or expired session.

**Response — 403 Forbidden**: signed in but not superadmin.

## Key Design Decisions

### D1 — `POST /api/users` rather than `POST /api/admin/users`

**Decision**: Mount the new endpoint under `/api/users` next to the existing `PUT /api/users/{id}` and `DELETE /api/users/{id}` write routes, gated by the same `RequireSuperadmin` middleware. Do not create a parallel `/api/admin/users` collection.

**Why**: The `/api/admin/*` namespace is for routes that operate on imports, registration queues, and bulk operations (`/api/admin/users/bulk-create`, `/api/admin/registrations`). A single-user `create` is the natural inverse of `delete` and `update`, both of which live on the `/api/users` collection. Keeping the verbs together makes the handler tests easier to colocate and the route map easier to reason about.

### D2 — Dialog, not a dedicated route

**Decision**: The "Add User" entry point is a shadcn `Dialog` mounted from `/dashboard/users/index.tsx`, not a separate route like `/dashboard/users/new`. The dialog reuses the layout pattern already established by `deleteDialogOpen` and `bulkDeleteDialogOpen` in the same file.

**Why**: Single-user creation is a small form (4 required fields, 2 optional). A full page route would introduce its own loading state, breadcrumb, and back-button flow for no functional gain. A dialog keeps the admin's mental context on the users list — the new user appears in the list as soon as the dialog closes, which matches the "30 seconds end-to-end" success criterion (SC-001).

### D3 — Application-level duplicate detection on `(LOWER(full_name), nric_last5)`

**Decision**: Before insert, the handler issues a single `SELECT id, verified FROM "user" WHERE LOWER(full_name) = LOWER($1) AND nric_last5 = $2 LIMIT 1`. If a row is found, return 409 with the existing id and verified flag. Do not add a database-level unique constraint in this feature.

**Why**: The composite uniqueness is not enforced at the DB layer today (only `full_name` lookup is indexed for sign-in), and adding a partial unique index would require backfilling and resolving any historical near-duplicates — a separate spec. The application-level check is sufficient because every write path (RegisterUser, BulkCreateUsers, CreateUser, the future ImportHandler commit) is gated through the backend; there is no other source of writes to `"user"`.

### D4 — Tier override stays out of the create form

**Decision**: The create form and endpoint do not accept `tier_override` or `is_superadmin`. Setting either is done afterwards via the existing user detail page (`PUT /api/users/{id}`).

**Why**: Keeps the create form small (matches SC-001 — under 30 seconds), avoids re-implementing tier-override validation in a second place, and stays consistent with `RegisterUser` and `BulkCreateUsers`, neither of which exposes these fields. Admin can elevate the new user in a second step on the detail page.

## Open Questions For Phase 0 Research

1. Should we add a partial unique index `CREATE UNIQUE INDEX user_full_name_nric_unique ON "user"(LOWER(full_name), nric_last5)` to make D3 a backstop? Defer to a separate spec because of the historical-data audit it would require, but flag here for visibility.
2. Should the create endpoint accept `is_superadmin = true` for non-CPT ranks (i.e. let a superadmin promote arbitrarily on create)? **No, v1.** Out of scope; admin can promote via the detail page after create. Revisit if there's a real workflow that requires it.
3. Should the dialog support an "Add another" flow (clear and re-open on success)? Nice-to-have; deferred to a follow-up if admins ask for it after first real use.

## Complexity Tracking

No constitution violations. No new dependency, no new schema, no new middleware. The single new endpoint is a thin sibling of existing handlers.

| Possible complexity | Why it's accepted | Simpler alternative considered |
|---------------------|-------------------|--------------------------------|
| New `add-user-dialog.tsx` component | Encapsulates the form, the duplicate-conflict UI, and the API call in one place so `/dashboard/users/index.tsx` stays focused on listing | Inline the form into `index.tsx` — rejected because the dialog already contains non-trivial conflict-handling UI that would crowd the page component |
| Per-conflict response shape with `existingUserId` + `verified` | Lets the frontend deep-link to the right destination (detail vs registrations) without a second round-trip | Return a plain 409 with no details — rejected because it forces the admin to manually search for the existing record, undermining SC-001 |
