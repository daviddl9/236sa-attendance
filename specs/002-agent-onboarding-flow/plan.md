# Implementation Plan: Agent-Assisted Onboarding And Explicit Signup For New Users

**Branch**: `002-agent-onboarding-flow` | **Date**: 2026-05-22 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `specs/002-agent-onboarding-flow/spec.md`

## Summary

Add two cooperating capabilities. First, an administrator-only document-import flow where the backend invokes a Claude-based agent (Anthropic Messages API with native PDF / document understanding, structured JSON output, and access to the Anthropic-published `pdf`, `docx`, `xlsx` skills via the Claude Agent SDK runtime) to parse a personnel source document into structured rows, presents a preview that distinguishes existing-user matches from new-user rows, and writes to the user table only after the administrator explicitly chooses a merge mode ("fill gaps only" vs "override fields from document"). Second, a sign-in flow change that stops silently auto-creating user accounts: when the submitted Full Name is unknown, the sign-in page reveals an explicit "Create your account" block with a separate confirmation NRIC Last 5 field where paste, drag-drop, context-menu paste, and browser autofill are blocked. The four-digits-plus-letter rule from feature 001 remains the authoritative format check throughout.

## Technical Context

**Language/Version**: Go 1.25.2 backend; TypeScript 5.9.3 with React 19.2 frontend (TanStack Router + Query)

**Primary Dependencies**:
- Backend: go-chi router, pgx/PostgreSQL, golang.org/x/crypto bcrypt, godotenv. **New**: Anthropic SDK access via Messages API and Files API. Preferred client: a thin internal HTTP wrapper in `backend/internal/services/agent/` (no third-party Anthropic Go SDK adoption in v1 — keep dependency footprint small).
- Frontend: shadcn/ui (Radix primitives), Tailwind, sonner toasts, xlsx parser (existing). **New**: a small `NoPasteInput` component that suppresses paste / drop / context-menu paste / autofill.

**Storage**: PostgreSQL. Existing `user` table is the authoritative store for personnel; no schema change to it. **New tables (lightweight audit)**:
- `personnel_imports` — one row per import job (id, admin_user_id, document filename, merge_mode, row_counts, started_at, finished_at, status, error_message).
- `personnel_import_changes` — one row per per-user change inside an import (import_id, user_id, action [created|updated|skipped|failed], field_name, before_value, after_value, reason).

**Testing**: Go `go test ./internal/...` for handler, service, and parser units (package-mode, not file-mode — the smart-test hook false-positive applies). Frontend `npm run build` + `npm run lint` plus targeted Vitest/React-Testing-Library tests for sign-in branching and the `NoPasteInput` paste/drop/autofill suppression. End-to-end happy path verified manually against a real Anthropic API key in dev.

**Target Platform**: Web application — Go API container + Vite React SPA, deployed via existing GitHub Actions pipeline to `redcon.236sa.one` (port 8081, see CLAUDE.md).

**Project Type**: Full-stack web application; no new runtime introduced. The Anthropic agent is reached over HTTPS from the Go backend; no sidecar process or new container in v1.

**Performance Goals**:
- Document parse + preview round-trip under 30 seconds for documents up to 200 rows / ~5 MB on a warm container (Anthropic API latency dominates).
- Import commit (preview → DB write) under 5 seconds for 200 rows on the existing PostgreSQL container, reusing the parallel bcrypt-hash pattern from `BulkCreateUsers`.
- Sign-in latency unchanged (the lookup adds one keyed read of `user` by (full_name, nric_last5) and one of (full_name) when no match — both indexed paths).

**Constraints**:
- Anthropic API key MUST stay server-side. No browser-side fetch to `api.anthropic.com`. Loaded once via godotenv as `ANTHROPIC_API_KEY` (same loading pattern as the existing `POLAR_WEBHOOK_SECRET` in `backend/internal/handlers/webhook.go`).
- NRIC Last 5 / password values returned from the agent are validated against feature 001's regex before any DB write, and are never overwritten on existing users regardless of merge mode.
- Existing administrator sign-in path and existing bulk-Excel upload must remain functional and untouched in behaviour.
- Document upload size is bounded (initial limit: 10 MB; rejected with a clear error above that) so a single malformed file cannot DoS the agent call.
- The import is transactional at the database boundary: any Anthropic / parser failure leaves the user table unchanged (FR-011, SC-006).

**Scale/Scope**:
- Per-import: up to ~200 personnel rows (a typical SAF callup roster). v1 does not target 10k-row imports.
- Concurrency: one import job at a time per admin is sufficient; serialise per-admin via a simple in-memory mutex keyed by admin user id (or DB row lock on `personnel_imports`).
- Feature touches ~10 files across backend + frontend and adds two small DB tables. No new infra.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

The repository constitution (`.specify/memory/constitution.md`) still holds template placeholders and defines no enforceable project-specific gates. No constitution violations are identified. The plan follows the repo's working conventions: backend is the source of truth for validation and authentication; secrets stay server-side; database changes go through SQL migrations under `backend/migrations/`; UI changes reuse the existing shadcn/Tailwind component vocabulary; tests run in package mode (`go test ./internal/handlers/...`) to side-step the smart-test hook false positive recorded in project memory.

**Post-Design Recheck**: PASS. The design introduces one new third-party dependency (Anthropic HTTPS API), two narrow DB tables for audit, and no new runtime, container, or service. Complexity stays proportional to the feature.

## Project Structure

### Documentation (this feature)

```text
specs/002-agent-onboarding-flow/
├── plan.md              # This file
├── research.md          # Phase 0 — Anthropic API shape, doc-skill capability, paste-blocking technique survey
├── data-model.md        # Phase 1 — personnel_imports + personnel_import_changes schema and lifecycle
├── quickstart.md        # Phase 1 — local-dev steps to set ANTHROPIC_API_KEY and run an end-to-end import
├── contracts/
│   ├── admin-import-api.md         # POST /api/admin/users/import-document, /preview, /commit
│   ├── auth-sign-in-api.md         # Updated POST /api/auth/sign-in semantics + new "signup required" response
│   └── auth-signup-api.md          # POST /api/auth/sign-up explicit signup contract
└── tasks.md             # Phase 2 — produced by /speckit.tasks (not by this command)
```

### Source Code (repository root)

```text
backend/
├── cmd/api/main.go                              # wire new admin import routes + new signup route
├── internal/
│   ├── handlers/
│   │   ├── auth.go                              # SignIn: stop silent auto-create; add Signup handler; preserve admin path
│   │   ├── auth_test.go                         # branches: known user, unknown name, name match + wrong NRIC, admin
│   │   ├── import.go                            # NEW: PreviewImport, CommitImport (admin-only)
│   │   └── import_test.go                       # NEW: merge-mode behaviour, password-never-overwritten, transactionality
│   ├── services/agent/
│   │   ├── client.go                            # NEW: thin Anthropic Messages/Files HTTP client; reads ANTHROPIC_API_KEY
│   │   ├── parser.go                            # NEW: prompt + structured-output tool schema → []ParsedPersonnelRow
│   │   └── parser_test.go                       # NEW: fixture-driven parsing + error mapping (timeout, invalid key, no rows)
│   ├── middleware/rbac.go                       # unchanged; RequireSuperadmin gates new admin routes
│   └── models/
│       ├── user.go                              # unchanged
│       └── import.go                            # NEW: PersonnelImport, PersonnelImportChange structs
└── migrations/
    └── 00X_personnel_imports.sql                # NEW: personnel_imports + personnel_import_changes tables

frontend/
├── src/
│   ├── components/
│   │   └── no-paste-input.tsx                   # NEW: input that blocks paste, drop, context-menu paste, autofill
│   ├── routes/
│   │   ├── sign-in.tsx                          # add explicit "Create your account" block + confirm field
│   │   └── dashboard/users/
│   │       ├── import-document.tsx              # NEW: upload → preview → merge-mode → commit → summary
│   │       └── bulk-upload.tsx                  # unchanged behaviour; keep available as alternative
│   └── lib/
│       └── api-client.ts                        # add types + functions for import + signup endpoints
```

**Structure Decision**: Reuse the existing full-stack layout. Put the Anthropic-facing code behind a single `backend/internal/services/agent` package so the rest of the code talks to a typed `ParseDocument` function — never to the API directly. Keep the import handler thin (HTTP I/O, merge logic, transaction boundary) and push parsing concerns into the `agent` package; the same surface can later swap to a Claude Agent SDK sidecar without touching the handler.

## Key Design Decisions

### D1 — Agent integration shape: direct Anthropic API, not a sidecar

**Decision**: The Go backend talks to Anthropic over HTTPS directly. PDFs are sent via the Files API and referenced in a Messages API call with a structured-output tool that returns `{ rows: [{ full_name, rank, battery, nric_last5, extras, confidence }] }`. DOCX is converted to PDF server-side (via `unoconv`/`libreoffice` if available, else fallback to plain-text extraction) before upload; XLSX is parsed server-side with the existing xlsx logic and only the table content is sent to the model for normalisation. The Claude Agent SDK `pdf`/`docx`/`xlsx` skills cover the same surface area, and a sidecar running the Agent SDK is the documented fallback if a doc type turns out not to be handled well by the Messages API path alone.

**Why**: One deployable, one set of secrets, one place to audit. Adds zero new runtime to the existing Docker layout. The fallback path (sidecar) is real and we will state when we would adopt it, but we should not pay for it until Phase 0 research shows we have to.

**Phase 0 must confirm**: that native Messages API + Files API handles the document types this user base actually sends (SAF callup PDFs, ad-hoc Word and Excel rosters) at acceptable accuracy. If not, switch D1 to a sidecar Claude Agent SDK process with `pdf`/`docx`/`xlsx` skills installed; the handler boundary is designed to absorb that swap.

### D2 — Auth flow change: "signup required" is a distinct sign-in response

**Decision**: `POST /api/auth/sign-in` returns one of three outcomes: (a) success with session cookie, (b) `signup_required` with the submitted Full Name echoed back so the frontend can render the explicit signup block on the same page, or (c) generic `invalid_credentials`. The "Full Name matches but NRIC Last 5 doesn't" case maps to `invalid_credentials` so we never leak whether a name exists. Account creation moves to a dedicated `POST /api/auth/sign-up` endpoint that requires both `nric_last5` and `confirm_nric_last5` and the format check.

**Why**: Splitting login from account creation is the explicit-intent guarantee the spec asks for (FR-014, FR-015, FR-020, FR-021). Keeping the routing decision on the backend (rather than the frontend "guessing" from a 404) means the leak-prevention rule in FR-021 is enforced server-side.

### D3 — Merge mode is a per-import explicit choice, captured before any write

**Decision**: The preview endpoint returns parsed rows + match status only. A separate commit endpoint takes `merge_mode: "fill_gaps" | "override"` and the preview id. NRIC Last 5 / password is never written on `update`, only on `create`. Each per-user change is recorded in `personnel_import_changes` so SC-007 holds.

**Why**: Matches the user's "ask the user whether they want to override gaps" decision from the spec clarifications. Storing per-field before/after preserves audit value without inflating the schema with full snapshots.

### D4 — Paste-block scope: confirmation field only, autofill disabled, no JS heroics

**Decision**: A small `NoPasteInput` component handles `onPaste`, `onDrop`, `onContextMenu` (preventDefault), and sets `autoComplete="off"`, `autoCorrect="off"`, `autoCapitalize="off"`, `spellCheck={false}`, plus `inputMode="text"`. The original NRIC Last 5 field keeps normal paste behaviour. We do not attempt to defeat clipboard managers, browser extensions, or dev-tools bypass; the goal is friction against accidental paste, not adversarial defence (Assumptions, spec).

**Why**: Matches the user's "confirmation field only" decision. Implementing this once as a reusable component keeps the policy consistent if more confirm-style fields appear later.

## Open Questions For Phase 0 Research

1. Which Anthropic Messages API + Files API path handles SAF callup PDFs at acceptable accuracy? If accuracy is poor, do we switch to a Claude Agent SDK sidecar with `pdf` skill installed, and is the deployment cost justified?
2. What is the maximum document size and page count we should accept in v1? (Default proposed: 10 MB / 50 pages.)
3. How do we want the admin to provide the Anthropic API key — server env only (v1), or per-admin via a settings UI (later)? v1 commits to server env; flag this as a deliberate constraint to re-evaluate after first real use.
4. Are there existing PostgreSQL migrations we should follow for table naming / indexing conventions? Confirm against `backend/migrations/` so the new tables match the project's style.

## Complexity Tracking

No constitution violations. The single new external dependency (Anthropic API) is contained behind one internal package; two new DB tables are narrow and additive; no new runtime or container is introduced.

| Possible complexity | Why it's accepted | Simpler alternative considered |
|---------------------|-------------------|--------------------------------|
| New `services/agent` package | Isolates Anthropic API surface so handlers stay testable and the implementation can swap to a sidecar later without touching callers | Calling `net/http` directly from `handlers/import.go` — rejected because it couples HTTP transport, prompt design, and DB orchestration in one file |
| Two new DB tables for import audit | Required for SC-007 ("identify every changed user and every changed field") and for the admin's post-import summary in User Story 5 | Returning the summary in-memory only — rejected because the admin loses the record after a refresh and we have no way to trace a regression to the import that caused it |
