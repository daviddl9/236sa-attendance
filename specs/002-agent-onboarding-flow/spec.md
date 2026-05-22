# Feature Specification: Agent-Assisted Onboarding And Explicit Signup For New Users

**Feature Branch**: `002-agent-onboarding-flow`

**Created**: 2026-05-22

**Status**: Draft

**Input**: User description: "Improve user onboarding. Login should first check against the existing user database, which an admin can populate by handing source documents to a Claude agent (the admin supplies an Anthropic API key; the agent uses Anthropic document skills). The agent parses documents and imports or overrides the user table, and is smart enough to recognise existing users. For any new users that are not in the database, the signup flow must be explicit: ask them to confirm the last 5 characters of their NRIC because we are creating a new account for them, and disallow copy-paste in the confirmation field."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Administrator Imports Personnel From A Source Document (Priority: P1)

As an administrator, I want to upload a personnel source document (PDF, Word, Excel, or scanned roster) and have a Claude-powered agent extract the personnel rows, so I can populate the user database without manually transcribing each row.

**Why this priority**: This is the foundation of the improved onboarding flow. If the admin cannot ingest the source-of-truth roster, none of the downstream sign-in or signup improvements deliver value. Today admins must hand-shape rosters into Excel before bulk upload; documents arrive in many shapes (callup PDFs, scanned forms, mixed Word tables), and that manual reshaping is the slowest part of onboarding a new intake.

**Independent Test**: An administrator can open the admin dashboard, navigate to an "Import personnel via document" screen, supply an Anthropic API key, upload one document, and see a structured preview of parsed personnel rows ready for confirmation — without any rows yet being written to the database.

**Acceptance Scenarios**:

1. **Given** an administrator is on the document-import screen with a valid Anthropic API key configured, **When** they upload a supported source document containing personnel rows, **Then** the system invokes the agent, parses the document, and presents a row-level preview that shows Full Name, Rank, Battery, NRIC Last 5, and any extra fields detected.
2. **Given** the agent finishes parsing, **When** the administrator views the preview, **Then** each row is labelled as either "New" (no match in user database) or "Existing match" (matched against an existing user by Full Name plus NRIC Last 5).
3. **Given** the document contains rows the agent could not confidently parse, **When** the preview is shown, **Then** unparseable or low-confidence rows are surfaced separately with the original source snippet so the administrator can fix them or skip them before import.
4. **Given** the administrator has not configured an Anthropic API key, **When** they attempt to start a document import, **Then** the system blocks the upload and clearly states that an API key is required before the agent can run.

---

### User Story 2 - Administrator Chooses Merge Behaviour Before Import (Priority: P1)

As an administrator, I want to choose how the import treats existing users before any database writes happen, so I never accidentally overwrite personnel data that has already been completed in the app.

**Why this priority**: The same source document is often re-imported as rosters are updated. Without a deliberate merge choice, admins risk either wiping rank/battery values that personnel filled in themselves, or silently leaving stale records. Making the merge mode an explicit per-import decision protects user-entered data while still allowing intentional refreshes.

**Independent Test**: A tester can run two imports of the same document — one with each merge mode — and confirm that the second import only changes user records in the way the administrator selected, with all changes summarised before they are applied.

**Acceptance Scenarios**:

1. **Given** the preview is shown, **When** the administrator confirms the import, **Then** the system asks them to choose between "Fill gaps only" (only write fields that are currently empty on the existing user) and "Override fields from document" (replace existing field values with the document's values).
2. **Given** the administrator selects "Fill gaps only", **When** the import runs, **Then** existing users have only their empty fields populated and any already-filled values are preserved exactly.
3. **Given** the administrator selects "Override fields from document", **When** the import runs, **Then** existing users have their fields replaced with the document's values, except the NRIC Last 5 / password value which is never overwritten by an import.
4. **Given** the import completes, **When** the administrator sees the result, **Then** the system reports counts for created, updated, skipped, and failed rows, and lists each updated user with the fields that changed.

---

### User Story 3 - Returning User Signs In Against The Imported Database (Priority: P1)

As a returning personnel user who was imported from a source document, I want to sign in with my Full Name and NRIC Last 5 and be recognised immediately, so I do not have to go through a signup flow that creates a duplicate account.

**Why this priority**: The whole point of importing the roster is to make sign-in frictionless for users the admin already knows about. If a known user gets dropped into the signup flow, the import has failed.

**Independent Test**: A tester can import a roster containing a known user, then sign in as that user with matching Full Name and NRIC Last 5 and confirm they enter the app directly without any "create account" prompts.

**Acceptance Scenarios**:

1. **Given** a user record exists in the database with matching Full Name and NRIC Last 5, **When** the user submits the sign-in form with those values, **Then** the system authenticates them against the existing record without any signup prompt.
2. **Given** an imported user has been matched and signed in successfully, **When** they view their profile, **Then** any rank, battery, or extra fields that the import populated are already present.

---

### User Story 4 - New User Goes Through Explicit Signup With NRIC Confirmation (Priority: P1)

As a personnel user who is not in the imported database, I want the sign-in page to clearly signal that a new account is being created for me and to make me confirm my NRIC Last 5 before the account is created, so I understand what is happening and an account is not silently created with a typo.

**Why this priority**: Today the system silently auto-creates an account on first sign-in for any unknown name. That hides intent, makes typos permanent (because the typo becomes the canonical account), and creates duplicate ghost accounts when the same person mistypes their name. An explicit signup path with confirmation is the safeguard that turns "log in" and "sign up" into separate, intentional actions.

**Independent Test**: A tester can submit the sign-in form with a name not present in the database and confirm that the system stops the automatic account creation, switches to an explicit "Create your account" path on the same page, requires confirmation of the NRIC Last 5, and only creates the account after the confirmation matches.

**Acceptance Scenarios**:

1. **Given** a user submits the sign-in form, **When** their Full Name is not found in the database, **Then** the system stops the silent auto-create behaviour and instead presents an explicit "Create your account" path on the sign-in page that names the user and warns that a new account is about to be created.
2. **Given** the user is on the explicit signup path, **When** the form is shown, **Then** it includes a "Confirm NRIC Last 5" field separate from the original NRIC Last 5 field and clearly explains why confirmation is required.
3. **Given** the user attempts to paste into the "Confirm NRIC Last 5" field, **When** they trigger a paste action (keyboard, context menu, or drag-and-drop), **Then** the paste is rejected and the field communicates that the value must be typed manually.
4. **Given** the user types a value into the "Confirm NRIC Last 5" field that does not match the original entry, **When** they submit the form, **Then** the system rejects the submission, highlights the mismatch, and does not create an account.
5. **Given** the user types matching values into both NRIC fields and they meet the existing four-digits-plus-letter format from feature 001, **When** they submit the form, **Then** the system creates the new user record and signs them in.
6. **Given** an administrator user signs in with administrator credentials, **When** the system processes the request, **Then** the explicit signup flow is not triggered and administrator sign-in continues to work as today.

---

### User Story 5 - Administrator Reviews What The Agent Changed (Priority: P2)

As an administrator, I want to see what the agent-driven import added or changed after it runs, so I can spot mistakes early and trust the agent over time.

**Why this priority**: Confidence in the agent is built by visibility. Without a clear post-import summary, admins will hesitate to use "Override" mode and the feature loses its main benefit.

**Independent Test**: A tester can run an import that creates some users, updates some users, and skips some rows, then open a post-import summary and confirm every change is listed with enough detail to identify the user and the affected field.

**Acceptance Scenarios**:

1. **Given** an import has completed, **When** the administrator opens the summary, **Then** they see counts of created, updated, skipped, and failed rows, plus per-row detail.
2. **Given** a row was updated in "Override" mode, **When** the administrator inspects that row, **Then** the summary shows the previous value and the new value for each changed field.
3. **Given** a row was skipped, **When** the administrator inspects that row, **Then** the summary shows the reason (for example, "no NRIC Last 5 detected" or "duplicate within the document").

---

### Edge Cases

- The agent returns rows whose NRIC Last 5 does not match the four-digits-plus-letter format from feature 001; those rows must be marked invalid in the preview and must not be imported even if the administrator confirms.
- The agent returns two rows in the same document that match the same existing user; the preview must collapse or flag them so the import does not attempt two conflicting updates to one record.
- The Anthropic API call fails, times out, or returns an error; the import must report the failure clearly without writing partial data to the user table.
- The administrator's Anthropic API key is rejected; the system must explain that the key is invalid and must not retry silently or log the key value.
- The uploaded document contains no recognisable personnel rows; the preview must say so plainly rather than silently importing zero rows.
- A new user tries to sign up with a Full Name that matches an existing record but a different NRIC Last 5; the system must treat this as an authentication failure, not as a new account, and must not leak whether the name exists.
- A new user successfully signs up, then later the administrator imports a roster that contains them; the import must match them and apply the chosen merge policy rather than creating a duplicate.
- A user attempts to bypass the paste block on the confirmation field via browser autofill; the field must additionally disable autofill so the most common bypass paths do not silently re-enable paste-like behaviour.
- Administrator credentials must continue to work; the explicit signup flow must never be triggered by an administrator sign-in.

## Requirements *(mandatory)*

### Functional Requirements

#### Agent-driven document import

- **FR-001**: The system MUST provide an administrator-only screen for importing personnel from a source document via a Claude-based agent.
- **FR-002**: The system MUST accept an Anthropic API key from the administrator, store it as a server-side secret, and never expose it to the browser or include it in logs.
- **FR-003**: The system MUST invoke a Claude-based agent that uses Anthropic document skills (PDF, DOCX, XLSX, and equivalent) to parse the uploaded document into structured personnel rows.
- **FR-004**: The system MUST present a preview of the parsed rows before any database write, including Full Name, Rank, Battery, NRIC Last 5, and any extra fields detected by the agent.
- **FR-005**: The system MUST label every previewed row as either matching an existing user (by Full Name plus NRIC Last 5) or as a new user.
- **FR-006**: The system MUST surface rows the agent could not confidently parse with the source snippet so the administrator can correct or skip them.
- **FR-007**: The system MUST require the administrator to explicitly choose, per import, between "Fill gaps only" and "Override fields from document" before any database write happens.
- **FR-008**: The system MUST never overwrite an existing user's NRIC Last 5 / password value during an import, regardless of the chosen merge mode.
- **FR-009**: The system MUST apply the four-digits-plus-letter NRIC Last 5 format check from feature 001 to every parsed row and MUST reject rows that do not match.
- **FR-010**: The system MUST report the result of the import with counts for created, updated, skipped, and failed rows, plus a per-row record of what changed for updated rows.
- **FR-011**: The system MUST fail the import as a single unit when the Anthropic API errors, times out, or returns an invalid response, and MUST NOT write partial data in that case.

#### Sign-in against the imported database

- **FR-012**: The system MUST check sign-in attempts against the existing user database (by Full Name plus NRIC Last 5) before deciding whether to authenticate the user or route them to signup.
- **FR-013**: The system MUST authenticate any user whose Full Name and NRIC Last 5 match an existing record, without triggering the explicit signup flow.
- **FR-014**: The system MUST stop silently auto-creating user accounts for unknown Full Names on sign-in.

#### Explicit signup for new users

- **FR-015**: The system MUST present an explicit "Create your account" path on the sign-in page when a submitted Full Name has no matching user, naming the user and stating that a new account is about to be created.
- **FR-016**: The system MUST require a separate "Confirm NRIC Last 5" field on the explicit signup path that the user fills in addition to the original NRIC Last 5 field.
- **FR-017**: The system MUST block paste actions on the "Confirm NRIC Last 5" field, including keyboard shortcut, context menu, and drag-and-drop, and MUST also disable browser autofill on that field.
- **FR-018**: The system MUST allow paste on the original NRIC Last 5 field; the paste block applies only to the confirmation field.
- **FR-019**: The system MUST reject the signup submission when the original and confirmation NRIC Last 5 values do not match, with feedback that names the mismatch.
- **FR-020**: The system MUST only create the new user account after both values match and the format check from feature 001 passes.
- **FR-021**: The system MUST treat a sign-in attempt where the Full Name matches an existing user but the NRIC Last 5 does not as an authentication failure, and MUST NOT offer the explicit signup path in that case or reveal whether the name exists.
- **FR-022**: The system MUST preserve administrator sign-in behaviour and MUST NOT trigger the explicit signup flow for administrator credentials.

### Key Entities

- **Personnel Document Import**: A single administrator-initiated job that ingests one source document via the Claude agent, holds a parsed-row preview, captures the chosen merge mode, and records the resulting import summary.
- **Parsed Personnel Row**: A structured row produced by the agent (Full Name, Rank, Battery, NRIC Last 5, extras) with a match status (new vs existing) and a validity flag against the feature 001 NRIC Last 5 format.
- **User Record**: The existing personnel record in the user table; the import reads it for matching, may write to its non-password fields under the chosen merge mode, and is what sign-in authenticates against.
- **Explicit Signup Submission**: A new-user signup request from the sign-in page carrying Full Name, original NRIC Last 5, and confirmation NRIC Last 5; the system creates a user record only when both NRIC values match and the format is valid.
- **Anthropic API Credential**: A server-side secret supplied by the administrator that authorises the agent to call the Anthropic API; never exposed to the browser or logged.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An administrator can import a typical roster document end-to-end (upload, preview, choose merge mode, confirm, see summary) in under five minutes for a roster of up to 200 rows.
- **SC-002**: 100% of imported users with matching Full Name and NRIC Last 5 sign in without entering the explicit signup flow.
- **SC-003**: 0% of sign-in attempts with an unknown Full Name result in a silently auto-created account; every such attempt either enters the explicit signup flow or is rejected.
- **SC-004**: 100% of explicit signup attempts where the original and confirmation NRIC Last 5 values differ are rejected before any user record is created.
- **SC-005**: Paste actions on the "Confirm NRIC Last 5" field are blocked in 100% of attempts via keyboard shortcut, context menu, and drag-and-drop on the supported browsers, and browser autofill is disabled on that field.
- **SC-006**: 100% of failed imports (Anthropic API error, invalid key, unparseable document) leave the user table unchanged.
- **SC-007**: For every successful import, the administrator can identify every changed user and every changed field from the post-import summary without consulting the database directly.

## Assumptions

- The Claude-based agent is invoked from the backend (not from the browser), so the Anthropic API key stays server-side and the agent can use Anthropic document skills (PDF, DOCX, XLSX) hosted alongside the backend.
- The administrator's Anthropic API key is configured once per environment (or per administrator) via existing admin settings; provisioning the key UI is in scope but rotation policy is not.
- "Existing user" matching uses Full Name plus NRIC Last 5 together, consistent with today's unique constraint on the user table.
- Existing administrator sign-in (separate admin account) is untouched by this feature.
- The four-digits-plus-letter NRIC Last 5 format check from feature 001 (`001-password-format-check`) is the authoritative rule for valid passwords throughout this feature.
- Existing personnel records that were silently auto-created before this feature shipped remain valid; this feature does not retroactively force them through the explicit signup flow.
- The current bulk Excel upload (`/dashboard/users/bulk-upload`) remains available as an alternative path; the agent-driven importer is additive, not a replacement, and may share the underlying write path so merge-mode behaviour is consistent.
- Disabling paste is best-effort on the web: it covers keyboard shortcut, context menu, drag-and-drop, and browser autofill, but a sufficiently determined user with dev tools can still bypass it; the goal is friction against accidental copy-paste, not adversarial defence.
