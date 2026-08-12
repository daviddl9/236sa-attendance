# Task 3 report — Telegram client, webhook endpoint, and authenticity

Status: DONE

## Commits

- `282abbc` — add the standard-library Telegram client, configuration and update decoding.
- `b3a319d` — add private-message routing, reply composition, callback acknowledgement and the asynchronous reply dispatcher.
- `9036e94` — add the authenticated public webhook and read-only pairing lookup.
- `be76643` — register the public webhook route and optional runtime; document environment variables.
- `17c381c` — strengthen authentication tests and reject empty configured secrets.
- `a6407d5` — make dispatcher shutdown safe while requests are in flight.

## Files changed

- `backend/internal/telegram/client.go` — thin JSON `sendMessage`, `answerCallbackQuery` and `setWebhook` client over `net/http`; transport is injectable and errors never include the token.
- `backend/internal/telegram/config.go` — environment configuration and stable opaque webhook URL composition.
- `backend/internal/telegram/update.go` — safe decoding of messages and callback queries.
- `backend/internal/telegram/router.go` — private-chat-only routing, own-name-only reply composition, callback acknowledgement and asynchronous outbound queue.
- `backend/internal/telegram/telegram_test.go` — no-network client, decoding, routing, privacy, configuration and timing tests.
- `backend/internal/handlers/telegram.go` — constant-time secret-header authentication, public webhook handling and read-only `telegram_pairing` lookup.
- `backend/internal/handlers/telegram_test.go` — webhook authentication, no-outbound-on-rejection, group filtering, unknown-account replies, malformed input and disabled-runtime tests.
- `backend/cmd/api/main.go` — optional Telegram runtime and public route registration outside protected middleware; no automatic `setWebhook` call.
- `backend/.env.example` — blank optional Telegram environment entries.
- `.superpowers/sdd/telegram-bot/task-3-report.md` — this report.

No attendance service, session deep-link resolver, pairing writer, pairing-request reader, attendance table writer, or marking-method constraint was changed or called. A repository search over the new Telegram code and handler returned no `attendance.Mark`, SQL write, `telegram_pairing_request`, or `deeplink` use.

## Required test verification

- Correct, wrong, missing and empty secret headers: `handlers.TestTelegramWebhookAuthenticity`. The fake action sink proves rejected requests enqueue no outbound action; the accepted request queues one action.
- Constant-time comparison: `backend/internal/handlers/telegram.go` imports `crypto/subtle` and calls `subtle.ConstantTimeCompare`; there is no secret equality comparison.
- Group and supergroup messages: `telegram.TestBotIgnoresNonPrivateMessages` and `handlers.TestTelegramWebhookGroupMessageHasNoActionOrReply`; neither performs a pairing lookup nor queues a reply.
- Unknown private account: `telegram.TestBotRepliesToUnknownPrivateAccount` and `handlers.TestTelegramWebhookRepliesToUnknownPrivateAccount`; both assert the fixed not-yet-linked text.
- Update decoding: `telegram.TestDecodeUpdateVariants` covers a private message, `/start` payload, callback query and missing fields; `TestDecodeUpdateRejectsMalformedJSON` covers malformed and multiple JSON values. The webhook returns 200 for authenticated malformed/missing updates.
- No-name leakage: `telegram.TestBotRepliesWithOnlyPairedAccountName` asserts the paired account receives only its own name; unknown replies contain no personnel name. The database lookup is keyed by the sender's Telegram ID and joins only that paired user's row.
- No-network client: `telegram.TestClientUsesJSONAPIWithoutLeakingErrors` and `TestSetWebhookAndCallbackMethodsUseExpectedAPI` use an injected `RoundTripper` and verify JSON payloads. The token fixture is the non-secret placeholder `redacted-token`, not a Telegram credential.
- Missing configuration: `newTelegramRuntime` logs one disabled message when `TELEGRAM_BOT_TOKEN` is absent and constructs a rejecting handler without a dispatcher. A real PostgreSQL-backed binary was started with all Telegram variables unset; it started normally and logged `Telegram bot disabled: set TELEGRAM_BOT_TOKEN to enable`. Curl requests to the registered disabled route returned 404 for both missing and wrong headers.
- Slow outbound timing: `telegram.TestDispatcherDoesNotBlockOnSender` uses a blocking fake sender and proves `Enqueue` returns while sending is blocked. The webhook only authenticates, decodes, routes and enqueues before returning 200; the worker owns the Telegram API call.
- No token-shape matches: `rg -n '[0-9]{8,10}:[A-Za-z0-9_-]{35}' backend/ frontend/` returned no matches.

## Outbound timing and webhook registration

Outbound operations are placed on a bounded, non-blocking background dispatcher. The handler does not wait for Telegram's network call, so a slow Telegram API cannot cause webhook retries. The dispatcher has a 512-action queue, a 10-second per-call context, and drains safely during shutdown. If the queue is saturated, an update is still acknowledged with 200 rather than making Telegram retry it.

`Client.SetWebhook(ctx, webhookURL, secretToken)` is an explicit callable operation. It is never called during API boot. The route is `/api/telegram/webhook/<opaque-segment>`; the segment is the first 16 hex characters of SHA-256 of the configured webhook secret, so it is stable from configuration but does not disclose the header secret in URL logs. The header remains mandatory and is the real authentication control.

## Verbatim validation output

### `go build ./...`

```text
```

Exit status: 0.

### `go vet ./...`

```text
```

Exit status: 0.

### `go test ./...` (with the supplied PostgreSQL 17 container and migrations)

```text
?    github.com/davidlivingston/go-nextjs-starter/backend/cmd/api [no test files]
?    github.com/davidlivingston/go-nextjs-starter/backend/cmd/migrate [no test files]
?    github.com/davidlivingston/go-nextjs-starter/backend/internal/database [no test files]
ok   github.com/davidlivingston/go-nextjs-starter/backend/internal/handlers 4.709s
ok   github.com/davidlivingston/go-nextjs-starter/backend/internal/middleware (cached)
?    github.com/davidlivingston/go-nextjs-starter/backend/internal/models [no test files]
ok   github.com/davidlivingston/go-nextjs-starter/backend/internal/services/agent (cached)
ok   github.com/davidlivingston/go-nextjs-starter/backend/internal/services/attendance 2.165s
ok   github.com/davidlivingston/go-nextjs-starter/backend/internal/services/deeplink 1.614s
ok   github.com/davidlivingston/go-nextjs-starter/backend/internal/services/matching (cached)
?    github.com/davidlivingston/go-nextjs-starter/backend/internal/sse [no test files]
ok   github.com/davidlivingston/go-nextjs-starter/backend/internal/telegram 1.048s
```

Exit status: 0.

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
dist/assets/index-XtLNV7bV.js   1,601.70 kB │ gzip: 493.10 kB

(!) Some chunks are larger than 500 kB after minification. Consider:
- Using dynamic import() to code-split your application
- Using build.rollupOptions.output.manualChunks to improve chunking
- Adjust chunk size limit with build.chunkSizeWarningLimit.
✓ built in 6.07s
```

Exit status: 0. The two warning lines in the captured output were the repository's existing browser-data/chunk-size advisories; they did not fail the build.

### `cd frontend && npm run lint`

```text
> go-nextjs-frontend@0.1.0 lint
> eslint .

[baseline-browser-mapping] The data in this module is over two months old. To ensure accurate Baseline data, please update: `npm i baseline-browser-mapping@latest -D`
```

Exit status: 0.

## Concerns

- The dispatcher intentionally acknowledges updates even when its bounded queue is saturated, as required to avoid Telegram retries. A future operational task may add metrics or a durable outbound queue if sustained Telegram outage handling is needed.
- Frontend build and lint emitted only pre-existing advisory warnings.
- No other disagreements with the brief.

## Working tree

`git diff --cached --name-status` was empty after validation. The only untracked path is the pre-existing `.pi-subagents/` runtime directory; it was not staged.

# Round 1 review fixes

Status: DONE

## Changes

- `Dispatcher.Enqueue` now accepts an entire update's action list atomically and returns `false` when the bounded queue has no room. The webhook returns HTTP 503 in that case, leaving Telegram to redeliver the update. Saturation is logged with a visible cumulative rejection count.
- Authentic but unactionable updates still return HTTP 200. Empty action lists are accepted by the dispatcher, while only queue capacity or another genuine enqueue failure returns 503.
- `TELEGRAM_WEBHOOK_PATH` is now an independent configuration value. `TELEGRAM_WEBHOOK_PATH=` is documented in `backend/.env.example`; an enabled bot with no path fails startup with `TELEGRAM_WEBHOOK_PATH must be set when Telegram is enabled`. No secret-derived fallback remains.

## Covering tests

- `backend/internal/handlers/telegram_test.go`: `TestTelegramWebhookReturns503WhenReplyQueueIsFull` fills the real dispatcher queue, asserts 503, preserves the queued action, and confirms a Telegram-style retry is accepted; `TestTelegramWebhookBodyLimitBoundary` covers exactly 1 MiB and 1 MiB + 1 byte; `TestTelegramWebhookHandlesNullAndUnexpectedFieldTypes` exercises null and wrong JSON types through the HTTP handler with panic detection.
- `backend/internal/telegram/telegram_test.go`: `TestConfigUsesIndependentWebhookPath` verifies the path does not change with the secret; `TestConfigRequiresWebhookPathWhenEnabled` verifies disabled configuration remains valid and enabled configuration fails without a path.

## Verbatim validation output

### `cd backend && go build ./...`

```text
```

Exit status: 0.

### `cd backend && go vet ./...`

```text
```

Exit status: 0.

### `cd backend && go test ./...`

```text
?    github.com/davidlivingston/go-nextjs-starter/backend/cmd/api	[no test files]
?    github.com/davidlivingston/go-nextjs-starter/backend/cmd/migrate	[no test files]
?    github.com/davidlivingston/go-nextjs-starter/backend/internal/database	[no test files]
ok   github.com/davidlivingston/go-nextjs-starter/backend/internal/handlers	1.552s
ok   github.com/davidlivingston/go-nextjs-starter/backend/internal/middleware	(cached)
?    github.com/davidlivingston/go-nextjs-starter/backend/internal/models	[no test files]
ok   github.com/davidlivingston/go-nextjs-starter/backend/internal/services/agent	(cached)
ok   github.com/davidlivingston/go-nextjs-starter/backend/internal/services/attendance	1.862s
ok   github.com/davidlivingston/go-nextjs-starter/backend/internal/services/deeplink	0.945s
ok   github.com/davidlivingston/go-nextjs-starter/backend/internal/services/matching	(cached)
?    github.com/davidlivingston/go-nextjs-starter/backend/internal/sse	[no test files]
ok   github.com/davidlivingston/go-nextjs-starter/backend/internal/telegram	(cached)
```

Exit status: 0. PostgreSQL 17 was started as `pg9i`, migrations were run with `go run ./cmd/migrate up ./migrations`, and the container was used for this suite.

### `cd frontend && npm run build`

```text
> go-nextjs-frontend@0.1.0 build
> tsc -b && vite build

vite v7.2.4 building client environment for production...
[baseline-browser-mapping] The data in this module is over two months old. To ensure accurate Baseline data, please update: `npm i baseline-browser-mapping@latest -D`
transforming...
Browserslist: browsers data (caniuse-lite) is 9 months old. Please run:
  npx update-browserslist-db@latest
  Why? https://github.com/browserslist/update-db#readme
✓ 3574 modules transformed.
rendering chunks...
computing gzip size...
dist/index.html                     0.52 kB │ gzip:   0.32 kB
dist/assets/index-DIY95Nft.css     63.22 kB │ gzip:  11.20 kB
dist/assets/index-XtLNV7bV.js   1,601.70 kB │ gzip: 493.10 kB

(!) Some chunks are larger than 500 kB after minification. Consider:
- Using dynamic import() to code-split your application
- Use rollupOptions.output.manualChunks to improve chunking: https://rollupjs.org/configuration-options.html#output-manualchunks
- Adjust chunk size limit via build.chunkSizeWarningLimit.
✓ built in 4.43s
```

Exit status: 0. The build emitted the existing browser-data and chunk-size advisories.

### `cd frontend && npm run lint`

```text
> go-nextjs-frontend@0.1.0 lint
> eslint .

[baseline-browser-mapping] The data in this module is over two months old. To ensure accurate Baseline data, please update: `npm i baseline-browser-mapping@latest -D`
```

Exit status: 0.

### Additional checks

- `go test -race ./backend/internal/telegram ./backend/internal/handlers` passed.
- With Telegram variables unset, the API started normally and logged `Telegram bot disabled: set TELEGRAM_BOT_TOKEN to enable`.
- With a token but no webhook path, startup exited clearly with `Invalid Telegram configuration: TELEGRAM_WEBHOOK_PATH must be set when Telegram is enabled`.
- `rg -n '[0-9]{8,10}:[A-Za-z0-9_-]{35}' backend/ frontend/` returned no output.
- `git diff --cached --name-status` returned no output after each incremental commit; `.pi-subagents/` remained the only untracked path and was not staged.

## Fix concerns

- The configured webhook path must be provisioned as a sufficiently random, URL-safe segment by deployment configuration; the application no longer derives it from the header secret.
- Frontend build/lint emitted only existing advisory warnings.
