# Implementation Plan: Telegram Attendance for Soldiers

**Spec**: [spec.md](./spec.md)
**Branch**: `009-telegram-bot`
**Created**: 2026-08-11

## Decision 1 — no new Go project; a package inside the existing backend

You suggested a new folder for a separate Go project. **I recommend against it**, because it works against the goal of reusing existing code.

| Separate module / binary | Package in existing backend |
|---|---|
| Second `go.mod`, or a second `main` | Same module |
| Duplicates or imports models, database, scoping, matching | Imports them directly |
| Needs a new container in `/opt/apps/docker-compose.yml`, edited on the server, outside this repo | **No deploy change at all** |
| Needs its own HTTPS ingress | Reuses Caddy on `attendance.236sa.one` |
| Second DB pool, second config, second log stream | Reuses all three |

The deploy workflow rsyncs one binary (`backend/main`) and restarts one container. A second binary means editing server-side compose plus the workflow — new failure modes for no benefit.

**Shape:**

```
backend/internal/telegram/          bot logic: update routing, commands, replies
backend/internal/services/attendance/   extracted, shared by HTTP and Telegram
backend/internal/handlers/telegram.go   webhook endpoint on the existing router
```

Telegram delivers updates by webhook to an HTTPS URL. `attendance.236sa.one` already has TLS, so the webhook becomes one route on the existing chi router. No new process, no new port, no new certificate.

## Decision 2 — no Telegram library

This feature needs `sendMessage`, `answerCallbackQuery`, `setWebhook`, and update decoding. That is a thin JSON client over `net/http`, roughly 150 lines.

`go.mod` has six direct dependencies, and 008 hand-rolled Levenshtein rather than add one. A bot framework would be the largest dependency in the project for the smallest gain. **Use the standard library.**

## Decision 3 — the deep-link constraint, and how to fix it

Measured from the code:

```
sessionID      = 16 random bytes → 32 hex chars   (auth.go:457)
qr_code_secret = 32 random bytes → 64 hex chars   (auth.go:463)
token          = sessionID + ":" + secret         = 97 chars, contains ':'

Telegram /start payload: max 64 chars, charset [A-Za-z0-9_-]
```

The existing token cannot be used: too long, and `:` is not permitted.

**Add a per-session `deeplink_code`:** 16 random bytes encoded base64url without padding — **22 characters**, 128 bits of entropy, charset-legal, comfortably inside the limit. It replaces the token for Telegram only; the web `/qr/{token}` path is untouched.

```
QR encodes:  https://t.me/<bot_username>?start=<deeplink_code>
```

The code identifies **and** authorises the session, exactly as the current secret does. It is unguessable, so no separate secret is needed.

## Data model

```sql
-- one confirmed Telegram account per roster row, and vice versa
CREATE TABLE telegram_pairing (
    id            TEXT PRIMARY KEY,
    telegram_id   BIGINT NOT NULL UNIQUE,
    user_id       TEXT NOT NULL UNIQUE REFERENCES "user"(id) ON DELETE CASCADE,
    display_name  TEXT,
    confirmed_by  TEXT NOT NULL REFERENCES "user"(id) ON DELETE RESTRICT,
    "createdAt"   TIMESTAMP NOT NULL DEFAULT NOW()
);

-- awaiting a commander
CREATE TABLE telegram_pairing_request (
    id            TEXT PRIMARY KEY,
    telegram_id   BIGINT NOT NULL UNIQUE,
    display_name  TEXT NOT NULL,
    "createdAt"   TIMESTAMP NOT NULL DEFAULT NOW()
);

ALTER TABLE attendance_session
  ADD COLUMN deeplink_code TEXT;
CREATE UNIQUE INDEX idx_session_deeplink_code
  ON attendance_session (deeplink_code) WHERE deeplink_code IS NOT NULL;
```

Notes:

- `telegram_id` is `BIGINT`. Telegram IDs already exceed 32-bit range.
- Both `UNIQUE` constraints on `telegram_pairing` enforce FR-006 in the database, not just in code.
- `ON DELETE CASCADE` on `user_id` is safe here: a pairing is derived data, unlike the attendance records that made 008's migration dangerous.
- `confirmed_by` is `RESTRICT`, so an audit trail cannot be silently erased.
- Backfill `deeplink_code` for existing open sessions so QR regeneration is not required mid-cycle.

## The central refactor: extract an attendance service

`handlers/attendance.go` currently mixes HTTP concerns with the marking rules: parse token, verify session active, verify secret, resolve user, check duplicate, insert, broadcast SSE. Telegram needs all of that except the HTTP parts.

**Duplicating those rules is the main risk in this feature.** Two copies of scope and duplicate handling will diverge, and the divergence will show up as wrong attendance.

So extract `internal/services/attendance`:

```go
type MarkRequest struct {
    SessionID string
    UserID    string
    Method    string   // qr_scan | telegram_scan | manual
    MarkedBy  *string  // set for manual marks
}

type MarkOutcome int
const (
    Marked MarkOutcome = iota
    AlreadyMarked
    SessionClosed
    OutOfScope
)

func Mark(ctx, tx, MarkRequest) (MarkOutcome, error)
```

Both `HandleQRScan` and the Telegram handler become thin adapters. Rules live in one place.

This refactor lands **first, alone, with no behaviour change**, so any regression is attributable.

## Bot flows

| Trigger | Condition | Behaviour |
|---|---|---|
| `/start <code>` | paired, session active, in scope | Mark, confirm with session name |
| `/start <code>` | paired, already marked | Say already marked, record nothing |
| `/start <code>` | paired, closed or out of scope | Decline, record nothing |
| `/start <code>` | unpaired | Create pairing request, tell them to see a commander |
| `/start <code>` | unknown code | Generic rejection, revealing nothing (FR-017) |
| `/start` or any DM | paired | Show own recent attendance only |
| `/start` or any DM | unpaired, no request yet | Create pairing request |
| `/start` or any DM | unpaired, request pending | Repeat the pending message, create nothing |
| any | not a direct message | Ignore entirely (FR-018) |
| name search | paired commander | Ranked roster rows within their authority |
| name search | paired non-commander | Refuse, return no name (FR-016) |

Authority comes from the paired roster row's existing tier, so `middleware/rbac.go` semantics are reused rather than reimplemented.

## Security

- **Webhook authenticity**: register with Telegram's `secret_token` and require the `X-Telegram-Bot-Api-Secret-Token` header to match, using a constant-time comparison. Reject otherwise (FR-019). The webhook path also contains a random segment, but the header is the real control.
- **Idempotency**: `attendance_record` already has `UNIQUE (session_id, user_id)`. Treat a unique violation as `AlreadyMarked`, which makes retried webhook delivery harmless (FR-012).
- **Always answer 200** for updates that are authentic but unactionable, so Telegram stops retrying. Reserve non-2xx for genuine server faults.
- **No roster leakage**: a soldier's replies contain only their own name. Search is commander-only and battery-scoped.
- **Config**: `TELEGRAM_BOT_TOKEN`, `TELEGRAM_WEBHOOK_SECRET`, `TELEGRAM_BOT_USERNAME` from the environment, never committed. The bot stays disabled when the token is absent, so local and test runs are unaffected.

## Reusing the 008 pairing UI

Feature 008's approval screen already does exactly what pairing needs: take a request carrying a claimed name, rank roster candidates by fuzzy score, show mismatch chips, and require an explicit confirmation.

The only change is the intake channel. A pairing request carries a Telegram display name instead of a chosen username. `services/matching` is reused unchanged, including the score-80 strong/weak split and the always-reachable weak matches — which matter more here, since Telegram display names are frequently nicknames and will often score weak.

## PR breakdown

| PR | Contents | Risk |
|---|---|---|
| **1** | Extract `services/attendance`; rewire `HandleQRScan`; no behaviour change | Medium — regression risk, so it ships alone with tests first |
| **2** | Migration: pairing tables, `deeplink_code`, backfill | Low |
| **3** | Telegram client + webhook endpoint + auth; bot replies to a DM but does not mark | Low |
| **4** | Pairing request intake and the commander confirm/unpair UI | Medium |
| **5** | Marking from `/start <code>`; session QR generates the `t.me` link | **Highest** |
| **6** | Commander marking others from the bot (User Story 4) | Low, deferrable |

Each under ~500 lines. PR5 is the one to review closely.

## Test plan

### Unit — no network, no Telegram

- Deep-link code: 22 chars, charset legal, unique, unguessable.
- Update decoding: private message, group message, callback, malformed JSON, missing fields.
- Webhook auth: correct secret accepted; wrong, absent and empty rejected; comparison is constant-time.
- Group messages ignored.
- Reply text for a soldier never contains another person's name.

### Integration — real PostgreSQL

- Mark succeeds for a paired soldier on an active session.
- Second identical update produces exactly one record (idempotency via the unique constraint).
- Closed session, out-of-scope battery, unknown code, unpaired account: nothing recorded.
- Pairing request created once; a second contact does not duplicate it.
- Confirming a pairing preserves attendance history (seed 3 records, confirm, assert 3 remain).
- One Telegram account cannot pair to two roster rows; one roster row cannot take two accounts.
- Unpairing allows re-pairing.
- Commander search returns only their battery; a soldier's search returns nothing.
- **Parity**: for the same session and soldier, the Telegram path and the web path produce the same outcome for active, closed, duplicate and out-of-scope cases. This is the test that stops the two surfaces diverging.

### Load

- Simulate 431 marks in 90 seconds against local PostgreSQL; assert every record lands, no duplicates, and no error responses. This is the failure mode that rules out the Sheets prototype, so it must be measured rather than assumed.

## Validation plan

For the verifier. Run from repo root.

### V1 — build, vet, test

```sh
docker run --rm --name pg9 -e POSTGRES_PASSWORD=pw -e POSTGRES_DB=att -d -p 55440:5432 postgres:17-alpine
export DATABASE_URL="postgres://postgres:pw@localhost:55440/att?sslmode=disable"
export TEST_DATABASE_URL="$DATABASE_URL"
cd backend && go run ./cmd/migrate up ./migrations   # dir arg required; the CLI panics without it
go build ./... && go vet ./... && go test ./... -v
cd frontend && npm run generate && npm run build && npm run lint
```

Expected: all exit 0. Clean up with `docker rm -f pg9`.

### V2 — no secrets committed

```sh
rg -n 'TELEGRAM_BOT_TOKEN\s*=\s*["0-9]' backend/ frontend/ --glob '!*.md'
git log -p -3 | rg -n '[0-9]{8,10}:[A-Za-z0-9_-]{35}'
```

Expected: no output. The second pattern is the shape of a Telegram bot token.

### V3 — bot disabled without configuration

Start the API with no `TELEGRAM_BOT_TOKEN`. Expected: it starts normally, logs that Telegram is disabled, and every existing endpoint behaves as before. This mirrors how the Anthropic agent already degrades.

### V4 — webhook rejects forgeries

```sh
curl -sS -o /dev/null -w '%{http_code}\n' -X POST localhost:8090/api/telegram/webhook/<path> \
  -H 'Content-Type: application/json' -d '{"message":{"text":"/start"}}'
```

Expected: rejected without the secret header; rejected with a wrong one; accepted with the correct one. No reply is ever sent for a rejected request.

### V5 — end-to-end against the real bot

Requires a test bot from BotFather and a public URL.

1. Create a session on the dashboard; confirm its QR encodes `https://t.me/<bot>?start=<22-char code>`.
2. Scan with an unpaired account. Expect no record and a pairing request visible to a commander.
3. Confirm the pairing; expect the correct roster row ranked first for a display name with a typo.
4. Scan again. Expect Present within 10 seconds, and the session board to move that soldier from Missing to Present.
5. Scan a third time. Expect "already marked" and still exactly one record.
6. Close the session and scan. Expect a decline and no record.
7. Paste the deep link into a group chat. Expect the bot to ignore it.
8. As a soldier, search a name. Expect refusal with no name disclosed.

Screenshots to `specs/009-telegram-bot/artifacts/`, following the 007 and 008 convention.

### V6 — load

Drive 431 concurrent marks against local PostgreSQL. Expected: 431 records, zero duplicates, zero errors. Record the wall time in the PR.

## Risks

| Risk | Severity | Mitigation |
|---|---|---|
| Marking rules diverge between web and Telegram | **High** — silently wrong attendance | Extract the service in PR1; parity tests |
| Wrong pairing marks the wrong soldier present | **High** | Commander confirms every pairing; unpair exists; never auto-pair |
| Shared phone marks the wrong soldier | Medium | One account per roster row; commander can see and undo pairings |
| Telegram display names are nicknames, so matching often scores weak | Medium | Weak matches always reachable (008 PR3d); commander decides |
| Bot token leaked | **High** — anyone can impersonate the bot | Environment only; V2 scan; rotate via BotFather |
| Webhook forgery | High | `secret_token` header, constant-time compare |
| Telegram outage during a parade | Medium | Manual marking retained (FR-025) |
| Soldiers not on Telegram | Medium | Manual marking; web pages are not removed by this feature |
| Retried webhooks double-mark | Medium | Unique constraint plus idempotent handling |
| QR image shared off-parade | Low, pre-existing | Session closure, as today |

## Out of scope

Retiring the soldier web pages, WhatsApp, dropping `nric_last5`/`dob` (008 PR5), replacing PostgreSQL, notifications, and any soldier-visible roster.
