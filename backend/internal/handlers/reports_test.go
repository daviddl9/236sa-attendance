package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/davidlivingston/go-nextjs-starter/backend/internal/database"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/middleware"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/models"
	"github.com/go-chi/chi/v5"
)

func strptr(s string) *string { return &s }

func TestGetMissingUsersExcludesUnverifiedUsers(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run the database regression test")
	}

	db, err := database.NewPostgresDB(databaseURL)
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	prefix := fmt.Sprintf("missing-regression-%d", time.Now().UnixNano())
	creatorID := prefix + "-creator"
	sessionID := prefix + "-session"
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO "user" (id, "full_name", rank, battery, password, extras, verified, "is_superadmin", "createdAt", "updatedAt")
		VALUES ($1, $2, 'CPT', 'HQ', 'test', '{}'::jsonb, true, true, NOW(), NOW())
	`, creatorID, creatorID); err != nil {
		t.Fatalf("insert creator: %v", err)
	}
	defer func() {
		_, _ = db.Pool.Exec(ctx, `DELETE FROM attendance_session WHERE id = $1`, sessionID)
		_, _ = db.Pool.Exec(ctx, `DELETE FROM "user" WHERE id LIKE $1`, prefix+"-%")
	}()

	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("%s-roster-%d", prefix, i)
		if _, err := db.Pool.Exec(ctx, `
			INSERT INTO "user" (id, "full_name", rank, battery, password, extras, verified, "is_superadmin", "createdAt", "updatedAt")
			VALUES ($1, $2, 'PTE', 'HQ', 'test', '{}'::jsonb, true, false, NOW(), NOW())
		`, id, id); err != nil {
			t.Fatalf("insert roster user %d: %v", i, err)
		}
	}
	for i := 0; i < 3; i++ {
		id := fmt.Sprintf("%s-pending-%d", prefix, i)
		if _, err := db.Pool.Exec(ctx, `
			INSERT INTO "user" (id, "full_name", rank, battery, password, extras, verified, "is_superadmin", "createdAt", "updatedAt")
			VALUES ($1, $2, 'PTE', 'HQ', 'test', '{}'::jsonb, false, false, NOW(), NOW())
		`, id, id); err != nil {
			t.Fatalf("insert pending user %d: %v", i, err)
		}
	}
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO attendance_session (id, name, qr_code, qr_code_secret, scope, batteries, status, created_by, start_time)
		VALUES ($1, 'Missing regression', $2, $3, 'unit_wide', '{}', 'active', $4, NOW())
	`, sessionID, prefix+"-qr", prefix+"-secret", creatorID); err != nil {
		t.Fatalf("insert session: %v", err)
	}

	routeContext := chi.NewRouteContext()
	routeContext.URLParams.Add("id", sessionID)
	req := httptest.NewRequest(http.MethodGet, "/api/reports/sessions/"+sessionID+"/missing", nil)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeContext))
	rec := httptest.NewRecorder()
	NewReportsHandler(db).GetMissingUsers(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	var missing []UserInfo
	if err := json.NewDecoder(rec.Body).Decode(&missing); err != nil {
		t.Fatalf("decode missing users: %v", err)
	}
	// Count only this test's own rows. Asserting the total made the test fail
	// against any database that already had personnel in it, which both produced
	// spurious failures and could mask a real regression.
	var seeded, pendingLeaked int
	for _, user := range missing {
		if !strings.HasPrefix(user.ID, prefix) {
			continue
		}
		seeded++
		if strings.Contains(user.ID, "-pending-") {
			pendingLeaked++
		}
	}
	if seeded != 5 || pendingLeaked != 0 {
		t.Fatalf("seeded missing = %d (want 5), unverified leaked = %d (want 0); unverified rows must not count as absent", seeded, pendingLeaked)
	}
}

func TestGetSessionAnalyticsPreservesCountsBreakdownsAndMissingStatus(t *testing.T) {
	db, prefix := openRegistrationDB(t)
	ctx := context.Background()
	creatorID := prefix + "-creator"
	presentID := prefix + "-present"
	missingAlphaID := prefix + "-missing-alpha"
	missingBravoID := prefix + "-missing-bravo"
	sessionID := prefix + "-session"
	statusID := prefix + "-status"
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(context.Background(), `DELETE FROM user_status WHERE id = $1`, statusID)
	})

	seedUser(t, db, creatorID, "Analytics creator", models.RankCPT, models.BatteryHQ, "", true)
	seedUser(t, db, presentID, "Alice Present", models.RankPTE, models.BatteryAlpha, "", true)
	seedUser(t, db, missingAlphaID, "Alicia Missing", models.RankPTE, models.BatteryAlpha, "", true)
	seedUser(t, db, missingBravoID, "Bruno Missing", models.RankLCP, models.BatteryBravo, "", true)
	if _, err := db.Pool.Exec(ctx, `UPDATE "user" SET "is_superadmin" = true WHERE id = $1`, creatorID); err != nil {
		t.Fatalf("mark analytics creator superadmin: %v", err)
	}
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO attendance_session (id, name, qr_code, qr_code_secret, scope, batteries, status, created_by, start_time)
		VALUES ($1, 'Analytics regression', $2, $3, 'custom_list', '{}', 'active', $4, NOW())
	`, sessionID, sessionID+"-qr", sessionID+"-secret", creatorID); err != nil {
		t.Fatalf("insert analytics session: %v", err)
	}
	for _, userID := range []string{presentID, missingAlphaID, missingBravoID} {
		if _, err := db.Pool.Exec(ctx, `
			INSERT INTO session_participants (session_id, user_id) VALUES ($1, $2)
		`, sessionID, userID); err != nil {
			t.Fatalf("insert analytics participant %s: %v", userID, err)
		}
	}
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO attendance_record (id, session_id, user_id, marking_method, marked_at)
		VALUES ($1, $2, $3, 'qr_scan', NOW())
	`, sessionID+"-record", sessionID, presentID); err != nil {
		t.Fatalf("insert present attendance: %v", err)
	}
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO user_status (id, user_id, status_type, start_date, end_date, created_by)
		VALUES ($1, $2, 'mc', CURRENT_DATE - 1, CURRENT_DATE + 1, $3)
	`, statusID, missingAlphaID, creatorID); err != nil {
		t.Fatalf("insert active missing-user status: %v", err)
	}

	actor := &models.User{ID: creatorID, IsSuperadmin: true}
	routeContext := chi.NewRouteContext()
	routeContext.URLParams.Add("id", sessionID)
	req := httptest.NewRequest(http.MethodGet, "/api/reports/sessions/"+sessionID, nil)
	req = req.WithContext(context.WithValue(context.WithValue(req.Context(), chi.RouteCtxKey, routeContext), middleware.UserKey, actor))
	rec := httptest.NewRecorder()
	NewReportsHandler(db).GetSessionAnalytics(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("analytics status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	var analytics SessionAnalytics
	if err := json.NewDecoder(rec.Body).Decode(&analytics); err != nil {
		t.Fatalf("decode analytics response: %v", err)
	}
	if analytics.TotalUsers != 3 || analytics.PresentCount != 1 {
		t.Fatalf("analytics counts = (total %d, present %d), want (3, 1)", analytics.TotalUsers, analytics.PresentCount)
	}
	if want := float64(1) / 3 * 100; analytics.AttendancePercentage != want {
		t.Fatalf("attendance percentage = %v, want %v", analytics.AttendancePercentage, want)
	}
	if len(analytics.PresentUsers) != 1 || analytics.PresentUsers[0].ID != presentID {
		t.Fatalf("present users = %+v, want only %s", analytics.PresentUsers, presentID)
	}
	if len(analytics.MissingUsers) != 2 {
		t.Fatalf("missing users = %+v, want 2 rows", analytics.MissingUsers)
	}
	missingByID := make(map[string]UserInfo, len(analytics.MissingUsers))
	for _, user := range analytics.MissingUsers {
		missingByID[user.ID] = user
	}
	missingAlpha, ok := missingByID[missingAlphaID]
	if !ok || missingAlpha.ActiveStatus == nil || missingAlpha.ActiveStatus.StatusType != "mc" {
		t.Fatalf("missing Alpha status = %+v, want active mc status", missingAlpha.ActiveStatus)
	}
	if _, ok := missingByID[missingBravoID]; !ok {
		t.Fatalf("missing users omitted %s: %+v", missingBravoID, analytics.MissingUsers)
	}
	if got := analytics.ByBattery[models.BatteryAlpha]; got != (BatteryStats{Total: 2, Present: 1}) {
		t.Fatalf("Alpha breakdown = %+v, want total 2/present 1", got)
	}
	if got := analytics.ByBattery[models.BatteryBravo]; got != (BatteryStats{Total: 1, Present: 0}) {
		t.Fatalf("Bravo breakdown = %+v, want total 1/present 0", got)
	}
	if got := analytics.ByRank[models.RankPTE]; got != (RankStats{Total: 2, Present: 1}) {
		t.Fatalf("PTE breakdown = %+v, want total 2/present 1", got)
	}
	if got := analytics.ByRank[models.RankLCP]; got != (RankStats{Total: 1, Present: 0}) {
		t.Fatalf("LCP breakdown = %+v, want total 1/present 0", got)
	}
}

func TestGetSessionAnalyticsCountsWalkInsAsPresentOnly(t *testing.T) {
	db, prefix := openRegistrationDB(t)
	ctx := context.Background()
	creatorID := prefix + "-creator"
	presentID := prefix + "-present"
	missingID := prefix + "-missing"
	walkInID := prefix + "-walk-in"
	sessionID := prefix + "-session"

	seedUser(t, db, creatorID, "Analytics creator", models.RankCPT, models.BatteryHQ, "", true)
	seedUser(t, db, presentID, "Alice Present", models.RankPTE, models.BatteryAlpha, "", true)
	seedUser(t, db, missingID, "Alicia Missing", models.RankPTE, models.BatteryAlpha, "", true)
	seedUser(t, db, walkInID, "Walter Walkin", models.RankLCP, models.BatteryBravo, "", true)
	if _, err := db.Pool.Exec(ctx, `UPDATE "user" SET "is_superadmin" = true WHERE id = $1`, creatorID); err != nil {
		t.Fatalf("mark analytics creator superadmin: %v", err)
	}
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO attendance_session (id, name, qr_code, qr_code_secret, scope, batteries, status, created_by, start_time)
		VALUES ($1, 'Walk-in analytics', $2, $3, 'custom_list', '{}', 'active', $4, NOW())
	`, sessionID, sessionID+"-qr", sessionID+"-secret", creatorID); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	for _, userID := range []string{presentID, missingID} {
		if _, err := db.Pool.Exec(ctx, `
			INSERT INTO session_participants (session_id, user_id) VALUES ($1, $2)
		`, sessionID, userID); err != nil {
			t.Fatalf("insert participant %s: %v", userID, err)
		}
	}
	for i, userID := range []string{presentID, walkInID} {
		if _, err := db.Pool.Exec(ctx, `
			INSERT INTO attendance_record (id, session_id, user_id, marking_method, marked_at)
			VALUES ($1, $2, $3, 'qr_scan', NOW())
		`, fmt.Sprintf("%s-record-%d", sessionID, i), sessionID, userID); err != nil {
			t.Fatalf("insert attendance %s: %v", userID, err)
		}
	}

	actor := &models.User{ID: creatorID, IsSuperadmin: true}
	routeContext := chi.NewRouteContext()
	routeContext.URLParams.Add("id", sessionID)
	req := httptest.NewRequest(http.MethodGet, "/api/reports/sessions/"+sessionID, nil)
	req = req.WithContext(context.WithValue(context.WithValue(req.Context(), chi.RouteCtxKey, routeContext), middleware.UserKey, actor))
	rec := httptest.NewRecorder()
	NewReportsHandler(db).GetSessionAnalytics(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("analytics status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	var analytics SessionAnalytics
	if err := json.NewDecoder(rec.Body).Decode(&analytics); err != nil {
		t.Fatalf("decode analytics response: %v", err)
	}
	if analytics.TotalUsers != 3 || analytics.PresentCount != 2 {
		t.Fatalf("analytics counts = (total %d, present %d), want (3, 2)", analytics.TotalUsers, analytics.PresentCount)
	}
	presentByID := make(map[string]UserInfo, len(analytics.PresentUsers))
	for _, user := range analytics.PresentUsers {
		presentByID[user.ID] = user
	}
	if len(analytics.PresentUsers) != 2 {
		t.Fatalf("present users = %+v, want roster present + walk-in", analytics.PresentUsers)
	}
	if _, ok := presentByID[presentID]; !ok {
		t.Fatalf("present users omitted roster member %s: %+v", presentID, analytics.PresentUsers)
	}
	if walkIn, ok := presentByID[walkInID]; !ok || walkIn.FullName == nil || *walkIn.FullName != "Walter Walkin" {
		t.Fatalf("present users omitted walk-in %s: %+v", walkInID, analytics.PresentUsers)
	}
	// The walk-in is never absent.
	if len(analytics.MissingUsers) != 1 || analytics.MissingUsers[0].ID != missingID {
		t.Fatalf("missing users = %+v, want only %s", analytics.MissingUsers, missingID)
	}
	if got := analytics.ByBattery[models.BatteryBravo]; got != (BatteryStats{Total: 1, Present: 1}) {
		t.Fatalf("Bravo breakdown = %+v, want total 1/present 1 (walk-in)", got)
	}
}

func TestGetActiveSessionsPreservesNullForNoRows(t *testing.T) {
	db, _ := openRegistrationDB(t)
	actor := &models.User{Rank: strptr(models.Rank3SG)}
	req := httptest.NewRequest(http.MethodGet, "/api/sessions/active", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserKey, actor))
	rec := httptest.NewRecorder()
	NewSessionHandler(db, nil).GetActiveSessions(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("active sessions status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if got := strings.TrimSpace(rec.Body.String()); got != "null" {
		t.Fatalf("empty active sessions JSON = %q, want null", got)
	}
}

func TestGetMissingUsersPreservesNullForNoRows(t *testing.T) {
	db, prefix := openRegistrationDB(t)
	ctx := context.Background()
	creatorID := prefix + "-creator"
	sessionID := prefix + "-session"
	seedUser(t, db, creatorID, "Empty custom creator", models.RankSSG, models.BatteryHQ, "", true)
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO attendance_session (id, name, qr_code, qr_code_secret, scope, batteries, status, created_by, start_time)
		VALUES ($1, 'Empty custom', $2, $3, 'custom_list', '{}', 'active', $4, NOW())
	`, sessionID, sessionID+"-qr", sessionID+"-secret", creatorID); err != nil {
		t.Fatalf("insert empty custom session: %v", err)
	}

	routeContext := chi.NewRouteContext()
	routeContext.URLParams.Add("id", sessionID)
	actor := &models.User{ID: creatorID, Rank: strptr(models.RankSSG)}
	req := httptest.NewRequest(http.MethodGet, "/api/reports/sessions/"+sessionID+"/missing", nil)
	req = req.WithContext(context.WithValue(context.WithValue(req.Context(), chi.RouteCtxKey, routeContext), middleware.UserKey, actor))
	rec := httptest.NewRecorder()
	NewReportsHandler(db).GetMissingUsers(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("missing users status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if got := strings.TrimSpace(rec.Body.String()); got != "null" {
		t.Fatalf("empty missing users JSON = %q, want null", got)
	}
}

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
		{"enlisted without battery gets empty scope (no roster)", &models.User{Rank: strptr(models.RankREC)}, strptr("")},
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
