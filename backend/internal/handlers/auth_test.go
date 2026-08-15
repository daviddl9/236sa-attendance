package handlers

import (
	"fmt"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

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

func TestVerifyLegacyCandidatesDOB(t *testing.T) {
	row := func(id, dob string) userRow { return userRow{id: id, dob: dob} }
	tests := []struct {
		name       string
		candidates []userRow
		dob        string
		wantID     string // "" means no match
	}{
		{"single matching DOB", []userRow{row("a", "06.11.1986")}, "1986-11-06", "a"},
		{"stored and typed formats differ", []userRow{row("a", "06.11.1986")}, "06111986", "a"},
		{"no matching DOB", []userRow{row("a", "06.11.1986")}, "01.01.2000", ""},
		{"no candidates", nil, "06.11.1986", ""},
		{"ambiguous shared name and DOB", []userRow{row("a", "06.11.1986"), row("b", "06.11.1986")}, "06.11.1986", ""},
		{"DOB disambiguates namesakes", []userRow{row("a", "06.11.1986"), row("b", "01.01.1990")}, "06.11.1986", "a"},
		{"invalid typed DOB", []userRow{row("a", "06.11.1986")}, "not-a-date", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := verifyLegacyCandidates(tt.candidates, "", tt.dob)
			if tt.wantID == "" {
				if got != nil {
					t.Fatalf("verifyLegacyCandidates() = %+v, want nil", got)
				}
				return
			}
			if got == nil || got.id != tt.wantID {
				t.Fatalf("verifyLegacyCandidates() = %+v, want id %q", got, tt.wantID)
			}
		})
	}
}

func TestVerifyLegacyCandidatesPassword(t *testing.T) {
	hash := func(t *testing.T, s string) string {
		t.Helper()
		h, err := bcrypt.GenerateFromPassword([]byte(s), bcrypt.MinCost)
		if err != nil {
			t.Fatalf("hash: %v", err)
		}
		return string(h)
	}
	row := func(id, password string) userRow { return userRow{id: id, password: password} }
	tests := []struct {
		name       string
		candidates []userRow
		password   string
		wantID     string // "" means no match
	}{
		{"single match", []userRow{row("a", hash(t, "1234A"))}, "1234A", "a"},
		{"lowercase typed against uppercased hash", []userRow{row("a", hash(t, "5678B"))}, "5678b", "a"},
		{"wrong password", []userRow{row("a", hash(t, "1234A"))}, "9999Z", ""},
		{"no candidates", nil, "1234A", ""},
		{"ambiguous shared name and password", []userRow{row("a", hash(t, "4321X")), row("b", hash(t, "4321X"))}, "4321X", ""},
		{"password disambiguates namesakes", []userRow{row("a", hash(t, "1234A")), row("b", hash(t, "5678B"))}, "5678B", "b"},
		{"candidate list is capped before bcrypt", func() []userRow {
			rows := make([]userRow, 0, maxLegacyNameCandidates+1)
			for i := 0; i < maxLegacyNameCandidates; i++ {
				rows = append(rows, row(fmt.Sprintf("wrong-%d", i), hash(t, "0000A")))
			}
			// The only correct match sits beyond the cap and must be unreachable.
			return append(rows, row("right", hash(t, "1234A")))
		}(), "1234A", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := verifyLegacyCandidates(tt.candidates, tt.password, "")
			if tt.wantID == "" {
				if got != nil {
					t.Fatalf("verifyLegacyCandidates() = %+v, want nil", got)
				}
				return
			}
			if got == nil || got.id != tt.wantID {
				t.Fatalf("verifyLegacyCandidates() = %+v, want id %q", got, tt.wantID)
			}
		})
	}
}
