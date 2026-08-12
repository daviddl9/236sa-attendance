# Telegram Admin UI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a secure Telegram-native admin UI for creating attendance sessions, sending QR/link capabilities, viewing scoped attendance, manually marking soldiers, undoing the actor's own manual marks, and closing owned sessions.

**Architecture:** Keep the existing Telegram soldier flow and webhook boundary. Add a pure Telegram admin state machine backed by a PostgreSQL adapter, persistent `telegram_chat_context`, and shared session/report/attendance services so Telegram does not call cookie-authenticated handlers or duplicate attendance decisions. Extend the Telegram client and dispatcher for URL buttons and QR photo delivery, with a text-link fallback after a committed session.

**Tech Stack:** Go 1.25.2, Chi, pgx/pgxpool, PostgreSQL, Goose migrations, existing Telegram Bot API client, `github.com/skip2/go-qrcode`, Go unit/integration tests, disposable PostgreSQL 17, existing Vite/React frontend unchanged.

## Global Constraints

- Telegram admin interactions are accepted only from private chats.
- Pairing, effective tier, battery, session scope, target scope, session status, and attendance ownership are rechecked on every message/callback operation.
- Tier 2 commanders cannot create or close events; they can view and manually manage only their battery-authorized attendance.
- Tier 3 unit commanders can create unit-wide or battery-specific events within their unit and close only events they created.
- Superadmins can create and close any permitted event and view/mark within their permitted scope.
- Telegram-created event names are 1–80 trimmed characters.
- Telegram-created events require an end time selected from exactly 30 minutes, 1 hour, 2 hours, or 4 hours from server time.
- Telegram-created events support `unit_wide` and `battery_specific`; custom Excel rosters are excluded.
- QR delivery sends both an in-memory PNG and a clickable Telegram URL button; a failed photo delivery sends the link as text.
- Status pages contain at most 10 roster rows; name search returns at most 10 authorized candidates.
- Manual records use `marking_method=manual` and `marked_by=<paired actor user ID>`.
- Undo can delete only a manual record for the selected session/target whose `marked_by` equals the current actor.
- Callback data is an action hint only. It never grants authority or identifies the trusted actor.
- Raw bot tokens, webhook secrets, QR secrets, and raw database deep-link codes must not appear in source, logs, errors, API responses, Telegram messages, or tests.
- Production data must not be copied into the repository or test artifacts. Integration tests use synthetic fixtures and disposable PostgreSQL.
- Preserve existing dashboard behavior and soldier `/start <code>` behavior.
- Use TDD: each production behavior starts with a failing test, then minimal implementation, then refactoring while green.
- Stage files explicitly; do not use `git add .`.

---

### Task 1: Extract shared session, report, and manual-attendance services

**Files:**
- Create: `backend/internal/services/sessions/service.go`
- Create: `backend/internal/services/sessions/service_test.go`
- Create: `backend/internal/services/reports/service.go`
- Create: `backend/internal/services/reports/service_test.go`
- Modify: `backend/internal/services/attendance/attendance.go`
- Modify: `backend/internal/services/attendance/attendance_test.go`
- Modify: `backend/internal/handlers/session.go`
- Modify: `backend/internal/handlers/reports.go`
- Modify: `backend/internal/handlers/session_deeplink_test.go`
- Modify: relevant handler/report tests when constructor signatures change

**Interfaces:**

Create `sessions.Service` with these public types and methods:

```go
type CreateRequest struct {
    Name      string
    Scope     string
    Batteries []string
    EndTime   *time.Time
    CreatedBy string
}

type Service struct { /* database and Telegram-link configuration */ }

func NewService(db *database.DB, botUsername string) *Service
func (s *Service) Create(ctx context.Context, req CreateRequest) (models.AttendanceSession, error)
func (s *Service) ListActive(ctx context.Context, actor *models.User) ([]models.AttendanceSession, error)
func (s *Service) Close(ctx context.Context, sessionID string, actor *models.User) error
func (s *Service) Get(ctx context.Context, sessionID string) (models.AttendanceSession, error)
```

`Create` must validate the existing two standard scopes and allowed batteries, generate a random QR secret and 128-bit Telegram deep-link code, and insert the session atomically. The Telegram link is composed from the configured bot username and validated deep-link code. `ListActive` must restrict Tier 2 to sessions usable for the actor's battery; Tier 3+ uses the existing unit scope. `Close` allows the creator or a superadmin only and updates an active row once.

Create `reports.Service` with:

```go
type UserRow struct {
    ID      string
    Name    string
    Rank    string
    Battery string
}

type Summary struct {
    SessionName string
    Total       int
    Present     int
    Missing     int
    Percentage  float64
}

type MissingPage struct {
    Rows       []UserRow
    Page       int
    PageCount  int
    Total      int
    HasPrevious bool
    HasNext     bool
}

func NewService(db *database.DB) *Service
func (s *Service) Summary(ctx context.Context, sessionID string, actor *models.User) (Summary, error)
func (s *Service) Missing(ctx context.Context, sessionID string, actor *models.User, query string, page, pageSize int) (MissingPage, error)
```

`Summary` and `Missing` must use the same session-scope and Tier 2 battery rules as the existing reports handlers. `Missing` must filter the name query in SQL, exclude already-marked users, return deterministic name/ID ordering, clamp `pageSize` to 10 for Telegram callers, and never return an unauthorized roster row. Preserve active personnel status data only in the existing web response; the Telegram service returns no personnel statuses.

Add to `services/attendance`:

```go
type UndoRequest struct {
    SessionID string
    UserID    string
    MarkedBy  string
}

type UndoOutcome int

const (
    Undone UndoOutcome = iota
    UndoNotFound
    UndoNotOwned
    UndoNotManual
)

func UndoManual(ctx context.Context, tx pgx.Tx, req UndoRequest) (UndoOutcome, error)
```

`UndoManual` must inspect the existing row and delete only when its method is `manual` and `marked_by` equals the actor. It must not change the existing web endpoint's broader behavior; the Telegram adapter uses this stricter operation.

- [ ] **Step 1: Write failing service tests** for session validation/random codes, active-session scope, close ownership, report scope/pagination/search, and manual undo ownership.
- [ ] **Step 2: Run the focused tests**.

Run:

```sh
cd backend
go test ./internal/services/sessions ./internal/services/reports ./internal/services/attendance -count=1
```

Expected: FAIL because the new packages/methods do not exist or the new assertions are unmet.

- [ ] **Step 3: Implement the services and move the repeated session/report query logic out of HTTP handlers.** Keep JSON response shapes and existing dashboard permissions unchanged. The session service must reuse `services/deeplink.GenerateCode`; do not derive codes from names or timestamps.
- [ ] **Step 4: Run focused tests and existing handler tests**.

Expected: PASS for the new tests and all touched handler tests.

- [ ] **Step 5: Commit the service foundation**.

```sh
git add backend/internal/services/sessions backend/internal/services/reports backend/internal/services/attendance backend/internal/handlers/session.go backend/internal/handlers/reports.go backend/internal/handlers/session_deeplink_test.go
 git diff --staged --check
git commit -m "refactor: share attendance session services"
```

---

### Task 2: Persist Telegram admin context and expose a role-aware admin store

**Files:**
- Create: `backend/migrations/20260815000000_telegram_admin_context.sql`
- Create: `backend/internal/telegram/admin_types.go`
- Create: `backend/internal/handlers/telegram_admin.go`
- Create: `backend/internal/handlers/telegram_admin_test.go`
- Create: `backend/internal/handlers/telegram_admin_integration_test.go`
- Modify: `backend/internal/handlers/telegram.go`
- Modify: `backend/internal/handlers/telegram_pairing_test.go` when shared store construction changes

**Interfaces:**

Define the Telegram-facing types in `backend/internal/telegram/admin_types.go`:

```go
type AdminActor struct {
    Pairing    Pairing
    FullName   string
    Tier       models.AccessTier
    Battery    string
    IsSuperadmin bool
}

type AdminDraft struct {
    Name     string
    Scope    string
    Battery  string
    EndTime  time.Time
}

type AdminContext struct {
    TelegramID int64
    SessionID  string
    State      string
    Draft      AdminDraft
    Version    int64
    ExpiresAt  *time.Time
}

type ActiveEvent struct {
    ID           string
    Name         string
    Scope        string
    Batteries    []string
    EndTime      *time.Time
    TelegramLink string
}

type AttendancePage struct {
    SessionName string
    Total       int
    Present     int
    Missing     int
    Percentage  float64
    Rows        []AdminUser
    Page        int
    PageCount   int
    HasPrevious bool
    HasNext     bool
}

type AdminUser struct {
    ID      string
    Name    string
    Rank    string
    Battery string
}
```

Define `AdminStore`:

```go
type AdminStore interface {
    Actor(ctx context.Context, pairing Pairing) (AdminActor, bool, error)
    LoadContext(ctx context.Context, telegramID int64) (AdminContext, error)
    SaveContext(ctx context.Context, next AdminContext) error
    ClearContext(ctx context.Context, telegramID int64) error
    ClearContextForSession(ctx context.Context, telegramID int64, sessionID string, expectedVersion int64) error
    ActiveEvents(ctx context.Context, actor AdminActor) ([]ActiveEvent, error)
    CreateEvent(ctx context.Context, actor AdminActor, draft AdminDraft) (ActiveEvent, error)
    CloseEvent(ctx context.Context, actor AdminActor, sessionID string) error
    Status(ctx context.Context, actor AdminActor, sessionID, query string, page int) (AttendancePage, error)
    MarkManual(ctx context.Context, actor AdminActor, sessionID, targetUserID string) error
    OwnManualMarks(ctx context.Context, actor AdminActor, sessionID string, page int) ([]AdminUser, error)
    UndoManual(ctx context.Context, actor AdminActor, sessionID, targetUserID string) error
}
```

The production implementation must:

- join `telegram_pairing` to the current verified roster row when resolving an actor;
- reload tier, battery, and superadmin state from PostgreSQL for each operation;
- call the shared session/report/attendance services rather than duplicating their decisions;
- enforce Tier 2 battery scope and creator-only close ownership;
- use an optimistic `version` update for context writes, including an insert-vs-insert conflict for a missing row;
- clear contexts by resetting a versioned tombstone rather than deleting the row, so stale callbacks cannot recreate old state;
- clear a selected context after close only when its stored session still matches the session that was closed;
- treat expired drafts as idle and closed/expired selected sessions as unavailable;
- broadcast committed mark, undo, and close changes through the existing SSE hub after the database transaction commits;
- return unavailable-style errors that do not disclose unauthorized session/person existence.

The migration must make `telegram_chat_context.session_id` nullable and add exactly:

```sql
state TEXT NOT NULL DEFAULT 'idle',
draft_name TEXT,
draft_scope TEXT,
draft_battery TEXT,
draft_end_time TIMESTAMP,
expires_at TIMESTAMP,
version BIGINT NOT NULL DEFAULT 0
```

The down migration must first delete only idle/draft context rows whose `session_id` is null, then restore the original non-null `session_id` constraint and drop the added columns. Existing selected-session rows must remain valid during rollback.

- [ ] **Step 1: Write failing migration/store tests** covering actor role resolution, context create/update/reload, optimistic version conflict, Tier 2 filtering, creator-only close, and closed-session cleanup.
- [ ] **Step 2: Run the focused tests against disposable PostgreSQL**.

```sh
export TEST_DATABASE_URL="postgres://postgres:pw@localhost:55441/app?sslmode=disable"
cd backend
go test ./internal/handlers -run 'TelegramAdmin|TelegramPairing' -count=1
```

Expected: FAIL on missing migration/store behavior; tests must skip only when `TEST_DATABASE_URL` is absent.

- [ ] **Step 3: Add the migration and implement `TelegramAdminStore`** using explicit transactions for event creation, manual marking, undo, status reads, and close. The transaction must reload the paired actor and target/session scope before mutation; `ClearContextForSession` must use the selected session ID and expected context version so concurrent drafts or another selected event are not erased.
- [ ] **Step 4: Run focused integration tests and migration up/down checks**.

Expected: PASS; a fresh database contains the new context columns, seeded pairings reload after a new store instance, and no production database is accessed.

- [ ] **Step 5: Commit the context/store foundation**.

```sh
git add backend/migrations/20260815000000_telegram_admin_context.sql backend/internal/telegram/admin_types.go backend/internal/handlers/telegram_admin.go backend/internal/handlers/telegram_admin_test.go backend/internal/handlers/telegram_admin_integration_test.go backend/internal/handlers/telegram.go
git diff --staged --check
git commit -m "feat: add Telegram admin context store"
```

---

### Task 3: Extend Telegram transport for URL buttons and QR photos

**Files:**
- Create: `backend/internal/telegram/qr.go`
- Create: `backend/internal/telegram/qr_test.go`
- Modify: `backend/internal/telegram/router.go`
- Modify: `backend/internal/telegram/client.go`
- Modify: `backend/internal/telegram/telegram_test.go`
- Modify: `go.mod`
- Modify: `go.sum`

**Interfaces:**

Extend `InlineKeyboardButton` with:

```go
URL string `json:"url,omitempty"`
```

Extend `Action` with:

```go
Photo       []byte
Caption     string
FallbackText string
```

Add:

```go
const SendPhoto ActionKind = ...

type PhotoSender interface {
    SendPhoto(ctx context.Context, chatID int64, photo []byte, caption string, replyMarkup *InlineKeyboardMarkup) error
}

func QRPNG(link string) ([]byte, error)
```

`Client.SendPhoto` must use multipart form data with `chat_id`, `photo`, `caption`, and JSON-encoded `reply_markup`. It must never include the bot token in returned errors. `Dispatcher` must send `FallbackText` through `SendMessage` if a `SendPhoto` action fails; if the sender does not implement `PhotoSender`, it must use the same fallback without panicking.

Use the already-present `github.com/skip2/go-qrcode` module and promote it to a direct dependency only if `go mod tidy` requires the declaration.

- [ ] **Step 1: Write failing client/QR/dispatcher tests** for URL-only buttons, callback-only buttons, PNG output, multipart fields, token-safe errors, photo delivery, and link fallback.
- [ ] **Step 2: Run the focused Telegram tests**.

```sh
cd backend
go test ./internal/telegram -run 'URL|QR|Photo|Dispatcher|Markup' -count=1
```

Expected: FAIL on the new transport behavior.

- [ ] **Step 3: Implement the transport and QR helper** with no network calls in unit tests.
- [ ] **Step 4: Run the full Telegram package tests**.

Expected: PASS with existing pairing/deep-link callback behavior unchanged.

- [ ] **Step 5: Commit the transport changes**.

```sh
git add backend/internal/telegram go.mod go.sum
git diff --staged --check
git commit -m "feat: deliver Telegram QR photos and links"
```

---

### Task 4: Implement the Telegram admin state machine and menus

**Files:**
- Create: `backend/internal/telegram/admin.go`
- Create: `backend/internal/telegram/admin_test.go`
- Modify: `backend/internal/telegram/router.go`
- Modify: `backend/internal/telegram/telegram_test.go`
- Modify: `backend/internal/handlers/telegram_admin.go` to satisfy the admin-store interface if needed

**Interfaces:**

Create:

```go
type AdminFlow interface {
    HandleMessage(ctx context.Context, message *Message, pairing Pairing) (actions []Action, handled bool, err error)
    HandleCallback(ctx context.Context, query *CallbackQuery, pairing Pairing) (actions []Action, handled bool, err error)
}

type AdminRouter struct { /* AdminStore and bot username */ }

func NewAdminRouter(store AdminStore) *AdminRouter
```

`ActiveEvent.TelegramLink` is the authorized, opaque `https://t.me/...` link and is the only QR input returned to the router; the raw deep-link code and QR secret remain inside the service/store.

The router must implement these behavior contracts:

- `/menu` loads the actor and returns the role-specific menu.
- `/cancel` clears draft state and returns the role-specific menu.
- A paired admin's ordinary linked response includes an `Open commander menu` button; a paired soldier receives the existing text-only response.
- Tier 2 menu contains only `Active events`.
- Tier 3+ menu contains `Create event` and `Active events`.
- `Create event` transitions through `creating_name`, `choosing_scope`, optional `choosing_battery`, `choosing_duration`, and `confirming_creation`.
- Name input trims and rejects empty or over-80-character values.
- Scope and duration callbacks accept only the exact button values in the design.
- Creation confirmation calls `AdminStore.CreateEvent` once and emits a text link plus `SendPhoto` action using `ActiveEvent.TelegramLink` and QR PNG.
- Selecting an existing active event uses its authorized `TelegramLink` for `Send QR + link` without loading or exposing a raw deep-link code.
- Active-event selection persists the selected session and returns the selected-event menu.
- Status pages render summary plus no more than 10 rows with deterministic `Prev`, `Next`, `Refresh`, and `Back` callbacks.
- Search prompts for text, calls `AdminStore.Status` with the query, and renders no more than 10 matching authorized rows.
- Mark callbacks require a selected session, call `MarkManual`, and return a result with an `Undo`/`My manual marks` path.
- Undo callbacks call `UndoManual` and render safe results for not-found, not-owned, not-manual, closed, and successful outcomes.
- Close callbacks require creator ownership or superadmin state, ask for confirmation, then clear selected context after success.
- Every admin callback is acknowledged promptly by the existing `Bot` callback path.
- Unknown/stale callbacks are harmless and do not disclose whether a session or person exists.

Use namespaced callback data (`p:` remains pairing; `a:` is admin) and keep each payload within Telegram's 64-byte limit. The admin router must not accept actor IDs from callback data.

Update the `Bot` constructors without breaking existing tests:

```go
func NewBotWithAdmin(pairings PairingLookup, marker AttendanceMarker, admin AdminFlow) *Bot
```

`NewBot` and `NewBotWithAttendance` remain valid compatibility constructors. Paired `/start <payload>` continues to take the soldier attendance path before admin routing.

- [ ] **Step 1: Write failing pure-router tests** for all three role menus, wizard transitions, invalid input, cancellation, active selection, status pagination, search, mark/undo, close ownership, stale callbacks, and soldier compatibility.
- [ ] **Step 2: Run the pure-router tests**.

```sh
cd backend
go test ./internal/telegram -run 'Admin|Menu|Wizard|Status|Mark|Close' -count=1
```

Expected: FAIL because the admin flow and callbacks do not exist.

- [ ] **Step 3: Implement the state machine and integrate it into `Bot.HandleUpdate`** while preserving pairing callback parsing and soldier deep-link behavior.
- [ ] **Step 4: Run the entire Telegram package suite**.

```sh
cd backend
go test ./internal/telegram -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit the admin UI flow**.

```sh
git add backend/internal/telegram backend/internal/handlers/telegram_admin.go
git diff --staged --check
git commit -m "feat: add Telegram commander admin menus"
```

---

### Task 5: Wire production runtime and add end-to-end coverage

**Files:**
- Modify: `backend/cmd/api/main.go`
- Modify: `backend/internal/handlers/telegram.go`
- Create or modify: `backend/internal/handlers/telegram_admin_e2e_test.go`
- Modify: `backend/internal/telegram/attendance_routing_test.go` if constructor wiring affects fixtures
- Modify: `specs/009-telegram-bot/spec.md`
- Modify: `specs/009-telegram-bot/plan.md`
- Modify: `docs/superpowers/specs/2026-08-12-telegram-admin-ui-design.md` only if implementation reveals a verified contract correction

**Interfaces:**

`newTelegramRuntime` must construct one shared set of session/report services, one role-aware Telegram admin store, one `AdminRouter`, and the existing soldier pairing/attendance adapters. The public webhook route and secret-header authentication remain unchanged.

Add a synthetic PostgreSQL E2E test that drives the bot through the same `Bot.HandleUpdate` boundary with a fake Telegram sender and these fixtures:

1. paired Tier 3 creator creates a battery-specific event, confirms one session row, receives a URL button and PNG photo action, and reloads the selected session through a new store/router instance;
2. paired Tier 2 cannot create or close, sees only an authorized active event, sees only own-battery missing rows, marks an unpaired target manually, and can undo only that manual row;
3. a paired Tier 3 cannot close another creator's event, while a superadmin can close it;
4. a stale callback after closure cannot mark attendance;
5. two repeated creation confirmations cannot create duplicate session rows;
6. existing soldier `/start <code>` marks attendance once and remains compatible with the new admin wiring;
7. malformed/unauthenticated/group updates do not send admin actions or leak names.

The test must use unique synthetic IDs and clean up all rows by prefix or transaction. It must not read production credentials or data.

- [ ] **Step 1: Write the failing runtime/E2E tests and update the Telegram spec acceptance criteria** to include the approved admin surface while retaining the dashboard.
- [ ] **Step 2: Run the E2E tests against disposable PostgreSQL**.

```sh
export TEST_DATABASE_URL="postgres://postgres:pw@localhost:55441/app?sslmode=disable"
cd backend
go test ./internal/handlers ./internal/telegram -run 'Telegram.*Admin|Admin.*E2E' -count=1
```

Expected: FAIL before runtime wiring and end-to-end behavior are complete.

- [ ] **Step 3: Wire `newTelegramRuntime` and complete the synthetic E2E flow.**
- [ ] **Step 4: Run the feature validation suite**.

From repository root:

```sh
docker run --rm --name telegram-admin-pg -e POSTGRES_PASSWORD=pw -e POSTGRES_DB=app -d -p 55441:5432 postgres:17-alpine
trap 'docker rm -f telegram-admin-pg >/dev/null 2>&1 || true' EXIT
export DATABASE_URL="postgres://postgres:pw@localhost:55441/app?sslmode=disable"
export TEST_DATABASE_URL="$DATABASE_URL"
cd backend
go run ./cmd/migrate up ./migrations
go build ./...
go vet ./...
go test ./... -count=1
go test ./... -race -count=1
cd ../frontend
npm run generate
npm run build
npm run lint
```

Expected: every command exits 0; PostgreSQL migration output includes `20260815000000`; no Telegram token is printed.

- [ ] **Step 5: Commit runtime wiring, E2E tests, and spec updates**.

```sh
git add backend/cmd/api/main.go backend/internal/handlers/telegram.go backend/internal/handlers/telegram_admin_e2e_test.go backend/internal/telegram specs/009-telegram-bot/spec.md specs/009-telegram-bot/plan.md
git diff --staged --check
git commit -m "feat: wire Telegram admin attendance flow"
```

---

## Whole-branch verification and deployment

After all task commits are reviewed and the branch is clean:

1. Run `git diff --check`, `git status --short`, `git diff origin/main...HEAD --stat`, and the complete validation block from Task 5.
2. Scan for secrets:

```sh
rg -n 'TELEGRAM_BOT_TOKEN\s*=\s*["0-9]|TELEGRAM_WEBHOOK_SECRET\s*=\s*["A-Za-z0-9]' backend/ frontend/ --glob '!*.md' || true
git log -p origin/main..HEAD | rg -n '[0-9]{8,10}:[A-Za-z0-9_-]{35}' || true
```

Expected: no credential-shaped output.

3. Push `010-telegram-admin-ui` and open a draft PR against `main` with a concise description, role matrix, migration note, and exact validation commands.
4. Wait for repository checks. If checks pass, mark the PR ready and merge it using the repository owner account. Do not merge a preview branch.
5. Watch the `main` deployment workflow to completion. The migration timestamp is newer than the currently deployed Telegram migrations, so it should apply normally without out-of-order recovery.
6. On `redcon.236sa.one`, verify only presence/length of the four Telegram environment values, the API container is `Up`, migration `20260815000000` is applied, and public `/health` returns HTTP 200.
7. Verify the existing webhook remains registered with Telegram and send one authenticated malformed webhook update; expect HTTP 200 with no outbound action. Send the same request without the secret header; expect the intentional HTTP 404 rejection.
8. Do not claim a real Telegram user-flow smoke test until the user opens `https://t.me/our_236_bot` and sends `/menu`; report that final manual step separately.
