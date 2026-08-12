package deeplink

import (
	"encoding/base64"
	"testing"
)

func TestGenerateCode(t *testing.T) {
	seen := make(map[string]struct{}, 256)
	for i := 0; i < 256; i++ {
		code, err := GenerateCode()
		if err != nil {
			t.Fatalf("GenerateCode: %v", err)
		}
		if len(code) != 22 {
			t.Fatalf("code length = %d, want 22 (%q)", len(code), code)
		}
		decoded, err := base64.RawURLEncoding.DecodeString(code)
		if err != nil {
			t.Fatalf("decode code %q: %v", code, err)
		}
		if len(decoded) != codeBytes {
			t.Fatalf("decoded length = %d, want %d", len(decoded), codeBytes)
		}
		if _, exists := seen[code]; exists {
			t.Fatalf("duplicate code generated: %q", code)
		}
		seen[code] = struct{}{}
	}
}
