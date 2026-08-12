package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/davidlivingston/go-nextjs-starter/backend/internal/middleware"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/models"
	"github.com/go-chi/chi/v5"
)

func TestMarkAttendanceMapsClosedSessionOutcome(t *testing.T) {
	db, prefix := openRegistrationDB(t)
	userID := prefix + "-attendance-user"
	sessionID := prefix + "-closed-session"
	secret := prefix + "-secret"
	seedUser(t, db, userID, "CLOSED SESSION USER", "PTE", "Alpha", "closed-user", true)
	_, err := db.Pool.Exec(context.Background(), `
		INSERT INTO attendance_session (id, name, qr_code, qr_code_secret, scope, batteries, status, created_by, start_time)
		VALUES ($1, 'Closed session', $2, $3, 'unit_wide', '{}', 'closed', $4, NOW())
	`, sessionID, sessionID+"-qr", secret, userID)
	if err != nil {
		t.Fatalf("seed closed session: %v", err)
	}

	body := fmt.Sprintf(`{"qrData":%q}`, sessionID+":"+secret+":"+fmt.Sprint(time.Now().Unix()))
	req := httptest.NewRequest(http.MethodPost, "/api/attendance/mark", strings.NewReader(body))
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserKey, &models.User{ID: userID}))
	rec := httptest.NewRecorder()
	NewAttendanceHandler(db, nil).MarkAttendance(rec, req)

	if rec.Code != http.StatusBadRequest || strings.TrimSpace(rec.Body.String()) != "Session is not active" {
		t.Fatalf("closed-session response = (%d, %q), want 400 service outcome", rec.Code, rec.Body.String())
	}
	var records int
	if err := db.Pool.QueryRow(context.Background(), `
		SELECT count(*) FROM attendance_record WHERE session_id = $1
	`, sessionID).Scan(&records); err != nil {
		t.Fatalf("count closed-session records: %v", err)
	}
	if records != 0 {
		t.Fatalf("closed-session records = %d, want 0", records)
	}
}

func TestHandleQRScanClosedSessionAuthenticatesBeforeState(t *testing.T) {
	db, prefix := openRegistrationDB(t)
	userID := prefix + "-qr-user"
	sessionID := prefix + "-qr-session"
	sessionToken := prefix + "-session-token"
	secret := prefix + "-secret"
	seedUser(t, db, userID, "QR USER", "PTE", "Alpha", "qr-user", true)
	_, err := db.Pool.Exec(context.Background(), `
		INSERT INTO attendance_session (id, name, qr_code, qr_code_secret, scope, batteries, status, created_by, start_time)
		VALUES ($1, 'Closed QR session', $2, $3, 'unit_wide', '{}', 'closed', $4, NOW())
	`, sessionID, sessionID+"-qr", secret, userID)
	if err != nil {
		t.Fatalf("seed closed QR session: %v", err)
	}
	_, err = db.Pool.Exec(context.Background(), `
		INSERT INTO session (id, "expiresAt", token, "userId", "createdAt", "updatedAt")
		VALUES ($1, NOW() + INTERVAL '1 hour', $2, $3, NOW(), NOW())
	`, prefix+"-auth-session", sessionToken, userID)
	if err != nil {
		t.Fatalf("seed QR auth session: %v", err)
	}

	qrToken := sessionID + ":" + secret
	tests := []struct {
		name         string
		token        string
		cookie       bool
		wantStatus   int
		wantBody     string
		wantRedirect bool
	}{
		{
			name:       "wrong secret is rejected before session state",
			token:      sessionID + ":wrong-secret",
			wantStatus: http.StatusUnauthorized,
			wantBody:   "Invalid QR code",
		},
		{
			name:         "unauthenticated caller is redirected before session state",
			token:        qrToken,
			wantStatus:   http.StatusFound,
			wantRedirect: true,
		},
		{
			name:       "authenticated caller receives the service closed outcome",
			token:      qrToken,
			cookie:     true,
			wantStatus: http.StatusBadRequest,
			wantBody:   "Session is not active",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/qr/"+tt.token, nil)
			if tt.cookie {
				req.AddCookie(&http.Cookie{Name: "session", Value: sessionToken})
			}
			routeContext := chi.NewRouteContext()
			routeContext.URLParams.Add("token", tt.token)
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeContext))
			rec := httptest.NewRecorder()
			NewAttendanceHandler(db, nil).HandleQRScan(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("QR response status = %d, want %d (body %q)", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.wantBody != "" && strings.TrimSpace(rec.Body.String()) != tt.wantBody {
				t.Fatalf("QR response body = %q, want %q", rec.Body.String(), tt.wantBody)
			}
			if tt.wantRedirect {
				location := rec.Header().Get("Location")
				if !strings.Contains(location, "/sign-in?redirect=/qr/") || !strings.Contains(location, "qrToken="+tt.token) {
					t.Fatalf("QR redirect = %q, want sign-in redirect carrying token", location)
				}
			}
		})
	}
}

func TestManualMarkAttendanceAllClosedReturnsLegacyError(t *testing.T) {
	db, prefix := openRegistrationDB(t)
	commanderID := prefix + "-commander"
	firstUserID := prefix + "-first-user"
	secondUserID := prefix + "-second-user"
	sessionID := prefix + "-closed-session"
	seedUser(t, db, commanderID, "COMMANDER", "3SG", "HQ", "commander", true)
	seedUser(t, db, firstUserID, "FIRST USER", "PTE", "Alpha", "first-user", true)
	seedUser(t, db, secondUserID, "SECOND USER", "PTE", "Bravo", "second-user", true)
	_, err := db.Pool.Exec(context.Background(), `
		INSERT INTO attendance_session (id, name, qr_code, qr_code_secret, scope, batteries, status, created_by, start_time)
		VALUES ($1, 'Closed manual session', $2, $3, 'unit_wide', '{}', 'closed', $4, NOW())
	`, sessionID, sessionID+"-qr", sessionID+"-secret", commanderID)
	if err != nil {
		t.Fatalf("seed closed manual session: %v", err)
	}

	body := fmt.Sprintf(`{"userIds":[%q,%q]}`, firstUserID, secondUserID)
	req := newManualAttendanceRequest(sessionID, body, &models.User{ID: commanderID, IsSuperadmin: true})
	rec := httptest.NewRecorder()
	NewAttendanceHandler(db, nil).ManualMarkAttendance(rec, req)

	if rec.Code != http.StatusBadRequest || strings.TrimSpace(rec.Body.String()) != "Session is not active" {
		t.Fatalf("closed manual response = (%d, %q), want 400 legacy error", rec.Code, rec.Body.String())
	}
	var records int
	if err := db.Pool.QueryRow(context.Background(), `
		SELECT count(*) FROM attendance_record WHERE session_id = $1
	`, sessionID).Scan(&records); err != nil {
		t.Fatalf("count closed manual records: %v", err)
	}
	if records != 0 {
		t.Fatalf("closed manual records = %d, want 0", records)
	}
}

func TestManualMarkAttendanceClosedMixedBatchReturnsDetails(t *testing.T) {
	db, prefix := openRegistrationDB(t)
	commanderID := prefix + "-commander"
	targetID := prefix + "-target"
	missingID := prefix + "-missing"
	sessionID := prefix + "-closed-mixed-session"
	seedUser(t, db, commanderID, "COMMANDER", "3SG", "HQ", "commander", true)
	seedUser(t, db, targetID, "TARGET USER", "PTE", "Alpha", "target-user", true)
	_, err := db.Pool.Exec(context.Background(), `
		INSERT INTO attendance_session (id, name, qr_code, qr_code_secret, scope, batteries, status, created_by, start_time)
		VALUES ($1, 'Closed mixed session', $2, $3, 'unit_wide', '{}', 'closed', $4, NOW())
	`, sessionID, sessionID+"-qr", sessionID+"-secret", commanderID)
	if err != nil {
		t.Fatalf("seed closed mixed session: %v", err)
	}

	body := fmt.Sprintf(`{"userIds":[%q,%q]}`, targetID, missingID)
	req := newManualAttendanceRequest(sessionID, body, &models.User{ID: commanderID, IsSuperadmin: true})
	rec := httptest.NewRecorder()
	NewAttendanceHandler(db, nil).ManualMarkAttendance(rec, req)

	var response struct {
		SuccessCount int      `json:"successCount"`
		Errors       []string `json:"errors"`
	}
	if rec.Code != http.StatusOK || json.NewDecoder(rec.Body).Decode(&response) != nil {
		t.Fatalf("closed mixed response = (%d, %q), want JSON 200", rec.Code, rec.Body.String())
	}
	if response.SuccessCount != 0 || len(response.Errors) != 2 || response.Errors[0] != "Session is not active" || response.Errors[1] != "User "+missingID+" not found" {
		t.Fatalf("closed mixed details = %+v, want per-target errors", response)
	}
}

func TestManualMarkAttendanceMixedBatchReturnsPerItemDetails(t *testing.T) {
	db, prefix := openRegistrationDB(t)
	commanderID := prefix + "-commander"
	markedID := prefix + "-marked-user"
	alreadyMarkedID := prefix + "-already-marked-user"
	sessionID := prefix + "-mixed-session"
	seedUser(t, db, commanderID, "COMMANDER", "3SG", "HQ", "commander", true)
	seedUser(t, db, markedID, "MARKED USER", "PTE", "Alpha", "marked-user", true)
	seedUser(t, db, alreadyMarkedID, "ALREADY MARKED USER", "PTE", "Alpha", "already-marked-user", true)
	_, err := db.Pool.Exec(context.Background(), `
		INSERT INTO attendance_session (id, name, qr_code, qr_code_secret, scope, batteries, status, created_by, start_time)
		VALUES ($1, 'Mixed manual session', $2, $3, 'unit_wide', '{}', 'active', $4, NOW())
	`, sessionID, sessionID+"-qr", sessionID+"-secret", commanderID)
	if err != nil {
		t.Fatalf("seed mixed manual session: %v", err)
	}
	_, err = db.Pool.Exec(context.Background(), `
		INSERT INTO attendance_record (id, session_id, user_id, marking_method)
		VALUES ($1, $2, $3, 'manual')
	`, sessionID+"-existing-record", sessionID, alreadyMarkedID)
	if err != nil {
		t.Fatalf("seed existing attendance record: %v", err)
	}

	body := fmt.Sprintf(`{"userIds":[%q,%q]}`, markedID, alreadyMarkedID)
	req := newManualAttendanceRequest(sessionID, body, &models.User{ID: commanderID, IsSuperadmin: true})
	rec := httptest.NewRecorder()
	NewAttendanceHandler(db, nil).ManualMarkAttendance(rec, req)

	var response struct {
		SuccessCount int      `json:"successCount"`
		Errors       []string `json:"errors"`
	}
	if rec.Code != http.StatusOK || json.NewDecoder(rec.Body).Decode(&response) != nil {
		t.Fatalf("mixed response = (%d, %q), want JSON 200", rec.Code, rec.Body.String())
	}
	if response.SuccessCount != 1 || len(response.Errors) != 1 || response.Errors[0] != "User "+alreadyMarkedID+" already marked" {
		t.Fatalf("mixed details = %+v, want one success and one per-item error", response)
	}
	var records int
	if err := db.Pool.QueryRow(context.Background(), `
		SELECT count(*) FROM attendance_record WHERE session_id = $1
	`, sessionID).Scan(&records); err != nil {
		t.Fatalf("count mixed records: %v", err)
	}
	if records != 2 {
		t.Fatalf("mixed records = %d, want 2", records)
	}
}

func newManualAttendanceRequest(sessionID, body string, user *models.User) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/sessions/"+sessionID+"/attendance/manual", strings.NewReader(body))
	routeContext := chi.NewRouteContext()
	routeContext.URLParams.Add("id", sessionID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeContext))
	return req.WithContext(context.WithValue(req.Context(), middleware.UserKey, user))
}

func TestManualMarkAttendanceUsesOneBatchTimestamp(t *testing.T) {
	db, prefix := openRegistrationDB(t)
	commanderID := prefix + "-commander"
	firstUserID := prefix + "-first-user"
	secondUserID := prefix + "-second-user"
	sessionID := prefix + "-manual-session"
	seedUser(t, db, commanderID, "COMMANDER", "3SG", "HQ", "commander", true)
	seedUser(t, db, firstUserID, "FIRST USER", "PTE", "Alpha", "first-user", true)
	seedUser(t, db, secondUserID, "SECOND USER", "PTE", "Bravo", "second-user", true)
	_, err := db.Pool.Exec(context.Background(), `
		INSERT INTO attendance_session (id, name, qr_code, qr_code_secret, scope, batteries, status, created_by, start_time)
		VALUES ($1, 'Manual session', $2, $3, 'unit_wide', '{}', 'active', $4, NOW())
	`, sessionID, sessionID+"-qr", sessionID+"-secret", commanderID)
	if err != nil {
		t.Fatalf("seed manual session: %v", err)
	}

	body := fmt.Sprintf(`{"userIds":[%q,%q]}`, firstUserID, secondUserID)
	req := httptest.NewRequest(http.MethodPost, "/api/sessions/"+sessionID+"/attendance/manual", strings.NewReader(body))
	routeContext := chi.NewRouteContext()
	routeContext.URLParams.Add("id", sessionID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeContext))
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserKey, &models.User{ID: commanderID, IsSuperadmin: true}))
	rec := httptest.NewRecorder()
	NewAttendanceHandler(db, nil).ManualMarkAttendance(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("manual response = (%d, %q), want 200", rec.Code, rec.Body.String())
	}
	var firstMarkedAt, secondMarkedAt time.Time
	for _, userID := range []string{firstUserID, secondUserID} {
		var markedAt time.Time
		if err := db.Pool.QueryRow(context.Background(), `
			SELECT marked_at FROM attendance_record WHERE session_id = $1 AND user_id = $2
		`, sessionID, userID).Scan(&markedAt); err != nil {
			t.Fatalf("read mark for %s: %v", userID, err)
		}
		if userID == firstUserID {
			firstMarkedAt = markedAt
		} else {
			secondMarkedAt = markedAt
		}
	}
	if !firstMarkedAt.Equal(secondMarkedAt) {
		t.Fatalf("manual timestamps = (%s, %s), want one batch timestamp", firstMarkedAt, secondMarkedAt)
	}
}
