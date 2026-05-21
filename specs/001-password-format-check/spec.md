# Feature Specification: Password Format Check

**Feature Branch**: `001-password-format-check`

**Created**: 2026-05-21

**Status**: Draft

**Input**: User description: "Add a check that the password is 4 digits plus a character at the end"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Personnel Sign In With Valid NRIC Last 5 (Priority: P1)

As a regular personnel user, I want the password field to accept the expected NRIC Last 5 format so I can sign in without confusion when I enter four digits followed by the final NRIC letter.

**Why this priority**: Signing in is the primary path for attendance and account access, so valid users must not be blocked by the new check.

**Independent Test**: A tester can enter a regular personnel identifier with a password such as `1234A` and confirm the sign-in attempt proceeds through normal authentication.

**Acceptance Scenarios**:

1. **Given** a regular personnel user is on a sign-in screen, **When** they enter a password made of four digits followed by one letter, **Then** the password format check passes and sign-in continues.
2. **Given** a regular personnel user enters the final letter in lowercase, **When** they submit the sign-in form, **Then** the format check treats the password as valid.

---

### User Story 2 - Personnel Get Clear Feedback For Invalid Password Format (Priority: P1)

As a regular personnel user, I want to be told when my password format is wrong so I can correct it before repeatedly trying invalid credentials.

**Why this priority**: The feature exists to prevent incorrect password shapes; the user must understand exactly what to fix.

**Independent Test**: A tester can try invalid passwords such as `12345`, `123A4`, `1234@`, `1234AB`, and `123A`, then confirm each is rejected with a clear format message.

**Acceptance Scenarios**:

1. **Given** a regular personnel user enters fewer or more than five characters, **When** they submit the sign-in form, **Then** the system rejects the password format and explains the expected format.
2. **Given** a regular personnel user enters a password whose first four characters are not all digits, **When** they submit the sign-in form, **Then** the system rejects the password format.
3. **Given** a regular personnel user enters a password whose final character is not a letter, **When** they submit the sign-in form, **Then** the system rejects the password format.

---

### User Story 3 - Administrators Add Personnel With Valid NRIC Last 5 Values (Priority: P2)

As an administrator, I want personnel creation and upload flows to enforce the same NRIC Last 5 password format so imported or manually added users can sign in successfully later.

**Why this priority**: Preventing bad personnel records at creation avoids downstream sign-in failures and support work.

**Independent Test**: A tester can attempt to create or upload personnel rows with valid and invalid NRIC Last 5 values and confirm invalid rows are reported while valid rows remain eligible for creation.

**Acceptance Scenarios**:

1. **Given** an administrator uploads personnel records, **When** a row has an NRIC Last 5 value that is not four digits followed by one letter, **Then** the row is rejected with a row-specific error.
2. **Given** an administrator creates or updates a personnel record, **When** the NRIC Last 5 value matches four digits followed by one letter, **Then** the record can pass the format check.

---

### Edge Cases

- Passwords with leading or trailing spaces should not be accepted as a different value; the user should be guided to enter only the five-character NRIC Last 5.
- The final letter should be accepted in either uppercase or lowercase for user convenience.
- Existing administrator sign-in credentials must not be constrained by the regular personnel NRIC Last 5 rule.
- Invalid personnel password format should not create a new incomplete personnel account.
- Bulk operations should report every invalid row found in a submission instead of stopping at the first invalid row.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The system MUST define a valid regular personnel password as exactly five characters: four numeric digits followed by one alphabetic letter.
- **FR-002**: The system MUST apply this format check wherever a regular personnel password or NRIC Last 5 value is entered, created, imported, or updated.
- **FR-003**: The system MUST reject regular personnel sign-in attempts when the password does not match the required format.
- **FR-004**: The system MUST show a clear user-facing message for invalid personnel password format that states the expected shape and includes an example such as `1234A`.
- **FR-005**: The system MUST allow regular personnel sign-in attempts to continue when the password matches the required format, subject to the existing credential checks.
- **FR-006**: The system MUST reject personnel creation, upload, or update records whose NRIC Last 5 value does not match the required format.
- **FR-007**: The system MUST identify invalid personnel records clearly enough for an administrator to find and correct them, including row-level feedback for bulk submissions.
- **FR-008**: The system MUST preserve existing administrator sign-in behavior and MUST NOT require administrator passwords to match the regular personnel NRIC Last 5 format.
- **FR-009**: The system MUST preserve the current attendance and QR-code sign-in flows after a regular personnel password passes the format check.
- **FR-010**: The system MUST accept the final alphabetic character case-insensitively while presenting examples in uppercase.

### Key Entities

- **Regular Personnel Credential**: The password value used by non-admin personnel to sign in; it corresponds to the user's NRIC Last 5 and must follow the four-digits-plus-letter format.
- **Personnel Record**: A user profile created, imported, or updated by an administrator; its NRIC Last 5 value becomes the regular personnel password and must pass the same format check.
- **Administrator Credential**: A privileged sign-in credential that remains outside the regular personnel NRIC Last 5 format rule.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 100% of regular personnel password entry points reject values that are not exactly four digits followed by one letter.
- **SC-002**: 100% of personnel creation, upload, and update paths reject invalid NRIC Last 5 values before the personnel record is accepted.
- **SC-003**: Valid regular personnel passwords such as `1234A` and `1234a` continue through sign-in without any additional required steps.
- **SC-004**: Invalid format feedback is visible to the user or administrator within one submission attempt and names the expected format.
- **SC-005**: Bulk personnel submissions identify all rows with invalid NRIC Last 5 values in the response for that submission.

## Assumptions

- "A character at the end" refers to the NRIC final alphabetic letter, consistent with the app's existing "NRIC Last 5" examples.
- The rule applies to regular personnel credentials only; administrator credentials are intentionally excluded.
- Existing personnel with passwords that already match four digits followed by one letter should not need to reset or change credentials.
- This specification covers validation and user feedback, not a broader password-strength or credential-reset policy.
