// Package identity derives the stable key used to recognise a person across
// roster imports.
//
// Imports used to match on (full_name, nric_last5), which forced the NRIC to be
// stored in readable form. The key is now an HMAC over the normalised name and
// date of birth, so the same person is recognised on re-import without either
// value being retrievable from the database.
//
// The HMAC must be keyed. Date of birth for a national-service cohort has only
// about 7,000 possibilities, and names are not secret, so an unkeyed hash of the
// same input is recoverable by brute force in milliseconds regardless of which
// hash algorithm is used. The secret is what makes the stored key opaque, and it
// must live outside the database — otherwise a leaked dump contains both halves.
package identity

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"regexp"
	"strings"
	"unicode"
)

// ErrNoSecret reports that no signing secret is configured. Callers must fail
// closed rather than falling back to a weaker key such as the name alone: a
// name-only match would silently merge the people on this roster who share a
// full name.
var ErrNoSecret = errors.New("identity: no signing secret configured")

var punctuation = regexp.MustCompile(`[.'\-,]`)
var whitespace = regexp.MustCompile(`\s+`)
var digits = regexp.MustCompile(`\D`)

// Deriver turns a name and date of birth into an opaque, stable key.
type Deriver struct {
	secret []byte
}

// NewDeriver returns a Deriver, or ErrNoSecret when the secret is empty.
func NewDeriver(secret string) (*Deriver, error) {
	trimmed := strings.TrimSpace(secret)
	if trimmed == "" {
		return nil, ErrNoSecret
	}
	return &Deriver{secret: []byte(trimmed)}, nil
}

// NormaliseName makes the name comparison tolerant of the formatting differences
// that appear between exports of the same roster: case, extra spaces, and the
// punctuation used in names such as "LEE XUE WEN, BERTRAM".
func NormaliseName(name string) string {
	upper := strings.ToUpper(name)
	upper = punctuation.ReplaceAllString(upper, " ")
	upper = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == ' ' {
			return r
		}
		return ' '
	}, upper)
	return strings.TrimSpace(whitespace.ReplaceAllString(upper, " "))
}

// NormaliseDOB reduces a date of birth to its digits, so that "06.11.1986",
// "06/11/1986" and "06111986" all produce the same key.
func NormaliseDOB(dob string) string {
	return digits.ReplaceAllString(dob, "")
}

// Key returns the import key for a person, or an empty string when either input
// is missing. An empty key must never be treated as a match — two people both
// lacking a date of birth are not the same person.
func (d *Deriver) Key(name, dob string) string {
	n := NormaliseName(name)
	b := NormaliseDOB(dob)
	if n == "" || b == "" {
		return ""
	}
	mac := hmac.New(sha256.New, d.secret)
	mac.Write([]byte(n))
	mac.Write([]byte("|"))
	mac.Write([]byte(b))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
