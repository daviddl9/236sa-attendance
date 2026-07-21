# Feature Specification: Session Board Access After Scan

**Feature Branch**: `007-session-board-access`

**Created**: 2026-07-21

**Status**: Draft

**Input**: User description: "After scanning / recording attendance, clicking OK should route to that session's board (commanders immediately see who's missing / present and can mark manually). On mobile, tapping the missing / present panel opens that searchable, filterable list, with mark / unmark for commanders. Non-commanders should also see the board read-only — status, missing list, present list, and top-level battery stats — but cannot mark / unmark."

## Context

Marking attendance and *seeing* attendance are disconnected today:

- **After a scan there is no path to the session board.** The in-app scanner (`frontend/src/routes/dashboard/attendance/scan.tsx`) shows a toast + green tick and auto-resets after 3s — no button, no routing. The public QR link flow lands soldiers on `frontend/src/routes/attendance/marked.tsx`, whose only action is "Back to Dashboard". A commander who just scanned in at a parade point has to manually navigate to Sessions → find the session to see the live board.
- **The board is commander-only.** The session detail page (`frontend/src/routes/dashboard/sessions/$sessionId.tsx`) and its data (`GET /api/sessions/{id}`, `GET /api/reports/sessions/{id}/analytics`) are gated at Tier 2+ (`RequireBatteryNCO`). A Tier 1 soldier routed there gets a 403 — so we cannot send soldiers to the board without giving them a read-only view.
- **On mobile the board is a long scroll.** The Missing Users and Present Users cards (added in feature 006) sit at the bottom of the page. From the stats at the top there is no quick way to jump to "who is missing".

The backend already does the heavy lifting:

- `GET /api/reports/sessions/{id}/analytics` (`backend/internal/handlers/reports.go`) returns `presentUsers`, `missingUsers`, `byBattery`, and totals, and **already battery-scopes Tier 2 users to their own battery** (`GetSessionAnalytics`, the `GetTier() == TierBatteryNCO` branch). Tier 3+ see all batteries.
- `GET /api/sessions/{id}` (`GetSession`) returns session metadata (name, start time, scope, `qrCode`) — no roster.
- Mark / unmark (`POST /{id}/attendance/manual`, `DELETE /{id}/attendance/{userId}`) are Tier 2+, battery-scoped in the handlers (feature 006).
- QR data already carries the session id as its first segment (`sessionId:secret:timestamp`), so the client can derive the destination session with no extra call.

**This is mostly a frontend feature.** The backend change is limited to (a) relaxing the two *read* route guards so Tier 1 can view the board, and (b) widening one battery-scope condition so Tier 1 is scoped like Tier 2. Marking, exports, session management, schema, and SSE are untouched.

## Roles

| Tier | Label | Reach board after scan | Sees board | Battery scope | Mark / unmark | QR / export / admin controls |
|---|---|---|---|---|---|---|
| 1 (Enlisted) | user | Yes (new) | Yes — read-only (new) | Own battery only | No | None |
| 2 (Battery NCO) | battery_nco | Yes (new) | Yes | Own battery only | Yes — own battery | Unchanged (existing rules) |
| 3 (Unit Commander) | unit_commander | Yes (new) | Yes | All batteries | Yes — own battery | Unchanged (existing rules) |
| 4 (Superadmin) | superadmin | Yes (new) | Yes | All batteries | Yes — any user | Unchanged (existing rules) |

"Commander" below means Tier 2+ (`canAccessCommanderFeatures`).

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Scan Routes To The Session Board (Priority: P1) 🎯 MVP

As a commander scanning in at a parade point, after my attendance is marked I want an **OK / View Session** action that takes me straight to that session's board, so I can immediately see who is missing and start marking stragglers without hunting for the session.

**Why this priority**: This is the core connective tissue the whole request is about. It works for both scan entry points and needs no new data — the session id is already in the scanned token.

**Independent Test**: As a Tier 2+ user, scan (or manually enter) a session QR on `/dashboard/attendance/scan`. On success a confirmation appears; clicking its primary action lands on `/dashboard/sessions/{sessionId}` for that exact session. Repeat via the public `/qr/<token>` link flow → the marked-confirmation page's primary action lands on the same board.

**Acceptance Scenarios**:

1. **Given** a Tier 2+ user on the in-app scanner, **When** a scan succeeds, **Then** a success confirmation with a primary **View Session** action MUST appear (replacing the current silent toast + auto-reset), and activating it MUST navigate to `/dashboard/sessions/{sessionId}` where `sessionId` is the first segment of the scanned token.
2. **Given** the public QR link flow that lands on `/attendance/marked?session={sessionId}`, **When** the confirmation renders, **Then** its primary action MUST be **View Session** → `/dashboard/sessions/{sessionId}` (a secondary "Back to Dashboard" MAY remain).
3. **Given** a scan whose token cannot be parsed into a session id, **When** it fails, **Then** the existing error handling MUST remain (error toast, no navigation).
4. **Given** a manual-entry mark (Enter Manually dialog), **When** it succeeds, **Then** the same routing behavior as a camera scan MUST apply.

---

### User Story 2 - Soldier Sees A Read-Only Board (Priority: P1)

As an enlisted soldier who just scanned in, I want to land on a read-only board for my battery — top-level stats (total / present / missing / %) plus the present and missing name lists with status — so I can see my battery's parade state, without any ability to mark or unmark.

**Why this priority**: Story 1 routes soldiers to the board; without this they hit a 403. The two ship together.

**Independent Test**: As a Tier 1 user with a battery, open `/dashboard/sessions/{sessionId}` for a session that includes their battery. The page renders: battery-scoped stats, a Present list, and a Missing list — each showing rank, name, battery, and status badge — with **no** mark/unmark buttons, **no** QR/export/close/delete controls, and no cross-battery data.

**Acceptance Scenarios**:

1. **Given** a Tier 1 user, **When** they open a session board, **Then** `GET /api/sessions/{id}` and `GET /api/reports/sessions/{id}/analytics` MUST return 200 (not 403).
2. **Given** a Tier 1 user in battery Alpha viewing a unit-wide session, **When** analytics loads, **Then** the present/missing lists and the stat totals MUST contain **only** Alpha personnel (same scoping the backend already applies to Tier 2).
3. **Given** a Tier 1 user on the board, **When** the page renders, **Then** it MUST NOT show the QR Code card, Export card, Close Session, Delete Session, or any Mark/Unmark control, and MUST NOT show the All/HQ/Alpha/Bravo battery tabs or the per-list battery filter (their scope is a single battery).
4. **Given** a Tier 1 user, **When** present/missing rows render, **Then** each MUST display the active-status badge (e.g. MC / leave / course) exactly as commanders see it.
5. **Given** a Tier 2+ user, **When** they open the board, **Then** they MUST see the full experience unchanged from today (all batteries, tabs, filters, QR, export, mark/unmark per their tier).

---

### User Story 3 - Tap A Stat Tile To Jump To Its List On Mobile (Priority: P2)

As anyone viewing the board on a phone, I want to tap the **Present** or **Missing** stat tile and have the page jump straight to that list — which is already searchable and battery-filterable — so I can look someone up fast instead of scrolling the whole page.

**Why this priority**: Pure navigation polish on top of the lists that already exist (feature 006). Valuable but not required for Stories 1–2 to be useful.

**Independent Test**: On a narrow viewport, open a session board, tap the Present stat tile → the page smooth-scrolls to the Present Users card and briefly highlights it. Tap Missing → scrolls to the Missing Users card. The tiles read as tappable (affordance) and keyboard/screen-reader operable.

**Acceptance Scenarios**:

1. **Given** the board on any viewport, **When** the user activates the Present tile, **Then** the view MUST scroll to the Present Users card; activating the Missing tile MUST scroll to the Missing Users card.
2. **Given** the stat tiles are interactive, **When** rendered, **Then** they MUST carry a visible affordance (e.g. chevron / "tap to view") and be operable by keyboard and assistive tech (button semantics, focus target on the destination card).
3. **Given** a commander used the tile to reach a list, **When** they are on that card, **Then** the existing search, battery filter (Tier 3+), and mark/unmark controls MUST work exactly as today — the tile only navigates, it does not replace the list.
4. **Given** the desktop layout, **When** rendered, **Then** the existing two-card layout MUST be unchanged; the tap-to-scroll behavior is additive, not a mobile-only alternate list.

---

### Edge Cases

- **Soldier with no battery**: if a Tier 1 user has `battery = null`, analytics has nothing to scope to. The board MUST render an empty-but-valid state (zeroed stats, empty lists with the existing empty-state messages), never a 500 or a crash.
- **Soldier opening a session for another battery**: `GET /api/sessions/{id}` returns only metadata, so a battery-specific session for another battery still renders name/time but the analytics lists come back empty (scoped out). Acceptable — no roster leaks. Not treated as an error.
- **Closed sessions**: the board (read-only for Tier 1, full for commanders) MUST still render for closed sessions; routing after a scan is unaffected by session status.
- **Already-marked re-scan**: the public QR flow already redirects an already-marked user to `/attendance/marked?session=…`; that confirmation's primary action MUST still route to the board (they can view even if the mark was a no-op).
- **Deep link / refresh**: navigating directly to `/dashboard/sessions/{sessionId}` as Tier 1 MUST behave identically to arriving via a scan (the board access is a property of the route + tier, not of the scan).
- **Tile scroll target missing**: if a list card is not rendered (e.g. zero-length and collapsed), tapping its tile MUST be a safe no-op, not an error.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The in-app scanner MUST, on a successful mark, present a confirmation with a primary action that navigates to `/dashboard/sessions/{sessionId}`, replacing the current toast-only + auto-reset behavior. `sessionId` MUST be derived from the first `:`-separated segment of the scanned/entered token.
- **FR-002**: The public-scan confirmation page (`/attendance/marked`) MUST offer a primary **View Session** action to `/dashboard/sessions/{sessionId}` using its existing `session` search param; a secondary dashboard link MAY remain.
- **FR-003**: `GET /api/sessions/{id}` and `GET /api/reports/sessions/{id}/analytics` MUST be reachable by any authenticated user (Tier 1+). All other session/report/attendance routes keep their current guards (mark/unmark Tier 2+, export/close Tier 3+, delete Tier 4).
- **FR-004**: `GetSessionAnalytics` MUST battery-scope any user below Tier 3 (Unit Commander) to their own battery — i.e. Tier 1 is scoped exactly as Tier 2 is today. Tier 3+ continue to see all batteries. The scoping change MUST be a widened condition, not new query paths.
- **FR-005**: On the session board, Tier 1 users MUST see: battery-scoped stats (total / present / missing / %), the Present list, and the Missing list, each row showing rank, name, battery, and active-status badge — and MUST NOT see QR, export, close, delete, battery tabs, per-list battery filter, or any mark/unmark control.
- **FR-006**: Commander (Tier 2+) rendering of the board MUST remain unchanged from feature 006 (tabs, filters, QR, export, mark/unmark subject to existing tier/battery rules).
- **FR-007**: The Present and Missing stat tiles MUST be interactive controls (button semantics) that scroll the page to the corresponding list card and are operable by keyboard and assistive technology; the destination cards MUST carry stable ids as scroll anchors.
- **FR-008**: The tap-to-scroll behavior MUST be additive — desktop keeps its two-card layout, and the lists retain their existing search / filter / mark / unmark exactly as today.

### Non-Functional / Constraints

- **NFR-001**: Backend changes are limited to route-guard placement (two read routes) and one widened battery-scope condition in `GetSessionAnalytics`. No schema, SSE, handler-logic (beyond the scope condition), or marking-flow changes.
- **NFR-002**: Reuse existing components and patterns — `UserTable`, the existing Select/Input filters, shadcn Card/Badge/Button, `StatusBadge`, `useAuth`, and the `canAccessCommanderFeatures` / `getUserTier` helpers. No new list component and no new endpoints.
- **NFR-003**: Tier-conditional rendering MUST rely on the server-provided tier (`getUserTier` / `canAccessCommanderFeatures`), never on ad-hoc rank string checks in the page.

### Out of Scope

- Any bottom-sheet / modal overlay for the lists (explicitly decided against — tiles scroll to the existing cards).
- Battery-scoping of `GET /api/sessions/{id}` metadata (name/time/QR); only the roster-bearing analytics is scoped.
- Changes to marking, unmarking, exports, session creation/close/delete, or SSE.
- Routing soldiers anywhere other than the read-only board (no new soldier-specific pages).

## Success Criteria

1. A commander who scans in reaches the live session board in **one tap** after the mark, for both the in-app scanner and the public QR link.
2. An enlisted soldier who scans in sees their battery's parade state (stats + present + missing with status) read-only, with zero mark/unmark/admin controls and zero cross-battery data.
3. On a phone, any viewer can go from the stats to the specific missing/present person they are looking for without manually scrolling the page (tap tile → search).
4. Backend diff is confined to two route-guard moves and one widened scope condition; commander-facing behavior from feature 006 is visibly unchanged.
