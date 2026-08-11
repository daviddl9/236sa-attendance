package handlers

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/davidlivingston/go-nextjs-starter/backend/internal/models"
)

const (
	nricLast5FormatMessage        = "NRIC Last 5 must be exactly 5 characters: 4 numbers followed by an alphabet letter (e.g., 1234A)"
	migratedPendingUsernamePrefix = "__migrated_pending__"
)

var nricShapedPassword = regexp.MustCompile(`^\d{4}[A-Za-z]$`)

func normalizeUsername(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func validateSignUpRequest(req SignUpRequest) (SignUpRequest, error) {
	req.Username = strings.TrimSpace(req.Username)
	req.FullName = strings.TrimSpace(req.FullName)
	req.Rank = strings.TrimSpace(req.Rank)
	req.Battery = strings.TrimSpace(req.Battery)

	switch {
	case req.Username == "":
		return req, newSignUpValidationError("Username is required")
	case strings.HasPrefix(normalizeUsername(req.Username), migratedPendingUsernamePrefix):
		return req, newSignUpValidationError("Username is unavailable")
	}
	if err := validatePassword(req.Password, req.ConfirmPassword); err != nil {
		return req, err
	}
	switch {
	case req.FullName == "":
		return req, newSignUpValidationError("Full name is required")
	case !isValidRank(req.Rank):
		return req, newSignUpValidationError("Invalid rank")
	case !isValidBattery(req.Battery):
		return req, newSignUpValidationError("Invalid battery (must be HQ, Alpha, or Bravo)")
	default:
		return req, nil
	}
}

func validatePassword(password, confirmation string) error {
	switch {
	case password == "":
		return newSignUpValidationError("Password is required")
	case nricShapedPassword.MatchString(password):
		return newSignUpValidationError("Do not use your NRIC digits as your password")
	case utf8.RuneCountInString(password) < 8:
		return newSignUpValidationError("Password must be at least 8 characters")
	case confirmation != password:
		return newSignUpValidationError("Passwords do not match")
	default:
		return nil
	}
}

type signUpValidationError struct{ message string }

func newSignUpValidationError(message string) error {
	return &signUpValidationError{message: message}
}

func (e *signUpValidationError) Error() string { return e.message }

func isValidRank(rank string) bool {
	for _, valid := range models.ValidRanks {
		if rank == valid {
			return true
		}
	}
	return false
}

func isValidBattery(battery string) bool {
	return battery == models.BatteryHQ || battery == models.BatteryAlpha || battery == models.BatteryBravo
}

func normalizeNRICLast5(value string) (string, bool) {
	if !isValidNRICLast5(value) {
		return "", false
	}
	return strings.ToUpper(value), true
}

func prepareSignInCredential(identifier string, password string) (string, *string, bool) {
	if strings.EqualFold(identifier, "admin") {
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

// nameMatchesIdentifier reports whether every word the user typed appears as a
// word in the user's full name (case-insensitive, order-independent). It lets
// personnel sign in with any subset of their name's words in any order — e.g.
// "Andrew Tam" matches stored "Tam Le Xiang, Andrew", and "David" or "D David"
// match "D David Livingston". Punctuation is treated as a word separator.
func nameMatchesIdentifier(fullName *string, identifier string) bool {
	if fullName == nil {
		return false
	}
	typed := nameTokens(identifier)
	if len(typed) == 0 {
		return false
	}
	stored := make(map[string]struct{})
	for _, w := range nameTokens(*fullName) {
		stored[w] = struct{}{}
	}
	for _, w := range typed {
		if _, ok := stored[w]; !ok {
			return false
		}
	}
	return true
}

// nameTokens lowercases and splits on any non-alphanumeric rune, returning the
// word tokens. "Tam Le Xiang, Andrew" -> [tam le xiang andrew].
func nameTokens(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}
