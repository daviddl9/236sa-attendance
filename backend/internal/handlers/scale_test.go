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
