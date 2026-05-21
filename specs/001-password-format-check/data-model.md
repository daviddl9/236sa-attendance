# Data Model: Password Format Check

## Regular Personnel Credential

**Purpose**: The sign-in secret used by non-admin personnel. It corresponds to the user's NRIC Last 5 value.

**Fields**:
- `identifier`: Full name used with the password during regular personnel sign-in.
- `password`: User-entered credential value.
- `normalizedPassword`: Presentation/storage-normalized version when applicable, with final letter uppercase.

**Validation Rules**:
- Must be exactly five characters.
- Characters 1-4 must be numeric digits.
- Character 5 must be an alphabetic letter.
- Final letter is accepted case-insensitively.
- Leading and trailing spaces are not part of the valid value.
- Administrator credentials are excluded from this entity.

## Personnel Record

**Purpose**: A user profile created, imported, or updated through registration or administrator flows.

**Fields**:
- `fullName`: Personnel full name as in NRIC.
- `rank`: Personnel rank.
- `battery`: Personnel battery.
- `nricLast5`: Five-character credential value that must match the regular personnel credential format.
- `dob`: Date of birth where the existing registration/profile flow still uses it.
- `extras`: Optional imported metadata from upload files.

**Validation Rules**:
- `nricLast5` follows the same four-digits-plus-letter rule as regular personnel credentials.
- Bulk records report row-specific errors for invalid `nricLast5`.
- Updates that do not include `nricLast5` should not revalidate or modify the existing value.

## Administrator Credential

**Purpose**: Privileged sign-in credential for administrator access.

**Fields**:
- `identifier`: Administrator username.
- `password`: Administrator password.

**Validation Rules**:
- Not constrained by the regular personnel NRIC Last 5 format.
- Existing administrator authentication behavior remains unchanged.

## State Transitions

```text
Entered value
  -> Format invalid
      -> Reject submission, show expected format
  -> Format valid
      -> Continue existing authentication, creation, import, or update flow
```

No new persistent entity or database migration is expected.
