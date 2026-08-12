# Telegram Admin UI Design

**Date:** 2026-08-12
**Status:** Approved
**Branch:** `010-telegram-admin-ui`

## Goal

Add a Telegram-native inline-button UI for paired unit commanders and superadmins to create attendance events, deliver event QR codes and links, view attendance status, manually mark soldiers, undo their own manual marks, and close events.

The existing authenticated dashboard remains available. Telegram adds an in-chat admin surface; it does not replace the dashboard.

## Product decisions

| Decision | Choice |
|---|---|
| Surface | Telegram private-chat inline buttons and conversational wizard |
| Event creation | Unit commanders and superadmins only |
| Tier 2 commanders | Cannot create or close events; can view and manually manage attendance within their battery |
| Event fields | Name, unit-wide/battery-specific scope, battery when required, required end time |
| End-time UX | Preset durations: 30 minutes, 1 hour, 2 hours, or 4 hours |
| QR delivery | QR image plus clickable Telegram link |
| Status | Attendance summary and paginated missing list |
| Manual marking | Search or tap an authorized missing soldier, then confirm |
| Undo | Only manual attendance records created by the current commander |
| Custom rosters | Out of scope for Telegram creation |
| Personnel statuses | Out of scope (`MC`, `off-pass`, `course`, etc.) |
| Pairing review, exports, notifications | Out of scope |

### Role matrix

| Role | Create/close | View status | Manual mark/undo |
|---|---:|---:|---:|
| Tier 2 commander | No | Own battery | Own battery |
| Tier 3 unit commander | Yes | Unit | Unit |
| Superadmin | Yes | Permitted global scope | Permitted global scope |

These rules preserve the existing web permission boundary: event creation is a unit-commander-level operation, while manual attendance is available to commanders within their existing authority.

## User experience

### Entry menu

The bot accepts `/menu` in a private chat. A paired eligible account may also receive an `Open commander menu` button in the normal linked response.

```text
/menu
  |
  +-- Tier 2 commander
  |     +-- Active events
  |
  +-- Unit commander
  |     +-- Create event
  |     +-- Active events
  |
  +-- Superadmin
        +-- Create event
        +-- Active events
```

Unpaired accounts and non-commanders do not receive roster or admin information. Group and channel messages remain ignored.

### Create-event wizard

```text
Create event
  -> send event name
  -> [Unit-wide] [Battery-specific]
  -> choose battery, when required
  -> [30 min] [1 hour] [2 hours] [4 hours]
  -> review name, scope, battery, and end time
  -> [Confirm] [Cancel]
  -> create session
  -> send QR image and clickable link
```

Input rules:

- Event names are between 1 and 80 characters after trimming.
- The end time is required and is calculated from the server's current time using the selected duration.
- A unit commander can create unit-wide or battery-specific events for batteries within their unit.
- A superadmin can create events across their permitted scope.
- The first release does not accept custom Excel rosters or arbitrary timestamp text.

Event creation is confirmed explicitly. A replayed confirmation must not create a second session.

### Active events and selected-event menu

`Active events` returns only sessions the actor is allowed to use. Each button includes the event name and end time. Selecting one persists it as the actor's current Telegram context.

```text
First Parade · ends 09:30
  +-- Send QR + link
  +-- View status
  +-- Missing soldiers
  +-- Search soldier
  +-- My manual marks
  +-- Close event       (Tier 3+ only)
  +-- Back
```

A Tier 2 commander may view and use the QR for an authorized active session, but cannot close it or create a new one. Every action rechecks the actor and session rather than trusting the menu button.

### QR and link

The QR encodes the existing opaque Telegram deep link:

```text
https://t.me/<configured-bot>?start=<unguessable-session-code>
```

The bot sends the QR image with an event caption and a URL button. If photo delivery fails after the session is committed, the bot sends the link as a fallback. The bot token, webhook secret, QR secret, and raw database deep-link code are never included in the message or log output.

### Status

`View status` returns a bounded attendance summary and missing list:

```text
First Parade
Present: 38 / 42 (90.5%)
Missing: 4

[Mark missing soldier]
[Next page] [Refresh] [Back]
```

- At most 10 roster rows are shown per page.
- Tier 2 results are restricted to the actor's battery.
- Unit commanders see their unit scope.
- Superadmins see their permitted global scope.
- A status query never returns personnel-status records.

### Manual marking and undo

A commander can use the missing list or search by name:

```text
Search soldier
  -> enter name
  -> up to 10 matching authorized candidates
  -> select candidate
  -> [Mark present] [Cancel]
  -> mark with marking_method=manual and marked_by=<actor>
```

Search is case-insensitive and limited to the actor's existing authority. The target does not need a Telegram pairing. The candidate is reloaded and reauthorized when the callback is tapped.

`My manual marks` returns recent manual records created by the current actor. Undo is allowed only when the record is still the actor's manual mark; it cannot remove a Telegram scan, another commander's mark, or an arbitrary attendance row.

### Closing an event

```text
Close event
  -> confirmation
  -> session becomes closed
  -> scan and manual-mark attempts stop
  -> saved selected-session context is cleared
```

Expired sessions follow the same behavior as closed sessions.

## Architecture

```text
Telegram webhook
      |
      v
Bot router
      |
      +-- Existing soldier flow
      |     +-- pairing
      |     +-- deep-link attendance
      |
      +-- Admin flow
            +-- resolve paired actor
            +-- reload effective tier and battery
            +-- load persistent chat context
            +-- call shared application services
                    |
                    +-- session service
                    +-- report/missing service
                    +-- attendance service
                    +-- QR/link builder
```

### Shared application services

Telegram must not call cookie-authenticated HTTP handlers and must not duplicate their SQL. The implementation will extract or introduce small service boundaries used by both HTTP and Telegram adapters:

- Standard session creation with scope, batteries, end time, random deep-link code, and transaction boundaries.
- Active-session listing and authorized session lookup.
- Session closing with active/closed validation.
- Scoped attendance summary and missing-user queries.
- Manual mark and manual undo operations.

The existing `services/attendance.Mark` remains the source of truth for closed-session, scope, duplicate, and idempotency decisions. Manual marks preserve `marking_method=manual` and `marked_by`.

### Telegram admin actor

A Telegram admin operation resolves the actor from the confirmed pairing associated with the callback/message Telegram ID, then reloads the roster user from PostgreSQL. Authority is calculated from the current effective tier, unit, battery, and superadmin state.

The actor ID is taken from the authenticated Telegram update, never from callback data. Session and target IDs in callback data are only lookup hints and are always revalidated.

### Persistent context

The existing `telegram_chat_context` table becomes the durable conversation state. The migration makes `session_id` nullable and adds these exact fields:

- `state TEXT NOT NULL DEFAULT 'idle'`;
- `draft_name TEXT`;
- `draft_scope TEXT`;
- `draft_battery TEXT`;
- `draft_end_time TIMESTAMP`;
- `expires_at TIMESTAMP`;
- `version BIGINT NOT NULL DEFAULT 0`.

The existing `"updatedAt"` column is updated on every context write. The version is incremented with an optimistic update so replayed callbacks cannot overwrite a newer draft or create a second event.

A selected active session survives an API restart. A draft expires after a short timeout. Selecting a closed or expired session clears the context and returns the actor to the active-event menu.

### Callback and message handling

Callback data remains below Telegram's size limit and contains only an action plus bounded identifiers. It does not contain authority decisions or a trusted actor ID. Callbacks are acknowledged promptly, then validated and executed.

The admin flow uses a finite state machine for text input:

```text
idle
  -> creating_name
  -> choosing_scope
  -> choosing_battery       (battery-specific only)
  -> choosing_duration
  -> confirming_creation
  -> idle / selected_session

selected_session
  -> viewing_status
  -> searching_name
  -> confirming_mark
  -> reviewing_own_marks
  -> idle
```

Unexpected text, stale callbacks, and expired states return a safe prompt without exposing roster or session existence to unauthorized users.

### Telegram transport

The Telegram client gains:

- URL-button support in inline keyboards;
- `sendPhoto` support for an in-memory QR PNG with caption and markup;
- testable transport fakes for API request assertions.

QR generation uses the configured Telegram link, never a bot credential or raw database secret.

## Error and failure semantics

- Non-private updates are ignored.
- Unpaired or non-commander accounts receive the existing safe response.
- Unauthorized session/target lookups behave as unavailable; they do not reveal whether another session or person exists.
- Closed, expired, duplicate, and out-of-scope actions return user-safe status text and do not mutate attendance.
- A database commit is authoritative. If the subsequent QR photo call fails, the bot sends a link fallback and logs only a redacted delivery error.
- Telegram downtime does not disable dashboard session creation or attendance operations.

## Security and privacy requirements

- Recheck pairing and authority on every action and callback.
- Restrict all admin interactions to direct messages.
- Limit every list and search result to the actor's current authority.
- Never trust callback data for authorization.
- Never expose bot tokens, webhook secrets, QR secrets, or raw deep-link codes.
- Preserve attendance attribution and audit semantics.
- Use database constraints and idempotent service operations to handle Telegram redelivery and concurrent taps.

## Testing and acceptance

### Unit tests

- Role-specific menu construction.
- Wizard transitions, validation, cancellation, and expiry.
- Callback parsing and stale-action handling.
- Pagination and Telegram message-size bounds.
- Search candidate limits and authority filtering.
- URL buttons, QR photo requests, and fallback link delivery.
- No credential or raw secret leakage in requests, errors, or rendered text.

### PostgreSQL integration tests

- Tier 2 cannot create or close events.
- Unit commanders and superadmins can create permitted scopes only.
- Active-session listing is scoped correctly.
- Event creation is transactional and confirmation replay is idempotent.
- Context survives a process restart and is cleared after closure/expiry.
- Manual marks are attributed to the commander.
- A commander can mark an unpaired soldier within authority.
- Undo removes only the current actor's manual mark.
- Concurrent scan/manual-mark behavior follows the shared attendance service.
- Closed, expired, duplicate, and out-of-scope operations do not mutate records.

### End-to-end validation

Use a disposable synthetic PostgreSQL database and a fake Telegram transport. Do not copy production data into the repository or test artifacts. Run the existing full Go suite, race suite, migrations, frontend validation, and Telegram flow tests.

## Non-goals

This release does not add:

- custom Excel roster import in Telegram;
- personnel-status CRUD;
- pairing review or unpairing from the new menu;
- exports;
- notifications or broadcasts;
- a second web UI;
- removal of the existing dashboard flows.
