package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/davidlivingston/go-nextjs-starter/backend/internal/database"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/models"
	"github.com/go-chi/chi/v5"
)

// seedActiveSession inserts an active, unit-wide session with a known QR
// secret, backed by a creator user so the created_by FK is satisfied.
func seedActiveSession(t *testing.T, db *database.DB, sessionID, secret string) {
	t.Helper()
	creatorID := sessionID + "-creator"
	seedUser(t, db, creatorID, "SESSION CREATOR", "CPT", "HQ", "", true)
	_, err := db.Pool.Exec(context.Background(), `
		INSERT INTO attendance_session (id, name, qr_code, qr_code_secret, scope, status, created_by, start_time)
		VALUES ($1, 'Parade', $2, $3, 'unit_wide', 'active', $4, NOW())
	`, sessionID, sessionID+"-qr", secret, creatorID)
	if err != nil {
		t.Fatalf("seed active session: %v", err)
	}
}

func seedPendingWithQR(t *testing.T, db *database.DB, id, username, name, rank, battery, sessionID, secret string) {
	t.Helper()
	seedPending(t, db, id, username, name, rank, battery)
	_, err := db.Pool.Exec(context.Background(), `
		UPDATE pending_registration SET qr_session_id = $1, qr_secret = $2 WHERE id = $3
	`, sessionID, secret, id)
	if err != nil {
		t.Fatalf("attach qr token to pending: %v", err)
	}
}

func countRecords(t *testing.T, db *database.DB, sessionID, userID string) int {
	t.Helper()
	var count int
	if err := db.Pool.QueryRow(context.Background(), `
		SELECT count(*) FROM attendance_record WHERE session_id = $1 AND user_id = $2
	`, sessionID, userID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func recordMethod(t *testing.T, db *database.DB, sessionID, userID string) string {
	t.Helper()
	var method string
	if err := db.Pool.QueryRow(context.Background(), `
		SELECT marking_method FROM attendance_record WHERE session_id = $1 AND user_id = $2
	`, sessionID, userID).Scan(&method); err != nil {
		t.Fatalf("load marking method: %v", err)
	}
	return method
}

// TestApproveCreateAutoMarksAttendance: approving a create-mode registration
// that carried a QR token marks attendance for the created user in the same
// transaction.
func TestApproveCreateAutoMarksAttendance(t *testing.T) {
	db, prefix := openRegistrationDB(t)
	ctx := context.Background()
	pendingID := prefix + "-pending"
	sessionID := prefix + "-session"
	secret := prefix + "-secret"
	seedActiveSession(t, db, sessionID, secret)
	seedPendingWithQR(t, db, pendingID, "newuser", "NEW PERSON", "PTE", "Bravo", sessionID, secret)

	rec := approve(t, NewAdminHandler(db), pendingID, `{"mode":"create"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if count := countRecords(t, db, sessionID, pendingID); count != 1 {
		t.Fatalf("attendance records = %d, want 1", count)
	}
	if method := recordMethod(t, db, sessionID, pendingID); method != models.MarkingMethodApprovalAuto {
		t.Fatalf("marking method = %q, want %q", method, models.MarkingMethodApprovalAuto)
	}
	// The auto-mark must be committed atomically with the approval.
	var pending int
	if err := db.Pool.QueryRow(ctx, `SELECT count(*) FROM pending_registration WHERE id = $1`, pendingID).Scan(&pending); err != nil {
		t.Fatal(err)
	}
	if pending != 0 {
		t.Fatalf("pending rows after approval = %d, want 0", pending)
	}
}

// TestApproveLinkAutoMarksAttendance: in link mode the auto-mark lands on the
// roster row the commander chose, not on the pending row.
func TestApproveLinkAutoMarksAttendance(t *testing.T) {
	db, prefix := openRegistrationDB(t)
	rosterID := prefix + "-roster"
	pendingID := prefix + "-pending"
	sessionID := prefix + "-session"
	secret := prefix + "-secret"
	seedUser(t, db, rosterID, "TAN WEI MING", "CPL", "Alpha", "", false)
	seedActiveSession(t, db, sessionID, secret)
	seedPendingWithQR(t, db, pendingID, "tanwm", "TAN WEI MIMG", "LCP", "Alpha", sessionID, secret)

	rec := approve(t, NewAdminHandler(db), pendingID, `{"mode":"link","userId":"`+rosterID+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if count := countRecords(t, db, sessionID, rosterID); count != 1 {
		t.Fatalf("attendance records on linked user = %d, want 1", count)
	}
	if count := countRecords(t, db, sessionID, pendingID); count != 0 {
		t.Fatalf("attendance records on pending id = %d, want 0", count)
	}
}

// TestApproveClosedSessionSkipsAutoMark: a session that is no longer active at
// approval time must not be marked, and the approval itself must still succeed.
func TestApproveClosedSessionSkipsAutoMark(t *testing.T) {
	db, prefix := openRegistrationDB(t)
	pendingID := prefix + "-pending"
	sessionID := prefix + "-session"
	secret := prefix + "-secret"
	seedActiveSession(t, db, sessionID, secret)
	seedPendingWithQR(t, db, pendingID, "newuser", "NEW PERSON", "PTE", "Bravo", sessionID, secret)
	if _, err := db.Pool.Exec(context.Background(), `
		UPDATE attendance_session SET status = 'closed', closed_at = NOW() WHERE id = $1
	`, sessionID); err != nil {
		t.Fatal(err)
	}

	rec := approve(t, NewAdminHandler(db), pendingID, `{"mode":"create"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if count := countRecords(t, db, sessionID, pendingID); count != 0 {
		t.Fatalf("attendance records for closed session = %d, want 0", count)
	}
}

// TestApproveAlreadyMarkedSkipsAutoMark: an existing record (e.g. the
// commander manually marked the roster row) must not be duplicated when the
// approval auto-marks. Link mode is the realistic path here — the roster user
// exists before approval, so they can already hold a record.
func TestApproveAlreadyMarkedSkipsAutoMark(t *testing.T) {
	db, prefix := openRegistrationDB(t)
	rosterID := prefix + "-roster"
	pendingID := prefix + "-pending"
	sessionID := prefix + "-session"
	secret := prefix + "-secret"
	seedUser(t, db, rosterID, "TAN WEI MING", "CPL", "Alpha", "", false)
	seedActiveSession(t, db, sessionID, secret)
	seedPendingWithQR(t, db, pendingID, "tanwm", "TAN WEI MIMG", "LCP", "Alpha", sessionID, secret)
	if _, err := db.Pool.Exec(context.Background(), `
		INSERT INTO attendance_record (id, session_id, user_id, marking_method)
		VALUES ($1, $2, $3, 'manual')
	`, prefix+"-record", sessionID, rosterID); err != nil {
		t.Fatal(err)
	}

	rec := approve(t, NewAdminHandler(db), pendingID, `{"mode":"link","userId":"`+rosterID+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if count := countRecords(t, db, sessionID, rosterID); count != 1 {
		t.Fatalf("attendance records = %d, want 1 (no duplicate)", count)
	}
	if method := recordMethod(t, db, sessionID, rosterID); method != models.MarkingMethodManual {
		t.Fatalf("marking method = %q, want original %q preserved", method, models.MarkingMethodManual)
	}
}

// TestApproveNoQrTokenNoAutoMark: registrations without a QR token keep the
// existing behavior and never write an attendance record.
func TestApproveNoQrTokenNoAutoMark(t *testing.T) {
	db, prefix := openRegistrationDB(t)
	pendingID := prefix + "-pending"
	sessionID := prefix + "-session"
	secret := prefix + "-secret"
	seedActiveSession(t, db, sessionID, secret)
	seedPending(t, db, pendingID, "newuser", "NEW PERSON", "PTE", "Bravo")

	rec := approve(t, NewAdminHandler(db), pendingID, `{"mode":"create"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if count := countRecords(t, db, sessionID, pendingID); count != 0 {
		t.Fatalf("attendance records = %d, want 0", count)
	}
}

// TestApproveWrongSecretSkipsAutoMark: a QR secret that no longer matches the
// session must not mark attendance, and must not fail the approval.
func TestApproveWrongSecretSkipsAutoMark(t *testing.T) {
	db, prefix := openRegistrationDB(t)
	pendingID := prefix + "-pending"
	sessionID := prefix + "-session"
	seedActiveSession(t, db, sessionID, prefix+"-secret")
	seedPendingWithQR(t, db, pendingID, "newuser", "NEW PERSON", "PTE", "Bravo", sessionID, "stale-secret")

	rec := approve(t, NewAdminHandler(db), pendingID, `{"mode":"create"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if count := countRecords(t, db, sessionID, pendingID); count != 0 {
		t.Fatalf("attendance records = %d, want 0", count)
	}
}

// TestApproveResponseReportsAttendance: the approval response names the session
// that was auto-marked so the commander can see the outcome.
func TestApproveResponseReportsAttendance(t *testing.T) {
	db, prefix := openRegistrationDB(t)
	pendingID := prefix + "-pending"
	sessionID := prefix + "-session"
	secret := prefix + "-secret"
	seedActiveSession(t, db, sessionID, secret)
	seedPendingWithQR(t, db, pendingID, "newuser", "NEW PERSON", "PTE", "Bravo", sessionID, secret)

	rec := approve(t, NewAdminHandler(db), pendingID, `{"mode":"create"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var response struct {
		Attendance *struct {
			Marked    bool   `json:"marked"`
			SessionID string `json:"sessionId"`
		} `json:"attendance"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.Attendance == nil || !response.Attendance.Marked || response.Attendance.SessionID != sessionID {
		t.Fatalf("attendance response = %+v, want marked session %s", response.Attendance, sessionID)
	}
}

// TestSignUpStoresQrToken: signing up with a QR token persists the session and
// secret on the pending registration for the approval flow to consume.
func TestSignUpStoresQrToken(t *testing.T) {
	db, prefix := openRegistrationDB(t)
	ctx := context.Background()
	sessionID := prefix + "-session"
	secret := prefix + "-secret"
	seedActiveSession(t, db, sessionID, secret)

	body := fmt.Sprintf(`{"username":"%s-user","password":"correct horse","confirmPassword":"correct horse","fullName":"NEW PERSON","rank":"PTE","battery":"Bravo","qrToken":"%s:%s"}`, prefix, sessionID, secret)
	req := httptest.NewRequest(http.MethodPost, "/api/auth/sign-up", strings.NewReader(body))
	rec := httptest.NewRecorder()
	NewAuthHandler(db).SignUp(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", rec.Code, rec.Body.String())
	}

	var storedSessionID, storedSecret string
	if err := db.Pool.QueryRow(ctx, `
		SELECT qr_session_id, qr_secret FROM pending_registration WHERE username = $1
	`, prefix+"-user").Scan(&storedSessionID, &storedSecret); err != nil {
		t.Fatal(err)
	}
	if storedSessionID != sessionID || storedSecret != secret {
		t.Fatalf("stored qr = (%q, %q), want (%q, %q)", storedSessionID, storedSecret, sessionID, secret)
	}
}

// TestSignUpRejectsMalformedQrToken: a QR token that is not session:secret is
// a client bug and must be rejected before anything is stored.
func TestSignUpRejectsMalformedQrToken(t *testing.T) {
	db, prefix := openRegistrationDB(t)
	body := fmt.Sprintf(`{"username":"%s-user","password":"correct horse","confirmPassword":"correct horse","fullName":"NEW PERSON","rank":"PTE","battery":"Bravo","qrToken":"not-a-token"}`, prefix)
	req := httptest.NewRequest(http.MethodPost, "/api/auth/sign-up", strings.NewReader(body))
	rec := httptest.NewRecorder()
	NewAuthHandler(db).SignUp(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body.String())
	}
}

// TestFullFlowScanSignupApproveAutoMarks exercises the whole journey through
// the real HTTP handlers: a soldier scans a QR, is redirected to sign-in, signs
// up carrying the token, the commander's approval auto-marks the session, and
// the soldier can finally sign in.
func TestFullFlowScanSignupApproveAutoMarks(t *testing.T) {
	db, prefix := openRegistrationDB(t)
	ctx := context.Background()
	sessionID := prefix + "-session"
	secret := prefix + "-secret"
	token := sessionID + ":" + secret
	seedActiveSession(t, db, sessionID, secret)
	// The sign-up handler generates the pending id, so the approved user row
	// is not covered by the shared id-prefix cleanup. Delete it by username
	// (which is prefixed) so the strong-match roster stays clean across runs.
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(ctx, `DELETE FROM "user" WHERE username = $1`, prefix+"-user")
	})

	// 1. Soldier scans the QR while signed out → redirected to sign-in with
	//    the token so the intent survives the registration detour.
	handler := NewAttendanceHandler(db, nil)
	qrReq := httptest.NewRequest(http.MethodGet, "/api/qr/"+token, nil)
	qrCtx := chi.NewRouteContext()
	qrCtx.URLParams.Add("token", token)
	qrReq = qrReq.WithContext(context.WithValue(qrReq.Context(), chi.RouteCtxKey, qrCtx))
	qrRec := httptest.NewRecorder()
	handler.HandleQRScan(qrRec, qrReq)
	if qrRec.Code != http.StatusFound {
		t.Fatalf("qr scan status = %d, want 302 redirect to sign-in: %s", qrRec.Code, qrRec.Body.String())
	}
	if location := qrRec.Header().Get("Location"); !strings.Contains(location, "qrToken="+token) {
		t.Fatalf("redirect %q does not carry the qrToken", location)
	}

	// 2. Soldier signs up after scanning the QR, carrying the token.
	signUpBody := fmt.Sprintf(`{"username":"%s-user","password":"correct horse","confirmPassword":"correct horse","fullName":"NEW PERSON","rank":"PTE","battery":"Bravo","qrToken":"%s"}`, prefix, token)
	req := httptest.NewRequest(http.MethodPost, "/api/auth/sign-up", strings.NewReader(signUpBody))
	rec := httptest.NewRecorder()
	NewAuthHandler(db).SignUp(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("sign-up status = %d, want 201: %s", rec.Code, rec.Body.String())
	}

	var pendingID string
	if err := db.Pool.QueryRow(ctx, `SELECT id FROM pending_registration WHERE username = $1`, prefix+"-user").Scan(&pendingID); err != nil {
		t.Fatal(err)
	}

	// 3. Commander approves; the response reports the auto-mark.
	rec = approve(t, NewAdminHandler(db), pendingID, `{"mode":"create"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("approve status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var response struct {
		Attendance *struct {
			Marked      bool   `json:"marked"`
			SessionID   string `json:"sessionId"`
			SessionName string `json:"sessionName"`
		} `json:"attendance"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.Attendance == nil || !response.Attendance.Marked || response.Attendance.SessionID != sessionID || response.Attendance.SessionName != "Parade" {
		t.Fatalf("attendance response = %+v, want marked session %s (Parade)", response.Attendance, sessionID)
	}

	// 4. The created user holds an approval_auto record for the scanned session.
	if count := countRecords(t, db, sessionID, pendingID); count != 1 {
		t.Fatalf("attendance records = %d, want 1", count)
	}
	if method := recordMethod(t, db, sessionID, pendingID); method != models.MarkingMethodApprovalAuto {
		t.Fatalf("marking method = %q, want %q", method, models.MarkingMethodApprovalAuto)
	}

	// 5. The soldier can now sign in as a verified user.
	signInBody := fmt.Sprintf(`{"identifier":"%s-user","password":"correct horse"}`, prefix)
	req = httptest.NewRequest(http.MethodPost, "/api/auth/sign-in", strings.NewReader(signInBody))
	rec = httptest.NewRecorder()
	NewAuthHandler(db).SignIn(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("sign-in status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var signInResp SignInResponse
	if err := json.NewDecoder(rec.Body).Decode(&signInResp); err != nil {
		t.Fatal(err)
	}
	if signInResp.Outcome != SignInOutcomeAuthenticated {
		t.Fatalf("sign-in outcome = %q, want %q", signInResp.Outcome, SignInOutcomeAuthenticated)
	}
}
