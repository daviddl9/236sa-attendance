// Package deeplink generates unguessable Telegram session codes.
package deeplink

import (
	"crypto/rand"
	"encoding/base64"
)

const codeBytes = 16

// GenerateCode returns 16 random bytes encoded as unpadded base64url.
func GenerateCode() (string, error) {
	bytes := make([]byte, codeBytes)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}
