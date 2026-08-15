package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/davidlivingston/go-nextjs-starter/backend/internal/database"
)

// TestConcurrentSignInAndMarkAttendance exercises the two web-UI paths that a
// busy roll-call hits simultaneously: every soldier signs in (name + DOB) and
// then marks attendance via the QR scan, all in parallel. It asserts that the
// concurrent marking stays race-safe (exactly one record per user, none lost)
// and that every sign-in either authenticates or is rejected cleanly.
func TestConcurrentSignInAndMarkAttendance(t *testing.T) {
	db, prefix := openConcurrencyDB(t)
	h := NewAuthHandler(db)
	att := NewAttendanceHandler(db, nil)

	const n = 100
	names := make([]string, n)
	for i := range names {
		names[i] = fmt.Sprintf("%s CONCURRENT USER %03d", prefix, i)
		dob := fmt.Sprintf("%02d.%02d.%d", (i%28)+1, (i%12)+1, 1990+i%25)
		canon, ok := canonicalDOB(dob)
		if !ok {
			t.Fatalf("bad test dob %q", dob)
		}
		seedConcurrentUser(t, db, fmt.Sprintf("%s-u%03d", prefix, i), names[i], canon)
	}

	sessionID := prefix + "-session"
	seedConcurrentSession(t, db, sessionID, prefix+" PARADE", "unit_wide")

	// Phase A: every user signs in concurrently with name + DOB.
	type signInResult struct {
		code   int
		cookie string
	}
	signIns := make([]signInResult, n)

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			dob := readUserDOB(t, db, names[i])
			req := httptest.NewRequest(http.MethodPost, "/api/auth/sign-in",
				strings.NewReader(fmt.Sprintf(`{"identifier":%q,"dob":%q}`, names[i], dob)))
			rec := httptest.NewRecorder()
			h.SignIn(rec, req)
			signIns[i].code = rec.Code
			for _, c := range rec.Result().Cookies() {
				if c.Name == "session" {
					signIns[i].cookie = c.Value
				}
			}
		}(i)
	}
	wg.Wait()

	var authOK, authFail int
	for _, r := range signIns {
		if r.code == http.StatusOK && r.cookie != "" {
			authOK++
		} else {
			authFail++
		}
	}
	t.Logf("concurrent sign-in: ok=%d fail=%d", authOK, authFail)
	if authOK == 0 {
		t.Fatal("no concurrent sign-in succeeded")
	}

	// Phase B: every successfully-signed-in user marks attendance in parallel.
	now := time.Now().Unix()
	markCodes := make([]int, n)
	var mu sync.Mutex
	failures := map[int][]string{}
	for i := 0; i < n; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			if signIns[i].cookie == "" {
				markCodes[i] = -1 // no auth: skipped
				return
			}
			qrData := fmt.Sprintf("%s:%s:%d", sessionID, "secret", now)
			req := httptest.NewRequest(http.MethodPost, "/api/attendance/mark",
				strings.NewReader(fmt.Sprintf(`{"qrData":%q}`, qrData)))
			req.AddCookie(&http.Cookie{Name: "session", Value: signIns[i].cookie})
			rec := httptest.NewRecorder()
			att.MarkAttendance(rec, req)
			markCodes[i] = rec.Code
			if rec.Code != http.StatusOK {
				mu.Lock()
				failures[rec.Code] = append(failures[rec.Code], rec.Body.String())
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	var marked, skipped, failed int
	for _, c := range markCodes {
		switch {
		case c == http.StatusOK:
			marked++
		case c == -1:
			skipped++
		default:
			failed++
		}
	}
	t.Logf("concurrent mark: ok=%d skipped(no-auth)=%d failed=%d", marked, skipped, failed)
	if failed > 0 {
		t.Fatalf("mark failures by status: %v", failures)
	}

	var records, unique int
	err := db.Pool.QueryRow(context.Background(), `
		SELECT COUNT(*), COUNT(DISTINCT user_id) FROM attendance_record WHERE session_id = $1
	`, sessionID).Scan(&records, &unique)
	if err != nil {
		t.Fatalf("count records: %v", err)
	}
	t.Logf("attendance records: total=%d unique=%d", records, unique)
	if records != unique {
		t.Fatalf("duplicate records detected: total=%d unique=%d", records, unique)
	}
	if records != marked {
		t.Fatalf("record count %d != marked responses %d (lost or phantom marks)", records, marked)
	}
	if authOK != marked+skipped {
		t.Fatalf("auth ok=%d but mark got ok+skipped=%d; marks may have been lost", authOK, marked+skipped)
	}
}

func openConcurrencyDB(t *testing.T) (*database.DB, string) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to run the concurrency integration test")
	}
	db, err := database.NewPostgresDB(url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	prefix := fmt.Sprintf("conc-%d", time.Now().UnixNano()%100000)
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(context.Background(),
			`DELETE FROM attendance_record WHERE session_id = $1`, prefix+"-session")
		_, _ = db.Pool.Exec(context.Background(), `DELETE FROM "user" WHERE id LIKE $1`, prefix+"-%")
		_, _ = db.Pool.Exec(context.Background(), `DELETE FROM attendance_session WHERE id = $1`, prefix+"-session")
		db.Close()
	})
	return db, prefix
}

func seedConcurrentUser(t *testing.T, db *database.DB, id, name, dob string) {
	t.Helper()
	_, err := db.Pool.Exec(context.Background(), `
		INSERT INTO "user" (id, "full_name", rank, battery, dob, extras, verified, "is_superadmin", "createdAt", "updatedAt")
		VALUES ($1, $2, 'PTE', 'Alpha', $3, '{}'::jsonb, true, false, NOW(), NOW())
	`, id, name, dob)
	if err != nil {
		t.Fatalf("seed user %s: %v", id, err)
	}
}

func seedConcurrentSession(t *testing.T, db *database.DB, id, name, scope string) {
	t.Helper()
	_, err := db.Pool.Exec(context.Background(), `
		INSERT INTO attendance_session (id, name, qr_code, qr_code_secret, scope, batteries, status, created_by, start_time, deeplink_code, "createdAt", "updatedAt")
		VALUES ($1, $2, $3, 'secret', $4, '{}', 'active', (SELECT id FROM "user" WHERE is_superadmin LIMIT 1), NOW(), 'code', NOW(), NOW())
	`, id, name, id+":secret")
	if err != nil {
		t.Fatalf("seed session %s: %v", id, err)
	}
}

func readUserDOB(t *testing.T, db *database.DB, name string) string {
	t.Helper()
	var dob string
	err := db.Pool.QueryRow(context.Background(), `
		SELECT dob FROM "user" WHERE upper("full_name") = upper($1)
	`, name).Scan(&dob)
	if err != nil {
		t.Fatalf("read dob for %s: %v", name, err)
	}
	return dob
}
