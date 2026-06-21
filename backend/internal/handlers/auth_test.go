package handlers

import "testing"

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
