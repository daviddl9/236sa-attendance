package handlers

import (
	"context"
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
