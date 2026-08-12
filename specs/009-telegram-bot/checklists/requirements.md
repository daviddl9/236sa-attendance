# Specification Quality Checklist: Telegram Attendance for Soldiers

**Purpose**: Validate specification completeness and quality before proceeding to tasks
**Created**: 2026-08-11
**Feature**: [spec.md](../spec.md)

## Content Quality

- [x] No implementation details (languages, frameworks, APIs)
- [x] Focused on user value and business needs
- [x] Written for non-technical stakeholders
- [x] All mandatory sections completed

Note: the Context section cites measured facts (Sheets API call counts, quota) because they are the evidence for rejecting the prototype's storage. Requirements themselves stay technology-agnostic; Telegram is named only because it is the product decision, not an implementation choice.

## Requirement Completeness

- [x] No [NEEDS CLARIFICATION] markers remain
- [x] Requirements are testable and unambiguous
- [x] Success criteria are measurable
- [x] Success criteria are technology-agnostic
- [x] All acceptance scenarios are defined
- [x] Edge cases are identified
- [x] Scope is clearly bounded
- [x] Dependencies and assumptions identified

## Feature Readiness

- [x] All functional requirements have clear acceptance criteria
- [x] User scenarios cover primary flows
- [x] Feature meets measurable outcomes defined in Success Criteria
- [x] No implementation details leak into specification

## Notes

- **34 functional requirements across 4 user stories.** Stories 1 and 2 are both P1 and mutually dependent: pairing must exist before marking is possible.
- **Story 4 was promoted from P3 to P2.** Manual marking happens through the bot, so it is the fallback covering soldiers not on Telegram, not a convenience. Without it a commander runs the parade from a laptop and a phone at once.
- **FR-034 exists because the prototype's QR authorises nothing.** Its deep-link payload is base64 of the event name plus a timestamp (`app.py:172-178`), so it is derivable and brute-forceable in seconds; anyone could mark themselves present without seeing the QR. It also silently breaks for event names over 27 characters. The fixed-length random code keeps the authorisation property the existing web QR already has.
- **The prototype's identity model is adopted; its storage is rejected.** `~/Projects/236sa-attendance-bot` marks one soldier present with 4 un-batched Google Sheets calls, so 350 soldiers in 90 seconds implies roughly 930 calls/min against a ~300/min quota. SC-003 and the load test exist to hold the replacement to a measured standard rather than an assumed one.
- **This feature does not remove the soldier web pages.** Doing both at once would leave no fallback if Telegram adoption stalls. Retirement is tracked separately.
- **008 is not superseded.** Commanders still need accounts, so PR4 (bulk approve, password reset) remains worthwhile at a scale of 9 rather than 431, and PR5 (dropping NRIC and DOB) is still the highest-value security work outstanding.
- **The largest technical risk is duplicated marking logic**, not Telegram integration. The plan's first PR extracts a shared attendance service specifically so scope and duplicate rules cannot diverge between the two surfaces, with parity tests to enforce it.
