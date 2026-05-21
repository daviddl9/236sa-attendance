package handlers

import "testing"

func TestIsValidNRICLast5(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "valid uppercase", value: "1234A", want: true},
		{name: "valid lowercase", value: "1234a", want: true},
		{name: "valid leading zeroes", value: "0001Z", want: true},
		{name: "all digits invalid", value: "12345", want: false},
		{name: "letter before end invalid", value: "123A4", want: false},
		{name: "symbol ending invalid", value: "1234@", want: false},
		{name: "too long invalid", value: "1234AB", want: false},
		{name: "too short invalid", value: "123A", want: false},
		{name: "leading space invalid", value: " 1234A", want: false},
		{name: "trailing space invalid", value: "1234A ", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isValidNRICLast5(tt.value); got != tt.want {
				t.Fatalf("isValidNRICLast5(%q) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}

func TestNormalizeNRICLast5(t *testing.T) {
	got, ok := normalizeNRICLast5("1234a")
	if !ok {
		t.Fatal("normalizeNRICLast5 returned ok=false for lowercase final letter")
	}
	if got != "1234A" {
		t.Fatalf("normalizeNRICLast5() = %q, want %q", got, "1234A")
	}

	if got, ok := normalizeNRICLast5("12345"); ok || got != "" {
		t.Fatalf("normalizeNRICLast5 invalid value = (%q, %v), want empty false", got, ok)
	}
}
