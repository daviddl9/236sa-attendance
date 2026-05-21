package handlers

import "strings"

const nricLast5FormatMessage = "NRIC Last 5 must be 4 digits followed by a letter (e.g., 1234A)"

func normalizeNRICLast5(value string) (string, bool) {
	if !isValidNRICLast5(value) {
		return "", false
	}
	return strings.ToUpper(value), true
}

func prepareSignInCredential(identifier string, password string) (string, *string, bool) {
	if identifier == "admin" {
		return password, nil, true
	}

	normalizedPassword, ok := normalizeNRICLast5(password)
	if !ok {
		return "", nil, false
	}
	return normalizedPassword, &normalizedPassword, true
}

func isValidNRICLast5(value string) bool {
	if len(value) != 5 {
		return false
	}
	if strings.TrimSpace(value) != value {
		return false
	}
	for i := 0; i < 4; i++ {
		if value[i] < '0' || value[i] > '9' {
			return false
		}
	}
	last := value[4]
	return (last >= 'A' && last <= 'Z') || (last >= 'a' && last <= 'z')
}
