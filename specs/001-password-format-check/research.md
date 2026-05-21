# Research: Password Format Check

## Decision: Treat the personnel password as the NRIC Last 5 format

**Decision**: A valid regular-personnel password is exactly four numeric digits followed by one alphabetic letter, such as `1234A`.

**Rationale**: The current product copy and bulk-upload examples already describe "NRIC Last 5" values such as `4567A`, `1234A`, and `5678B`. The requested rule maps directly to that existing domain language.

**Alternatives considered**:
- Any final character: rejected because the app examples and NRIC convention imply a final letter, not punctuation or another digit.
- Length-only validation: rejected because it would allow values like `12345`, which fails the requested structure.

## Decision: Backend validation is the source of truth

**Decision**: All backend paths that accept a regular-personnel password or NRIC Last 5 value must enforce the format, even when frontend validation exists.

**Rationale**: Frontend checks improve feedback but cannot protect API access, imports, or future clients. Backend validation also prevents invalid personnel records from being stored.

**Alternatives considered**:
- Frontend-only validation: rejected because invalid records could still be created through API calls or backend upload paths.
- Database constraint: deferred because no schema change is required for this narrow validation feature and existing invalid production data, if any, would need migration handling.

## Decision: Keep administrator credentials exempt

**Decision**: The rule applies only to regular personnel credentials and NRIC Last 5 values. Administrator sign-in must keep its current password behavior.

**Rationale**: The spec requires preserving administrator sign-in behavior. Admin credentials are not NRIC Last 5 values and may have a different length or structure.

**Alternatives considered**:
- Apply the rule to every password: rejected because it would risk locking out administrator access.

## Decision: Include registration and update flows in scope

**Decision**: Planning includes sign-in, public registration, admin user update, backend bulk upload, frontend-parsed bulk creation, and user-facing copy where these surfaces mention NRIC Last 4/5.

**Rationale**: The feature requirement says the rule should apply wherever a regular personnel password or NRIC Last 5 value is entered, created, imported, or updated. The current codebase has multiple entry points, including older "NRIC Last 4 + DOB" registration copy that conflicts with the target format.

**Alternatives considered**:
- Sign-in only: rejected because invalid records could still be created and later fail sign-in.
- Admin bulk only: rejected because direct sign-in and registration would remain inconsistent.

## Decision: Use focused tests around changed behavior

**Decision**: Add backend tests for valid and invalid format handling, especially bulk validation and API rejection behavior. Run frontend build/lint to catch type and UI regressions.

**Rationale**: Backend validation is the durable behavioral contract. The frontend currently has no dedicated test runner configured, so build and lint are the practical checks for UI/type changes unless a test runner is added separately.

**Alternatives considered**:
- Add a new frontend test stack: rejected for this small validation feature because it would expand scope beyond the requested change.
