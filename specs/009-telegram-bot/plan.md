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

### What the prototype does, and why not to copy it

`~/Projects/236sa-attendance-bot/app.py:172-178`:

```python
header = f"{event_name}_{timestamp_z}"        # "First Parade_2026-08-11T07:30:00Z"
deep_link = f"https://t.me/{BOT}?start={base64url(header)}"
```

The payload is base64url of the event name plus an ISO timestamp. Two defects:

1. **It authorises nothing.** The payload is derivable. Given the naming convention, the timestamp is the only unknown, and parades start at predictable times, so it is brute-forceable in seconds. Anyone can mark themselves present without ever seeing the QR code. The code comments that the link is "embedded ONLY in the QR", which is obscurity, not authorisation.
2. **It fails silently on length.** base64 expands 4:3, so a 64-character cap allows a 48-byte header, leaving **27 characters for the event name**. A longer name produces a broken link with no validation.

The existing web QR is stronger: `attendance.go:65` compares a 64-hex-character random secret server-side. Possessing the QR is what authorises the mark. **Keep that property** (FR-034).

**Add a per-session `deeplink_code`:** 16 random bytes encoded base64url without padding — **22 characters**, 128 bits of entropy, charset-legal, comfortably inside the limit. It replaces the token for Telegram only; the web `/qr/{token}` path is untouched.

```
QR encodes:  https://t.me/<bot_username>?start=<deeplink_code>
             22 chars, fixed length, independent of the session name
```

The code identifies **and** authorises the session, exactly as the current secret does. It is unguessable, so no separate secret is needed, and being fixed-length it cannot overflow the payload limit however the session is named.

## Data model

```sql
-- one confirmed Telegram account per roster row, and vice versa
CREATE TABLE telegram_pairing (
    id            TEXT PRIMARY KEY,
    telegram_id   BIGINT NOT NULL UNIQUE,
    user_id       TEXT NOT NULL UNIQUE REFERENCES "user"(id) ON DELETE CASCADE,
    display_name  TEXT,
    confirmed_by  TEXT REFERENCES "user"(id) ON DELETE SET NULL,
    self_confirmed BOOLEAN NOT NULL DEFAULT false,
    "createdAt"   TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Pairing attempts awaiting resolution: weak/ambiguous attempts await
-- commander review, while strong proposals await soldier confirmation.
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

-- which session a commander is currently marking against (FR-030)
CREATE TABLE telegram_chat_context (
    telegram_id  BIGINT PRIMARY KEY,
    session_id   TEXT NOT NULL REFERENCES attendance_session(id) ON DELETE CASCADE,
    "updatedAt"  TIMESTAMP NOT NULL DEFAULT NOW()
);
```

`telegram_chat_context` is deliberately a table, not memory. The prototype holds this in `SESSIONS = {}` (`app.py:36`), so a restart mid-parade loses every commander's working session and they must rescan. On a host that restarts or scales to more than one instance, that breaks exactly when the parade is busiest.

Context is only honoured while the referenced session is still active, so a stale context cannot attach records to a closed parade.

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
| `/start <code>` | unpaired | Ignore the opaque code as identity input; use only Telegram FirstName+LastName for the normal pairing attempt, propose one strong candidate for explicit soldier confirmation, or prompt/direct to a commander when empty or weak |
| `/start <code>` | paired, unknown code | Generic rejection, revealing nothing (FR-017) |
| `/start <code>` | unpaired, any payload | Use only Telegram FirstName+LastName for the normal pairing attempt; never mark or pass/store the opaque payload |
| `/start` or any DM | paired | Show own recent attendance only |
| `/start` or any DM | unpaired, non-empty display name | Propose one strong candidate for explicit soldier confirmation, or direct to a commander without naming anyone |
| `/start` or any DM | unpaired, empty display name | Prompt normally for a name; do not search or disclose a person |
| any | not a direct message | Ignore entirely (FR-018) |
| name search | paired commander | Ranked roster rows within their authority |
| name search | paired non-commander | Refuse, return no name (FR-016) |

Authority comes from the paired roster row's existing tier, so `middleware/rbac.go` semantics are reused rather than reimplemented.

## Security

- **Webhook authenticity**: register with Telegram's `secret_token` and require the `X-Telegram-Bot-Api-Secret-Token` header to match, using a constant-time comparison. Reject otherwise (FR-019). The webhook path also contains a random segment, but the header is the real control.
- **Idempotency**: `attendance_record` already has `UNIQUE (session_id, user_id)`. Treat a unique violation as `AlreadyMarked`, which makes retried webhook delivery harmless (FR-012).
- **Always answer 200** for updates that are authentic but unactionable, so Telegram stops retrying. Reserve non-2xx for genuine server faults.
- **No roster leakage**: non-commanders cannot perform arbitrary roster searches, receive candidate lists, or receive unrelated names. The sole exception is one strong candidate proposed in direct response to that account's own explicit pairing attempt, including an unpaired deep-link `/start`; the opaque payload is never passed to matching. Search is commander-only and battery-scoped.
- **Config**: `TELEGRAM_BOT_TOKEN`, `TELEGRAM_WEBHOOK_SECRET`, `TELEGRAM_BOT_USERNAME`, and `TELEGRAM_WEBHOOK_PATH` are all required for an enabled bot, never committed. With none configured the bot stays disabled; partial configuration fails closed. Session Telegram links are empty unless all four are present.

## Pairing model: soldier confirms, commander reviews by exception

The bot uses Telegram's FirstName+LastName as the normal pairing hint. It ranks the roster with the 008 matcher and, **only if there is a strong match**, proposes exactly one person: *"Are you CPL TAN WEI MING, Bravo?"* On explicit soldier confirmation the pairing is live and they can mark attendance immediately; a commander reviews routine confirmations and handles weak matches or conflicts by exception. No opaque deep-link payload is used as identity input.

Three properties keep that safe, and all three must hold:

1. **The system proposes; the soldier confirms.** Never a list to pick from, so the roster is never enumerated, and never a silent system decision.
2. **Strong matches only.** Below `StrongCandidateScore` the bot proposes nobody and sends them to a commander. That bounds how wrong an automatic pairing can be.
3. **One account per roster row.** A second account claiming a held row is refused and raised as a conflict, so impersonation is loud rather than silent.

Why this is acceptable here when it was not on the web: a Telegram claim is bound to a real, traceable, bannable account, whereas the web form was anonymous. The residual risk is a soldier deliberately claiming a mate's identity — which is what already happens today with a shared NRIC, except now it leaves a trail and collides visibly.

Rate-limit proposals per Telegram account: because the bot answers "are you X?", an unbounded attacker could use it to test which names are on the roster.

## Reusing the 008 pairing UI

Feature 008's approval screen already does exactly what pairing needs: take a request carrying a claimed name, rank roster candidates by fuzzy score, show mismatch chips, and require an explicit confirmation.

The only change is the intake channel. A pairing attempt carries a Telegram FirstName+LastName display hint instead of a chosen username. `services/matching` is reused unchanged, including the score-80 strong/weak split and the always-reachable weak matches — which matter more here, since Telegram display names are frequently nicknames and will often score weak. An unpaired deep-link payload never reaches the matcher and is not stored; valid and unknown payloads must not produce a session-existence signal.

## PR breakdown

| PR | Contents | Risk |
|---|---|---|
| **1** | Extract `services/attendance`; rewire `HandleQRScan`; no behaviour change | Medium — regression risk, so it ships alone with tests first |
| **2** | Migration: pairing tables, `deeplink_code`, backfill | Low |
| **3** | Telegram client + webhook endpoint + auth; bot replies to a DM but does not mark | Low |
| **4** | Pairing: soldier self-confirms a proposed match; commander review-by-exception and unpair | Medium |
| **5** | Marking from `/start <code>`; session QR generates the `t.me` link | **Highest** |
| **6** | Commander manual marking from the bot: missing list, tap to mark, search, undo, session context | Medium |

Each under ~500 lines. PR5 is the one to review closely.

PR6 is **not deferrable**. It is the manual-marking path for soldiers who are not on Telegram, so without it a commander must run the parade from a laptop and a phone simultaneously. It reuses `GetMissingUsers` and the existing unmark endpoint from 006 rather than adding new reporting logic.

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
- An unpaired deep link with synthetic FirstName+LastName follows the normal pairing attempt/proposal path, proposes at most one strong candidate for explicit confirmation, creates no attendance row, and neither stores nor matches the opaque payload; a valid and unknown payload produce no session-existence distinction.
- Pairing request created once; a second contact does not duplicate it.
- Confirming a pairing preserves attendance history (seed 3 records, confirm, assert 3 remain).
- One Telegram account cannot pair to two roster rows; one roster row cannot take two accounts.
- Unpairing allows re-pairing.
- Commander search returns only their battery; a soldier's search returns nothing.
- A commander can mark a soldier who has no pairing at all (FR-029).
- Consecutive marks reuse the stored session context without rescanning (FR-030).
- Session context is ignored once the session closes.
- Tapping a soldier who was marked a moment ago by their own scan reports already-marked, not an error.
- A commander can undo a mark they made (FR-032).
- A deep-link code cannot be derived from the session name or start time: two sessions created with identical names one second apart get unrelated codes (FR-034).
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
2. Scan with an unpaired account whose synthetic Telegram FirstName+LastName is a strong roster hint. Expect no record, one proposed candidate, and an explicit confirmation keyboard; the opaque payload must not appear in the proposal or pairing data.
3. Confirm the proposal; expect the correct roster row and commander review-by-exception visibility.
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
| Wrong pairing marks the wrong soldier present | **High** | Soldier explicitly confirms one strong proposal; weak/ambiguous matches go to commander review; unpair exists; never pair silently |
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
