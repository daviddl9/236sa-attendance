package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/davidlivingston/go-nextjs-starter/backend/internal/middleware"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/models"
)

func TestPersonnelRecordNRICLast5ValidationRule(t *testing.T) {
	validValues := map[string]string{
		"0001z": "0001Z",
		"1234A": "1234A",
	}
	for value, want := range validValues {
		t.Run("valid "+value, func(t *testing.T) {
			got, ok := normalizeNRICLast5(value)
			if !ok {
				t.Fatalf("normalizeNRICLast5(%q) ok = false, want true", value)
			}
			if got != want {
				t.Fatalf("normalizeNRICLast5(%q) = %q, want %q", value, got, want)
			}
		})
	}

	invalidValues := []string{"12345", "123A4", "1234@", "1234AB", "123A", " 1234A", "1234A "}
	for _, value := range invalidValues {
		t.Run("invalid "+value, func(t *testing.T) {
			if got, ok := normalizeNRICLast5(value); ok || got != "" {
				t.Fatalf("normalizeNRICLast5(%q) = (%q, %v), want empty false", value, got, ok)
			}
		})
	}
}

// --- CreateUser validation (pure function — no DB required) ---

func TestNormalizeFullName(t *testing.T) {
	tests := map[string]string{
		"John Doe":            "John Doe",
		"  John Doe  ":        "John Doe",
		"  John   Doe  ":      "John Doe",
		"John\tDoe":           "John Doe",
		"John\nDoe":           "John Doe",
		"  John   M.   Doe  ": "John M. Doe",
		"":                    "",
		"   ":                 "",
	}
	for in, want := range tests {
		got := normalizeFullName(in)
		if got != want {
			t.Errorf("normalizeFullName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValidateCreateUserHappyPath(t *testing.T) {
	got, err := validateCreateUser(CreateUserRequest{
		FullName:  "  John   Doe  ",
		Rank:      "PTE",
		Battery:   "Alpha",
		NRICLast5: "1234a",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.FullName != "John Doe" {
		t.Errorf("FullName = %q, want \"John Doe\" (trimmed + collapsed)", got.FullName)
	}
	if got.NRICLast5 != "1234A" {
		t.Errorf("NRICLast5 = %q, want \"1234A\" (uppercased)", got.NRICLast5)
	}
}

func TestValidateCreateUserDOB(t *testing.T) {
	t.Run("empty DOB is allowed", func(t *testing.T) {
		got, err := validateCreateUser(CreateUserRequest{
			FullName: "John Doe", Rank: "PTE", Battery: "Alpha", NRICLast5: "1234A", DOB: "",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.DOB != "" {
			t.Errorf("DOB = %q, want empty", got.DOB)
		}
	})
	t.Run("six-char DOB is accepted as-is", func(t *testing.T) {
		got, err := validateCreateUser(CreateUserRequest{
			FullName: "John Doe", Rank: "PTE", Battery: "Alpha", NRICLast5: "1234A", DOB: "150393",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.DOB != "150393" {
			t.Errorf("DOB = %q, want \"150393\"", got.DOB)
		}
	})
	t.Run("non-six-char DOB is rejected", func(t *testing.T) {
		_, err := validateCreateUser(CreateUserRequest{
			FullName: "John Doe", Rank: "PTE", Battery: "Alpha", NRICLast5: "1234A", DOB: "15-03-93",
		})
		if err == nil {
			t.Fatal("expected error for non-DDMMYY DOB, got nil")
		}
		if !strings.Contains(err.Error(), "DDMMYY") {
			t.Errorf("error %q should mention DDMMYY", err.Error())
		}
	})
}

func TestValidateCreateUserRejectsBadInput(t *testing.T) {
	cases := []struct {
		name string
		req  CreateUserRequest
		want string // substring expected in the error message
	}{
		{"empty fullName", CreateUserRequest{FullName: "   ", Rank: "PTE", Battery: "Alpha", NRICLast5: "1234A"}, "Full name"},
		{"empty rank", CreateUserRequest{FullName: "John", Rank: "", Battery: "Alpha", NRICLast5: "1234A"}, "Rank"},
		{"invalid rank", CreateUserRequest{FullName: "John", Rank: "FOO", Battery: "Alpha", NRICLast5: "1234A"}, "Invalid rank"},
		{"invalid battery", CreateUserRequest{FullName: "John", Rank: "PTE", Battery: "Foxtrot", NRICLast5: "1234A"}, "battery"},
		{"bad NRIC format", CreateUserRequest{FullName: "John", Rank: "PTE", Battery: "Alpha", NRICLast5: "12345"}, "NRIC"},
		{"bad NRIC short", CreateUserRequest{FullName: "John", Rank: "PTE", Battery: "Alpha", NRICLast5: "1A"}, "NRIC"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := validateCreateUser(tc.req)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q should mention %q", err.Error(), tc.want)
			}
		})
	}
}

// TestCreateUser_RejectsInvalidJSON — short-circuit before DB.
func TestCreateUser_RejectsInvalidJSON(t *testing.T) {
	h := &UserHandler{db: nil}
	req := httptest.NewRequest(http.MethodPost, "/api/users", bytes.NewBufferString("not-json"))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(ctxWithSuperadmin(req.Context()))
	rec := httptest.NewRecorder()

	h.CreateUser(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

// TestCreateUser_RejectsValidationErrorsBeforeDB — every short-circuit
// branch of validateCreateUser must return 400 without touching the DB.
// (h.db is nil, so any DB access would panic.)
func TestCreateUser_RejectsValidationErrorsBeforeDB(t *testing.T) {
	cases := []CreateUserRequest{
		{FullName: "", Rank: "PTE", Battery: "Alpha", NRICLast5: "1234A"},
		{FullName: "John", Rank: "BOGUS", Battery: "Alpha", NRICLast5: "1234A"},
		{FullName: "John", Rank: "PTE", Battery: "Bogus", NRICLast5: "1234A"},
		{FullName: "John", Rank: "PTE", Battery: "Alpha", NRICLast5: "ZZZZZ"},
		{FullName: "John", Rank: "PTE", Battery: "Alpha", NRICLast5: "1234A", DOB: "bad-dob"},
	}
	for i, req := range cases {
		h := &UserHandler{db: nil}
		body, _ := json.Marshal(req)
		httpReq := httptest.NewRequest(http.MethodPost, "/api/users", bytes.NewReader(body))
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq = httpReq.WithContext(ctxWithSuperadmin(httpReq.Context()))
		rec := httptest.NewRecorder()

		h.CreateUser(rec, httpReq)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("case %d (%+v): status = %d, want 400", i, req, rec.Code)
		}
	}
}

// --- BulkDeleteUsers (pure helper + short-circuit paths) ---

func TestPartitionBulkDeleteIDs(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		targets, skipped := partitionBulkDeleteIDs(
			[]string{"a", "b", "c"}, "self-id",
		)
		if !equalStrings(targets, []string{"a", "b", "c"}) {
			t.Errorf("targets = %v, want [a b c]", targets)
		}
		if len(skipped) != 0 {
			t.Errorf("skipped = %v, want empty", skipped)
		}
	})
	t.Run("self is skipped", func(t *testing.T) {
		targets, skipped := partitionBulkDeleteIDs(
			[]string{"self-id", "b"}, "self-id",
		)
		if !equalStrings(targets, []string{"b"}) {
			t.Errorf("targets = %v, want [b]", targets)
		}
		if len(skipped) != 1 || skipped[0].ID != "self-id" || skipped[0].Reason != "self" {
			t.Errorf("skipped = %v, want one self skip", skipped)
		}
	})
	t.Run("seeded admin is skipped", func(t *testing.T) {
		targets, skipped := partitionBulkDeleteIDs(
			[]string{seededAdminID, "b"}, "self-id",
		)
		if !equalStrings(targets, []string{"b"}) {
			t.Errorf("targets = %v, want [b]", targets)
		}
		if len(skipped) != 1 || skipped[0].ID != seededAdminID || skipped[0].Reason != "system_admin" {
			t.Errorf("skipped = %v, want one system_admin skip", skipped)
		}
	})
	t.Run("duplicates are coalesced", func(t *testing.T) {
		targets, _ := partitionBulkDeleteIDs(
			[]string{"a", "a", "b", "a"}, "self-id",
		)
		if !equalStrings(targets, []string{"a", "b"}) {
			t.Errorf("targets = %v, want [a b] (dedup)", targets)
		}
	})
	t.Run("empty strings are dropped", func(t *testing.T) {
		targets, _ := partitionBulkDeleteIDs(
			[]string{"", "a", ""}, "self-id",
		)
		if !equalStrings(targets, []string{"a"}) {
			t.Errorf("targets = %v, want [a] (empty dropped)", targets)
		}
	})
}

func TestBulkDeleteUsers_RequiresAuthentication(t *testing.T) {
	h := &UserHandler{db: nil}
	body, _ := json.Marshal(BulkDeleteRequest{UserIDs: []string{"abc"}})
	req := httptest.NewRequest(http.MethodPost, "/api/users/bulk-delete", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.BulkDeleteUsers(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestBulkDeleteUsers_RejectsInvalidJSON(t *testing.T) {
	h := &UserHandler{db: nil}
	req := httptest.NewRequest(http.MethodPost, "/api/users/bulk-delete", bytes.NewBufferString("not-json"))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(ctxWithSuperadmin(req.Context()))
	rec := httptest.NewRecorder()

	h.BulkDeleteUsers(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestBulkDeleteUsers_RejectsEmptyArray(t *testing.T) {
	h := &UserHandler{db: nil}
	body, _ := json.Marshal(BulkDeleteRequest{UserIDs: []string{}})
	req := httptest.NewRequest(http.MethodPost, "/api/users/bulk-delete", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(ctxWithSuperadmin(req.Context()))
	rec := httptest.NewRecorder()

	h.BulkDeleteUsers(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

// TestBulkDeleteUsers_AllSkipped_NoDBAccess — when every requested id is
// either self or the system admin, the handler must not call the DB. We
// pass a nil DB and rely on the handler short-circuiting before touching
// it.
func TestBulkDeleteUsers_AllSkipped_NoDBAccess(t *testing.T) {
	h := &UserHandler{db: nil}
	body, _ := json.Marshal(BulkDeleteRequest{
		UserIDs: []string{"superadmin-test", seededAdminID},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/users/bulk-delete", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(ctxWithSuperadmin(req.Context()))
	rec := httptest.NewRecorder()

	h.BulkDeleteUsers(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var resp BulkDeleteResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Deleted) != 0 {
		t.Errorf("deleted = %v, want empty", resp.Deleted)
	}
	if len(resp.Skipped) != 2 {
		t.Errorf("skipped = %v, want 2 entries", resp.Skipped)
	}
	reasons := map[string]string{}
	for _, s := range resp.Skipped {
		reasons[s.ID] = s.Reason
	}
	if reasons["superadmin-test"] != "self" {
		t.Errorf("self skip reason = %q, want \"self\"", reasons["superadmin-test"])
	}
	if reasons[seededAdminID] != "system_admin" {
		t.Errorf("system_admin skip reason = %q, want \"system_admin\"", reasons[seededAdminID])
	}
	if resp.Summary.Requested != 2 || resp.Summary.Deleted != 0 || resp.Summary.Skipped != 2 || resp.Summary.Failed != 0 {
		t.Errorf("summary = %+v, want {2 0 2 0}", resp.Summary)
	}
}

// --- helpers ---

// ctxWithSuperadmin injects a fake superadmin user into context so that
// middleware.GetUserFromContext returns (user, true). The user id is
// "superadmin-test" so tests can pass it as a "self" id when probing the
// self-protection branch.
func ctxWithSuperadmin(ctx context.Context) context.Context {
	user := &models.User{ID: "superadmin-test", IsSuperadmin: true}
	return context.WithValue(ctx, middleware.UserKey, user)
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
