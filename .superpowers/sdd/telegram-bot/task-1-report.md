# Feature 009 Task 1 report — shared attendance service

Status: DONE
Branch: `009-task1-attendance-service`

## Commits

- `80ea36f` — add PostgreSQL integration tests for the marking rules.
- `74d1088` — add the transaction-bound `services/attendance` service.
- `8fe07c1` — route the three attendance handlers through the service.
- `4df6f64` — make service test fixtures match the current user schema.
- `4d2a138` — document the supported mark methods on `MarkRequest`.

## Changed files

- `backend/internal/services/attendance/attendance.go` — adds `MarkRequest`, `MarkOutcome`, and `Mark`, including active-session checks, all three scope checks, idempotent unique handling, and `marked_by` persistence.
- `backend/internal/services/attendance/attendance_test.go` — adds real-PostgreSQL integration coverage for every required service rule and concurrent duplicate marking.
- `backend/internal/handlers/attendance.go` — makes `HandleQRScan`, `MarkAttendance`, and `ManualMarkAttendance` transaction-owning adapters and preserves their existing HTTP responses.

No migration was added or changed. The `marking_method` CHECK constraint remains limited to `qr_scan` and `manual`. No Telegram code was added.

## Service tests and how each brief case was verified

All service tests ran against PostgreSQL 17 in Docker after applying `backend/migrations` with the required directory argument.

- Active unit-wide mark: `TestMarkActiveUnitWideSession` inserts one record and asserts `Marked`.
- Duplicate mark: `TestMarkAlreadyMarkedIsIdempotent` marks twice, asserts `Marked` then `AlreadyMarked`, and counts one record.
- Closed session: `TestMarkClosedSessionDoesNotInsert` asserts `SessionClosed` and zero records.
- Battery-specific in scope: `TestMarkBatterySpecificScope/in_scope` uses a matching `Alpha` battery and asserts `Marked`.
- Battery-specific out of scope: `TestMarkBatterySpecificScope/out_of_scope` uses `Bravo` against an `Alpha` session and asserts `OutOfScope` plus zero records.
- Custom-list participant: `TestMarkCustomListScope/participant_is_in_scope` inserts `session_participants` membership and asserts `Marked`.
- Custom-list non-participant: `TestMarkCustomListScope/non-participant_is_out_of_scope` omits membership and asserts `OutOfScope` plus zero records.
- Manual attribution: `TestMarkManualRecordsMarkedBy` asserts the stored `marked_by` equals the commander ID.
- Concurrent marks: `TestMarkConcurrentDuplicateIsIdempotent` runs two transactions concurrently, asserts one `Marked`, one `AlreadyMarked`, no errors, and one record.

The existing handler tests passed unchanged. No existing test was modified to accommodate this refactor; only the new service test file was added.

## SSE decision

SSE remains in `handlers`, not in the service. `Mark` only decides and persists the attendance mark inside the caller-owned transaction. Each handler broadcasts only after its transaction commits, so live clients cannot observe a mark that later rolls back. Broadcasting is external state and does not belong in the reusable marking decision.

## Validation commands

Database setup passed:

```sh
docker run --rm --name pg9a -e POSTGRES_PASSWORD=pw -e POSTGRES_DB=att -d -p 55450:5432 postgres:17-alpine
export DATABASE_URL="postgres://postgres:pw@localhost:55450/att?sslmode=disable"
export TEST_DATABASE_URL="$DATABASE_URL"
cd backend && go run ./cmd/migrate up ./migrations
```

All migrations through `20260802000000_operating_at_scale.sql` applied successfully. The container was cleaned up with `docker rm -f pg9a` after validation.

### `go test ./backend/internal/services/attendance -v`

```text
=== RUN   TestMarkActiveUnitWideSession
--- PASS: TestMarkActiveUnitWideSession (0.02s)
=== RUN   TestMarkAlreadyMarkedIsIdempotent
--- PASS: TestMarkAlreadyMarkedIsIdempotent (0.02s)
=== RUN   TestMarkClosedSessionDoesNotInsert
--- PASS: TestMarkClosedSessionDoesNotInsert (0.01s)
=== RUN   TestMarkBatterySpecificScope
=== RUN   TestMarkBatterySpecificScope/in_scope
=== RUN   TestMarkBatterySpecificScope/out_of_scope
--- PASS: TestMarkBatterySpecificScope (0.03s)
    --- PASS: TestMarkBatterySpecificScope/in_scope (0.01s)
    --- PASS: TestMarkBatterySpecificScope/out_of_scope (0.01s)
=== RUN   TestMarkCustomListScope
=== RUN   TestMarkCustomListScope/participant_is_in_scope
=== RUN   TestMarkCustomListScope/non-participant_is_out_of_scope
--- PASS: TestMarkCustomListScope (0.03s)
    --- PASS: TestMarkCustomListScope/participant_is_in_scope (0.01s)
    --- PASS: TestMarkCustomListScope/non-participant_is_out_of_scope (0.01s)
=== RUN   TestMarkManualRecordsMarkedBy
--- PASS: TestMarkManualRecordsMarkedBy (0.01s)
=== RUN   TestMarkConcurrentDuplicateIsIdempotent
--- PASS: TestMarkConcurrentDuplicateIsIdempotent (0.01s)
PASS
ok  	github.com/davidlivingston/go-nextjs-starter/backend/internal/services/attendance	(cached)
```

### `cd backend && go build ./...`

Verbatim output: no stdout or stderr. Exit status 0.

### `cd backend && go vet ./...`

Verbatim output: no stdout or stderr. Exit status 0.

### `cd backend && go test ./...`

```text
?   	github.com/davidlivingston/go-nextjs-starter/backend/cmd/api	[no test files]
?   	github.com/davidlivingston/go-nextjs-starter/backend/cmd/migrate	[no test files]
?   	github.com/davidlivingston/go-nextjs-starter/backend/internal/database	[no test files]
ok  	github.com/davidlivingston/go-nextjs-starter/backend/internal/handlers	4.940s
ok  	github.com/davidlivingston/go-nextjs-starter/backend/internal/middleware	(cached)
?   	github.com/davidlivingston/go-nextjs-starter/backend/internal/models	[no test files]
ok  	github.com/davidlivingston/go-nextjs-starter/backend/internal/services/agent	(cached)
ok  	github.com/davidlivingston/go-nextjs-starter/backend/internal/services/attendance	0.744s
ok  	github.com/davidlivingston/go-nextjs-starter/backend/internal/services/matching	(cached)
?   	github.com/davidlivingston/go-nextjs-starter/backend/internal/sse	[no test files]
```

### `cd frontend && npm run build`

```text
> go-nextjs-frontend@0.1.0 build
> tsc -b && vite build

vite v7.2.4 building client environment for production...
[baseline-browser-mapping] The data in this module is over two months old.  To ensure accurate Baseline data, please update: `npm i baseline-browser-mapping@latest -D`
transforming...
Browserslist: browsers data (caniuse-lite) is 9 months old. Please run:
  npx update-browserslist-db@latest
  Why you should do it regularly: https://github.com/browserslist/update-db#readme
✓ 3574 modules transformed.
rendering chunks...
computing gzip size...
dist/index.html                     0.52 kB │ gzip:   0.32 kB
dist/assets/index-DIY95Nft.css     63.22 kB │ gzip:  11.20 kB
dist/assets/index-BR8TA05r.js   1,601.59 kB │ gzip: 493.08 kB

(!) Some chunks are larger than 500 kB after minification. Consider:
- Using dynamic import() to code-split the application
- Use build.rollupOptions.output.manualChunks to improve chunking
- Adjust chunk size limit for this warning via build.chunkSizeWarningLimit.
✓ built in 4.78s
```

### `cd frontend && npm run lint`

```text
> go-nextjs-frontend@0.1.0 lint
> eslint .

[baseline-browser-mapping] The data in this module is over two months old.  To ensure accurate Baseline data, please update: `npm i baseline-browser-mapping@latest -D`
```

Build and lint both exited 0. Frontend emitted only existing dependency freshness and bundle-size warnings.

## Residual risks

- Frontend build warnings are pre-existing and non-blocking.
- The runtime-created untracked `.pi-subagents/` directory was not staged and is unrelated to this task.
- Handler outcome mapping for the newly centralized `OutOfScope` value uses HTTP 403 for QR/API paths and an error entry for manual marking; no previous endpoint test covered that formerly un-enforced path.

The working tree has no staged files after the implementation commits.
