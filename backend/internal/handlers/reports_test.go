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
