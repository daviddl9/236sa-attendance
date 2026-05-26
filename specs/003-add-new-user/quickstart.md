# Quickstart: Add A New User (feature 003)

Under-30-seconds local verification path for the single-user create flow on `/dashboard/users`.

## Prerequisites

- Docker + Docker Compose installed and running.
- Frontend dev tools: Node 20+, `bun` or `npm`.
- A superadmin account in the dev database (the seeded admin row at id `00000000000000000000000000000000` qualifies).

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

### US1 — Superadmin creates a single user

1. Sign in as the seeded admin (or any superadmin user already in the DB).
2. Navigate to `/dashboard/users`. Confirm the **Add User** button is visible in the page header (it lives next to the existing **Bulk Upload** action).
3. Click **Add User**. The dialog opens with Full Name, Rank, Battery, NRIC Last 5, and an "Optional fields" toggle revealing DOB and extras rows.
4. Fill in: Full Name `Test One`, Rank `PTE`, Battery `Alpha`, NRIC Last 5 `1234A`. Leave DOB and extras empty.
5. Click **Create User**. Confirm:
   - Dialog closes.
   - Success toast appears: "Created Test One".
   - `Test One` appears in the users list on the next render with rank `PTE` and battery `Alpha`.
6. Sign out. Sign in as `Test One` with NRIC Last 5 `1234A`. Confirm direct landing into the dashboard — no explicit signup flow, no pending-approval screen (FR-013 + SC-002).

### US2 — NRIC last 5 format is enforced

1. Sign back in as superadmin. Open the **Add User** dialog.
2. Fill Full Name `Test Two`, Rank `PTE`, Battery `Alpha`, NRIC Last 5 `12345` (no letter). Confirm the form's inline validation rejects the value with the same message used by Bulk Upload, and the **Create User** button is disabled (FR-006).
3. Change NRIC Last 5 to `1234a` (lowercase). Confirm the form accepts the value. Submit. Confirm the created user can sign in with `1234A` (uppercase) — i.e. the backend normalised correctly (FR-007 + SC-002).

### US3 — CPT+ auto-superadmin still applies

1. Open the **Add User** dialog.
2. Fill Full Name `Test CPT`, Rank `CPT`, Battery `HQ`, NRIC Last 5 `5678B`. Submit.
3. Sign out. Sign in as `Test CPT / 5678B`.
4. Open the browser dev tools and inspect the response of `/api/auth/session`. Confirm `tier = 4` and `isSuperadmin = true` (FR-011 + SC-004).
5. Confirm the new user can access the registrations approval page (a superadmin-only route).

### US4 — Duplicate detection

1. Sign back in as superadmin. Open the **Add User** dialog.
2. Fill Full Name `Test One`, Rank `PTE`, Battery `Alpha`, NRIC Last 5 `1234A` (same values as US1). Submit.
3. Confirm the dialog stays open with a clear "User already exists" message, and a link offering to navigate to that user's detail page. Click the link; confirm landing at `/dashboard/users/{id}` for `Test One` (FR-014 + FR-015 + SC-005).
4. Repeat with a Full Name that exists only as a pending registration (use `/dashboard/admin/registrations` to confirm one exists, or create one via the public `/users/register` flow). Confirm the conflict message instead points at `/dashboard/admin/registrations`.

### US5 — Access control

1. Sign out. Sign in as a non-superadmin user (Tier 1, 2, or 3).
2. Navigate to `/dashboard/users`. Confirm the **Add User** button is not visible (FR-001 + SC-006).
3. In the browser console, attempt the request directly:
   ```js
   await fetch('/api/users', {
     method: 'POST',
     credentials: 'include',
     headers: { 'Content-Type': 'application/json' },
     body: JSON.stringify({ fullName: 'Bypass', rank: 'PTE', battery: 'HQ', nricLast5: '9999Z' }),
   }).then(r => r.status);
   ```
   Confirm `403` (FR-002 + SC-006). Confirm no `Bypass` row appears in the users list.

### US6 — Optional fields round-trip

1. Sign back in as superadmin. Open the **Add User** dialog.
2. Fill required fields, then expand the optional section and enter DOB `150393` plus one extras row `Section: Recon`.
3. Submit. Open the new user's detail page at `/dashboard/users/{id}`. Confirm DOB and the extras row are present (FR-009 + US6 acceptance scenarios).

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

## RBAC verification finding

The existing `middleware.RequireSuperadmin` (see `backend/internal/middleware/`) checks `user.IsSuperadmin` on the request context and returns 403 otherwise. It is sufficient for the new `POST /api/users` route — no middleware change required.
