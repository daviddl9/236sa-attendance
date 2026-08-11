package matching

import (
	"testing"
)

func TestRankCandidates(t *testing.T) {
	str := func(value string) *string { return &value }
	tests := []struct {
		name         string
		registration Registration
		roster       []RosterRow
		wantID       string
		wantMin      int
		wantRank     bool
		wantBattery  bool
		wantClaimed  bool
		wantOrder    []string
	}{
		{
			name:         "exact",
			registration: Registration{Name: "TAN WEI MING", Rank: "PTE", Battery: "Alpha"},
			roster:       []RosterRow{{ID: "exact", Name: "TAN WEI MING", Rank: "PTE", Battery: "Alpha"}},
			wantID:       "exact", wantMin: 100,
		},
		{
			name:         "single typo",
			registration: Registration{Name: "TAN WEI MIMG", Rank: "PTE", Battery: "Alpha"},
			roster:       []RosterRow{{ID: "typo", Name: "TAN WEI MING", Rank: "PTE", Battery: "Alpha"}},
			wantID:       "typo", wantMin: 90,
		},
		{
			name:         "reordered",
			registration: Registration{Name: "WEI MING TAN", Rank: "PTE", Battery: "Alpha"},
			roster:       []RosterRow{{ID: "reordered", Name: "TAN WEI MING", Rank: "PTE", Battery: "Alpha"}},
			wantID:       "reordered", wantMin: 100,
		},
		{
			name:         "patronymic dropped",
			registration: Registration{Name: "MUHAMMAD ALI", Rank: "PTE", Battery: "Alpha"},
			roster:       []RosterRow{{ID: "patronymic", Name: "MUHAMMAD BIN ALI", Rank: "PTE", Battery: "Alpha"}},
			wantID:       "patronymic", wantMin: 100,
		},
		{
			name:         "wrong rank keeps right name first",
			registration: Registration{Name: "TAN WEI MING", Rank: "LCP", Battery: "Alpha"},
			roster: []RosterRow{
				{ID: "right-name", Name: "TAN WEI MING", Rank: "CPL", Battery: "Alpha"},
				{ID: "wrong-name", Name: "TAN WEI MIN", Rank: "LCP", Battery: "Alpha"},
			},
			wantID: "right-name", wantMin: 90, wantRank: true,
		},
		{
			name:         "wrong battery keeps right name first",
			registration: Registration{Name: "TAN WEI MING", Rank: "PTE", Battery: "Alpha"},
			roster: []RosterRow{
				{ID: "right-name", Name: "TAN WEI MING", Rank: "PTE", Battery: "Bravo"},
				{ID: "wrong-name", Name: "TAN WEI MIN", Rank: "PTE", Battery: "Alpha"},
			},
			wantID: "right-name", wantMin: 90, wantBattery: true,
		},
		{
			name:         "two same-named soldiers use battery bonus",
			registration: Registration{Name: "TAN WEI MING", Rank: "PTE", Battery: "Bravo"},
			roster: []RosterRow{
				{ID: "alpha", Name: "TAN WEI MING", Rank: "PTE", Battery: "Alpha"},
				{ID: "bravo", Name: "TAN WEI MING", Rank: "PTE", Battery: "Bravo"},
			},
			wantID: "bravo", wantMin: 100, wantOrder: []string{"bravo", "alpha"},
		},
		{
			name:         "unrelated name is low and not preselected",
			registration: Registration{Name: "LIM AH KOW", Rank: "PTE", Battery: "Alpha"},
			roster:       []RosterRow{{ID: "unrelated", Name: "TAN WEI MING", Rank: "CPL", Battery: "Bravo"}},
			wantID:       "unrelated", wantMin: 0,
		},
		{
			name:         "already claimed is returned flagged and unavailable",
			registration: Registration{Name: "TAN WEI MING", Rank: "PTE", Battery: "Alpha"},
			roster:       []RosterRow{{ID: "claimed", Name: "TAN WEI MING", Rank: "PTE", Battery: "Alpha", Username: str("existing")}},
			wantID:       "claimed", wantMin: 100, wantClaimed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RankCandidates(tt.registration, tt.roster)
			if len(got) == 0 {
				t.Fatal("RankCandidates returned no candidates")
			}
			if got[0].ID != tt.wantID {
				t.Fatalf("top candidate = %q, want %q; candidates = %+v", got[0].ID, tt.wantID, got)
			}
			if got[0].Score < tt.wantMin {
				t.Fatalf("top score = %d, want at least %d", got[0].Score, tt.wantMin)
			}
			if tt.wantRank && !got[0].RankMismatch {
				t.Errorf("rank mismatch = %v, want true", got[0].RankMismatch)
			}
			if tt.wantBattery && !got[0].BatteryMismatch {
				t.Errorf("battery mismatch = %v, want true", got[0].BatteryMismatch)
			}
			if got[0].AlreadyClaimed != tt.wantClaimed {
				t.Errorf("already claimed = %v, want %v", got[0].AlreadyClaimed, tt.wantClaimed)
			}
			if tt.wantClaimed && got[0].Selectable {
				t.Error("claimed candidate is selectable")
			}
			if tt.wantOrder != nil {
				if len(got) != len(tt.wantOrder) {
					t.Fatalf("candidate count = %d, want %d", len(got), len(tt.wantOrder))
				}
				for i, wantID := range tt.wantOrder {
					if got[i].ID != wantID {
						t.Errorf("candidate %d = %q, want %q", i, got[i].ID, wantID)
					}
				}
			}
			if tt.name == "unrelated name is low and not preselected" && got[0].Preselected {
				t.Error("unrelated candidate was preselected")
			}
		})
	}
}

func TestCandidateStrengthClassification(t *testing.T) {
	tests := []struct {
		name         string
		registration Registration
		rosterName   string
		wantStrong   bool
	}{
		{
			name:         "MUTHU and MOHAN uses name ratio instead of absolute distance",
			registration: Registration{Name: "MUTHU", Rank: "PTE", Battery: "Alpha"},
			rosterName:   "MOHAN",
			wantStrong:   false,
		},
		{
			name:         "TAN and LIM is weak",
			registration: Registration{Name: "TAN", Rank: "PTE", Battery: "Alpha"},
			rosterName:   "LIM",
			wantStrong:   false,
		},
		{
			name:         "single typo on a normal name is strong",
			registration: Registration{Name: "TAN WEI MIMG", Rank: "PTE", Battery: "Alpha"},
			rosterName:   "TAN WEI MING",
			wantStrong:   true,
		},
		{
			name:         "three typos with matching attributes reach strong",
			registration: Registration{Name: "TAN XEI XANG", Rank: "PTE", Battery: "Alpha"},
			rosterName:   "TAN WEI MING",
			wantStrong:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidates := RankCandidates(tt.registration, []RosterRow{{
				ID: "candidate", Name: tt.rosterName, Rank: "PTE", Battery: "Alpha",
			}})
			candidate := candidates[0]
			if candidate.Strong != tt.wantStrong {
				t.Fatalf("candidate score = %d, strong = %v, want %v", candidate.Score, candidate.Strong, tt.wantStrong)
			}
			if tt.wantStrong && candidate.Score < StrongCandidateScore {
				t.Fatalf("candidate score = %d, want at least %d", candidate.Score, StrongCandidateScore)
			}
			if !tt.wantStrong && candidate.Score >= StrongCandidateScore {
				t.Fatalf("candidate score = %d, want below %d", candidate.Score, StrongCandidateScore)
			}
			if !tt.wantStrong && candidate.Preselected {
				t.Fatal("weak candidate was preselected")
			}
		})
	}
}

func TestRankCandidatesReturnsTopFiveDescending(t *testing.T) {
	roster := make([]RosterRow, 7)
	for i := range roster {
		roster[i] = RosterRow{ID: string(rune('a' + i)), Name: "TAN WEI MING", Rank: "PTE", Battery: "Alpha"}
	}
	got := RankCandidates(Registration{Name: "TAN WEI MING", Rank: "PTE", Battery: "Alpha"}, roster)
	if len(got) != 5 {
		t.Fatalf("candidate count = %d, want 5", len(got))
	}
	for i := 1; i < len(got); i++ {
		if got[i].Score > got[i-1].Score {
			t.Fatalf("scores not descending: %+v", got)
		}
	}
}

func TestRankCandidatesAmbiguousTopTwoAreNotPreselected(t *testing.T) {
	candidates := []Candidate{
		{ID: "top", Score: 96, Selectable: true},
		{ID: "runner-up", Score: 94, Selectable: true},
	}
	got := MarkPreselection(candidates)
	for _, candidate := range got {
		if candidate.Preselected {
			t.Fatalf("candidate %q was preselected in an ambiguous pair", candidate.ID)
		}
	}
}

func TestNormalizeName(t *testing.T) {
	got := NormalizeName("  Tan  Wei-Ming, S/O  ")
	if got != "TAN WEIMING" {
		t.Fatalf("NormalizeName() = %q, want %q", got, "TAN WEIMING")
	}
}
