# Task 5 — Telegram admin runtime and E2E report

Date: 2026-08-15

## Status

Implemented and committed the Task 5 Telegram commander/admin acceptance flow. Task 4's `newTelegramRuntime` wiring was already present on the base (`5e47a3c`) and was intentionally not duplicated or changed: production continues to construct one shared `TelegramPairingStore`, `NewBotWithAdmin`, `AdminRouter`, and dispatcher.

## TDD evidence

Created `backend/internal/handlers/telegram_admin_e2e_test.go` before runtime changes and ran the required focused command against the disposable PostgreSQL database. The first meaningful run failed in the new E2E coverage because realistic prefixed synthetic roster IDs made `mark-confirm`/`mark-cancel` callback data exceed Telegram's 64-byte limit, so the confirmation keyboard was empty. The runtime fix preserves the descriptive callback actions when they fit and falls back to short `mc`/`mx` aliases otherwise; both aliases are parsed by the admin router. The stale-callback E2E then exposed/verified the active-session recheck before presenting a mark confirmation.

## E2E coverage

The synthetic PostgreSQL E2E drives the same `telegram.Bot.HandleUpdate` boundary used by production and dispatches actions through a fake sender implementing `Sender`, optional markup delivery, and `PhotoSender`. It asserts:

- Tier 3 battery-specific creation, one committed session, future end time, creator/scope/battery rows, opaque link URL button, valid PNG QR delivery, repeated confirmation idempotency, and selected context reload through a new store/router.
- Tier 2 menu restrictions, active-event visibility, own-battery missing rows, exclusion of a Bravo target, marking an unpaired Alpha target with `manual` and the Tier 2 actor ID, own-mark listing, rejection of another commander's undo, and successful undo of only the actor's mark.
- Paired soldier `/start <code>` through the shared new bot wiring, exactly one `telegram_scan` attendance row with NULL `marked_by`, and the already-marked replay response.
- Tier 3 creator ownership denial, superadmin close, closed-session status, and stale pre-close callback rejection without attendance mutation or name leakage.
- Callback acknowledgements, fake message/photo delivery, URL markup, and safe malformed, unknown-account, wrong-secret, and group updates.

All fixtures use unique `tg-admin-*` prefixes and Telegram IDs; existing cleanup helpers remove rows by prefix/ID. No migration, production credential, or production data was touched.

## Validation

Passed:

```text
TEST_DATABASE_URL='postgres://postgres:pw@localhost:55471/app?sslmode=disable' go test ./internal/handlers ./internal/telegram -run 'Telegram.*Admin|Admin.*E2E' -count=1
  PASS (handlers and telegram)

go test ./internal/telegram -count=1
  PASS

TEST_DATABASE_URL='postgres://postgres:pw@localhost:55471/app?sslmode=disable' go test ./... -count=1
  PASS

TEST_DATABASE_URL='postgres://postgres:pw@localhost:55471/app?sslmode=disable' go test ./... -race -count=1
  PASS

go vet ./...
  PASS

git diff --check
  PASS
```

A first full-suite attempt had one transient reports integration failure while the concurrently-running disposable-database suite was cleaning synthetic rows; rerunning the exact full suite passed, and the race suite also passed.

## Changed files

- `backend/internal/handlers/telegram_admin_e2e_test.go`
- `backend/internal/telegram/admin.go`
- `specs/009-telegram-bot/spec.md`
- `specs/009-telegram-bot/plan.md`
- `.superpowers/sdd/2026-08-12-telegram-admin-ui/task-5-report.md`

## Residual concerns

- The required full validation used the already-running disposable PostgreSQL instance at port 55471; no production database or Telegram API was contacted.
- No real Telegram user-flow smoke test was performed, by design; the fake transport covers outbound action shape and delivery.
- Task 4 runtime wiring remains unchanged and should be reviewed together with this E2E when the branch is accepted.
