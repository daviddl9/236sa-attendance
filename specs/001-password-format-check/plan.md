# Implementation Plan: Password Format Check

**Branch**: `001-password-format-check` | **Date**: 2026-05-21 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `specs/001-password-format-check/spec.md`

## Summary

Add a consistent regular-personnel password validation rule across sign-in, registration, admin user update, and bulk personnel creation paths. The required format is exactly four digits followed by one alphabetic letter, matching the app's existing "NRIC Last 5" examples. Backend validation is the source of truth, while frontend validation gives immediate feedback before submission.

## Technical Context

**Language/Version**: Go 1.25.2 backend; TypeScript 5.9.3 with React 19.2 frontend

**Primary Dependencies**: Go chi router, pgx/PostgreSQL, bcrypt, Vite, TanStack Router, TanStack Query, shadcn-style UI components, sonner toasts, xlsx parsing for imports

**Storage**: PostgreSQL `user` records with `nric_last5` and hashed `password` fields; no schema change expected

**Testing**: Go `go test ./...`; frontend `npm run build` and `npm run lint`; targeted handler/unit tests for validation behavior

**Target Platform**: Web application with Go API server and Vite React SPA

**Project Type**: Full-stack web application

**Performance Goals**: Validation completes within the same request or form submission and adds no user-visible latency to sign-in or bulk validation

**Constraints**: Preserve administrator password behavior; prevent invalid regular-personnel password format from creating or updating personnel records; preserve QR and attendance redirects after valid sign-in

**Scale/Scope**: Narrow validation change across existing authentication, registration, user management, and bulk upload surfaces

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

The generated constitution still contains placeholder principles and no enforceable project-specific gates. No constitution violations are identified. Implementation should still follow the repository's existing conventions: keep changes scoped, preserve user changes already in the worktree, validate on the backend as source of truth, and add focused tests around changed behavior.

**Post-Design Recheck**: PASS. The design uses the existing backend/frontend structure, introduces no new storage or infrastructure, and keeps the change limited to validation and user feedback.

## Project Structure

### Documentation (this feature)

```text
specs/001-password-format-check/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   └── password-format-api.md
└── tasks.md
```

### Source Code (repository root)

```text
backend/
├── internal/handlers/
│   ├── auth.go          # sign-in validation and admin exemption
│   ├── user.go          # registration and user update validation
│   ├── admin.go         # bulk upload / bulk create validation
│   └── admin_test.go    # expand validation coverage or add nearby tests
└── cmd/api/main.go      # existing route wiring, no new routes expected

frontend/
├── src/lib/
│   ├── api-client.ts    # align request types with NRIC Last 5
│   └── parse-excel.ts   # keep parsed NRIC Last 5 values compatible with validation
├── src/routes/
│   ├── index.tsx        # root sign-in form validation
│   ├── sign-in.tsx      # sign-in form validation and copy
│   └── attendance/register.tsx # registration field naming and validation
└── src/routes/dashboard/users/
    ├── bulk-upload.tsx  # bulk upload feedback/copy
    └── $userId.tsx      # user edit flow if NRIC Last 5 is editable here
```

**Structure Decision**: Use the existing full-stack layout. Keep the validation rule near the surfaces that already process personnel credentials; extract a small shared helper only where it reduces duplicated checks without creating a broader validation framework.

## Complexity Tracking

No constitution violations or new architectural complexity are planned.
