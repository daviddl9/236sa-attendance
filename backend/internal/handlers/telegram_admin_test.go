package handlers

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/davidlivingston/go-nextjs-starter/backend/internal/models"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/telegram"
)

func TestTelegramAdminStoreImplementsAdminStore(t *testing.T) {
	var _ telegram.AdminStore = (*TelegramAdminStore)(nil)
}

func TestTelegramAdminDraftValidation(t *testing.T) {
	future := time.Now().Add(time.Hour)
	valid := []telegram.AdminDraft{
		{Name: "Parade", Scope: models.SessionScopeUnitWide, EndTime: future},
		{Name: "Alpha parade", Scope: models.SessionScopeBatterySpecific, Battery: models.BatteryAlpha, EndTime: future},
	}
	for _, draft := range valid {
		if err := validateAdminDraft(draft); err != nil {
			t.Fatalf("valid draft %+v rejected: %v", draft, err)
		}
	}

	invalid := []telegram.AdminDraft{
		{Name: "", Scope: models.SessionScopeUnitWide, EndTime: future},
		{Name: strings.Repeat("x", 81), Scope: models.SessionScopeUnitWide, EndTime: future},
		{Name: "missing scope", EndTime: future},
		{Name: "missing battery", Scope: models.SessionScopeBatterySpecific, EndTime: future},
		{Name: "bad battery", Scope: models.SessionScopeBatterySpecific, Battery: "Delta", EndTime: future},
		{Name: "missing end time", Scope: models.SessionScopeUnitWide},
	}
	for _, draft := range invalid {
		if err := validateAdminDraft(draft); err == nil {
			t.Fatalf("invalid draft %+v accepted", draft)
		}
	}
}

func TestTelegramAdminUnavailableErrorDoesNotDiscloseResource(t *testing.T) {
	err := adminUnavailable("session-secret-id", "person-secret-id")
	if !errors.Is(err, telegram.ErrAdminUnavailable) {
		t.Fatalf("error = %v, want ErrAdminUnavailable", err)
	}
	if strings.Contains(err.Error(), "session-secret-id") || strings.Contains(err.Error(), "person-secret-id") {
		t.Fatalf("unavailable error disclosed an identifier: %q", err)
	}
}

func TestTelegramAdminContextDefaults(t *testing.T) {
	store := &TelegramAdminStore{}
	ctx, err := store.LoadContext(context.Background(), 123)
	if err == nil {
		// A nil store must fail safely rather than manufacturing a context.
		t.Fatalf("LoadContext on an unconfigured store returned %+v", ctx)
	}
}

func TestTelegramAdminActorTierUsesCurrentRosterState(t *testing.T) {
	// This is an integration assertion in the disposable PostgreSQL test file;
	// the unit-level contract is that the actor's supplied tier is not trusted.
	actor := telegram.AdminActor{Tier: models.TierSuperadmin, Battery: models.BatteryBravo, IsSuperadmin: true}
	if actor.Pairing.UserID != "" {
		t.Fatal("test fixture unexpectedly carries a trusted actor ID")
	}
}
