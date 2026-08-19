# Feature Specification: Group Member Management & Ops-Group Seeding

**Feature Branch**: `020-group-member-management`

**Created**: 2026-08-17

**Status**: Draft

**Input**: User description: "Add a simple UI to create and add people in groups to this app. Seed more groups by ops group (e.g. RnS, FDC/BOC, BCS, CSS, PSO (1SG and above), A/B/HQ Bty)."

## Context

The app already supports reusable **participant groups** (created from an Excel roster upload), and sessions created from those groups. But groups today can only be built by uploading a file — there is no way to hand-assemble a group, inspect who is in it, or adjust membership after creation.

Commanders and administrators need finer-grained control:

1. **Build & manage groups by hand** — create new people (roster users) and add them to groups directly, without an Excel file; view members; remove members.
2. **Seed the unit's operational groups** — the organisation runs on well-known ops groupings (Recce & Survey, Fire Direction Centre / Battery Operations Centre, Casualty Station, Combat Service Support, Personnel Support, and the three batteries). These should be pre-created from the roster with the right people, so commanders don't have to rebuild them each callup.

The roster file (`236SA 2026 ICT 6 callup (test) 2 (1).xlsx`, password `yeoec`) encodes the ops-group membership in three columns: **Position Description**, **Sub-Unit 1** (BN HQ, BN OPS CEN, BN RECCE & SURVEY TM, FIRE DIRECTION CEN, FIELD ARTY BTY A/B, HQ BTY, S1 BR, ...), and **Sub-Unit 2** (BTY COMMAND POST, BTY HQ, BTY RECCE GP, COMBAT TRAIN, GUN DET 1–6, HQ COMBAT TRAIN, MEDICAL PL, MT PL, PERSONNEL SP PL, QM & SVCS PL, S1 CELL, SIGNAL PL, No Data).

The seed rules below were confirmed with the unit owner. Battery-level personnel are deliberately excluded from the functional ops groups — they are accounted at battery level instead (A Bty, B Bty, HQ Bty).

### Seeded groups & rules

| Group | Members (rule over roster columns) | n |
|---|---|---|
| **RnS** (Recce & Survey) | Sub-Unit 1 = `BN RECCE & SURVEY TM` | 8 |
| **FDC/BOC** (Fire Direction Centre / Bty Ops Centre) | Sub-Unit 1 = `FIRE DIRECTION CEN` | 11 |
| **BCS** (Battalion Casualty Station) | Sub-Unit 2 = `MEDICAL PL` (incl. Medical Officer) | 16 |
| **CSS** (Combat Service Support) | S1 CELL (in S1 BR) + MT PL + MEDICAL PL + HQ COMBAT TRAIN + S4/OC HQ + HQ-battery BSM. **Excludes** PERSONNEL SP PL, QM & SVCS PL, battery COMBAT TRAINs. | 76 |
| **PSO** (1SG and above) | Rank ≥ 1SG across the whole roster | 40 |
| **A Bty** | Sub-Unit 1 = `FIELD ARTY BTY A` | 90 |
| **B Bty** | Sub-Unit 1 = `FIELD ARTY BTY B` | 89 |
| **HQ Bty** | Sub-Unit 1 = `HQ BTY` | 139 |
| **MT Platoon** | Sub-Unit 2 = `MT PL` | 39 |
| **Technicians** | Vocation ∈ {`AUTO TECH`, `AUTO SPEC TECH`, `ARMT TECH`, `ARMT SPEC TECH`} | 16 |
| **CSS Commanders** | CSS members with rank ≥ 3SG | 22 |
| **A Commanders** | A Bty members with rank ≥ 3SG | 29 |
| **B Commanders** | B Bty members with rank ≥ 3SG | 32 |
| **HQ Commanders** | HQ Bty members with rank ≥ 3SG | 29 |
| **All Commanders** | All roster members with rank ≥ 3SG | 123 |

People in `BN HQ`, `BN OPS CEN`, `HL 236 SA`, `NON-ESTAB`, `RSM GP`, and the `PERSONNEL SP PL` staff (~71 people) are **not assigned to any seeded group**. They are left ungrouped so commanders can add them manually after seeding.

The seed must be **idempotent and non-destructive**: groups that already exist are not recreated; members are only ever added, never removed — so manual additions made afterwards survive a re-seed.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Commander builds and adjusts a group by hand (Priority: P1)

A Tier 3+ commander opens the Groups page. Instead of (or in addition to) uploading an Excel roster, they click into a group and manage who is in it: view the member list, remove someone, and add people — either by searching the existing roster or by creating a brand-new person inline.

**Why this priority**: This is the core of the request — people management inside groups without depending on a file.

**Independent Test**: With an existing group, the user can open its member dialog, remove a member, search for an existing user and add them, and create a new user (name/rank/battery/NRIC) that is automatically added to the group.

**Acceptance Scenarios**:

1. **Given** a group with members, **When** a Tier 3+ user opens the group, **Then** they see the list of members with name, rank, and battery.
2. **Given** a member in a group, **When** the user removes them, **Then** the member is removed from the group and the group's member count updates.
3. **Given** a user search query, **When** the user searches, **Then** matching roster users appear and can be selected to add to the group.
4. **Given** a new person's details (full name, rank, battery, NRIC last 5), **When** the user creates them from inside the group flow, **Then** a roster user is created and immediately added to the group.
5. **Given** a member who is in multiple groups, **When** the user removes them from one group, **Then** only that group's membership is affected.
6. **Given** a non-Tier-3 user, **When** they open the Groups page, **Then** they cannot modify membership (consistent with existing Tier 3+ guard on all group routes).

---

### User Story 2 - Unit seeds its ops groups from the roster (Priority: P1)

An administrator runs a CLI command (once per callup or after roster updates) that reads the roster Excel, matches each row to the roster user by NRIC last 5, and creates/seeds the ops groups.

**Why this priority**: This delivers the "seed more groups by ops group" request — the unit's real operational structure becomes immediately available as groups.

**Independent Test**: Run the seed command against a database that already has the roster users imported. Confirm the seeded groups exist with the expected member counts, and that re-running the command makes no destructive change.

**Acceptance Scenarios**:

1. **Given** a database with the current roster imported, **When** the seed command runs with the roster file, **Then** the seeded groups are created with the members listed in the table above (matched by NRIC last 5).
2. **Given** a roster row that matches **no** existing user, **When** the seed runs, **Then** that row is skipped and reported in the output; the command still succeeds.
3. **Given** a group that already exists before seeding, **When** the seed runs, **Then** the existing group is reused (not duplicated) and its members are updated to include any missing matches.
4. **Given** a member manually added to a group before a re-seed, **When** the seed runs again, **Then** the manual member is retained (seeding only ever adds, never removes).
5. **Given** the seed command run twice, **When** the second run completes, **Then** no groups are duplicated and no members are lost.

---

### User Story 3 - Commander starts a session from a seeded ops group (Priority: P2)

Once the ops groups exist, a commander can start an attendance session from any of them (existing capability), e.g. a CSS session or an A Bty session.

**Why this priority**: The primary value of groups — reusable participant lists for sessions — now extends to the unit's real ops groups. This story is mostly delivered by existing functionality; the seed makes it useful.

**Independent Test**: Select a seeded group and start a session; confirm the session's participant list matches the group's members.

**Acceptance Scenarios**:

1. **Given** a seeded ops group (e.g. CSS), **When** a Tier 3+ user starts a session from it, **Then** the session participants equal the group's member list at that moment.

---

### Edge Cases

- **Person appears in multiple groups** — e.g. the Medical Officer is in both BCS and CSS, and most officers are also in PSO; and every battery member is in their battery group *and* possibly a functional group. Membership is per-group and independent.
- **Roster row with no matching user** — the seed must skip and report it, not fail the whole run.
- **NRIC last 5 matches more than one user** — match is keyed on NRIC last 5 + name; ambiguous matches are reported rather than guessed.
- **Group with zero members** — a group created from scratch with no members yet must still appear in the list and be usable (add members later).
- **Removing a member who is not in the group** — must be a no-op, not an error.
- **Seeding when no users exist yet** — the seed reports matches found and creates only the group rows (or nothing), without failing.
- **Groups page offline/load failure** — the page degrades to the existing loading/error states.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Authorized users (Tier 3+) MUST be able to view the members of a group, with each member's name, rank, and battery.
- **FR-002**: Authorized users MUST be able to replace a group's full member list in a single operation (adding and removing members together).
- **FR-003**: Authorized users MUST be able to remove a single member from a group.
- **FR-004**: The system MUST support creating a new roster user and adding them to a group in the same flow.
- **FR-005**: The system MUST allow searching the roster to find users to add to a group.
- **FR-006**: The system MUST seed the ops groups (RnS, FDC/BOC, BCS, CSS, PSO, MT Platoon, Technicians, CSS Commanders, A/B/HQ Commanders, All Commanders, A Bty, B Bty, HQ Bty) from the roster file using the confirmed rules.
- **FR-007**: The seed MUST be idempotent: re-running it MUST NOT duplicate groups or remove members.
- **FR-008**: The seed MUST match roster rows to existing users by NRIC last 5 (with name as a tie-breaker) and report rows it could not match.
- **FR-009**: Existing group features (create from Excel, start session) MUST continue to work unchanged.
- **FR-010**: Membership changes MUST be reflected in group member counts and in sessions started from the group afterward.

### Key Entities

- **Participant Group**: A named, reusable participant list (existing). Gains dynamic membership management and seeded instances.
- **Group Membership**: The link between a group and a roster user (existing `participant_group_member`); now mutable per group via the new operations.
- **Roster User**: Existing user record (name, rank, battery, NRIC last 5, extras); the unit of membership.
- **Ops Group Seed Definition**: The confirmed mapping from roster columns (Position Description / Sub-Unit 1 / Sub-Unit 2 / Rank / Vocation) to the seeded groups.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A Tier 3+ user can view, add, and remove group members through the UI without uploading any file.
- **SC-002**: Creating a person and adding them to a group completes in under 1 minute for an experienced user.
- **SC-003**: The seed command creates all seeded groups from the provided roster with the member counts in the table (RnS 8, FDC/BOC 11, BCS 16, CSS 76, PSO 40, A Bty 90, B Bty 89, HQ Bty 139, MT Platoon 39, Technicians 16, CSS Commanders 22, A Commanders 29, B Commanders 32, HQ Commanders 29, All Commanders 123).
- **SC-004**: Running the seed command twice yields the same groups and member sets as running it once (idempotence).
- **SC-005**: No roster row that matches an existing user is silently dropped by the seed; every unmatched row is reported.
- **SC-006**: Existing session-from-group creation works for both hand-built and seeded groups.

## Assumptions

- **Seed runs as a CLI** against the application database (chosen by the user over an in-app admin button).
- **Roster users already exist** in the database (imported via the existing roster import flow) before the seed is run; the seed matches against them rather than creating users.
- **Membership overlap is normal**: a person may belong to multiple groups, including their battery group and a functional group.
- **Group membership changes take effect for new sessions** started from the group; they do not retroactively alter already-started sessions' participant lists.
- **Only the roster's NRIC-last-5 column is authoritative for matching**; name matching is used only as a confirmation/tie-breaker.
- **Seed is one-way and additive**: there is no "unseed" requirement; groups can be deleted manually via the existing delete-group UI if desired.

## Out of Scope

- Editing roster users' base profile fields from inside the group flow (the existing user management page covers editing).
- Auto-creating users during seeding (roster users are assumed already imported).
- Nested/child groups or group hierarchies.
- Per-group visibility/access control beyond the existing Tier 3+ gate.
- Bulk remove of members (single removal only; full replacement via FR-002 covers bulk cases).
- Rebasing already-started sessions' participants when a group changes.
