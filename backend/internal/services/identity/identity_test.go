package identity

import (
	"errors"
	"testing"
)

func TestNewDeriverRequiresSecret(t *testing.T) {
	if _, err := NewDeriver("   "); !errors.Is(err, ErrNoSecret) {
		t.Fatalf("expected ErrNoSecret for a blank secret, got %v", err)
	}
}

func TestKeyIsStableAcrossFormatting(t *testing.T) {
	d, err := NewDeriver("test-secret")
	if err != nil {
		t.Fatal(err)
	}
	// The same person exported from the same roster twice, formatted differently.
	want := d.Key("LEE XUE WEN, BERTRAM", "06.11.1986")
	cases := []struct{ name, dob string }{
		{"lee xue wen bertram", "06111986"},
		{"  LEE  XUE  WEN,  BERTRAM ", "06/11/1986"},
		{"Lee Xue Wen, Bertram", "06-11-1986"},
	}
	for _, c := range cases {
		if got := d.Key(c.name, c.dob); got != want {
			t.Fatalf("Key(%q,%q) = %q, want %q", c.name, c.dob, got, want)
		}
	}
}

func TestKeyDistinguishesPeopleSharingAName(t *testing.T) {
	// The real roster contains people who share a full name; the date of birth is
	// what separates them.
	d, _ := NewDeriver("test-secret")
	a := d.Key("MUHAMMAD ALI", "01.01.1990")
	b := d.Key("MUHAMMAD ALI", "02.01.1990")
	if a == b || a == "" {
		t.Fatalf("namesakes with different DOBs must get different keys (a=%q b=%q)", a, b)
	}
}

func TestKeyDistinguishesPeopleSharingADOB(t *testing.T) {
	d, _ := NewDeriver("test-secret")
	a := d.Key("TAN WEI MING", "14.09.1990")
	b := d.Key("LIM AH KOW", "14.09.1990")
	if a == b {
		t.Fatal("different people sharing a DOB must get different keys")
	}
}

func TestKeyIsEmptyWhenInputMissing(t *testing.T) {
	d, _ := NewDeriver("test-secret")
	for _, c := range []struct{ name, dob string }{
		{"", "06.11.1986"},
		{"TAN WEI MING", ""},
		{"", ""},
		{"   ", "   "},
	} {
		if got := d.Key(c.name, c.dob); got != "" {
			t.Fatalf("Key(%q,%q) = %q, want empty so it can never be treated as a match", c.name, c.dob, got)
		}
	}
}

func TestKeyDependsOnSecret(t *testing.T) {
	// This is the property that makes the stored key opaque. Without it, the key
	// is recoverable by brute force over the ~7,000 plausible dates of birth.
	a, _ := NewDeriver("secret-one")
	b, _ := NewDeriver("secret-two")
	if a.Key("TAN WEI MING", "14.09.1990") == b.Key("TAN WEI MING", "14.09.1990") {
		t.Fatal("keys must differ under different secrets")
	}
}

func TestKeyIsNotReversibleByGuessingDOB(t *testing.T) {
	// Simulates the attack that rules out an unkeyed hash: the attacker knows the
	// name and tries every plausible date of birth. Without the secret, none of
	// the guesses reproduce the stored key.
	real, _ := NewDeriver("server-side-secret")
	target := real.Key("TAN WEI MING", "14.09.1990")

	attacker, _ := NewDeriver("attacker-does-not-know-the-secret")
	for year := 1986; year <= 2004; year++ {
		for month := 1; month <= 12; month++ {
			for day := 1; day <= 31; day++ {
				guess := attacker.Key("TAN WEI MING", padDate(day, month, year))
				if guess == target {
					t.Fatalf("recovered the key without the secret at %02d.%02d.%d", day, month, year)
				}
			}
		}
	}
}

func padDate(d, m, y int) string {
	return string(rune('0'+d/10)) + string(rune('0'+d%10)) + "." +
		string(rune('0'+m/10)) + string(rune('0'+m%10)) + "." +
		itoa4(y)
}

func itoa4(y int) string {
	return string(rune('0' + y/1000)) + string(rune('0'+(y/100)%10)) +
		string(rune('0'+(y/10)%10)) + string(rune('0'+y%10))
}
