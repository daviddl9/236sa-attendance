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
