package handlers

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/davidlivingston/go-nextjs-starter/backend/internal/database"
	"golang.org/x/crypto/bcrypt"
)

// The legacy sign-in path used to filter on a plaintext `nric_last5` column.
// It now resolves identity from the full name plus the bcrypt hash alone, so the
// secret no longer has to be stored in readable form. These tests pin the
// behaviour that made that safe to do.

func openLegacyDB(t *testing.T) (*database.DB, string) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to run the legacy auth tests")
	}
	db, err := database.NewPostgresDB(url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	prefix := fmt.Sprintf("legacyauth-%d", os.Getpid())
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(context.Background(), `DELETE FROM "user" WHERE id LIKE $1`, prefix+"%")
		db.Close()
	})
	return db, prefix
}

func seedLegacyUser(t *testing.T, db *database.DB, id, fullName, secret string, cost int) {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(secret), cost)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	_, err = db.Pool.Exec(context.Background(), `
		INSERT INTO "user" (id, "full_name", rank, battery, password, extras, verified, "is_superadmin", "createdAt", "updatedAt")
		VALUES ($1, $2, 'CPL', 'Bravo', $3, '{}'::jsonb, true, false, NOW(), NOW())
	`, id, fullName, string(hash))
	if err != nil {
		t.Fatalf("seed %s: %v", id, err)
	}
}

func TestLegacyAuthResolvesByNameAndHash(t *testing.T) {
	db, prefix := openLegacyDB(t)
	h := NewAuthHandler(db)
	seedLegacyUser(t, db, prefix+"-a", prefix+" TAN WEI MING", "1234A", bcrypt.MinCost)

	matched, err := h.authenticateLegacyByName(context.Background(), prefix+" TAN WEI MING", "1234A", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if matched == nil || matched.id != prefix+"-a" {
		t.Fatalf("expected the seeded user to authenticate, got %v", matched)
	}
}

func TestLegacyAuthAcceptsLowercaseTypedSecret(t *testing.T) {
	// Pre-008 credentials were uppercased before hashing, so a soldier typing a
	// lowercase final letter must still get in.
	db, prefix := openLegacyDB(t)
	h := NewAuthHandler(db)
	seedLegacyUser(t, db, prefix+"-b", prefix+" LIM AH KOW", "5678B", bcrypt.MinCost)

	matched, err := h.authenticateLegacyByName(context.Background(), prefix+" LIM AH KOW", "5678b", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if matched == nil {
		t.Fatal("expected lowercase input to authenticate against an uppercased hash")
	}
}

func TestLegacyAuthRejectsWrongSecret(t *testing.T) {
	db, prefix := openLegacyDB(t)
	h := NewAuthHandler(db)
	seedLegacyUser(t, db, prefix+"-c", prefix+" GOH ZHENHAO", "1111C", bcrypt.MinCost)

	matched, err := h.authenticateLegacyByName(context.Background(), prefix+" GOH ZHENHAO", "9999Z", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if matched != nil {
		t.Fatal("a wrong secret must not authenticate")
	}
}

func TestLegacyAuthUnknownNameReturnsNoMatch(t *testing.T) {
	db, prefix := openLegacyDB(t)
	h := NewAuthHandler(db)

	matched, err := h.authenticateLegacyByName(context.Background(), prefix+" NOBODY AT ALL", "1234A", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if matched != nil {
		t.Fatal("an unknown name must not authenticate")
	}
}

func TestLegacyAuthDistinguishesPeopleSharingAName(t *testing.T) {
	// The real roster contains people who share a full name. Removing the
	// nric_last5 filter means the name lookup returns several rows, so the hash
	// is what must separate them.
	db, prefix := openLegacyDB(t)
	h := NewAuthHandler(db)
	shared := prefix + " MUHAMMAD ALI"
	seedLegacyUser(t, db, prefix+"-d1", shared, "1234A", bcrypt.MinCost)
	seedLegacyUser(t, db, prefix+"-d2", shared, "5678B", bcrypt.MinCost)

	first, err := h.authenticateLegacyByName(context.Background(), shared, "1234A", "")
	if err != nil || first == nil || first.id != prefix+"-d1" {
		t.Fatalf("expected the first namesake to authenticate, got %v err=%v", first, err)
	}
	second, err := h.authenticateLegacyByName(context.Background(), shared, "5678B", "")
	if err != nil || second == nil || second.id != prefix+"-d2" {
		t.Fatalf("expected the second namesake to authenticate, got %v err=%v", second, err)
	}
}

func TestLegacyAuthRefusesWhenTwoNamesakesShareASecret(t *testing.T) {
	// If two people share both a name and a secret there is no way to tell who is
	// signing in. Picking one would silently attribute attendance to the wrong
	// person, so this must fail closed.
	db, prefix := openLegacyDB(t)
	h := NewAuthHandler(db)
	shared := prefix + " TAN AH SENG"
	seedLegacyUser(t, db, prefix+"-e1", shared, "4321X", bcrypt.MinCost)
	seedLegacyUser(t, db, prefix+"-e2", shared, "4321X", bcrypt.MinCost)

	matched, err := h.authenticateLegacyByName(context.Background(), shared, "4321X", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if matched != nil {
		t.Fatalf("an ambiguous match must fail closed, got %s", matched.id)
	}
}

func TestLegacyAuthUpgradesWeakHashOnSuccess(t *testing.T) {
	// Pre-008 hashes were written at bcrypt cost 4. A successful sign-in should
	// retire them without asking the soldier to do anything.
	db, prefix := openLegacyDB(t)
	h := NewAuthHandler(db)
	seedLegacyUser(t, db, prefix+"-f", prefix+" LOW COST USER", "2468D", bcrypt.MinCost)

	if _, err := h.authenticateLegacyByName(context.Background(), prefix+" LOW COST USER", "2468D", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var stored string
	if err := db.Pool.QueryRow(context.Background(), `SELECT password FROM "user" WHERE id = $1`, prefix+"-f").Scan(&stored); err != nil {
		t.Fatalf("read back: %v", err)
	}
	cost, err := bcrypt.Cost([]byte(stored))
	if err != nil {
		t.Fatalf("cost: %v", err)
	}
	if cost != passwordHashCost {
		t.Fatalf("hash cost = %d, want %d after upgrade", cost, passwordHashCost)
	}
	// The upgraded hash must still verify the same secret.
	if !comparePassword(stored, "2468D") {
		t.Fatal("upgraded hash no longer verifies the original secret")
	}
}

func TestCanonicalDOBNormalizesFormats(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"06.11.1986", "1986-11-06", true},
		{"06/11/1986", "1986-11-06", true},
		{"06-11-1986", "1986-11-06", true},
		{"1986-11-06", "1986-11-06", true},
		{"06111986", "1986-11-06", true},
		{"19861106", "1986-11-06", true},
		{"", "", false},
		{"not-a-date", "", false},
		{"31132000", "", false}, // month 13 is invalid
	}
	for _, c := range cases {
		got, ok := canonicalDOB(c.in)
		if ok != c.ok || (ok && got != c.want) {
			t.Fatalf("canonicalDOB(%q) = %q, %v; want %q, %v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestLegacyAuthResolvesByNameAndDOB(t *testing.T) {
	db, prefix := openLegacyDB(t)
	h := NewAuthHandler(db)
	seedLegacyUserWithDOB(t, db, prefix+"-dob", prefix+" TAN WEI MING", "06.11.1986")

	matched, err := h.authenticateLegacyByName(context.Background(), prefix+" TAN WEI MING", "", "06.11.1986")
	if err != nil || matched == nil || matched.id != prefix+"-dob" {
		t.Fatalf("expected DOB to authenticate, got %v err=%v", matched, err)
	}

	// A wrong DOB must not authenticate.
	matched, err = h.authenticateLegacyByName(context.Background(), prefix+" TAN WEI MING", "", "01.01.2000")
	if err != nil || matched != nil {
		t.Fatalf("wrong DOB must not authenticate, got %v err=%v", matched, err)
	}

	// A missing DOB falls back to the password path, which must fail here.
	matched, err = h.authenticateLegacyByName(context.Background(), prefix+" TAN WEI MING", "1234A", "")
	if err != nil || matched != nil {
		t.Fatalf("password fallback must not match a DOB-only user, got %v err=%v", matched, err)
	}
}

func seedLegacyUserWithDOB(t *testing.T, db *database.DB, id, fullName, dob string) {
	t.Helper()
	_, err := db.Pool.Exec(context.Background(), `
		INSERT INTO "user" (id, "full_name", rank, battery, dob, extras, verified, "is_superadmin", "createdAt", "updatedAt")
		VALUES ($1, $2, 'CPL', 'Bravo', $3, '{}'::jsonb, true, false, NOW(), NOW())
	`, id, fullName, dob)
	if err != nil {
		t.Fatalf("seed %s: %v", id, err)
	}
}
