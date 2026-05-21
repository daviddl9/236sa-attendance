# Quickstart: Password Format Check

## 1. Review Planned Scope

Read:
- `specs/001-password-format-check/spec.md`
- `specs/001-password-format-check/plan.md`
- `specs/001-password-format-check/contracts/password-format-api.md`

## 2. Implement Validation

Expected touch points:
- Backend sign-in validation in `backend/internal/handlers/auth.go`
- Backend registration and update validation in `backend/internal/handlers/user.go`
- Backend bulk validation in `backend/internal/handlers/admin.go`
- Frontend sign-in forms in `frontend/src/routes/index.tsx` and `frontend/src/routes/sign-in.tsx`
- Frontend registration flow in `frontend/src/routes/attendance/register.tsx`
- Frontend API types in `frontend/src/lib/api-client.ts`
- Bulk-upload guidance or pre-submit handling in `frontend/src/routes/dashboard/users/bulk-upload.tsx`

## 3. Validate Backend Behavior

From the repository root:

```bash
go test ./...
```

Manual/API checks:
- Non-admin sign-in rejects `12345`, `123A4`, `1234@`, `1234AB`, and `123A`.
- Non-admin sign-in accepts the format of `1234A` and `1234a`, then continues normal credential verification.
- Admin sign-in still allows the existing administrator password.
- Bulk create reports row-level errors for invalid NRIC Last 5 values.

## 4. Validate Frontend Behavior

From `frontend/`:

```bash
npm run lint
npm run build
```

Manual UI checks:
- `/sign-in` and `/` show a clear error before submission for invalid regular-personnel password format.
- Registration asks for NRIC Last 5 and enforces four digits followed by one letter.
- Bulk-upload copy and examples continue to use values like `1234A`.

## 5. Regression Checks

- QR sign-in redirect still works after valid personnel sign-in.
- Attendance marking paths are unchanged after authentication succeeds.
- Existing personnel records with valid NRIC Last 5 values continue to sign in.
