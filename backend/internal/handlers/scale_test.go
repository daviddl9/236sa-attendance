package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/davidlivingston/go-nextjs-starter/backend/internal/database"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/middleware"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/models"
	"github.com/go-chi/chi/v5"
)

func TestBulkApprovePartialSuccess(t *testing.T) {
	db, prefix := openRegistrationDB(t)
	ids := make([]string, 0, 20)
	for i := 0; i < 18; i++ {
		id := fmt.Sprintf("%s-pending-%02d", prefix, i)
		ids = append(ids, id)
		seedPending(t, db, id, fmt.Sprintf("bulk-%02d", i), "PERSON", "PTE", "HQ")
	}
	ids = append(ids, prefix+"-invalid-1", prefix+"-invalid-2")
	body, _ := json.Marshal(map[string]any{"registrationIds": ids})
	rec := httptest.NewRecorder()
	NewAdminHandler(db).BulkApproveRegistrations(rec, httptest.NewRequest(http.MethodPost, "/api/admin/registrations/bulk-approve", strings.NewReader(string(body))))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var got struct {
		Results  []bulkApprovalResult `json:"results"`
		Approved int                  `json:"approved"`
		Failed   int                  `json:"failed"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Approved != 18 || got.Failed != 2 || len(got.Results) != 20 {
		t.Fatalf("result = %+v, want 18 approved and 2 failed", got)
	}
	var users, pending int
	ctx := context.Background()
	_ = db.Pool.QueryRow(ctx, `SELECT count(*) FROM "user" WHERE id LIKE $1`, prefix+"-pending-%").Scan(&users)
	_ = db.Pool.QueryRow(ctx, `SELECT count(*) FROM pending_registration WHERE id LIKE $1`, prefix+"-%").Scan(&pending)
	if users != 18 || pending != 0 {
		t.Fatalf("users=%d pending=%d, want 18 and 0", users, pending)
	}
}

func TestListPendingRegistrationsFiltersAndScopesBattery(t *testing.T) {
	db, prefix := openRegistrationDB(t)
	for _, battery := range []string{models.BatteryHQ, models.BatteryAlpha, models.BatteryBravo} {
		seedPending(t, db, prefix+"-"+battery, battery+"-user", battery+" PERSON", "PTE", battery)
	}
	h := NewAdminHandler(db)
	rec := httptest.NewRecorder()
	h.ListPendingRegistrations(rec, httptest.NewRequest(http.MethodGet, "/api/admin/registrations?battery=Alpha", nil))
	var got struct {
		Registrations []PendingRegistration `json:"registrations"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&got)
	if rec.Code != http.StatusOK || len(got.Registrations) != 1 || got.Registrations[0].Battery != models.BatteryAlpha {
		t.Fatalf("filtered response = (%d, %+v)", rec.Code, got.Registrations)
	}
	commander := &models.User{ID: prefix + "-commander", Rank: stringPtr("3SG"), Battery: stringPtr(models.BatteryAlpha)}
	req := httptest.NewRequest(http.MethodGet, "/api/admin/registrations?battery=Bravo", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserKey, commander))
	rec = httptest.NewRecorder()
	h.ListPendingRegistrations(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("out-of-scope status = %d, want 403", rec.Code)
	}
}

func TestProvisionCredentialsAndForcedChange(t *testing.T) {
	db, prefix := openRegistrationDB(t)
	targetID := prefix + "-target"
	seedUser(t, db, targetID, "TARGET PERSON", "PTE", "Alpha", "", true)
	seedUser(t, db, prefix+"-taken", "TAKEN PERSON", "PTE", "HQ", "TanWM ", true)
	seedPending(t, db, prefix+"-pending", "ReservedName", "PENDING PERSON", "PTE", "HQ")
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(context.Background(), `DELETE FROM credential_audit WHERE actor_user_id = $1`, targetID)
	})
	actor := &models.User{ID: targetID, IsSuperadmin: true}
	h := NewAdminHandler(db)
	for _, username := range []string{"tanwm", " reservedname "} {
		if rec := provisionAttempt(t, h, targetID, username, actor); rec.Code != http.StatusConflict {
			t.Fatalf("duplicate %q status = %d, want 409", username, rec.Code)
		}
	}
	first := provision(t, h, targetID, "TargetUser", actor)
	if len(first.TemporaryPassword) < 12 || regexp.MustCompile(`^\d{4}[A-Za-z]$`).MatchString(first.TemporaryPassword) || strings.ContainsAny(first.TemporaryPassword, "0O1lI") {
		t.Fatalf("temporary password = %q", first.TemporaryPassword)
	}
	assertSignIn(t, db, "targetuser", first.TemporaryPassword, http.StatusOK)
	if !passwordChangeRequired(t, db, targetID) {
		t.Fatal("temporary sign-in did not require a password change")
	}
	second := provision(t, h, targetID, "TargetUser", actor)
	assertSignIn(t, db, "targetuser", first.TemporaryPassword, http.StatusUnauthorized)
	assertSignIn(t, db, "targetuser", second.TemporaryPassword, http.StatusOK)
	change := httptest.NewRequest(http.MethodPost, "/api/auth/change-password", strings.NewReader(`{"password":"new secure password","confirmPassword":"new secure password"}`))
	change = change.WithContext(context.WithValue(change.Context(), middleware.UserIDKey, targetID))
	out := httptest.NewRecorder()
	NewAuthHandler(db).ChangePassword(out, change)
	if out.Code != http.StatusOK || passwordChangeRequired(t, db, targetID) {
		t.Fatalf("change response = %d or flag remains set", out.Code)
	}
	assertSignIn(t, db, "targetuser", second.TemporaryPassword, http.StatusUnauthorized)
	assertSignIn(t, db, "targetuser", "new secure password", http.StatusOK)
	bad := httptest.NewRequest(http.MethodPost, "/api/auth/change-password", strings.NewReader(`{"password":"1234A","confirmPassword":"1234A"}`))
	bad = bad.WithContext(context.WithValue(bad.Context(), middleware.UserIDKey, targetID))
	badOut := httptest.NewRecorder()
	NewAuthHandler(db).ChangePassword(badOut, bad)
	if badOut.Code != http.StatusBadRequest || !strings.Contains(badOut.Body.String(), "Do not use your NRIC digits as your password") {
		t.Fatalf("NRIC-shaped response = (%d, %q)", badOut.Code, badOut.Body.String())
	}
	var audits int
	_ = db.Pool.QueryRow(context.Background(), `SELECT count(*) FROM credential_audit WHERE actor_user_id = $1`, targetID).Scan(&audits)
	if audits != 2 {
		t.Fatalf("audit rows = %d, want 2", audits)
	}
}

func provisionAttempt(t *testing.T, h *AdminHandler, id, username string, actor *models.User) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ProvisionCredentials(rec, credentialRequest(id, username, actor))
	return rec
}

func provision(t *testing.T, h *AdminHandler, id, username string, actor *models.User) ProvisionCredentialsResponse {
	t.Helper()
	rec := provisionAttempt(t, h, id, username, actor)
	if rec.Code != http.StatusOK {
		t.Fatalf("provision status = %d: %s", rec.Code, rec.Body.String())
	}
	var response ProvisionCredentialsResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	return response
}

func credentialRequest(id, username string, actor *models.User) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/admin/users/"+id+"/credentials", strings.NewReader(fmt.Sprintf(`{"username":%q}`, username)))
	rc := chi.NewRouteContext()
	rc.URLParams.Add("id", id)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rc))
	return req.WithContext(context.WithValue(req.Context(), middleware.UserKey, actor))
}

func assertSignIn(t *testing.T, db *database.DB, username, password string, want int) {
	t.Helper()
	if got := authSignIn(t, NewAuthHandler(db), username, password).Code; got != want {
		t.Fatalf("sign-in %q status = %d, want %d", username, got, want)
	}
}

func passwordChangeRequired(t *testing.T, db *database.DB, id string) bool {
	t.Helper()
	var required bool
	if err := db.Pool.QueryRow(context.Background(), `SELECT password_change_required FROM "user" WHERE id = $1`, id).Scan(&required); err != nil {
		t.Fatal(err)
	}
	return required
}

func stringPtr(value string) *string { return &value }
