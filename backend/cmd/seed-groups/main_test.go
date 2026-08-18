package main

import (
	"testing"
)

func ruleFor(t *testing.T, name string) rule {
	t.Helper()
	for _, r := range rules {
		if r.group == name {
			return r
		}
	}
	t.Fatalf("rule %q not found", name)
	return rule{}
}

func TestMTPlatoonRule(t *testing.T) {
	r := ruleFor(t, "MT Platoon")
	cases := []struct {
		name string
		row  row
		want bool
	}{
		{"MT PL member", row{sub2: "MT PL"}, true},
		{"medical platoon", row{sub2: "MEDICAL PL"}, false},
		{"no sub-unit", row{}, false},
	}
	for _, tc := range cases {
		if got := r.matches(tc.row); got != tc.want {
			t.Errorf("%s: matches(%+v) = %v, want %v", tc.name, tc.row, got, tc.want)
		}
	}
}

func TestTechniciansRule(t *testing.T) {
	r := ruleFor(t, "Technicians")
	cases := []struct {
		name string
		row  row
		want bool
	}{
		{"auto tech", row{voc: "AUTO TECH"}, true},
		{"auto spec tech", row{voc: "AUTO SPEC TECH"}, true},
		{"armt tech", row{voc: "ARMT TECH"}, true},
		{"armt spec tech", row{voc: "ARMT SPEC TECH"}, true},
		{"supply assistant", row{voc: "SUP ASST(ORD)"}, false},
		{"driver", row{voc: "DVR"}, false},
		{"empty vocation", row{}, false},
	}
	for _, tc := range cases {
		if got := r.matches(tc.row); got != tc.want {
			t.Errorf("%s: matches(%+v) = %v, want %v", tc.name, tc.row, got, tc.want)
		}
	}
}

func TestMatchesCSS(t *testing.T) {
	cases := []struct {
		name string
		row  row
		want bool
	}{
		{"S1 cell", row{sub1: "S1 BR", sub2: "S1 CELL"}, true},
		{"MT platoon", row{sub2: "MT PL"}, true},
		{"medical platoon", row{sub2: "MEDICAL PL"}, true},
		{"HQ combat train", row{sub2: "HQ COMBAT TRAIN"}, true},
		{"S4/OC HQ", row{posDesc: "S4/OC HQ"}, true},
		{"HQ-bty BSM", row{sub2: "BTY HQ", posDesc: "BSM"}, true},
		{"battery combat train", row{sub2: "COMBAT TRAIN"}, false},
		{"personnel support platoon", row{sub2: "PERSONNEL SP PL"}, false},
		{"QM and services platoon", row{sub2: "QM & SVCS PL"}, false},
		{"battery recce group", row{sub2: "BTY RECCE GP"}, false},
	}
	for _, tc := range cases {
		if got := matchesCSS(tc.row); got != tc.want {
			t.Errorf("%s: matchesCSS(%+v) = %v, want %v", tc.name, tc.row, got, tc.want)
		}
	}
}

func TestMatchesCSSCommander(t *testing.T) {
	cases := []struct {
		name string
		row  row
		want bool
	}{
		{"1SG in CSS", row{rank: "1SG", sub2: "MT PL"}, true},
		{"CPT in CSS", row{rank: "CPT", sub2: "MEDICAL PL"}, true},
		{"3WO in CSS", row{rank: "3WO", sub2: "S1 CELL", sub1: "S1 BR"}, true},
		{"2SG in CSS is below threshold", row{rank: "2SG", sub2: "MT PL"}, false},
		{"1SG outside CSS", row{rank: "1SG", sub2: "QM & SVCS PL"}, false},
		{"unknown rank in CSS", row{rank: "MAJOR", sub2: "MT PL"}, false},
	}
	for _, tc := range cases {
		if got := matchesCSSCommander(tc.row); got != tc.want {
			t.Errorf("%s: matchesCSSCommander(%+v) = %v, want %v", tc.name, tc.row, got, tc.want)
		}
	}
}

// TestCSSCommandersSubsetOfCSS ensures every CSS Commander row is also a CSS
// member, so the commanders group can never outgrow its parent group.
func TestCSSCommandersSubsetOfCSS(t *testing.T) {
	commander := ruleFor(t, "CSS Commanders")
	for _, tc := range []struct {
		name string
		row  row
	}{
		{"1SG MT WO", row{rank: "1SG", sub2: "MT PL"}},
		{"CPT S4/OC HQ", row{rank: "CPT", posDesc: "S4/OC HQ"}},
		{"3WO chief admin", row{rank: "3WO", sub1: "S1 BR", sub2: "S1 CELL"}},
		{"LTA MTO", row{rank: "LTA", sub2: "MT PL"}},
	} {
		if !commander.matches(tc.row) {
			t.Fatalf("precondition: %s should match CSS Commanders", tc.name)
		}
		if !matchesCSS(tc.row) {
			t.Errorf("%s: matches CSS Commanders but not CSS", tc.name)
		}
	}
}
