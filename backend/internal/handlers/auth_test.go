package handlers

import (
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestPrepareSignInCredentialRegularPersonnel(t *testing.T) {
	password, nricLast5, ok := prepareSignInCredential("PTE Tan", "1234a")
	if !ok {
		t.Fatal("prepareSignInCredential returned ok=false for valid lowercase personnel password")
	}
	if password != "1234A" {
		t.Fatalf("password = %q, want %q", password, "1234A")
	}
	if nricLast5 == nil || *nricLast5 != "1234A" {
		t.Fatalf("nricLast5 = %v, want 1234A", nricLast5)
	}
}

func TestPrepareSignInCredentialRejectsInvalidPersonnelPassword(t *testing.T) {
	tests := []string{"12345", "123A4", "1234@", "1234AB", "123A"}
	for _, password := range tests {
		t.Run(password, func(t *testing.T) {
			if _, nricLast5, ok := prepareSignInCredential("PTE Tan", password); ok || nricLast5 != nil {
				t.Fatalf("prepareSignInCredential(%q) = ok %v nric %v, want false nil", password, ok, nricLast5)
			}
		})
	}
}

func TestPrepareSignInCredentialExemptsAdminPasswordFormat(t *testing.T) {
	password, nricLast5, ok := prepareSignInCredential("admin", "not-a-nric-password")
	if !ok {
		t.Fatal("prepareSignInCredential returned ok=false for admin password")
	}
	if password != "not-a-nric-password" {
		t.Fatalf("password = %q, want original admin password", password)
	}
	if nricLast5 != nil {
		t.Fatalf("nricLast5 = %v, want nil for admin sign-in", nricLast5)
	}
}

func TestValidateSignUpRequest(t *testing.T) {
	tests := []struct {
		name    string
		req     SignUpRequest
		wantErr string
	}{
		{
			name: "valid request trims username",
			req: SignUpRequest{
				Username: " TanWM ", Password: "correct horse", ConfirmPassword: "correct horse",
				FullName: "Tan Wei Ming", Rank: "PTE", Battery: "Alpha",
			},
		},
		{
			name: "password exactly eight characters is accepted",
			req: SignUpRequest{
				Username: "tanwm", Password: "12345678", ConfirmPassword: "12345678",
				FullName: "Tan Wei Ming", Rank: "PTE", Battery: "Alpha",
			},
		},
		{
			name: "password beginning with digits is accepted when eight characters",
			req: SignUpRequest{
				Username: "tanwm", Password: "1234ABCD", ConfirmPassword: "1234ABCD",
				FullName: "Tan Wei Ming", Rank: "PTE", Battery: "Alpha",
			},
		},
		{
			name:    "missing username",
			req:     SignUpRequest{Password: "correct horse", ConfirmPassword: "correct horse", FullName: "Tan", Rank: "PTE", Battery: "Alpha"},
			wantErr: "Username",
		},
		{
			name:    "short password",
			req:     SignUpRequest{Username: "tanwm", Password: "short", ConfirmPassword: "short", FullName: "Tan", Rank: "PTE", Battery: "Alpha"},
			wantErr: "8 characters",
		},
		{
			name:    "NRIC-shaped password",
			req:     SignUpRequest{Username: "tanwm", Password: "1234A", ConfirmPassword: "1234A", FullName: "Tan", Rank: "PTE", Battery: "Alpha"},
			wantErr: "do not use your NRIC",
		},
		{
			name:    "confirmation mismatch",
			req:     SignUpRequest{Username: "tanwm", Password: "correct horse", ConfirmPassword: "different", FullName: "Tan", Rank: "PTE", Battery: "Alpha"},
			wantErr: "match",
		},
		{
			name:    "invalid rank",
			req:     SignUpRequest{Username: "tanwm", Password: "correct horse", ConfirmPassword: "correct horse", FullName: "Tan", Rank: "soldier", Battery: "Alpha"},
			wantErr: "rank",
		},
		{
			name:    "invalid battery",
			req:     SignUpRequest{Username: "tanwm", Password: "correct horse", ConfirmPassword: "correct horse", FullName: "Tan", Rank: "PTE", Battery: "Delta"},
			wantErr: "battery",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validateSignUpRequest(tt.req)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validateSignUpRequest() error = %v", err)
				}
				if got.Username != strings.TrimSpace(tt.req.Username) {
					t.Errorf("Username = %q, want trimmed value", got.Username)
				}
				return
			}
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tt.wantErr)) {
				t.Fatalf("validateSignUpRequest() error = %v, want substring %q", err, tt.wantErr)
			}
		})
	}
}

func TestPasswordHashCost(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("correct horse"), passwordHashCost)
	if err != nil {
		t.Fatalf("GenerateFromPassword() error = %v", err)
	}
	cost, err := bcrypt.Cost(hash)
	if err != nil {
		t.Fatalf("bcrypt.Cost() error = %v", err)
	}
	if cost != 12 {
		t.Fatalf("bcrypt cost = %d, want 12", cost)
	}
}

func TestNormalizeUsername(t *testing.T) {
	if got := normalizeUsername("  TanWM  "); got != "tanwm" {
		t.Fatalf("normalizeUsername() = %q, want tanwm", got)
	}
}

func TestNameMatchesIdentifier(t *testing.T) {
	str := func(s string) *string { return &s }
	tests := []struct {
		name       string
		fullName   *string
		identifier string
		want       bool
	}{
		// Accept: any subset of the name's words, in any order.
		{"full exact name", str("D David Livingston"), "D David Livingston", true},
		{"single word", str("D David Livingston"), "David", true},
		{"adjacent subset", str("D David Livingston"), "David Livingston", true},
		{"subset incl standalone initial", str("D David Livingston"), "D David", true},
		{"reordered + lowercase", str("D David Livingston"), "livingston david", true},
		{"reordered, comma ignored", str("Tam Le Xiang, Andrew"), "Andrew Tam", true},
		{"lowercase", str("Tam Le Xiang, Andrew"), "andrew tam", true},
		{"single word of many", str("Tam Le Xiang, Andrew"), "Tam", true},

		// Reject: any typed word not present in the name, or no words at all.
		{"wrong word", str("Tam Le Xiang, Andrew"), "Andrew Smith", false},
		{"near-miss whole word (Tan != Tam)", str("Tam Le Xiang, Andrew"), "Andrew Tan", false},
		{"extra word not in name", str("Tam Le Xiang, Andrew"), "Andrew Tam Lim", false},
		{"partial word, not whole", str("Alexander Tan"), "Alex", false},
		{"empty identifier", str("Alexander Tan"), "", false},
		{"whitespace-only identifier", str("Alexander Tan"), "   ", false},
		{"punctuation-only identifier", str("Alexander Tan"), "%", false},
		{"comma-only identifier", str("Alexander Tan"), ",", false},
		{"nil full name", nil, "David", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nameMatchesIdentifier(tt.fullName, tt.identifier); got != tt.want {
				t.Fatalf("nameMatchesIdentifier(fullName, %q) = %v, want %v", tt.identifier, got, tt.want)
			}
		})
	}
}
