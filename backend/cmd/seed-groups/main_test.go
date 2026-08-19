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

// TestCommanderRankThreshold exercises the shared commander threshold (3SG)
// through CSS Commanders: 3SG is the lowest rank included.
func TestCommanderRankThreshold(t *testing.T) {
	r := ruleFor(t, "CSS Commanders")
	cases := []struct {
		name string
		row  row
		want bool
	}{
		{"3SG in CSS", row{rank: "3SG", sub2: "MT PL"}, true},
		{"2SG in CSS", row{rank: "2SG", sub2: "MT PL"}, true},
		{"1SG in CSS", row{rank: "1SG", sub2: "MT PL"}, true},
		{"CPT in CSS", row{rank: "CPT", sub2: "MEDICAL PL"}, true},
		{"3WO in CSS", row{rank: "3WO", sub2: "S1 CELL", sub1: "S1 BR"}, true},
		{"CFC in CSS is below threshold", row{rank: "CFC", sub2: "MT PL"}, false},
		{"3SG outside CSS", row{rank: "3SG", sub2: "QM & SVCS PL"}, false},
		{"unknown rank in CSS", row{rank: "MAJOR", sub2: "MT PL"}, false},
	}
	for _, tc := range cases {
		if got := r.matches(tc.row); got != tc.want {
			t.Errorf("%s: matches(%+v) = %v, want %v", tc.name, tc.row, got, tc.want)
		}
	}
}

func TestBatteryCommanders(t *testing.T) {
	cases := []struct {
		name string
		rule string
		row  row
		want bool
	}{
		{"A 3SG gun det", "A Commanders", row{rank: "3SG", sub1: "FIELD ARTY BTY A"}, true},
		{"A CPL gun det", "A Commanders", row{rank: "CPL", sub1: "FIELD ARTY BTY A"}, false},
		{"A 3SG in B bty", "A Commanders", row{rank: "3SG", sub1: "FIELD ARTY BTY B"}, false},
		{"B 2SG det cmd", "B Commanders", row{rank: "2SG", sub1: "FIELD ARTY BTY B"}, true},
		{"B LCP gun det", "B Commanders", row{rank: "LCP", sub1: "FIELD ARTY BTY B"}, false},
		{"HQ 3SG QM", "HQ Commanders", row{rank: "3SG", sub1: "HQ BTY"}, true},
		{"HQ PTE sig", "HQ Commanders", row{rank: "PTE", sub1: "HQ BTY"}, false},
		{"HQ S1 3SG excluded", "HQ Commanders", row{rank: "3SG", sub1: "S1 BR"}, false},
		{"HQ RnS 2SG excluded", "HQ Commanders", row{rank: "2SG", sub1: "BN RECCE & SURVEY TM"}, false},
		{"HQ FDC 3SG excluded", "HQ Commanders", row{rank: "3SG", sub1: "FIRE DIRECTION CEN"}, false},
		{"HQ A-bty 3SG excluded", "HQ Commanders", row{rank: "3SG", sub1: "FIELD ARTY BTY A"}, false},
		{"HQ B-bty 3SG excluded", "HQ Commanders", row{rank: "3SG", sub1: "FIELD ARTY BTY B"}, false},
	}
	for _, tc := range cases {
		r := ruleFor(t, tc.rule)
		if got := r.matches(tc.row); got != tc.want {
			t.Errorf("%s: %s matches(%+v) = %v, want %v", tc.name, tc.rule, tc.row, got, tc.want)
		}
	}
}

func TestAllCommanders(t *testing.T) {
	r := ruleFor(t, "All Commanders")
	cases := []struct {
		name string
		row  row
		want bool
	}{
		{"3SG anywhere", row{rank: "3SG", sub1: "BN HQ"}, true},
		{"CPT anywhere", row{rank: "CPT", sub1: "NON-ESTAB"}, true},
		{"CFC anywhere", row{rank: "CFC", sub1: "HQ BTY"}, false},
		{"empty rank", row{}, false},
	}
	for _, tc := range cases {
		if got := r.matches(tc.row); got != tc.want {
			t.Errorf("%s: matches(%+v) = %v, want %v", tc.name, tc.row, got, tc.want)
		}
	}
}

// TestCommandersSubsetOfBase ensures every commanders group is a subset of its
// base group, so a commanders group can never outgrow the parent.
func TestCommandersSubsetOfBase(t *testing.T) {
	pairs := []struct {
		commander string
		base      func(r row) bool
	}{
		{"CSS Commanders", matchesCSS},
		{"A Commanders", matchesA},
		{"B Commanders", matchesB},
		{"HQ Commanders", matchesHQ},
		{"All Commanders", matchesAll},
	}
	samples := []row{
		{rank: "3SG", sub1: "FIELD ARTY BTY A", sub2: "GUN DET 1"},
		{rank: "2SG", sub1: "FIELD ARTY BTY B", sub2: "BTY HQ"},
		{rank: "1SG", sub1: "HQ BTY", sub2: "MT PL"},
		{rank: "CPT", sub1: "S1 BR", sub2: "S1 CELL"},
		{rank: "LTA", sub1: "S1 BR", sub2: "PERSONNEL SP PL"},
		{rank: "3WO", sub1: "RSM GP"},
	}
	for _, p := range pairs {
		r := ruleFor(t, p.commander)
		for _, s := range samples {
			if r.matches(s) && !p.base(s) {
				t.Errorf("%s: %+v matches but base does not", p.commander, s)
			}
		}
	}
}
