# Feature Specification: Superadmin Adds A Single New User From The Users Dashboard

**Feature Branch**: `003-add-new-user`

**Created**: 2026-05-26

**Status**: Draft

**Input**: User description: "Add a way for a superadmin to add a new user one at a time. Today the only paths into the user table are the public self-registration flow (which lands in pending approval and needs admin review) and the bulk Excel / agent document import paths. The dashboard has no 'create user' button for a single person, which is awkward when an admin just needs to add one new joinee."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Superadmin Creates A Single User From The Users Page (Priority: P1) 🎯 MVP

As a superadmin, I want an "Add User" action on the users dashboard that opens a short form for one person and creates the user immediately on submit, so that adding a single new joinee does not force me to prepare a one-row Excel file or wait for them to self-register and then approve them.

**Why this priority**: This is the core of the feature. Without it, the only way an admin can add a single user is to (a) build a one-row Excel file and use the bulk-upload path, or (b) wait for the user to self-register through `/users/register` and then approve them from the registrations queue. Both are heavy for the everyday "add one joinee" case, and (b) blocks the user behind admin response time. A direct create action collapses that into one step and matches the way admins describe the task verbally ("add Bob to the system").

**Independent Test**: A superadmin opens `/dashboard/users`, clicks "Add User", fills full name + rank + battery + NRIC last 5, submits, and the new user appears in the list on the next render with `verified = true` (i.e. they can sign in without further approval).

**Acceptance Scenarios**:

1. **Given** a signed-in superadmin is on the users list at `/dashboard/users`, **When** they activate the "Add User" action, **Then** the system presents a form with Full Name, Rank, Battery, NRIC Last 5, and (optional) Date of Birth fields, with rank limited to the project's valid ranks and battery limited to HQ / Alpha / Bravo.
2. **Given** the form is open with valid values in every required field, **When** the superadmin submits, **Then** the system creates a new user record with `verified = true`, hashes the NRIC last 5 as the password, returns success, refreshes the users list, and shows a success toast naming the new user.
3. **Given** the new user has been created, **When** that user signs in for the first time with Full Name and NRIC Last 5, **Then** they authenticate normally without entering the explicit signup flow and without `pending_approval`.
4. **Given** the form is open, **When** the superadmin cancels or closes it without submitting, **Then** no user record is created and no toast or error is shown.

---

### User Story 2 - NRIC Last 5 Format Is Enforced At Create Time (Priority: P1)

As a superadmin, I want the same four-digits-plus-letter NRIC last 5 rule from feature 001 to apply to the create form, so that the password I set for the new user matches every other entry point and the user can sign in without surprises.

**Why this priority**: NRIC last 5 doubles as the user's password everywhere else in the system (registration, bulk upload, document import, edit-user). If the admin create form drifts from that rule, admins will create users who cannot sign in, which is the worst possible failure mode for this feature.

**Independent Test**: A tester opens the form, types five characters that do not match `^\d{4}[A-Za-z]$` into NRIC Last 5, submits, and confirms the form blocks submission with the same message used elsewhere; then types `1234A` and confirms the create succeeds and the resulting user can sign in with `1234A`.

**Acceptance Scenarios**:

1. **Given** the form is open, **When** the superadmin types a value into NRIC Last 5 that is not exactly four digits followed by an alphabet letter, **Then** the form rejects the submission with the same error message used in bulk upload and the edit-user page, and does not call the backend.
2. **Given** the form is open with a valid NRIC last 5 in lowercase (e.g. `1234a`), **When** the superadmin submits, **Then** the system normalises the value to uppercase before saving so sign-in works regardless of input case.
3. **Given** any submission, **When** the backend receives the request, **Then** it re-validates the NRIC last 5 against the same rule and rejects the request if it does not match, even if the client somehow let it through.

---

### User Story 3 - CPT And Above Are Automatically Superadmin (Priority: P1)

As a superadmin, I want a user I create with rank CPT or above to be automatically marked as `is_superadmin = true`, so that the create form follows the same rank → superadmin rule already enforced by registration, bulk upload, and the edit-user handler.

**Why this priority**: Feature 003 (user tiers) made CPT+ → superadmin a system-wide invariant. If the create form is the one place that does not apply it, admins will silently end up with CPT users who cannot perform superadmin actions and will not know why. Catching this in the create path is cheaper than diagnosing it later.

**Independent Test**: A tester creates a user with rank CPT via the form, then signs in as that user and confirms the session response reports `tier = 4` and `isSuperadmin = true`, without any extra step.

**Acceptance Scenarios**:

1. **Given** the create form is being submitted, **When** the selected rank's order is at or above CPT, **Then** the backend sets `is_superadmin = true` on the new record without prompting the admin to confirm.
2. **Given** the create form is being submitted, **When** the selected rank's order is below CPT, **Then** the backend leaves `is_superadmin = false` and the user's effective tier is derived from rank as defined in feature 003.

---

### User Story 4 - Duplicate Users Are Detected Before Creation (Priority: P1)

As a superadmin, I want the create form to refuse to make a duplicate when a user with the same Full Name and NRIC Last 5 already exists, so I don't end up with two records that both try to authenticate the same person.

**Why this priority**: Sign-in matches on Full Name plus NRIC Last 5. Two rows with the same pair would create ambiguity in authentication and reporting, and there is no database-level unique constraint on the pair today, so the application layer must enforce it. The existing public registration handler already enforces this; the new admin path must match.

**Independent Test**: A tester creates a user `John Doe / 1234A`, then opens the form again, fills in `John Doe / 1234A`, submits, and confirms the system returns a clear conflict error and creates no second row.

**Acceptance Scenarios**:

1. **Given** a user record already exists with the same Full Name and NRIC Last 5 (case-insensitive on the name, normalised on the NRIC), **When** the superadmin submits the create form, **Then** the backend returns a conflict response and the form surfaces a message naming the existing user and offering to navigate to that user's detail page.
2. **Given** a pending unverified user record already exists with the same Full Name and NRIC Last 5 (from a self-registration awaiting approval), **When** the superadmin submits the create form, **Then** the backend returns the same conflict response and the form's message points the admin to the registrations approval page instead of creating a second record.
3. **Given** the conflict is shown, **When** the admin clicks the link to the existing user, **Then** they navigate to that user's detail page (or the registrations page in the pending case) without losing the form values.

---

### User Story 5 - Only Superadmins See And Use The Create Action (Priority: P2)

As a Tier 2 or Tier 3 user, I should not see or be able to use the "Add User" action, so that user creation remains a superadmin-only capability consistent with delete, bulk upload, and import.

**Why this priority**: Feature 003 made user write operations superadmin-only. Hiding the button avoids confusing Tier 2/3 commanders, and rejecting the call at the API protects against a non-superadmin discovering the endpoint directly. The UI gate alone is not enough, so both layers must enforce it.

**Independent Test**: A tester signs in as a Tier 3 unit commander, opens `/dashboard/users`, confirms the "Add User" button is not visible, then calls the create endpoint directly with the same session cookie and confirms a 403 response.

**Acceptance Scenarios**:

1. **Given** a Tier 1, 2, or 3 user signs in, **When** they open `/dashboard/users`, **Then** the "Add User" action is not rendered.
2. **Given** any non-superadmin makes a direct request to the new create endpoint, **When** the server processes it, **Then** the response is 403 Forbidden and no row is written.

---

### User Story 6 - Optional Extras Can Be Captured Without Forcing Them (Priority: P3)

As a superadmin, I want to optionally fill in Date of Birth and a small number of free-text "extras" key-value pairs when creating a user, so that a one-off creation can capture the same shape of data as bulk upload without forcing the admin to fill them out when they don't have the information yet.

**Why this priority**: DOB and extras already exist on the user record and are populated by other paths (registration, bulk upload, document import). Supporting them here keeps the new path consistent with the rest of the system, but the everyday "add a joinee" case rarely has those values to hand, so they must remain optional and must not block submission.

**Independent Test**: A tester creates a user with only the required fields and confirms success; then creates a second user with DOB and one extras row and confirms both values round-trip on the user detail page.

**Acceptance Scenarios**:

1. **Given** the form is open with only the required fields filled, **When** the superadmin submits, **Then** the user is created with `dob = null` and `extras = {}` and the form does not warn about empty optional fields.
2. **Given** the form is open and the superadmin enters a DOB in DDMMYY format, **When** they submit, **Then** the system stores the value as-is on the user record; if the DOB field is filled with anything other than six characters, the form rejects the submission with a clear inline message.
3. **Given** the form is open and the superadmin adds one or more "extras" key-value rows (e.g. `Section: Recon`), **When** they submit, **Then** the system saves the extras into the user's `extras` JSONB map and they appear on the user detail page.

---

### Edge Cases

- The Full Name field contains leading/trailing whitespace or doubled internal whitespace — the system MUST trim leading/trailing whitespace and collapse internal runs to a single space before persisting, so duplicate detection cannot be bypassed by spacing.
- The NRIC last 5 is entered in lowercase or mixed case — the system MUST normalise to uppercase before persisting and before the duplicate check.
- The admin submits the form twice in quick succession (double-click on the submit button) — the form MUST disable the submit button while the request is in flight so only one user is created.
- The admin fills the form, then their session expires before they submit — the system MUST return an authentication error and the form MUST surface a "please sign in again" message instead of silently failing.
- The admin selects a rank that the project does not recognise (e.g. via dev tools manipulating the select) — the backend MUST reject the request with an "Invalid rank" error matching the message already used by the bulk-upload and edit-user paths.
- A user record with the same Full Name + NRIC Last 5 + DOB triple already exists and is unverified (still in the registrations queue) — the create MUST be refused and the admin MUST be pointed to the registrations approval page rather than creating a parallel row.
- The admin attempts to set tier override or `is_superadmin = true` via dev tools on a sub-CPT rank — the backend MUST ignore those fields in the create request (the create form does not expose them) and rely on the rank-derived rule; tier override may be set later via the existing user detail page.
- The admin creates a CPT+ user, then later edits that user's rank down to LTA — out of scope for this feature; the existing edit-user handler already clears `is_superadmin` in that case.
- The backend errors after the user has been inserted but before a response is returned (e.g. connection drop) — the form MUST treat the request as failed; the admin retrying with the same values MUST be caught by the duplicate-detection rule rather than producing two rows.

## Requirements *(mandatory)*

### Functional Requirements

#### Entry point and access control

- **FR-001**: The system MUST expose an "Add User" action on the users dashboard at `/dashboard/users` that is visible only to users whose effective tier is Superadmin.
- **FR-002**: The system MUST provide a single-user create endpoint that is gated by the same `RequireSuperadmin` middleware used by `DELETE /api/users/{id}` and `POST /api/admin/users/bulk-create`, so non-superadmin callers receive a 403 even if they call the endpoint directly.

#### Form fields and validation

- **FR-003**: The create form MUST require Full Name, Rank, Battery, and NRIC Last 5; Date of Birth and "extras" key-value rows MUST be optional.
- **FR-004**: Rank input MUST be constrained to the project's `ValidRanks` list, matching the bulk-upload and edit-user surfaces.
- **FR-005**: Battery input MUST be constrained to HQ, Alpha, or Bravo, matching the bulk-upload and edit-user surfaces.
- **FR-006**: NRIC Last 5 input MUST be validated against the four-digits-plus-letter rule from feature 001 on both the client and the server, with the same user-facing message used by bulk upload and the edit-user page.
- **FR-007**: The system MUST normalise the NRIC Last 5 value to uppercase before duplicate-detection and before persistence.
- **FR-008**: The system MUST trim leading/trailing whitespace on Full Name, Rank, Battery, and NRIC Last 5 before persistence, and MUST collapse internal whitespace runs in Full Name to a single space.
- **FR-009**: If the Date of Birth field is filled, the system MUST require exactly six characters in DDMMYY format, matching the existing rule in registration and edit-user; if empty, the field MUST be persisted as null.

#### Persistence and side effects

- **FR-010**: On successful submission, the system MUST insert a new row into the `user` table with `verified = true`, the NRIC last 5 stored both as the `nric_last5` column and (bcrypt-hashed) as the `password` column.
- **FR-011**: The system MUST set `is_superadmin = true` automatically when the chosen rank's order is at or above CPT, matching the rule applied by registration, bulk upload, and edit-user.
- **FR-012**: The create endpoint MUST NOT accept or honour `tierOverride` or `is_superadmin` in the request body; tier overrides remain settable only via the existing `PUT /api/users/{id}` flow.
- **FR-013**: On success, the system MUST return the created user record (matching the shape used by the existing user list / detail endpoints) and the dashboard MUST invalidate the cached users query so the new row appears on the next render without a manual refresh.

#### Duplicate detection

- **FR-014**: Before insert, the system MUST check whether a row already exists with the same Full Name (case-insensitive) and same normalised NRIC Last 5; if one exists, the create MUST be refused with a 409 Conflict response.
- **FR-015**: The conflict response MUST identify the existing user's id and `verified` status so the form can route the admin to the user detail page (verified) or the registrations approval page (unverified).
- **FR-016**: The application MUST NOT rely on a database-level unique constraint for this check, because none exists on Full Name + NRIC Last 5 today; the application layer is authoritative.

#### Behaviour and UX

- **FR-017**: The submit action MUST be disabled while a request is in flight so a double-click cannot create two records.
- **FR-018**: Cancelling or closing the form before submission MUST NOT create any record and MUST NOT show a success toast.
- **FR-019**: On a successful create, the form MUST close, the users list MUST refresh, and a success toast naming the new user MUST be shown.
- **FR-020**: On any non-success backend response, the form MUST remain open with the values the admin entered intact, and MUST surface the backend's error message inline (not only in a toast) so the admin can correct and retry.

### Key Entities

- **User Record**: The existing `user` table row that holds Full Name, Rank, Battery, NRIC Last 5 (and hashed password), optional DOB, optional `extras`, `tier_override`, `verified`, and `is_superadmin`. The create endpoint writes a new row with `verified = true` and `is_superadmin` set per the rank rule.
- **Create User Request**: The payload submitted from the form, carrying Full Name, Rank, Battery, NRIC Last 5, and optionally DOB and an `extras` map; it does not carry `tier_override` or `is_superadmin`.
- **Duplicate Conflict**: The 409 response shape returned when an existing user matches on (case-insensitive Full Name, normalised NRIC Last 5); it carries enough information for the form to route the admin to either the user detail page or the registrations approval page.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A superadmin can add one new user end-to-end (open form, fill required fields, submit, see them in the list) in under 30 seconds without leaving the dashboard.
- **SC-002**: 100% of users created via this form can immediately sign in with the entered Full Name and NRIC Last 5, without entering the explicit signup flow and without `pending_approval`.
- **SC-003**: 0% of submissions with an NRIC Last 5 that does not match the four-digits-plus-letter rule result in a created user record.
- **SC-004**: 100% of CPT+ users created via this form report `tier = 4` and `isSuperadmin = true` on their next `/auth/session` call.
- **SC-005**: 0% of submissions whose Full Name + NRIC Last 5 collides with an existing row (verified or pending) result in a second row being created; 100% of those submissions return a conflict response that names the existing record.
- **SC-006**: The "Add User" action is invisible to 100% of non-superadmin users on `/dashboard/users`, and direct requests to the create endpoint from non-superadmins return 403 in 100% of cases.

## Assumptions

- The existing user tier model from feature 003 is authoritative: Superadmin = `is_superadmin = true` OR rank ≥ CPT; tier overrides remain a separate concept that this feature does not touch.
- The existing NRIC last 5 format rule (`^\d{4}[A-Za-z]$`) from feature 001 is the single source of truth for password validity; this feature reuses it verbatim and does not introduce a parallel rule.
- The existing `RequireSuperadmin` middleware in `backend/internal/middleware` is the single source of truth for the access gate; this feature does not introduce a new gate.
- The users dashboard route at `/dashboard/users` is the natural home for the "Add User" entry point; this feature does not require a separate top-level navigation entry.
- Tier override and explicit `is_superadmin` are intentionally out of scope for the create form — they remain settable only via the existing user detail page (`PUT /api/users/{id}`), so the create flow stays simple.
- DOB and `extras` are kept optional and free-text because today's other write paths (registration, bulk upload, document import) populate them inconsistently; forcing them here would diverge from those paths.
- The dashboard currently lists users via `apiClient.listUsers` with React Query; this feature reuses that query and invalidates it on successful create rather than introducing a separate data path.
- A future "Add User" entry point on the public registration page is out of scope; the public `POST /users/register` flow continues to land users in the pending approval queue per feature 003.
