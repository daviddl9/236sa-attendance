# Session Board Access After Scan — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** After marking attendance (in-app scan or QR link), route the user to that session's board; give Tier 1 soldiers a battery-scoped read-only board; and let mobile users tap a stat tile to jump to the missing/present list.

**Architecture:** Almost entirely frontend. Backend change is limited to (a) relaxing two read-route guards so Tier 1 can view the board, and (b) widening one battery-scope condition in `GetSessionAnalytics` so soldiers are scoped to their own battery exactly like Tier 2. The board itself is the existing `$sessionId.tsx` page rendered tier-conditionally; the mobile jump is `scrollIntoView` on the existing list cards.

**Tech Stack:** Go (chi router, pgx) backend; React 19 + TanStack Router/Query + shadcn/ui (Tailwind) frontend. Backend has Go unit tests (`go test`); **frontend has no test runner** — verify frontend tasks with `tsc`/`eslint` and end-to-end via `agent_browser`.

## Global Constraints

- Module path: `github.com/davidlivingston/go-nextjs-starter`; backend packages under `backend/internal/...`.
- Tiers: 1 Enlisted (REC–CFC), 2 BatteryNCO (3SG–1SG), 3 UnitCommander (SSG+), 4 Superadmin (CPT+ or `is_superadmin`). Use `user.GetTier()` / `models.Tier*` on the backend and `canAccessCommanderFeatures(user)` / `getUserTier(user)` on the frontend — never ad-hoc rank string checks.
- "Commander" = Tier 2+ (`canAccessCommanderFeatures`).
- Mark/unmark stays Tier 2+; export/close stays Tier 3+; delete stays Tier 4. Only the two **read** routes (`GET /sessions/{id}`, `GET /reports/sessions/{id}/analytics`) relax to Tier 1+.
- Reuse existing components (`UserTable`, shadcn Card/Badge/Button/Select/Input, `StatusBadge`). No new list component, no new endpoints, no schema/SSE changes.
- Commits: sign off as David — `git -c user.name="David" -c user.email="ddl.tdh@gmail.com" commit -m "..."`. No co-author trailers. Do **not** push to `main` (push triggers a prod deploy); work stays on branch `007-session-board-access`.

## File Structure

| File | Responsibility | Change |
|---|---|---|
| `backend/internal/handlers/reports.go` | Session analytics + battery scoping | Extract `batteryScopeForAnalytics` helper; scope Tier <3 |
| `backend/internal/handlers/reports_test.go` | Unit test for scope helper | Create |
| `backend/cmd/api/main.go` | Route wiring / RBAC guards | Relax `GET /sessions/{id}` + `GET /reports/sessions/{id}/analytics` to Tier 1+ |
| `frontend/src/routes/dashboard/attendance/scan.tsx` | In-app scanner | Success dialog → **View Session** route |
| `frontend/src/routes/attendance/marked.tsx` | Public-scan confirmation | Add **View Session** primary button |
| `frontend/src/routes/dashboard/sessions/$sessionId.tsx` | Session board | Tier-conditional read-only view + tappable stat tiles |

---

### Task 1: Backend — battery-scope analytics for Tier 1 (pure helper, TDD)

**Files:**
- Modify: `backend/internal/handlers/reports.go` (add helper; replace inline condition at the "Tier 2 users see only their own battery" block, ~lines 67–72)
- Test: `backend/internal/handlers/reports_test.go` (create)

**Interfaces:**
- Produces: `batteryScopeForAnalytics(user *models.User) *string` — returns the battery to restrict analytics to, or `nil` for all-batteries. Tier 1 & 2 with a battery → that battery; Tier 3+ or nil battery/user → `nil`.

- [ ] **Step 1: Write the failing test**

Create `backend/internal/handlers/reports_test.go`:

```go
package handlers

import (
	"testing"

	"github.com/davidlivingston/go-nextjs-starter/backend/internal/models"
)

func strptr(s string) *string { return &s }

func TestBatteryScopeForAnalytics(t *testing.T) {
	tests := []struct {
		name string
		user *models.User
		want *string
	}{
		{"nil user", nil, nil},
		{"enlisted with battery is scoped", &models.User{Rank: strptr(models.RankREC), Battery: strptr(models.BatteryAlpha)}, strptr(models.BatteryAlpha)},
		{"battery NCO with battery is scoped", &models.User{Rank: strptr(models.Rank3SG), Battery: strptr(models.BatteryBravo)}, strptr(models.BatteryBravo)},
		{"unit commander sees all batteries", &models.User{Rank: strptr(models.RankSSG), Battery: strptr(models.BatteryHQ)}, nil},
		{"superadmin sees all batteries", &models.User{IsSuperadmin: true, Battery: strptr(models.BatteryHQ)}, nil},
		{"enlisted without battery is unscoped", &models.User{Rank: strptr(models.RankREC)}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := batteryScopeForAnalytics(tt.user)
			if (got == nil) != (tt.want == nil) {
				t.Fatalf("batteryScopeForAnalytics() = %v, want %v", got, tt.want)
			}
			if got != nil && *got != *tt.want {
				t.Fatalf("batteryScopeForAnalytics() = %q, want %q", *got, *tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/handlers/ -run TestBatteryScopeForAnalytics -v`
Expected: FAIL — `undefined: batteryScopeForAnalytics`.

- [ ] **Step 3: Add the helper and use it**

In `backend/internal/handlers/reports.go`, add the helper immediately above `func (h *ReportsHandler) GetSessionAnalytics`:

```go
// batteryScopeForAnalytics returns the battery an analytics view is restricted
// to, or nil for no restriction. Enlisted soldiers (Tier 1) and battery NCOs
// (Tier 2) see only their own battery; unit commanders and superadmins (Tier
// 3+) see all batteries.
func batteryScopeForAnalytics(user *models.User) *string {
	if user == nil || user.Battery == nil {
		return nil
	}
	if user.GetTier() < models.TierUnitCommander {
		return user.Battery
	}
	return nil
}
```

Then replace the inline scope block inside `GetSessionAnalytics`:

```go
	// Tier 2 users see only their own battery's slice of the analytics.
	currentUser, _ := middleware.GetUserFromContext(r.Context())
	var batteryScope *string
	if currentUser != nil && currentUser.GetTier() == models.TierBatteryNCO && currentUser.Battery != nil {
		batteryScope = currentUser.Battery
	}
```

with:

```go
	// Tier 1–2 users see only their own battery's slice of the analytics.
	currentUser, _ := middleware.GetUserFromContext(r.Context())
	batteryScope := batteryScopeForAnalytics(currentUser)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd backend && go test ./internal/handlers/ -run TestBatteryScopeForAnalytics -v`
Expected: PASS (all sub-tests).

- [ ] **Step 5: Build + full package test**

Run: `cd backend && go build ./... && go test ./internal/handlers/`
Expected: build succeeds; package tests PASS.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/handlers/reports.go backend/internal/handlers/reports_test.go
git -c user.name="David" -c user.email="ddl.tdh@gmail.com" commit -m "Scope session analytics to own battery for Tier 1 soldiers"
```

---

### Task 2: Backend — relax read-route guards for the soldier board

**Files:**
- Modify: `backend/cmd/api/main.go` (session `GET /{id}` guard; `/reports` group)

**Interfaces:**
- Consumes: `batteryScopeForAnalytics` (Task 1) is what makes the now-Tier-1-reachable analytics safe.
- Produces: `GET /api/sessions/{id}` and `GET /api/reports/sessions/{id}/analytics` reachable by any authenticated user; all other reports/session routes unchanged.

- [ ] **Step 1: Relax the single-session read**

In `backend/cmd/api/main.go`, within the `r.Route("/sessions", ...)` block, replace:

```go
					// View routes: Tier 2+
					r.With(middleware.RequireBatteryNCO(db)).Get("/", sessionHandler.ListSessions)
					r.With(middleware.RequireBatteryNCO(db)).Get("/active", sessionHandler.GetActiveSessions)
					r.With(middleware.RequireBatteryNCO(db)).Get("/{id}", sessionHandler.GetSession)
```

with:

```go
					// List/active: Tier 2+
					r.With(middleware.RequireBatteryNCO(db)).Get("/", sessionHandler.ListSessions)
					r.With(middleware.RequireBatteryNCO(db)).Get("/active", sessionHandler.GetActiveSessions)
					// Single-session metadata: Tier 1+ (needed for the read-only board)
					r.Get("/{id}", sessionHandler.GetSession)
```

- [ ] **Step 2: Relax analytics, keep other reports Tier 2+**

In `backend/cmd/api/main.go`, replace the `/reports` route group:

```go
				// Reports routes: Tier 2+
				r.Route("/reports", func(r chi.Router) {
					r.Use(middleware.RequireBatteryNCO(db))
					reportsHandler := handlers.NewReportsHandler(db)
					r.Get("/sessions/{id}/analytics", reportsHandler.GetSessionAnalytics)
					r.Get("/sessions/{id}/missing", reportsHandler.GetMissingUsers)
					r.Get("/user/{userId}", reportsHandler.GetUserReport)
					r.Get("/battery/{battery}", reportsHandler.GetBatteryReport)
				})
```

with:

```go
				// Reports routes
				r.Route("/reports", func(r chi.Router) {
					reportsHandler := handlers.NewReportsHandler(db)
					// Analytics: Tier 1+ — soldiers get a read-only, battery-scoped board.
					r.Get("/sessions/{id}/analytics", reportsHandler.GetSessionAnalytics)
					// Everything else: Tier 2+
					r.Group(func(r chi.Router) {
						r.Use(middleware.RequireBatteryNCO(db))
						r.Get("/sessions/{id}/missing", reportsHandler.GetMissingUsers)
						r.Get("/user/{userId}", reportsHandler.GetUserReport)
						r.Get("/battery/{battery}", reportsHandler.GetBatteryReport)
					})
				})
```

- [ ] **Step 3: Build + vet**

Run: `cd backend && go build ./... && go vet ./cmd/api/`
Expected: no errors.

- [ ] **Step 4: Confirm the guards moved (static check)**

Run: `cd backend && grep -n 'Get("/{id}", sessionHandler.GetSession)\|sessions/{id}/analytics\|RequireBatteryNCO' cmd/api/main.go`
Expected: `GetSession` line has **no** `RequireBatteryNCO`; the analytics line is **outside** the `r.Group(... RequireBatteryNCO ...)`; missing/user/battery reports remain inside it.

- [ ] **Step 5: Commit**

```bash
git add backend/cmd/api/main.go
git -c user.name="David" -c user.email="ddl.tdh@gmail.com" commit -m "Allow Tier 1 to read single session + analytics for read-only board"
```

---

### Task 3: Frontend — in-app scan routes to the session board

**Files:**
- Modify: `frontend/src/routes/dashboard/attendance/scan.tsx`

**Interfaces:**
- Consumes: relaxed `GET /sessions/{id}` + analytics (Tasks 1–2) so the destination board renders for any tier.
- Produces: on successful mark, a success dialog whose **View Session** action navigates to `/dashboard/sessions/{sessionId}`, where `sessionId = qrData.split(':')[0]`.

- [ ] **Step 1: Import `useNavigate` and add state**

Change the router import line:

```tsx
import { createFileRoute } from '@tanstack/react-router';
```

to:

```tsx
import { createFileRoute, useNavigate } from '@tanstack/react-router';
```

Inside `ScanAttendancePage`, add near the other `useState` hooks:

```tsx
  const navigate = useNavigate();
  const [markedSessionId, setMarkedSessionId] = useState<string | null>(null);
```

- [ ] **Step 2: Replace the auto-reset success handler**

Replace the `markMutation` `onSuccess` handler:

```tsx
    onSuccess: () => {
      setScanResult('success');
      toast.success('Attendance marked successfully!');
      setTimeout(() => {
        setScanResult(null);
        setScannedData(null);
      }, 3000);
    },
```

with (second arg is the `qrData` passed to `mutate`, shaped `sessionId:secret:timestamp`):

```tsx
    onSuccess: (_data, qrData) => {
      setScanResult('success');
      setMarkedSessionId(qrData.split(':')[0]);
      toast.success('Attendance marked successfully!');
    },
```

- [ ] **Step 3: Add the success dialog**

Immediately after the closing `</Card>` of the QR Code Scanner card (before the `canViewSessions && activeSessions...` Active Sessions block), insert:

```tsx
        <Dialog
          open={!!markedSessionId}
          onOpenChange={(open) => {
            if (!open) {
              setMarkedSessionId(null);
              setScanResult(null);
              setScannedData(null);
            }
          }}
        >
          <DialogContent className="sm:max-w-md">
            <DialogHeader>
              <div className="flex justify-center mb-2">
                <div className="rounded-full bg-green-100 dark:bg-green-900 p-3">
                  <CheckCircle2 className="h-8 w-8 text-green-600 dark:text-green-400" />
                </div>
              </div>
              <DialogTitle className="text-center text-xl">Attendance Marked!</DialogTitle>
              <DialogDescription className="text-center">
                Your attendance has been recorded.
              </DialogDescription>
            </DialogHeader>
            <div className="flex justify-center gap-2 pt-2">
              <Button
                variant="outline"
                onClick={() => {
                  setMarkedSessionId(null);
                  setScanResult(null);
                  setScannedData(null);
                }}
              >
                Scan Another
              </Button>
              <Button
                onClick={() =>
                  markedSessionId &&
                  navigate({
                    to: '/dashboard/sessions/$sessionId',
                    params: { sessionId: markedSessionId },
                  })
                }
              >
                View Session
              </Button>
            </div>
          </DialogContent>
        </Dialog>
```

(`Dialog`, `DialogContent`, `DialogHeader`, `DialogTitle`, `DialogDescription`, `Button`, and `CheckCircle2` are already imported.)

- [ ] **Step 4: Typecheck + lint**

Run: `cd frontend && npx tsc -b && npm run lint`
Expected: no type errors; lint clean for `scan.tsx`.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/routes/dashboard/attendance/scan.tsx
git -c user.name="David" -c user.email="ddl.tdh@gmail.com" commit -m "Route to session board after in-app scan"
```

---

### Task 4: Frontend — public-scan confirmation gets a View Session button

**Files:**
- Modify: `frontend/src/routes/attendance/marked.tsx`

**Interfaces:**
- Consumes: existing `search.session` (the `session` query param already validated on this route).
- Produces: primary **View Session** action → `/dashboard/sessions/{search.session}`.

- [ ] **Step 1: Add the View Session button**

Replace the action block:

```tsx
            <div className="flex flex-col gap-2 pt-4">
              <Link to="/dashboard">
                <Button className="w-full">
                  <ArrowLeft className="mr-2 h-4 w-4" />
                  Back to Dashboard
                </Button>
              </Link>
            </div>
```

with:

```tsx
            <div className="flex flex-col gap-2 pt-4">
              {search.session && (
                <Link
                  to="/dashboard/sessions/$sessionId"
                  params={{ sessionId: search.session }}
                >
                  <Button className="w-full">View Session</Button>
                </Link>
              )}
              <Link to="/dashboard">
                <Button
                  variant={search.session ? 'outline' : 'default'}
                  className="w-full"
                >
                  <ArrowLeft className="mr-2 h-4 w-4" />
                  Back to Dashboard
                </Button>
              </Link>
            </div>
```

(`Link`, `Button`, `ArrowLeft` are already imported.)

- [ ] **Step 2: Typecheck + lint**

Run: `cd frontend && npx tsc -b && npm run lint`
Expected: no type errors; lint clean for `marked.tsx`.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/routes/attendance/marked.tsx
git -c user.name="David" -c user.email="ddl.tdh@gmail.com" commit -m "Add View Session action to QR-link confirmation page"
```

---

### Task 5: Frontend — tier-conditional read-only board

**Files:**
- Modify: `frontend/src/routes/dashboard/sessions/$sessionId.tsx`

**Interfaces:**
- Consumes: `canAccessCommanderFeatures(user)` (already imported); relaxed backend reads (Tasks 1–2).
- Produces: `isCommander` boolean gating QR/Export cards, battery tabs, and per-list battery filters. Mark/unmark already gated by `canMarkAttendance`/`canUnmarkAttendance`.

- [ ] **Step 1: Derive `isCommander`**

Replace:

```tsx
  const canMarkAttendance = canAccessCommanderFeatures(user);
  const canUnmarkAttendance = canAccessCommanderFeatures(user);
```

with:

```tsx
  const isCommander = canAccessCommanderFeatures(user);
  const canMarkAttendance = isCommander;
  const canUnmarkAttendance = isCommander;
```

- [ ] **Step 2: Hide the QR Code card from soldiers**

Wrap the QR Code `<Card>` (the one titled "QR Code", first child of the `grid gap-4 md:grid-cols-2` div). Change its opening `<Card>` to `{isCommander && (` before it and add `)}` after its closing `</Card>`:

```tsx
          {isCommander && (
            <Card>
              <CardHeader>
                <CardTitle>QR Code</CardTitle>
                <CardDescription>Scan this QR code to mark attendance</CardDescription>
              </CardHeader>
              {/* ...unchanged QR card body... */}
            </Card>
          )}
```

- [ ] **Step 3: Hide the battery tabs in Statistics from soldiers**

Inside the Statistics card, wrap the `<Tabs value={statsTab} ...>` block:

```tsx
                  {isCommander && (
                    <Tabs value={statsTab} onValueChange={(v) => setStatsTab(v as typeof statsTab)}>
                      <TabsList>
                        <TabsTrigger value="All">All</TabsTrigger>
                        <TabsTrigger value="HQ">HQ</TabsTrigger>
                        <TabsTrigger value="Alpha">Alpha</TabsTrigger>
                        <TabsTrigger value="Bravo">Bravo</TabsTrigger>
                      </TabsList>
                    </Tabs>
                  )}
```

(For soldiers `statsTab` stays `'All'`; `getStats()` then returns `analytics.totalUsers`/`presentCount`, which are already battery-scoped by the backend — so the numbers are their battery's.)

- [ ] **Step 4: Hide the Export Attendance card from soldiers**

Wrap the entire `<Card>` titled "Export Attendance" with `{isCommander && ( ... )}`.

- [ ] **Step 5: Hide the battery filter Selects from soldiers**

In **both** the Missing Users card header and the Present Users card header, wrap the `<Select ...>...</Select>` control with `{isCommander && ( ... )}`. Leave the search `<Input>` and `UserTable` untouched (mark/unmark already gate on `canMarkAttendance`/`canUnmarkAttendance`).

- [ ] **Step 6: Typecheck + lint**

Run: `cd frontend && npx tsc -b && npm run lint`
Expected: no type errors; lint clean.

- [ ] **Step 7: Commit**

```bash
git add frontend/src/routes/dashboard/sessions/$sessionId.tsx
git -c user.name="David" -c user.email="ddl.tdh@gmail.com" commit -m "Render session board read-only for Tier 1 soldiers"
```

---

### Task 6: Frontend — tappable stat tiles scroll to their list

**Files:**
- Modify: `frontend/src/routes/dashboard/sessions/$sessionId.tsx`

**Interfaces:**
- Consumes: the Missing/Present list `<Card>`s from Task 5.
- Produces: `scrollToCard(id)` handler + `highlightedCard` state; stable ids `missing-users-card` / `present-users-card`; Present & Missing stat tiles as buttons.

- [ ] **Step 1: Import `cn` and `ChevronRight`**

Add to the top imports:

```tsx
import { cn } from '../../../lib/utils';
```

Add `ChevronRight` to the existing `lucide-react` import list (alongside `ArrowLeft`, `Download`, etc.).

- [ ] **Step 2: Add scroll + highlight state/handler**

Inside the component body (near the other `useState` hooks), add:

```tsx
  const [highlightedCard, setHighlightedCard] = useState<string | null>(null);
  const scrollToCard = (id: string) => {
    const el = document.getElementById(id);
    if (!el) return;
    el.scrollIntoView({ behavior: 'smooth', block: 'start' });
    setHighlightedCard(id);
    setTimeout(() => setHighlightedCard((c) => (c === id ? null : c)), 1200);
  };
```

- [ ] **Step 3: Make the Present tile tappable**

Replace the Present tile:

```tsx
                    <div>
                      <p className="text-sm text-muted-foreground">Present</p>
                      <p className="text-2xl font-bold">{stats.present}</p>
                    </div>
```

with:

```tsx
                    <button
                      type="button"
                      onClick={() => scrollToCard('present-users-card')}
                      className="text-left group"
                      aria-label="View present users list"
                    >
                      <p className="text-sm text-muted-foreground flex items-center gap-1">
                        Present
                        <ChevronRight className="h-3 w-3 opacity-60 group-hover:opacity-100" />
                      </p>
                      <p className="text-2xl font-bold">{stats.present}</p>
                    </button>
```

- [ ] **Step 4: Make the Missing tile tappable**

Replace the Missing tile:

```tsx
                  <div className="pt-4 border-t">
                    <p className="text-sm text-muted-foreground mb-2">Missing Users</p>
                    <p className="text-lg font-semibold">{stats.missing}</p>
                  </div>
```

with:

```tsx
                  <div className="pt-4 border-t">
                    <button
                      type="button"
                      onClick={() => scrollToCard('missing-users-card')}
                      className="text-left group"
                      aria-label="View missing users list"
                    >
                      <p className="text-sm text-muted-foreground mb-2 flex items-center gap-1">
                        Missing Users
                        <ChevronRight className="h-3 w-3 opacity-60 group-hover:opacity-100" />
                      </p>
                      <p className="text-lg font-semibold">{stats.missing}</p>
                    </button>
                  </div>
```

- [ ] **Step 5: Add ids + highlight ring to the list cards**

On the Missing Users `<Card>`, set:

```tsx
          <Card id="missing-users-card" className={cn(highlightedCard === 'missing-users-card' && 'ring-2 ring-primary transition-shadow')}>
```

On the Present Users `<Card>`, set:

```tsx
          <Card id="present-users-card" className={cn(highlightedCard === 'present-users-card' && 'ring-2 ring-primary transition-shadow')}>
```

- [ ] **Step 6: Typecheck + lint**

Run: `cd frontend && npx tsc -b && npm run lint`
Expected: no type errors; lint clean.

- [ ] **Step 7: Commit**

```bash
git add frontend/src/routes/dashboard/sessions/$sessionId.tsx
git -c user.name="David" -c user.email="ddl.tdh@gmail.com" commit -m "Tap Present/Missing stat tiles to scroll to their list"
```

---

### Task 7: End-to-end verification + draft PR

**Files:** none (verification only)

**Interfaces:**
- Consumes: all prior tasks.
- Produces: screenshots + a draft PR from `007-session-board-access`.

- [ ] **Step 1: Start the stack**

Run: `make restart` (Postgres + backend via docker-compose), then in a second shell `cd frontend && npm run dev`.
Expected: backend healthy on `:8080`, frontend on Vite dev port. Confirm at least one **active session** exists and note a **Tier 1** user's sign-in credentials and a **Tier 2+** user's credentials (create/seed as your data allows).

- [ ] **Step 2: Commander flow (A) — scan → board**

With `agent_browser`, sign in as a Tier 2+ user, open `/dashboard/attendance/scan`, mark via **Enter Manually** using `sessionId:secret` for the active session, then click **View Session**.
Expected: lands on `/dashboard/sessions/{sessionId}` showing QR card, battery tabs, Missing + Present lists with Mark/Unmark. Screenshot → `specs/007-session-board-access/artifacts/commander-board.png`.

- [ ] **Step 3: Soldier flow (B) — read-only board**

Sign in as a Tier 1 user; open `/dashboard/sessions/{sessionId}` directly (simulating the post-scan route).
Expected: **200** (no 403); stats + Present + Missing lists visible with status badges; **no** QR card, **no** Export card, **no** battery tabs/filter, **no** Mark/Unmark; lists contain only the soldier's battery. Screenshot → `specs/007-session-board-access/artifacts/soldier-board.png`.

- [ ] **Step 4: Mobile tile scroll (Story 3)**

Resize the `agent_browser` viewport to ~390px wide on the board; tap the **Missing Users** stat tile.
Expected: page smooth-scrolls to the Missing Users card and it briefly shows a ring highlight. Repeat for **Present**. Screenshot → `specs/007-session-board-access/artifacts/mobile-scroll.png`.

- [ ] **Step 5: Backend regression**

Run: `cd backend && go build ./... && go test ./...`
Expected: build + all tests PASS.

- [ ] **Step 6: Push branch + open draft PR**

```bash
git push -u origin 007-session-board-access
gh pr create --draft --title "Session board access after scan (007)" \
  --body "Routes scan/QR confirmations to the session board; adds a battery-scoped read-only board for Tier 1 soldiers; mobile stat tiles scroll to the missing/present lists. See specs/007-session-board-access/. Screenshots attached."
```

Attach the three screenshots to the PR. Do **not** merge to `main` (prod deploy) without explicit approval.

---

## Self-Review

**Spec coverage:**
- FR-001 (in-app scan routes to board) → Task 3. FR-002 (marked page View Session) → Task 4. FR-003 (relax two read routes) → Task 2. FR-004 (Tier <3 battery-scope) → Task 1. FR-005 (soldier read-only elements hidden) → Task 5. FR-006 (commander view unchanged) → Tasks 5–6 (all changes gated by `isCommander` / additive). FR-007 (tappable tiles scroll, a11y) → Task 6. FR-008 (additive, desktop unchanged) → Task 6. Stories 1/2/3 → Tasks 3+4 / 1+2+5 / 6. Edge cases (no-battery soldier, missing scroll target) → handled by `batteryScopeForAnalytics` nil path and `scrollToCard` `if (!el) return`.
- No spec requirement left without a task.

**Placeholder scan:** No TBD/TODO; every code step shows full code; every command has expected output.

**Type consistency:** `batteryScopeForAnalytics(user *models.User) *string` defined in Task 1 and consumed only there; `scrollToCard(id: string)` / `highlightedCard` defined and used within Task 6; `isCommander` defined in Task 5 Step 1 before all uses; route target `'/dashboard/sessions/$sessionId'` with `params.sessionId` matches the TanStack route id used elsewhere in the codebase.
