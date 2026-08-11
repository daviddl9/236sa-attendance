package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/davidlivingston/go-nextjs-starter/backend/internal/database"
	"github.com/go-chi/chi/v5"
	"golang.org/x/crypto/bcrypt"
)

func TestApproveRegistrationLinkPreservesAttendance(t *testing.T) {
	db, prefix := openRegistrationDB(t)
	ctx := context.Background()
	rosterID := prefix + "-roster"
	pendingID := prefix + "-pending"
	seedUser(t, db, rosterID, "TAN WEI MING", "CPL", "Alpha", "", false)
	for i := 0; i < 3; i++ {
		seedSessionAndRecord(t, db, prefix, rosterID, i)
	}
	seedPending(t, db, pendingID, "tanwm", "TAN WEI MIMG", "LCP", "Alpha")

	rec := approve(t, NewAdminHandler(db), pendingID, `{"mode":"link","userId":"`+rosterID+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var userCount, recordCount int
	if err := db.Pool.QueryRow(ctx, `SELECT count(*) FROM "user" WHERE id = $1`, rosterID).Scan(&userCount); err != nil {
		t.Fatal(err)
	}
	if err := db.Pool.QueryRow(ctx, `SELECT count(*) FROM attendance_record ar JOIN "user" u ON u.id = ar.user_id WHERE u.id = $1`, rosterID).Scan(&recordCount); err != nil {
		t.Fatal(err)
	}
	if userCount != 1 || recordCount != 3 {
		t.Fatalf("linked user/history = (%d, %d), want (1, 3)", userCount, recordCount)
	}
}

func TestApproveRegistrationCreateInsertsOneUser(t *testing.T) {
	db, prefix := openRegistrationDB(t)
	pendingID := prefix + "-pending"
	seedPending(t, db, pendingID, "newuser", "NEW PERSON", "PTE", "Bravo")
	rec := approve(t, NewAdminHandler(db), pendingID, `{"mode":"create"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var count int
	if err := db.Pool.QueryRow(context.Background(), `SELECT count(*) FROM "user" WHERE id = $1`, pendingID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("created user count = %d, want 1", count)
	}
}

func TestMigratedPendingMustLinkAndCannotCreate(t *testing.T) {
	db, prefix := openRegistrationDB(t)
	migratedID := prefix + "-migrated"
	seedUser(t, db, migratedID, "CARRIED PERSON", "PTE", "HQ", "", false)
	seedPending(t, db, migratedID, migratedPendingUsernamePrefix+migratedID, "CARRIED PERSON", "PTE", "HQ")
	if rec := approve(t, NewAdminHandler(db), migratedID, `{"mode":"create"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("create status = %d, want 400", rec.Code)
	}
	if rec := approve(t, NewAdminHandler(db), migratedID, `{"mode":"link"}`); rec.Code != http.StatusOK {
		t.Fatalf("default link status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var verified bool
	var userCount int
	ctx := context.Background()
	if err := db.Pool.QueryRow(ctx, `SELECT verified FROM "user" WHERE id = $1`, migratedID).Scan(&verified); err != nil {
		t.Fatal(err)
	}
	if err := db.Pool.QueryRow(ctx, `SELECT count(*) FROM "user" WHERE id = $1`, migratedID).Scan(&userCount); err != nil {
		t.Fatal(err)
	}
	if !verified || userCount != 1 {
		t.Fatalf("migrated row = (verified %v, users %d), want (true, 1)", verified, userCount)
	}
}

func openRegistrationDB(t *testing.T) (*database.DB, string) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to run the database integration test")
	}
	db, err := database.NewPostgresDB(url)
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	prefix := fmt.Sprintf("pr3-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = db.Pool.Exec(ctx, `DELETE FROM pending_registration WHERE id LIKE $1 OR username LIKE $1`, prefix+"-%")
		_, _ = db.Pool.Exec(ctx, `DELETE FROM attendance_session WHERE id LIKE $1`, prefix+"-%")
		_, _ = db.Pool.Exec(ctx, `DELETE FROM "user" WHERE id LIKE $1`, prefix+"-%")
		db.Close()
	})
	return db, prefix
}

func seedUser(t *testing.T, db *database.DB, id, name, rank, battery, username string, verified bool) {
	t.Helper()
	password, err := bcrypt.GenerateFromPassword([]byte("password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Pool.Exec(context.Background(), `
		INSERT INTO "user" (id, "full_name", rank, battery, username, password, extras, "is_superadmin", verified, "createdAt", "updatedAt")
		VALUES ($1,$2,$3,$4,NULLIF($5,''),$6,'{}'::jsonb,false,$7,NOW(),NOW())
	`, id, name, rank, battery, username, string(password), verified)
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
}

func seedPending(t *testing.T, db *database.DB, id, username, name, rank, battery string) {
	t.Helper()
	hash, _ := bcrypt.GenerateFromPassword([]byte("password"), bcrypt.MinCost)
	_, err := db.Pool.Exec(context.Background(), `
		INSERT INTO pending_registration (id, username, password_hash, claimed_name, claimed_rank, claimed_battery)
		VALUES ($1,$2,$3,$4,$5,$6)
	`, id, username, string(hash), name, rank, battery)
	if err != nil {
		t.Fatalf("seed pending: %v", err)
	}
}

func seedSessionAndRecord(t *testing.T, db *database.DB, prefix, userID string, n int) {
	t.Helper()
	ctx := context.Background()
	sessionID := fmt.Sprintf("%s-session-%d", prefix, n)
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO attendance_session (id,name,qr_code,qr_code_secret,scope,status,created_by,start_time)
		VALUES ($1,'PR3',$2,$3,'unit_wide','closed',$4,NOW())
	`, sessionID, sessionID+"-qr", sessionID+"-secret", userID)
	if err != nil {
		t.Fatalf("seed session: %v", err)
	}
	_, err = db.Pool.Exec(ctx, `INSERT INTO attendance_record (id,session_id,user_id,marking_method) VALUES ($1,$2,$3,'manual')`, sessionID+"-record", sessionID, userID)
	if err != nil {
		t.Fatalf("seed attendance: %v", err)
	}
}

func approve(t *testing.T, h *AdminHandler, id, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/registrations/"+id+"/approve", strings.NewReader(body))
	rc := chi.NewRouteContext()
	rc.URLParams.Add("id", id)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rc))
	rec := httptest.NewRecorder()
	h.ApproveRegistration(rec, req)
	return rec
}
