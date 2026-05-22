# Quickstart: Agent-Assisted Onboarding (feature 002)

This is the under-30-seconds local verification path for the document-import flow and the explicit signup flow.

## Prerequisites

- Docker + Docker Compose installed and running.
- Anthropic API key with access to the Messages API and Files API. **Server-side only — never ship the key to the browser.**
- Frontend dev tools: Node 20+, `bun` or `npm`.

## 1. Configure the Anthropic API key

The backend reads the key from a single environment variable, `ANTHROPIC_API_KEY`, loaded via `godotenv` from `backend/.env` at startup (same pattern as `POLAR_WEBHOOK_SECRET`).

```bash
cd /Users/davidvallyblessed/Projects/236sa-attendance
cp -n backend/.env.example backend/.env   # creates .env if missing
echo "ANTHROPIC_API_KEY=sk-ant-..." >> backend/.env
```

Notes:

- When `ANTHROPIC_API_KEY` is unset, the admin document-import route returns a clear "API key not configured" error and the rest of the app is unaffected.
- The dev `docker-compose.yml` propagates the value via `env_file: ./backend/.env`. No `environment:` block edit is needed.

## 2. Start the stack

```bash
docker compose up -d postgres backend
docker compose logs -f backend          # confirm "Migrations applied"
```

In a second terminal, start the frontend:

```bash
cd frontend
bun install
bun run dev
```

## 3. End-to-end manual verification

Each step matches a user story from `spec.md`.

### US1+US2 — Admin imports a roster document

1. Sign in as the seeded admin (see `backend/internal/database/seed.go`).
2. Navigate to `/dashboard/users/import-document`.
3. Upload a sample document (PDF / DOCX / XLSX containing personnel rows). Confirm the preview shows Full Name / Rank / Battery / NRIC Last 5 with each row labelled `New` or `Existing match`.
4. Choose **Fill gaps only** and commit. Confirm the post-commit summary shows non-zero `created` / `updated` counts and zero `failed`.
5. Re-upload the same document, choose **Override fields from document**, and commit. Confirm pre-filled fields on existing users are now replaced — except the NRIC Last 5 column, which must be byte-equal to before.

### US3 — Returning user signs in

1. Sign out.
2. On `/sign-in`, submit Full Name and NRIC Last 5 of one of the imported users.
3. Confirm direct landing into the dashboard with rank / battery already populated.

### US4 — Explicit signup for an unknown user

1. Sign out.
2. On `/sign-in`, submit a Full Name that does NOT exist in the database. Confirm the page reveals an explicit "Create your account for {name}" block on the same page.
3. In the confirmation NRIC Last 5 field, attempt `Cmd/Ctrl+V`, right-click → Paste, and drag-and-drop a string. Confirm all three are blocked. Confirm the browser does not autofill the field.
4. Type non-matching values into the two NRIC fields. Confirm submit is rejected with a "values do not match" inline error and no user record is created.
5. Retry with matching values that satisfy the four-digits-plus-letter format (e.g. `1234A`). Confirm a new user is created and signed in.
6. Sign out. Submit the same Full Name with a wrong NRIC Last 5 (an existing user from step 1's import). Confirm a generic invalid-credentials error and that the explicit signup block does NOT appear (FR-021 leak prevention).

### US5 — Audit summary

1. As admin, visit `/dashboard/users/import-document/{import_id}` (or whatever the summary route ends up being).
2. Confirm counts + per-user before/after for updated rows + reasons for skipped/failed rows.

## 4. Build + tests

```bash
# Backend, package-mode (the smart-test hook's single-file mode is a false positive in this repo)
go test ./backend/internal/...

# Frontend
cd frontend && npm run build && npm run lint
```

## 5. Production rollout note

Production runs on `redcon.236sa.one` with `/opt/apps/docker-compose.yml`. To enable the feature in prod:

```bash
ssh -i ~/Projects/236sa-cloud/infra/deploy-key.pem ubuntu@redcon.236sa.one
sudo nano /opt/apps/.env                 # add ANTHROPIC_API_KEY=sk-ant-...
sudo docker compose -f /opt/apps/docker-compose.yml restart attendance-api
sudo docker logs apps-attendance-api-1 --tail 50   # confirm clean startup
```

## RBAC verification finding (T011)

The existing `middleware.RequireSuperadmin` (see `backend/internal/middleware/rbac.go`) checks `user.IsSuperadmin` from request context and returns 403 otherwise. It is sufficient for the new admin import routes — no middleware change required for this feature.
