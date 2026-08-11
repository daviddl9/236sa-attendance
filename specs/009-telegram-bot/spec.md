# Feature Specification: Telegram Attendance for Soldiers

**Feature Branch**: `009-telegram-bot`

**Created**: 2026-08-11

**Status**: Draft

**Input**: User description: "Write a Telegram bot equivalent in Go so we can reuse most of our existing code and logic. Most of the unit is on Telegram."

## Context

Feature 008 removed NRIC and date-of-birth collection and the SAF crest after Google Safe Browsing blacklisted the site for phishing. That fixed the immediate cause, but the underlying shape remains: **a public web form, and credentials for ~431 soldiers.** Every remaining problem descends from those two facts.

| Problem | Cause | Current handling |
|---|---|---|
| Google blacklisting | Public form on a private domain | Removed the flagged content (008 PR1) |
| ~431 passwords, forgotten weekly | Soldiers need credentials | A commander reset tool is planned (008 PR4) |
| Roster enumerable unauthenticated | Public sign-in distinguished unknown accounts | Responses made identical (008 PR3a) |
| 9 commanders lose access when NRIC is dropped | They authenticate via the legacy NRIC path | Unsolved; blocks 008 PR5 |
| Nominal roll must never be exposed | Any public page listing names | Ongoing care (FR-007) |

**Telegram removes the two causes rather than defending against them.** Telegram supplies a stable, unique account identifier for every soldier, so the system needs no soldier passwords and no public soldier-facing page.

Commanders are different: there are only 9, they need reporting, analytics, roster import and approvals, and 9 accounts is negligible support load. They keep the existing authenticated web dashboard.

The unit is already on Telegram, which is the precondition that makes this viable. A prototype exists at `~/Projects/236sa-attendance-bot` (Python, FastAPI, Google Sheets). Its identity model is correct and is adopted here. Its storage is not: marking one soldier present costs **4 un-batched Google Sheets API calls**, so 350 soldiers scanning within 90 seconds implies roughly **930 calls per minute against a ~300/min quota**. It would fail at exactly the moment it matters. This feature therefore keeps PostgreSQL and reuses the existing Go backend.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Soldier marks attendance from Telegram (Priority: P1)

A soldier scans the parade QR code with their phone camera. It opens the unit's Telegram bot. The bot recognises them and marks them present. They type nothing and remember no password.

**Why this priority**: This is the feature. Everything else supports it.

**Independent Test**: With a soldier already paired, scan an active session QR and confirm attendance is recorded and visible on the commander's session board.

**Acceptance Scenarios**:

1. **Given** a paired soldier and an active session, **When** they open the QR deep link, **Then** their attendance is recorded and the bot confirms with the session name.
2. **Given** a soldier who already marked this session, **When** they open the link again, **Then** the bot says they are already marked and records nothing further.
3. **Given** a closed session, **When** a soldier opens its link, **Then** the bot says the session is closed and records nothing.
4. **Given** an invalid or tampered deep link, **When** a soldier opens it, **Then** the bot rejects it without revealing whether the session exists.
5. **Given** a session scoped to specific batteries, **When** a soldier outside that scope opens the link, **Then** the bot declines and records nothing.
6. **Given** a paired soldier, **When** they message the bot without a QR link, **Then** the bot shows their own recent attendance and nothing about anyone else.

---

### User Story 2 - Soldier pairs their Telegram account once (Priority: P1)

A soldier opens the bot for the first time. It does not recognise them, so it registers a pairing request carrying their Telegram display name. A commander matches that request to their roster row and confirms. The soldier is told they are ready.

**Why this priority**: Nothing works before pairing. It is P1 alongside Story 1 but strictly precedes it.

**Independent Test**: Open the bot as an unknown Telegram account, confirm a pending pairing appears for a commander, link it, and confirm the soldier can then mark attendance.

**Acceptance Scenarios**:

1. **Given** an unrecognised Telegram account, **When** they open the bot, **Then** a pending pairing is created and the soldier is told a commander must confirm it.
2. **Given** a pending pairing, **When** a commander views it, **Then** candidate roster rows are ranked with the closest name first, tolerant of a wrong rank.
3. **Given** a commander confirms a pairing, **When** the soldier next opens the bot, **Then** they are recognised and can mark attendance.
4. **Given** an unpaired soldier, **When** they open a QR deep link, **Then** nothing is recorded and they are told to pair first.
5. **Given** a pending pairing, **When** the soldier opens the bot again, **Then** no duplicate pairing is created.
6. **Given** a Telegram account already paired to a roster row, **When** a commander tries to pair it to a second row, **Then** it is refused.
7. **Given** a commander rejects a pairing, **When** the soldier opens the bot again, **Then** they may request pairing afresh.

---

### User Story 3 - Commander runs the parade from the existing dashboard (Priority: P2)

A commander creates a session on the web dashboard as they do today, displays its QR code, and watches Present and Missing update as soldiers scan.

**Why this priority**: The dashboard already exists. Only QR generation and the pairing screen change, so this is adaptation rather than new capability.

**Independent Test**: Create a session, display the QR, have a paired soldier scan it, and confirm the board moves them from Missing to Present.

**Acceptance Scenarios**:

1. **Given** a commander creating a session, **When** it is created, **Then** its QR code opens the Telegram bot rather than a web page.
2. **Given** soldiers scanning, **When** a commander watches the session board, **Then** Present and Missing update without a manual refresh.
3. **Given** a soldier who cannot use Telegram, **When** a commander marks them manually, **Then** attendance is recorded exactly as it is today.
4. **Given** any attendance record, **When** a commander views it, **Then** it is clear whether it came from a Telegram scan or a manual mark.

---

### User Story 4 - Commander marks another soldier from Telegram (Priority: P3)

A commander at the parade square, without a laptop, marks a soldier present from their phone by searching for that soldier in the bot.

**Why this priority**: Genuinely useful at a parade square, but the dashboard already covers it. Deliberately last so it cannot delay Stories 1 and 2.

**Independent Test**: As a paired commander, search a soldier by name in the bot and mark them present; as a paired soldier, confirm the same search is unavailable.

**Acceptance Scenarios**:

1. **Given** a paired commander, **When** they search a name for an active session, **Then** matching roster rows within their authority are listed.
2. **Given** a paired soldier who is not a commander, **When** they attempt the same search, **Then** it is refused and no name is returned.
3. **Given** a commander marking another soldier, **When** it succeeds, **Then** the record is attributed to that commander as a manual mark.
4. **Given** a commander whose authority covers only their own battery, **When** they search, **Then** no soldier outside that battery is returned.

---

### Edge Cases

- **The deep link payload cannot carry the current QR token.** Today's token is `sessionID:secret`, 97 characters including a colon. Telegram limits `/start` payloads to 64 characters from `A-Za-z0-9_-`. A shorter session code is required.
- **Telegram display names are unreliable identifiers.** They are user-chosen, may be a nickname, a single word, or emoji, and can change at any time. They are a matching hint for a commander, never an identity.
- **Two soldiers sharing one phone.** Telegram identity is per account, not per device, so the second soldier would mark the first present. Pairing is one account to one roster row, and a commander must be able to see and undo a wrong pairing.
- **A soldier changes Telegram account** (new number, deleted account). Their old pairing is stale and a commander must be able to re-pair them.
- **Webhook delivery is retried.** Telegram redelivers on non-2xx and may deliver out of order, so marking must be idempotent.
- **Everyone scans at once.** 350 soldiers within roughly 90 seconds is the real load, not a uniform trickle.
- **QR photographed and shared off-parade.** Anyone paired who obtains the image can mark themselves present remotely, exactly as with the web QR today. Session closure is the existing control.
- **Bot blocked or never started.** A soldier who blocks the bot cannot be messaged; manual marking remains the fallback.
- **Group chats.** The bot must ignore group messages and only act on direct messages, so a QR link pasted into the unit group cannot mark someone in-place.

## Requirements *(mandatory)*

### Functional Requirements

**Identity and pairing**

- **FR-001**: The system MUST identify a soldier by their Telegram account identifier, never by a name, national identifier, or password.
- **FR-002**: The system MUST record a pairing request the first time an unrecognised Telegram account contacts the bot, capturing the Telegram identifier and display name.
- **FR-003**: The system MUST require a commander to confirm every pairing before that account can mark attendance.
- **FR-004**: The system MUST rank candidate roster rows for a pairing request by name similarity, tolerating misspellings and a wrong rank, reusing the ranking already built for 008.
- **FR-005**: The system MUST NOT pair automatically, however close the match.
- **FR-006**: The system MUST enforce one Telegram account per roster row and one roster row per Telegram account.
- **FR-007**: Commanders MUST be able to see existing pairings and remove one, so a wrong pairing or a changed Telegram account can be corrected.
- **FR-008**: Confirming a pairing MUST preserve that soldier's existing attendance history.
- **FR-009**: The system MUST NOT require a soldier to have a web account, a username, or a password.

**Marking attendance**

- **FR-010**: A paired soldier MUST be able to mark their own attendance by opening a session's QR deep link.
- **FR-011**: The system MUST apply the same session validity, scope and duplicate rules to Telegram marks as to existing marks, with no separate rule set.
- **FR-012**: Marking MUST be idempotent, so repeated webhook delivery cannot create duplicate records.
- **FR-013**: A soldier MUST NOT be able to mark anyone other than themselves.
- **FR-014**: The system MUST record how each attendance record was created, distinguishing a Telegram scan from a manual mark.
- **FR-015**: An unpaired account opening a deep link MUST NOT mark anyone, and MUST be directed to pair.

**Not disclosing the roster**

- **FR-016**: The bot MUST NOT reveal any personnel name to an account that is not a confirmed commander.
- **FR-017**: Rejections MUST NOT reveal whether a session or a person exists.
- **FR-018**: The bot MUST ignore messages that are not direct messages, so a link shared in a group cannot act on another member.
- **FR-019**: The system MUST reject webhook requests that do not prove they came from Telegram.

**Commanders**

- **FR-020**: Commanders MUST continue to create sessions, view boards, import rosters and approve pairings on the existing authenticated dashboard.
- **FR-021**: Session QR codes MUST open the Telegram bot.
- **FR-022**: Commanders MUST retain manual marking for soldiers who cannot or will not use Telegram.
- **FR-023**: A commander using the bot to mark another soldier MUST be restricted to soldiers within their existing authority, and the record MUST be attributed to them.

**Operational**

- **FR-024**: The system MUST handle the whole unit marking attendance within a few minutes of a parade starting.
- **FR-025**: A Telegram outage MUST NOT prevent commanders from recording attendance.
- **FR-026**: Telegram credentials MUST NOT be committed to the repository.

### Key Entities

- **Pairing**: A confirmed link between a Telegram account and a roster row. Holds the Telegram identifier, the roster row, who confirmed it and when.
- **Pairing request**: An unconfirmed pairing awaiting a commander, holding the Telegram identifier and the display name offered as a matching hint.
- **Session deep-link code**: A short, unguessable per-session code that fits a Telegram `/start` payload, replacing the current 97-character token for this purpose.
- **Attendance record**: Unchanged, except that its marking method distinguishes a Telegram scan.
- **User**: Unchanged. Gains no Telegram-specific fields beyond the pairing relationship.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A paired soldier marks attendance in under 10 seconds from raising their phone, with no typing.
- **SC-002**: Soldiers hold no password for this system, and no password reset is ever needed for a soldier.
- **SC-003**: The whole unit, about 431 people, can mark attendance within 5 minutes of a parade starting without failures.
- **SC-004**: No unauthenticated request and no non-commander Telegram account can obtain any personnel name.
- **SC-005**: A commander confirms a pairing in under 15 seconds, with the correct roster row offered first for a name containing a single typo.
- **SC-006**: Attendance history is unchanged by pairing.
- **SC-007**: Commanders can still record attendance for the whole unit when Telegram is unavailable.
- **SC-008**: Repeated delivery of the same scan produces exactly one attendance record.

## Assumptions

- Most of the unit already uses Telegram, and those who do not can be marked manually.
- Soldiers can open a Telegram deep link from their phone camera.
- The existing PostgreSQL database, session model, battery scoping and reporting are retained; this feature changes who talks to them and how.
- The existing commander dashboard remains the only web surface, stays behind authentication, and remains non-indexable.
- One bot serves both soldiers and commanders. Authority is derived from the paired roster row, so the prototype's two-bot split is unnecessary.
- Telegram's display name is treated as a hint for a human, never as proof of identity.
- The soldier-facing web pages are retired only after Telegram adoption is proven, so this feature does not remove them.
- Feature 008's remaining work (bulk approve, commander password reset, dropping NRIC) proceeds independently. Commanders still need accounts, so 008 PR4 remains worthwhile at a scale of 9 rather than 431.

## Out of Scope

- Retiring the soldier-facing web sign-in and signup pages. Tracked separately once adoption is proven.
- WhatsApp or any second messaging channel.
- Dropping `nric_last5` and `dob`, which remains 008 PR5.
- Replacing PostgreSQL, or adopting the prototype's Google Sheets storage.
- Notifications, reminders, or broadcast messaging to soldiers.
- Letting soldiers view unit-wide attendance or any roster.
- Changing rank, battery, tier or reporting behaviour.
