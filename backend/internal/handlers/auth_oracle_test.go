package handlers

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/davidlivingston/go-nextjs-starter/backend/internal/database"
	"golang.org/x/crypto/bcrypt"
)

func TestSignInUnknownAndWrongPasswordAreIdentical(t *testing.T) {
	db, prefix := openAuthTestDB(t)
	seedAuthUser(t, db, prefix+"-known", "known")
	h := NewAuthHandler(db)
	unknown := authSignIn(t, h, "unknown", "wrong-password")
	wrong := authSignIn(t, h, "known", "wrong-password")
	if unknown.Code != http.StatusUnauthorized || wrong.Code != http.StatusUnauthorized {
		t.Fatalf("statuses = (%d, %d), want (401, 401)", unknown.Code, wrong.Code)
	}
	if !bytes.Equal(unknown.Body.Bytes(), wrong.Body.Bytes()) {
		t.Fatalf("failure bodies differ: unknown=%q wrong=%q", unknown.Body.String(), wrong.Body.String())
	}
}

func openAuthTestDB(t *testing.T) (*database.DB, string) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to run the database integration test")
	}
	db, err := database.NewPostgresDB(url)
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	prefix := fmt.Sprintf("oracle-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(context.Background(), `DELETE FROM "user" WHERE id LIKE $1`, prefix+"-%")
		db.Close()
	})
	return db, prefix
}

func seedAuthUser(t *testing.T, db *database.DB, id, username string) {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Pool.Exec(context.Background(), `
		INSERT INTO "user" (id,"full_name",rank,battery,username,password,extras,"is_superadmin",verified,"createdAt","updatedAt")
		VALUES ($1,'KNOWN PERSON','PTE','HQ',$2,$3,'{}'::jsonb,false,true,NOW(),NOW())
	`, id, username, string(hash))
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
}

func authSignIn(t *testing.T, h *AuthHandler, identifier, password string) *httptest.ResponseRecorder {
	t.Helper()
	body := fmt.Sprintf(`{"identifier":%q,"password":%q}`, identifier, password)
	req := httptest.NewRequest(http.MethodPost, "/api/auth/sign-in", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.SignIn(rec, req)
	return rec
}
