# Implementation Plan: Simplify Authentication and Remove Sensitive Data Collection

**Spec**: [spec.md](./spec.md)
**Branch**: `008-simplify-auth`
**Created**: 2026-08-01

## Resolution of Q1

**Decision: option B with commander-side matching (the hybrid).**

Soldiers register fresh with username, password, full name, rank and battery. At approval the commander is shown **ranked roster candidates by edit distance** and chooses to link the signup to an existing roster row, or to create a new one.

- Linking preserves attendance history, because `attendance_record.user_id` keeps pointing at the original `user` row.
- Commander effort stays near option B's: one tap per approval, batched.
- Names, ranks and batteries are typed by soldiers and **will contain typos** (`LCP` vs `CPL`, missing `S/O`, reordered names), so matching must be fuzzy and must never auto-link.

## Architecture decision: pending signups leave the `user` table

Today self-registration inserts into `"user"` with `verified = false` (`admin.go:624`). Two problems:

1. `reports.go:97-119` builds the Missing list from `"user"` filtered only by `is_superadmin` and `battery` — **not** `verified`. Unapproved signups are therefore counted as absent today. This is a live bug.
2. Linking a signup to an existing roster row would mean merging two `user` rows and repointing foreign keys.

Both disappear if pending signups live in their own table. A pending signup is not yet a person on the roster; modelling it as one is the root error.

```
                 ┌──────────────────────┐
  soldier  ──────►  pending_registration │   (username, password_hash,
  signs up       └──────────┬───────────┘    claimed_name, claimed_rank,
                            │                claimed_battery)
                            │ commander decides
              ┌─────────────┴─────────────┐
              ▼                           ▼
      link to existing              create new
      "user" row                    "user" row
      (history preserved)           (genuinely new person)
```

## Data model

### New table

```sql
CREATE TABLE pending_registration (
    id               TEXT PRIMARY KEY,
    username         TEXT NOT NULL,
    password_hash    TEXT NOT NULL,
    claimed_name     TEXT NOT NULL,
    claimed_rank     TEXT NOT NULL,
    claimed_battery  TEXT NOT NULL,
    "createdAt"      TIMESTAMP NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX idx_pending_registration_username ON pending_registration (lower(trim(username)));
```

### `user` table changes

| Change | Note |
|---|---|
| `+ username TEXT` | nullable during rollout; unique index on `lower(trim(username))` where not null |
| `+ password_change_required BOOLEAN NOT NULL DEFAULT false` | forces change after a commander reset (FR-021) |
| `− nric_last5` | dropped in PR5 |
| `− dob` | dropped in PR5 |

Username uniqueness must be enforced **across both tables** — a partial unique index on each, plus an explicit cross-table check at signup and at approval. Two soldiers can otherwise reserve the same username while both are pending.

## Matching algorithm

Pure function, no dependencies — `backend/internal/services/matching/`. `go.mod` has no edit-distance library and this does not warrant one; Levenshtein is ~25 lines.

### Normalisation

```
uppercase → collapse internal whitespace → strip punctuation (. ' -)
→ drop patronymic tokens (S/O, D/O, BIN, BINTE, BT)
```

### Score, 0–100

| Component | Weight | Rationale |
|---|---|---|
| `max(levenshteinRatio, tokenSortedRatio)` | base, 0–100 | token-sorted handles reordered names (`TAN WEI MING` vs `WEI MING TAN`), which `auth.go:88-90` shows already happens in practice |
| battery equal | +8 | only 3 values, so a match is weak-but-real evidence |
| rank equal | +4 | **deliberately weak** — promotions and stale rosters make rank unreliable |

Final `= min(100, base + bonuses)`.

### Presentation rules

- Return the **top 5** candidates, descending.
- Each candidate shows roster name, rank, battery, score, and explicit mismatch chips: *"rank differs — roster CPL, entered LCP"*.
- Roster rows already linked to an account are returned **flagged, not hidden**, labelled *"already claimed by @username"*, so the commander understands why an expected name is unavailable.
- **Never auto-link.** If the top score is ≥95 and the runner-up <85, pre-select it in the UI — but still require an explicit tap. Auto-linking on a fuzzy match would silently graft one soldier's history onto another.

## PR breakdown

Each PR is independently shippable and under ~500 lines.

### PR1 — Public surface (no backend change) ← ship first

**This is the PR that clears Google.** It deliberately does **not** touch the credential check, so production logins keep working.

| Change | File |
|---|---|
| Replace crest favicon with a neutral icon | `frontend/public/favicon.jpg` |
| Retitle to "236 Attendance" | `frontend/index.html:7` |
| Delete the public registration route entirely | `frontend/src/routes/attendance/register.tsx` |
| Relabel `Full Name (as in NRIC)` → `Full Name` | `sign-in.tsx`, `index.tsx` |
| Relabel `NRIC Last 5 (e.g 1234A)` → `Password`; delete tooltip, placeholder, format hint | `sign-in.tsx`, `index.tsx` |
| Remove client-side NRIC format validation | `sign-in.tsx`, `index.tsx` |
| Add Terms and Privacy routes; link them from the footer | new `routes/terms.tsx`, `routes/privacy.tsx` |
| Add "Unofficial unit tool — not a MINDEF or SAF system" to the footer | `sign-in.tsx`, `index.tsx` |
| Add `robots.txt`; add `noindex` meta on auth pages | `frontend/public/robots.txt`, `index.html` |

The password field keeps accepting each user's existing value. The page simply stops *soliciting a national identifier* — which is what the classifier reads.

**On completion: request the Search Console review immediately.** Do not wait for PR2–PR6.

### PR2 — Username and password registration

- Migration: add `username`, `password_change_required`; create `pending_registration`; move existing `user` rows with `verified = false` into it.
- New signup page: username, password, confirm, full name, rank, battery. No NRIC, no DOB.
- Validation: username unique across both tables (case/space-insensitive); password ≥8 chars; reject `^\d{4}[A-Za-z]$` with an explicit "don't use your NRIC" message.
- bcrypt cost `4` → `12` at `admin.go:327`, `admin.go:513`, `auth.go:265`, `import.go:325`.
- Sign-in accepts username **or** (legacy) full name + existing password, so nobody is locked out mid-rollout.
- Fix `reports.go` Missing list — pending signups are no longer in `user`, so verify the count is correct.

### PR3a — Close the sign-in enumeration oracle

`POST /api/auth/sign-in` returns one byte-identical failure for an unknown identifier and a wrong password. Delete the `signup_required` outcome and client state; signup has its own page. Keep the legacy full-name + password path working until PR5.

### PR3b — Matching service and candidates endpoint

- `backend/internal/services/matching/` — Levenshtein, normalisation, scoring. Pure, heavily unit-tested.
- `GET /api/admin/registrations/:id/candidates` — top 5 with scores and mismatch reasons. **Commander-only**; this endpoint returns roster names and must never be reachable unauthenticated (FR-007).
- Approval behaviour remains unchanged in this PR; the endpoint is read-only.

### PR3c — Approval linking and UI

- `POST /api/admin/registrations/:id/approve` with `{ mode: "link", userId }` or `{ mode: "create" }`.
- Link: write `username` + `password_hash` onto the existing `user` row, set `verified = true`, delete the pending row.
- Approval UI shows candidates with scores and mismatch chips; links require an explicit commander selection.

**Carried-over rows must be linked, never created (FR-028).** The PR2 migration copied unapproved `user` rows into `pending_registration` reusing the same `id`, and deliberately did not delete the `user` row (see that migration's comment — deleting cascades attendance records away). These rows are identifiable by a `__migrated_pending__` username prefix: create-mode is refused and link-mode defaults to the row sharing their `id`.

### PR4 — Operating at 350

- Bulk approve (partial success reported per item, successes not rolled back).
- Battery filter on the pending list.
- Commander password reset → one-time temporary password, sets `password_change_required`.
- Forced password-change screen blocking all other routes.

### PR5 — Remove NRIC and DOB (point of no return)

Gated on rollout progress, not on code.

- `DROP COLUMN nric_last5, dob`.
- Delete `findUsersByNRICAndName` (`auth.go:88`), `frontend/src/lib/nric-password.ts`, `no-paste-input.tsx` if unused.
- Strip NRIC/DOB from roster import and bulk upload (`import.go`, `admin.go`, `parse-excel.ts`, `bulk-upload.tsx`, `import-document.tsx`).
- Remove the legacy name-based sign-in path.

### PR6 — Named commander accounts (FR-014, deferrable)

Replaces the shared `admin` login (`sign-in.tsx:100`). Not required to clear Google; needed for SC-007 attributability. Split out so it never blocks the fix.

## Test plan (TDD — tests first)

### Unit — matching (PR3b, the highest-value tests)

Plus:

| Case | Expected |
|---|---|
| Approve a `__migrated_pending__` row | Links to the `user` row sharing its id; no duplicate row; no 500 |
| Approve a `__migrated_pending__` row in create-mode | Refused |
| Sign-in, unknown username | Identical status and body to a wrong password |
| Sign-in, known username, wrong password | Identical status and body to an unknown username |
| `GET .../candidates` unauthenticated | Rejected, and leaks no name |

| Case | Input vs roster | Expected |
|---|---|---|
| Exact | `TAN WEI MING` / `TAN WEI MING` | 100, top |
| Single typo | `TAN WEI MIMG` | ≥90, top |
| Reordered | `WEI MING TAN` | ≥90 via token-sort |
| Patronymic dropped | `MUHAMMAD ALI` vs `MUHAMMAD BIN ALI` | ≥90 |
| Wrong rank, right name | `LCP` vs roster `CPL` | still top; rank-mismatch chip present |
| Wrong battery, right name | `Alpha` vs roster `Bravo` | still top; battery-mismatch chip present |
| Two same-named soldiers | duplicate names, different batteries | both returned; battery bonus orders them |
| Unrelated name | `LIM AH KOW` vs `TAN WEI MING` | low score, not pre-selected |
| Already claimed | linked roster row | returned, flagged, not selectable |
| Ambiguous top two | 96 and 94 | neither pre-selected |

### Unit — credentials (PR2)

- Password <8 rejected; exactly 8 accepted.
- `1234A` rejected with the NRIC-specific message; `1234ABCD` accepted (8 characters beginning with digits but not NRIC-shaped).
- Username `TanWM` collides with existing `tanwm ` (trailing space).
- Username collision across `user` and `pending_registration`.
- bcrypt cost is 12 on newly written hashes.

### Integration (PR3c/PR4)

- Approve-with-link preserves attendance history: seed a user with 3 records, register a fresh signup, link, assert the same 3 records resolve to the linked account and no duplicate `user` row exists.
- Approve-with-create inserts exactly one new row.
- Reject frees the username for reuse.
- Bulk approve of 20 with 2 invalid: 18 succeed, 2 reported, no rollback.
- Temp password forces a change; the old temp password stops working after the change.

### Oracle regression (PR3a)

- Unknown username and known username with a wrong password return byte-identical status and body.

### Regression (PR2)

- Missing list excludes pending signups. Seed 5 roster + 3 pending; assert Missing counts from the 5 only. **This is the pre-existing bug — assert it fails before the fix.**

## Validation plan

To be executed by the verifier independently. Run from repo root.

### V1 — Build and lint

```sh
cd backend && go build ./... && go vet ./...
cd frontend && npm run build && npm run lint
```

Expected: all exit 0. `npm run build` runs `tsc -b`, so type errors fail here.

### V2 — Backend tests

```sh
cd backend && go test ./... -v
```

Expected: all pass. Matching tests must cover every row in the table above.

### V3 — No NRIC or DOB remains (after PR5)

```sh
rg -i 'nric' --glob '!specs/**' --glob '!*.png' backend/ frontend/ | grep -v migrations/
```

Expected: **no output**. Historical migrations legitimately retain the name and are excluded.

```sh
rg -n 'GenerateFromPassword' backend/internal/handlers/
```

Expected: every call site uses cost 12 or `bcrypt.DefaultCost`; **no literal `, 4)`**.

### V4 — Public surface is clean (after PR1)

Local preview, signed out, in a fresh private window:

```sh
cd frontend && npm run build && npm run preview
```

- [ ] `/` — no crest, title "236 Attendance", no NRIC/DOB field, footer shows the unofficial disclaimer
- [ ] `/sign-in` — password field labelled "Password", no `1234A` example anywhere
- [ ] `/attendance/register` — returns not-found
- [ ] `/terms`, `/privacy` — load with real content
- [ ] `/robots.txt` — serves a real robots file, not the SPA shell
- [ ] View source: `noindex` present on auth pages
- [ ] No personnel name is visible anywhere while signed out

Capture screenshots to `specs/008-simplify-auth/artifacts/` using `agent_browser`, matching the convention in `specs/007-session-board-access/artifacts/`.

### V5 — End-to-end on preview

Per `AGENTS.md`, deploy to the server and verify there.

1. Register a soldier with a deliberately misspelt name (`TAN WEI MIMG`) and the wrong rank.
2. As commander, open the pending list. **Expect the correct roster row ranked first, with a rank-mismatch chip.**
3. Link it. Confirm the soldier's prior attendance history is still attached.
4. Sign in as that soldier, scan an active session QR, confirm attendance marks.
5. Confirm the session board shows them Present and the Missing count decreases by exactly 1.
6. Reset their password as commander; confirm the forced change screen appears and blocks other routes.
7. Bulk-approve 10 signups; confirm all 10 can sign in.

Screenshots for steps 2, 3 and 5 attach to the PR.

### V6 — Google review

After PR1 is live:

```sh
curl -s 'https://transparencyreport.google.com/safe-browsing/search?url=attendance.236sa.one'
```

Then request a review in Search Console under Security Issues. Expected end state: *"No unsafe content found"*. Review typically takes 24–72 hours; this cannot be verified synchronously and must be tracked to closure separately.

## Risks

| Risk | Severity | Mitigation |
|---|---|---|
| Wrong roster row linked → one soldier inherits another's history | **High** | Never auto-link; require an explicit tap; show mismatch chips; make link reversible |
| Soldiers reuse NRIC as their password out of habit | **High** — silently recreates the problem in a column you can't inspect | Reject `^\d{4}[A-Za-z]$`; remove every NRIC hint, example and placeholder |
| PR5 lands before soldiers have migrated → mass lockout | **High** | PR5 gates on rollout, not code; legacy sign-in stays until then; manual marking as fallback |
| Repeat offence → Google penalties harden on re-flag | High | Ship PR1 as pure removal; do not reintroduce sensitive fields anywhere, including admin-only pages |
| Password reset toil at 350 users | Medium | PR4 commander reset; expect 20–50 per cycle |
| Two soldiers reserve one username while pending | Medium | Cross-table uniqueness check plus partial unique indexes |
| Nominal roll exposed via an unauthenticated endpoint | **High** | FR-007; candidate matching is commander-only and must sit behind auth — assert this in tests |
| bcrypt 12 slows bulk import (~100ms × 350) | Low | Existing bulk path already hashes in parallel (`admin.go` "Phase 2") |

## Out of scope

Third-party sign-in, device-bound identity, enrolment QR codes, self-service email recovery, moving to an official domain. All considered and recorded in the spec.
