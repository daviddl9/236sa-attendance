package handlers

import "testing"

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
