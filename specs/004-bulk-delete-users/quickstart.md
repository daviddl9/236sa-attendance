# Quickstart: Selection-Based Bulk Delete Of Users (feature 004)

Under-30-seconds local verification path for the selection-driven bulk-delete flow on `/dashboard/users`.

## Prerequisites

- Docker + Docker Compose installed and running.
- Frontend dev tools: Node 20+, `bun` or `npm`.
- A superadmin account in the dev database (the seeded admin row qualifies).
- A reasonably populated `user` table so pagination is exercised — at least ~25 non-admin users. If the dev DB is sparse, run `/dashboard/users/bulk-upload` with the existing `test-callup-small.xlsx` first.

## 1. Start the stack

```bash
cd /Users/davidvallyblessed/Projects/236sa-attendance
docker compose up -d postgres backend
docker compose logs -f backend          # confirm "Migrations applied" — no new migration in this feature
```

In a second terminal:

```bash
cd frontend
bun install
bun run dev
```

## 2. End-to-end manual verification

Each step matches a user story from `spec.md`.

### US1 — Select a subset and delete just them

1. Sign in as a superadmin (NOT one of the rows you plan to delete) and navigate to `/dashboard/users`.
2. Confirm each user row now displays a leading checkbox column, and the table header shows a "select page" checkbox.
3. Tick three different user rows. Confirm:
   - An "**X selected**" indicator appears above the table (e.g. "3 selected · Clear").
   - The **Delete selected** button is now enabled (it was disabled when no rows were selected).
4. Click **Delete selected**. The confirmation dialog opens listing the three users (Full Name, Rank, Battery) and the count.
5. Confirm. Toast appears: "Deleted 3 users." The list refreshes, the three rows are gone, the selection indicator clears.
6. Open the database (`docker compose exec postgres psql -U postgres -d attendance -c 'SELECT id FROM "user" WHERE id = ANY($1)'` or just refresh `/dashboard/users`) and confirm those three rows are gone — every other row is intact (FR-001..FR-006 + SC-002).

### US2 — Selection survives pagination, search, and filter changes

1. With the users list paged at 20 rows per page, select two users on page 1.
2. Click **Next** to go to page 2. Confirm the "X selected" indicator still reads "2 selected".
3. On page 2, select one more user. Confirm the indicator now reads "3 selected".
4. Type into the search box to filter down to a different subset. Confirm:
   - The indicator still reads "3 selected" even if none of the originally selected rows are visible.
   - The **Delete selected** button is still enabled.
5. Open the **Delete selected** dialog. Confirm all three originally-selected users are listed, even though they are not currently visible under the active filter (FR-004 + FR-005 + US3 acceptance scenarios).
6. Cancel the dialog. Clear the search. Confirm the selection state is preserved.
7. Reload the page (Cmd/Ctrl+R). Confirm the selection is now empty (FR-004 — session-only, intentional).

### US3 — Header checkbox selects "this page" only

1. Apply a filter that yields ~30 matching users (across two pages of 20).
2. Tick the header checkbox on page 1. Confirm exactly 20 rows are selected (NOT 30).
3. Open the confirmation dialog. Confirm it shows 20 users (not 30). Cancel.
4. Navigate to page 2. Confirm the header checkbox is now unchecked (the rows on page 2 are not selected).
5. Tick the header checkbox on page 2. Confirm the indicator now reads "30 selected".
6. Tick one row on page 2 to deselect it. Confirm the header checkbox shows an indeterminate state (visually distinct from both checked and unchecked).
7. Hover the header checkbox. Confirm the accessible name / tooltip reads "Select all on this page" (FR-002, FR-003).

### US4 — Confirmation dialog names every user

1. Select 5 users (any combination).
2. Click **Delete selected**. Confirm the dialog:
   - Heading reads "Delete 5 users".
   - Lists all 5 users with Full Name, Rank, Battery.
   - Includes a clear warning that deletion also removes the user's attendance records, statuses, and session participation entries (FR-012).
   - The destructive button reads "Delete 5 users".
3. Select 60 users (across multiple pages). Open the dialog. Confirm:
   - All 60 are accessible in a scrollable list inside the dialog (no silent truncation).
   - The dialog opens and scrolls without freezing the UI (SC-006).

### US5 — Self-protection and system-account protection

1. As a superadmin, locate your own row in the users list. Confirm the row's checkbox is **disabled** with a tooltip explaining why ("You cannot delete your own account from here") (FR-007).
2. Confirm the seeded admin row (id `00000000000000000000000000000000`) does not appear in the list (preserved from current behaviour — FR-008).
3. From the browser console, attempt to bypass the UI:
   ```js
   await fetch('/api/users/bulk-delete', {
     method: 'POST',
     credentials: 'include',
     headers: { 'Content-Type': 'application/json' },
     body: JSON.stringify({ userIds: ['<your-own-id>', '00000000000000000000000000000000', '<some-real-id>'] }),
   }).then(r => r.json());
   ```
   Confirm the response shape has `<your-own-id>` and `00000000000000000000000000000000` in `skipped` with reasons `self` and `system_admin`, and `<some-real-id>` in `deleted` (FR-009 + SC-003).

### US6 — Partial success is reported per user

1. Open two browser tabs to the same dashboard.
2. In tab A, select 5 users.
3. In tab B, navigate to one of the 5 and delete just that single user via the existing per-row delete button.
4. Back in tab A, click **Delete selected** and confirm.
5. Confirm the success toast / dialog reports `4 deleted, 1 skipped (not_found)` and the four remaining target users are gone from the list (FR-016 + FR-017 + US6 acceptance scenarios).

### US7 — Old filter-based path is retired

1. Confirm the dashboard no longer renders a "Delete all matching filters" button — the only bulk-delete affordance is the selection-driven one (FR-023 + SC-007).
2. From the browser console:
   ```js
   await fetch('/api/users/bulk', {
     method: 'DELETE',
     credentials: 'include',
     headers: { 'Content-Type': 'application/json' },
     body: JSON.stringify({}),
   }).then(r => r.status);
   await fetch('/api/users/bulk/count', { credentials: 'include' }).then(r => r.status);
   ```
   Confirm both return 404 (route removed) (FR-024, FR-025).

## 3. Build + tests

```bash
# Backend, package-mode (single-file `go test file.go` is a known false positive in this repo)
go test ./backend/internal/handlers/...

# Frontend
cd frontend && npm run build && npm run lint
```

## 4. Production rollout note

No new env vars, no new migration. Standard deploy is enough:

```bash
git push origin main                       # triggers GitHub Actions deploy
gh run list --limit 1
gh run watch <id>
ssh -i ~/Projects/236sa-cloud/infra/deploy-key.pem ubuntu@redcon.236sa.one \
  'sudo docker logs apps-attendance-api-1 --tail 50'   # confirm clean startup
```

## API contract change note

Two endpoints are **removed** in this deploy:

- `DELETE /api/users/bulk` (filter-shaped body) — replaced by `POST /api/users/bulk-delete` (explicit `userIds`).
- `GET /api/users/bulk/count` — no longer used by the selection-based flow.

The dashboard is the only known caller. If a hidden client surfaces, it will receive 404 and must migrate to the new endpoint shape; the new shape is documented in `plan.md` § "Backend Contract".

## RBAC verification finding

The existing `middleware.RequireSuperadmin` (see `backend/internal/middleware/`) gates the new `POST /api/users/bulk-delete` route exactly as it gates the retired filter-based bulk-delete route — no middleware change required.
