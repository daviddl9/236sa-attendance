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
	bulkNames := []string{"ALPHA", "BRAVO", "CHARLIE", "DELTA", "ECHO", "FOXTROT", "GOLF", "HOTEL", "INDIA", "JULIET", "KILO", "LIMA", "MIKE", "NOVEMBER", "OSCAR", "PAPA", "QUEBEC", "ROMEO"}
	for i := 0; i < 18; i++ {
		id := fmt.Sprintf("%s-pending-%02d", prefix, i)
		ids = append(ids, id)
		seedPending(t, db, id, fmt.Sprintf("bulk-%02d", i), bulkNames[i], "PTE", "HQ")
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

func TestBulkApproveRefusesStrongAndMigratedRows(t *testing.T) {
	db, prefix := openRegistrationDB(t)
	actorID := prefix + "-actor"
	matchedID := prefix + "-matched"
	migratedID := prefix + "-migrated"
	newID := prefix + "-new"
	seedUser(t, db, actorID, "BULK ACTOR", "SSG", "HQ", "", true)
	seedUser(t, db, matchedID, "TAN WEI MING", "CPL", "Alpha", "", true)
	seedUser(t, db, migratedID, "CARRIED PERSON", "PTE", "HQ", "", false)
	seedPending(t, db, prefix+"-match-pending", "tanwm", "TAN WEI MIMG", "LCP", "Alpha")
	seedPending(t, db, prefix+"-migrated-pending", migratedPendingUsernamePrefix+migratedID, "CARRIED PERSON", "PTE", "HQ")
	seedPending(t, db, newID, "newbulk", "BRAND NEW", "PTE", "Bravo")

	ids := []string{prefix + "-match-pending", prefix + "-migrated-pending", newID}
	body, _ := json.Marshal(map[string]any{"registrationIds": ids})
	req := httptest.NewRequest(http.MethodPost, "/api/admin/registrations/bulk-approve", strings.NewReader(string(body)))
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserKey, &models.User{ID: actorID, IsSuperadmin: true}))
	rec := httptest.NewRecorder()
	NewAdminHandler(db).BulkApproveRegistrations(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("bulk status = %d: %s", rec.Code, rec.Body.String())
	}
	var got struct {
		Results  []bulkApprovalResult `json:"results"`
		Approved int                  `json:"approved"`
		Failed   int                  `json:"failed"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Approved != 1 || got.Failed != 2 || len(got.Results) != 3 {
		t.Fatalf("result = %+v, want one approved and two refused", got)
	}
	if got.Results[0].Success || got.Results[0].Error != "needs_link" {
		t.Fatalf("strong-match result = %+v, want needs_link refusal", got.Results[0])
	}
	if got.Results[1].Success || got.Results[1].Error != "migrated_pending" {
		t.Fatalf("migrated result = %+v, want migrated_pending refusal", got.Results[1])
	}
	if !got.Results[2].Success {
		t.Fatalf("new-row result = %+v, want success", got.Results[2])
	}

	ctx := context.Background()
	var matchedUsers, newUsers, pending int
	if err := db.Pool.QueryRow(ctx, `SELECT count(*) FROM "user" WHERE id = $1`, matchedID).Scan(&matchedUsers); err != nil {
		t.Fatal(err)
	}
	if err := db.Pool.QueryRow(ctx, `SELECT count(*) FROM "user" WHERE id = $1`, newID).Scan(&newUsers); err != nil {
		t.Fatal(err)
	}
	if err := db.Pool.QueryRow(ctx, `SELECT count(*) FROM pending_registration WHERE id = ANY($1)`, ids).Scan(&pending); err != nil {
		t.Fatal(err)
	}
	if matchedUsers != 1 || newUsers != 1 || pending != 2 {
		t.Fatalf("matched users=%d new users=%d pending=%d, want 1, 1, 2", matchedUsers, newUsers, pending)
	}
}

func TestDeleteUserWithCredentialAudit(t *testing.T) {
	db, prefix := openRegistrationDB(t)
	actorID := prefix + "-actor"
	targetID := prefix + "-target"
	seedUser(t, db, actorID, "AUDIT ACTOR", "SSG", "HQ", "", true)
	seedUser(t, db, targetID, "AUDITED TARGET", "PTE", "HQ", "audited", true)
	seedCredentialAudit(t, db, prefix+"-audit", actorID, targetID)

	req := httptest.NewRequest(http.MethodDelete, "/api/users/"+targetID, nil)
	rc := chi.NewRouteContext()
	rc.URLParams.Add("id", targetID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rc))
	rec := httptest.NewRecorder()
	NewUserHandler(db).DeleteUser(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete status = %d: %s", rec.Code, rec.Body.String())
	}

	var users, audits int
	ctx := context.Background()
	if err := db.Pool.QueryRow(ctx, `SELECT count(*) FROM "user" WHERE id = $1`, targetID).Scan(&users); err != nil {
		t.Fatal(err)
	}
	if err := db.Pool.QueryRow(ctx, `SELECT count(*) FROM credential_audit WHERE id = $1`, prefix+"-audit").Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if users != 0 || audits != 0 {
		t.Fatalf("deleted user/audit counts = (%d, %d), want (0, 0)", users, audits)
	}
}

func TestBulkDeleteUsersWithAuditedAndUnauditedTargets(t *testing.T) {
	db, prefix := openRegistrationDB(t)
	actorID := prefix + "-actor"
	auditedID := prefix + "-audited"
	plainID := prefix + "-plain"
	seedUser(t, db, actorID, "BULK ACTOR", "SSG", "HQ", "", true)
	seedUser(t, db, auditedID, "AUDITED TARGET", "PTE", "HQ", "audited", true)
	seedUser(t, db, plainID, "PLAIN TARGET", "PTE", "HQ", "plain", true)
	seedCredentialAudit(t, db, prefix+"-audit", actorID, auditedID)

	body := fmt.Sprintf(`{"userIds":[%q,%q]}`, auditedID, plainID)
	req := httptest.NewRequest(http.MethodPost, "/api/users/bulk-delete", strings.NewReader(body))
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserKey, &models.User{ID: actorID, IsSuperadmin: true}))
	rec := httptest.NewRecorder()
	NewUserHandler(db).BulkDeleteUsers(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("bulk delete status = %d: %s", rec.Code, rec.Body.String())
	}
	var response BulkDeleteResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.Summary.Deleted != 2 || response.Summary.Failed != 0 {
		t.Fatalf("bulk delete summary = %+v, want two deleted and no failures", response.Summary)
	}
	var remaining int
	if err := db.Pool.QueryRow(context.Background(), `SELECT count(*) FROM "user" WHERE id = ANY($1)`, []string{auditedID, plainID}).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 0 {
		t.Fatalf("remaining targets = %d, want 0", remaining)
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

func TestPasswordChangeRequiredBlocksQRScan(t *testing.T) {
	db, prefix := openRegistrationDB(t)
	userID := prefix + "-qr-user"
	sessionID := prefix + "-qr-session"
	token := prefix + "-session-token"
	secret := prefix + "-secret"
	seedUser(t, db, userID, "QR USER", "PTE", "HQ", "qruser", true)
	if _, err := db.Pool.Exec(context.Background(), `UPDATE "user" SET password_change_required = true WHERE id = $1`, userID); err != nil {
		t.Fatal(err)
	}
	_, err := db.Pool.Exec(context.Background(), `
		INSERT INTO attendance_session (id, name, qr_code, qr_code_secret, scope, batteries, status, created_by)
		VALUES ($1, 'QR test', $2, $3, 'unit_wide', '{}', 'active', $4)
	`, sessionID, sessionID+"-code", secret, userID)
	if err != nil {
		t.Fatalf("seed QR session: %v", err)
	}
	_, err = db.Pool.Exec(context.Background(), `
		INSERT INTO session (id, "expiresAt", token, "userId", "createdAt", "updatedAt")
		VALUES ($1, NOW() + INTERVAL '1 hour', $2, $3, NOW(), NOW())
	`, prefix+"-session", token, userID)
	if err != nil {
		t.Fatalf("seed auth session: %v", err)
	}

	qrToken := sessionID + ":" + secret
	req := httptest.NewRequest(http.MethodGet, "/api/qr/"+qrToken, nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	rc := chi.NewRouteContext()
	rc.URLParams.Add("token", qrToken)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rc))
	rec := httptest.NewRecorder()
	NewAttendanceHandler(db, nil).HandleQRScan(rec, req)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "Password change required") {
		t.Fatalf("QR response = (%d, %q), want 403 password-change error", rec.Code, rec.Body.String())
	}
	var records int
	if err := db.Pool.QueryRow(context.Background(), `SELECT count(*) FROM attendance_record WHERE session_id = $1`, sessionID).Scan(&records); err != nil {
		t.Fatal(err)
	}
	if records != 0 {
		t.Fatalf("QR records = %d, want 0", records)
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
	same := httptest.NewRequest(http.MethodPost, "/api/auth/change-password", strings.NewReader(fmt.Sprintf(`{"password":%q,"confirmPassword":%q}`, second.TemporaryPassword, second.TemporaryPassword)))
	same = same.WithContext(context.WithValue(same.Context(), middleware.UserIDKey, targetID))
	sameOut := httptest.NewRecorder()
	NewAuthHandler(db).ChangePassword(sameOut, same)
	if sameOut.Code != http.StatusBadRequest || !strings.Contains(sameOut.Body.String(), "differ") {
		t.Fatalf("reusing temporary password = (%d, %q), want 400 differing-password error", sameOut.Code, sameOut.Body.String())
	}
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

func seedCredentialAudit(t *testing.T, db *database.DB, id, actorID, targetID string) {
	t.Helper()
	_, err := db.Pool.Exec(context.Background(), `
		INSERT INTO credential_audit (id, actor_user_id, target_user_id, action)
		VALUES ($1, $2, $3, 'credential_provision')
	`, id, actorID, targetID)
	if err != nil {
		t.Fatalf("seed credential audit: %v", err)
	}
}
