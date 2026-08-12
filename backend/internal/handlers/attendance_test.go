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
