# Feature Specification: Simplify Authentication and Remove Sensitive Data Collection

**Feature Branch**: `008-simplify-auth`

**Created**: 2026-08-01

**Status**: Draft

**Input**: User description: "attendance.236sa.one was blacklisted by Google Safe Browsing for phishing. Remove the causes: SAF branding and collection of NRIC/DOB. Replace NRIC-as-password with a simple username and self-chosen password. No migration of existing accounts for now."

## Context

On 2026-07-24 Google Safe Browsing classified `attendance.236sa.one` as **"This site is unsafe — Try to trick visitors into sharing personal info"** (the Social Engineering category). The site is currently blocked in Chrome and demoted in Search.

Google's Social Engineering policy triggers when a page *"pretends to act, or look and feel, like a trusted entity — for example, a browser, operating system, bank, **or government**"*.

Three independent causes are present today:

| ID | Cause | Evidence |
|----|-------|----------|
| C1 | **Government impersonation** — the official 236th Battalion Singapore Artillery crest is served on a private `.one` domain | `frontend/public/favicon.jpg` |
| C2 | **National identifier harvesting** — public forms collect NRIC last-5, date of birth, full-name-as-per-NRIC, rank and unit | `frontend/src/routes/attendance/register.tsx`, `sign-in.tsx`, `index.tsx` |
| C3 | **Unauthenticated, indexable credential form** — no `robots.txt`, no `noindex`, and the promised Terms/Privacy pages do not exist | verified: `curl https://attendance.236sa.one/robots.txt` returns the SPA shell; no privacy or terms route exists |

Two pre-existing security defects were found while investigating and are in scope because they concern the same data:

| ID | Defect | Evidence |
|----|--------|----------|
| D1 | NRIC last-5 is stored **in plaintext** in an indexed column, defeating the password hash of the same secret | `backend/migrations/20240109000000_rename_nric_last4_to_last5.sql:10`, `backend/internal/handlers/auth.go:92` |
| D2 | Password hashing uses **bcrypt cost 4** (the minimum). Against NRIC's ~260,000-value keyspace the hashes are effectively reversible | `backend/internal/handlers/admin.go:327`, `admin.go:513`, `auth.go:265`, `import.go:325` |

The unit operates ~350 personnel across HQ, Alpha and Bravo batteries. This is an **unofficial** tool built for the battalion, not a MINDEF or SAF system.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Public pages stop collecting sensitive information (Priority: P1)

A visitor — including Google's crawler — reaches any page of the site without signing in. They find no military crest, no request for NRIC, and no request for date of birth. The site plainly identifies itself as an unofficial unit tool with reachable Terms and Privacy pages.

**Why this priority**: This is the entire cause of the blacklisting and the only story that unblocks the unit. It is almost entirely deletion, so it can ship on its own, ahead of any auth rework, and a Search Console review can be requested immediately.

**Independent Test**: Load every unauthenticated route in a fresh browser session and confirm no NRIC/DOB input exists, no crest is served, and Terms/Privacy resolve. Fully testable with no backend change.

**Acceptance Scenarios**:

1. **Given** a signed-out visitor, **When** they load the landing page, **Then** no battalion crest or "Singapore Artillery" wording is present.
2. **Given** a signed-out visitor, **When** they load any public route, **Then** no field requests NRIC, date of birth, or "full name as in NRIC".
3. **Given** a signed-out visitor, **When** they open the footer links, **Then** Terms of Service and Privacy Policy pages load with real content naming the operator.
4. **Given** a signed-out visitor, **When** they read the footer, **Then** it states the tool is unofficial and not a MINDEF/SAF system.
5. **Given** a crawler, **When** it requests `/robots.txt`, **Then** a real robots file is served and authentication pages are marked `noindex`.

---

### User Story 2 - Soldier signs up and signs in without sensitive data (Priority: P1)

A soldier creates an account with a username they choose and a password they choose, plus their name, rank and battery so a commander can recognise them. A commander approves the account. From then on the soldier signs in and scans the parade QR to mark attendance.

**Why this priority**: Without this the app has no working login once NRIC is removed. It is P1 alongside Story 1 but strictly depends on it.

**Independent Test**: Register a new account, approve it as a commander, sign in, scan a session QR, and confirm attendance marks — with no NRIC or DOB entered anywhere.

**Acceptance Scenarios**:

1. **Given** a soldier on the signup page, **When** they submit a unique username, a password of at least 8 characters, their full name, rank and battery, **Then** the account is created and shown as awaiting approval.
2. **Given** a username already taken, **When** the soldier submits, **Then** they are told the username is unavailable and no account is created.
3. **Given** a password shorter than 8 characters, **When** the soldier submits, **Then** submission is rejected with a clear reason.
4. **Given** a password shaped like an NRIC ending (4 digits then a letter), **When** the soldier submits, **Then** submission is rejected and they are told not to use NRIC digits.
5. **Given** an approved account, **When** the soldier signs in and scans an active session QR, **Then** their attendance is marked.
6. **Given** an unapproved account, **When** the soldier signs in, **Then** they are told the account awaits approval and no session is created.

---

### User Story 3 - Commander approves 350 soldiers without excessive toil (Priority: P2)

A commander opens the pending registrations list, filters to a battery, selects many pending soldiers at once and approves them in a single action.

**Why this priority**: Functionally the app works without it (Story 2 delivers one-at-a-time approval), but at 350 personnel one-at-a-time approval makes rollout impractical. Needed for real adoption, not for correctness.

**Independent Test**: Create 20 pending registrations, select all, approve once, and confirm all 20 can sign in.

**Acceptance Scenarios**:

1. **Given** many pending registrations, **When** a commander selects several and approves, **Then** all selected accounts become able to sign in.
2. **Given** a mixed selection, **When** approval partially fails, **Then** the commander is told which succeeded and which did not, and successful approvals are not rolled back.
3. **Given** pending registrations across batteries, **When** a commander filters by battery, **Then** only that battery's pending registrations are listed.

---

### User Story 4 - Commander resets a forgotten password (Priority: P2)

A soldier forgets their password. A commander opens that soldier's record and issues a one-time temporary password, which the soldier must replace on next sign-in.

**Why this priority**: With ~350 self-chosen passwords and no email delivery, forgotten passwords are the predictable ongoing support burden. Without this, a locked-out soldier cannot mark attendance at all.

**Independent Test**: Reset a soldier's password as a commander, sign in with the temporary password, and confirm a password change is forced before any other action.

**Acceptance Scenarios**:

1. **Given** a commander viewing a soldier's record, **When** they issue a reset, **Then** a temporary password is displayed once for the commander to pass on.
2. **Given** a soldier with a temporary password, **When** they sign in, **Then** they must set a new password before reaching any other page.
3. **Given** a soldier who has set a new password, **When** they sign in again, **Then** they are not prompted to change it.

---

### Edge Cases

- **A pending signup carried over from the pre-008 scheme shares its identifier with an existing roster row.** Approving it as a new person would create a duplicate of someone who already exists, so these must be linked, never created. Zero such rows exist in production today, but the path must be correct before it can occur.
- **Sign-in reveals roster membership.** Distinct responses for "unknown account" and "wrong password" let anyone test whether a named person is in the unit. With ~431 personnel and predictable name patterns, this makes the nominal roll enumerable without signing in.

- **Usernames differing only by case or surrounding spaces** — treated as the same username so `TanWM` and `tanwm` cannot both exist.
- **Duplicate full names** — expected at 350 personnel; the username disambiguates, and the approval list must show rank and battery so a commander can tell two same-named soldiers apart.
- **Soldier scans a QR while signed out** — must be returned to sign-in and, after signing in, land on the scan result rather than the dashboard.
- **Soldier scans a QR while unapproved** — attendance must not mark; they see the pending-approval message.
- **Existing accounts created under the old NRIC scheme** — see Assumptions and the open question below.
- **Rejected registration re-registering** — the username must become available again after rejection.
- **Commander resets a password twice** — only the most recent temporary password works.
- **Names, ranks and batteries must never render to a signed-out visitor** — the nominal roll of ~350 personnel is the unit's most sensitive remaining data, and Google would not flag its exposure, so it must be guarded deliberately.

## Requirements *(mandatory)*

### Functional Requirements

**Removing the blacklist causes**

- **FR-001**: The system MUST NOT display the battalion crest or "Singapore Artillery" / "236th Battalion" wording on any unauthenticated page, including the browser tab icon and page title.
- **FR-002**: The system MUST NOT request NRIC, any part of an NRIC, or date of birth anywhere, from any user, at any time.
- **FR-003**: The system MUST NOT label any field as "as in NRIC" or show NRIC-shaped examples, placeholders or format hints.
- **FR-004**: The system MUST serve reachable Terms of Service and Privacy Policy pages that name the operator and describe what data is held and why.
- **FR-005**: The system MUST state on unauthenticated pages that it is an unofficial unit tool and not a MINDEF or SAF system.
- **FR-006**: The system MUST serve a `robots.txt` and MUST mark authentication pages as not-indexable.
- **FR-007**: The system MUST NOT display any personnel name, rank or battery to an unauthenticated visitor.

**Authentication**

- **FR-008**: Users MUST be able to register with a self-chosen username, a self-chosen password, and their full name, rank and battery.
- **FR-009**: The system MUST enforce unique usernames, compared case-insensitively and ignoring surrounding whitespace.
- **FR-010**: The system MUST require passwords of at least 8 characters.
- **FR-011**: The system MUST reject passwords matching the NRIC-ending shape (4 digits followed by a letter) and explain why.
- **FR-012**: The system MUST identify users by username at sign-in, not by name combined with any personal identifier.
- **FR-013**: The system MUST keep the existing behaviour that a newly registered account cannot sign in until a commander approves it.
- **FR-014**: The system MUST replace the single shared `admin` login with named commander accounts, so that approvals and resets are attributable to a person.

**Data removal**

- **FR-015**: The system MUST permanently remove stored NRIC and date-of-birth values from the database.
- **FR-016**: The system MUST NOT accept NRIC or date of birth through any remaining interface, including roster import and bulk upload.
- **FR-017**: The system MUST store passwords using a hashing cost appropriate for credential storage, not the minimum permitted cost.

**Operating at 350 personnel**

- **FR-018**: Commanders MUST be able to approve multiple pending registrations in one action.
- **FR-019**: Commanders MUST be able to filter pending registrations by battery.
- **FR-020**: Commanders MUST be able to issue a one-time temporary password for a soldier.
- **FR-021**: The system MUST force a user signing in with a temporary password to set a new password before reaching any other page.

**Linking signups to existing roster rows**

- **FR-022**: At approval, the system MUST show the commander a ranked list of existing roster rows that plausibly match the signup, tolerant of misspellings in the submitted name.
- **FR-023**: The ranking MUST tolerate a wrong rank or wrong battery without demoting an otherwise strong name match, because both are frequently entered incorrectly.
- **FR-024**: Each candidate MUST show why it may not match — specifically, which of rank or battery differs from what the soldier submitted.
- **FR-025**: The system MUST NOT link a signup to a roster row automatically. A commander MUST confirm every link explicitly.
- **FR-026**: When a signup is linked to an existing roster row, that row's attendance history MUST remain intact and attributed to the same person.
- **FR-027**: The system MUST indicate roster rows that are already linked to an account, so a commander can see why an expected name is unavailable.
- **FR-028**: Approving a pending signup that was carried over from the pre-008 scheme MUST attach it to that person's existing roster row rather than creating a second row for the same person.

**Not disclosing who is on the roster**

- **FR-029**: Sign-in MUST NOT reveal whether a given name or username exists. A failed sign-in MUST be indistinguishable whether the account is unknown or the password is wrong.

### Key Entities

- **User**: A member of the battalion. Holds username, password hash, full name, rank, battery, approval state, and whether a password change is pending. **No longer holds NRIC or date of birth.**
- **Commander account**: A named user with approval and reset privileges, replacing the shared `admin` login.
- **Pending registration**: A user awaiting commander approval. Presented with name, rank, battery and submission time — never with a personal identifier.
- **Attendance record**: Unchanged. Links a user to a session with a marking method.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Google Safe Browsing reports `attendance.236sa.one` as carrying no unsafe content, and Chrome loads the site without an interstitial warning.
- **SC-002**: Zero NRIC values and zero date-of-birth values remain retrievable from the system.
- **SC-003**: A signed-out visitor can reach no page that requests personal information and no page that reveals any personnel name.
- **SC-004**: A soldier can register in under 60 seconds without consulting any identity document.
- **SC-005**: A commander can approve 50 pending registrations in under 2 minutes.
- **SC-006**: A locked-out soldier can be restored to marking attendance in under 1 minute of commander effort.
- **SC-007**: Every approval and password reset is attributable to a named commander.
- **SC-008**: For a signup whose submitted name contains a single-character typo, the correct roster row appears as the top-ranked candidate.
- **SC-009**: After linking, a soldier's attendance records from before the change still appear in their history and in per-person reports.
- **SC-010**: No unauthenticated request can be used to determine whether a named person is in the system.
- **SC-011**: No person appears twice on the roster as a result of approval.

## Assumptions

- **No bulk account migration is performed.** Existing NRIC-derived passwords stop working; each soldier registers fresh and is linked to their roster row at approval.
- **Attendance history is retained.** Removing NRIC and date-of-birth columns does not delete user rows or attendance records, and linking reuses the existing row, so historical reports remain intact.
- Commanders know their personnel well enough to confirm a match from name, rank and battery alone. This is the assumption that makes fuzzy matching safe.
- The roster is reasonably complete before rollout. Signups with no plausible match are approved as new people, which is expected but should be uncommon.
- Soldiers have a personal smartphone with a browser and a camera; QR scanning already works across devices.
- The roster is imported by commanders and is the denominator for "who has not scanned"; this already works via `GetMissingUsers` and does not depend on how soldiers authenticate.
- Manual marking remains available, so any soldier who cannot sign in during rollout can still be marked present.
- Rollout happens over more than one parade cycle rather than as a single cutover.
- Google Search Console access exists for the domain, which is required to request a review.
- No transactional email is available, so password recovery is commander-mediated rather than self-service.

## Resolved Decisions

- **Q1 — existing personnel regain access by registering fresh, and the commander links the signup to their existing roster row at approval time.** No bulk migration is run. Because linking reuses the existing row, attendance history is preserved without the ~350 temporary passwords that a direct migration would need. Soldiers mistype names and ranks, so candidate matching MUST be fuzzy and MUST NOT auto-link. Captured as FR-022 to FR-025 below.

## Out of Scope

- Self-service password recovery by email or SMS.
- Third-party sign-in (Google, Microsoft, Apple). Considered and set aside in favour of the simpler username and password scheme.
- Device-bound identity or per-soldier enrolment QR codes. Considered and set aside.
- Changes to session creation, QR generation, analytics, or reporting behaviour.
- Moving the application to an official domain.
