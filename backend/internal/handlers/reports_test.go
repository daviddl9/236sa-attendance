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
