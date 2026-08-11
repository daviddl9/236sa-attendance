// Package matching ranks pending registrations against existing roster rows.
package matching

import (
	"sort"
	"strings"
	"unicode"
)

// Registration contains the self-reported values used to find roster rows.
type Registration struct {
	Name    string
	Rank    string
	Battery string
}

// RosterRow contains the roster values relevant to matching. A non-empty
// Username means the row is already claimed by an account.
type RosterRow struct {
	ID       string
	Name     string
	Rank     string
	Battery  string
	Username *string
}

// StrongCandidateScore is the minimum score for a close roster match.
const StrongCandidateScore = 80

// Candidate is a ranked roster row. Preselected is only a UI suggestion; the
// approval endpoint still requires the commander to send an explicit user ID.
type Candidate struct {
	ID              string
	Name            string
	Rank            string
	Battery         string
	Score           int
	Strong          bool
	RankMismatch    bool
	BatteryMismatch bool
	AlreadyClaimed  bool
	ClaimedUsername string
	Selectable      bool
	Preselected     bool
	baseScore       int
}

// RankCandidates returns at most five candidates in descending score order.
func RankCandidates(registration Registration, roster []RosterRow) []Candidate {
	candidates := make([]Candidate, 0, len(roster))
	for _, row := range roster {
		score := Score(registration.Name, row.Name, registration.Rank, row.Rank, registration.Battery, row.Battery)
		candidate := Candidate{
			ID:              row.ID,
			Name:            row.Name,
			Rank:            row.Rank,
			Battery:         row.Battery,
			Score:           score,
			Strong:          score >= StrongCandidateScore,
			baseScore:       nameScore(registration.Name, row.Name),
			RankMismatch:    !attributesEqual(registration.Rank, row.Rank),
			BatteryMismatch: !attributesEqual(registration.Battery, row.Battery),
			Selectable:      row.Username == nil || strings.TrimSpace(*row.Username) == "",
		}
		if row.Username != nil {
			candidate.ClaimedUsername = strings.TrimSpace(*row.Username)
			candidate.AlreadyClaimed = candidate.ClaimedUsername != ""
			candidate.Selectable = !candidate.AlreadyClaimed
		}
		candidates = append(candidates, candidate)
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Score != candidates[j].Score {
			return candidates[i].Score > candidates[j].Score
		}
		if candidates[i].baseScore != candidates[j].baseScore {
			return candidates[i].baseScore > candidates[j].baseScore
		}
		if candidates[i].BatteryMismatch != candidates[j].BatteryMismatch {
			return !candidates[i].BatteryMismatch
		}
		if candidates[i].RankMismatch != candidates[j].RankMismatch {
			return !candidates[i].RankMismatch
		}
		return candidates[i].ID < candidates[j].ID
	})
	if len(candidates) > 5 {
		candidates = candidates[:5]
	}
	return MarkPreselection(candidates)
}

// MarkPreselection marks an unambiguous high-scoring candidate as a UI hint.
// It never marks claimed rows and never changes whether a link is explicit.
func MarkPreselection(candidates []Candidate) []Candidate {
	marked := append([]Candidate(nil), candidates...)
	for i := range marked {
		marked[i].Preselected = false
	}
	if len(marked) == 0 || !marked[0].Selectable || marked[0].Score < 95 || marked[0].Score < StrongCandidateScore {
		return marked
	}
	if len(marked) > 1 && marked[1].Score >= 85 {
		return marked
	}
	marked[0].Preselected = true
	return marked
}

// NormalizeName applies the matching normalization shared by both scores.
func NormalizeName(value string) string {
	value = strings.ToUpper(strings.Join(strings.Fields(value), " "))
	value = strings.Map(func(r rune) rune {
		switch r {
		case '.', '\'', '-', '/':
			return -1
		default:
			if unicode.IsPunct(r) {
				return ' '
			}
			return r
		}
	}, value)

	fields := strings.Fields(value)
	result := make([]string, 0, len(fields))
	for i := 0; i < len(fields); i++ {
		field := fields[i]
		if (field == "S" || field == "D") && i+1 < len(fields) && fields[i+1] == "O" {
			i++
			continue
		}
		if isPatronymic(field) {
			continue
		}
		result = append(result, field)
	}
	return strings.Join(result, " ")
}

// Score returns the 0–100 name score plus the weak rank and battery bonuses.
func Score(registrationName, rosterName, registrationRank, rosterRank, registrationBattery, rosterBattery string) int {
	base := nameScore(registrationName, rosterName)
	if attributesEqual(registrationBattery, rosterBattery) {
		base += 8
	}
	if attributesEqual(registrationRank, rosterRank) {
		base += 4
	}
	if base > 100 {
		return 100
	}
	return base
}

func nameScore(registrationName, rosterName string) int {
	left := NormalizeName(registrationName)
	right := NormalizeName(rosterName)
	return max(levenshteinRatio(left, right), tokenSortedRatio(left, right))
}

func isPatronymic(value string) bool {
	switch value {
	case "SO", "DO", "BIN", "BINTE", "BT":
		return true
	default:
		return false
	}
}

func attributesEqual(left, right string) bool {
	return strings.EqualFold(strings.TrimSpace(left), strings.TrimSpace(right))
}

func tokenSortedRatio(left, right string) int {
	leftTokens := strings.Fields(left)
	rightTokens := strings.Fields(right)
	sort.Strings(leftTokens)
	sort.Strings(rightTokens)
	return levenshteinRatio(strings.Join(leftTokens, " "), strings.Join(rightTokens, " "))
}

func levenshteinRatio(left, right string) int {
	leftRunes := []rune(left)
	rightRunes := []rune(right)
	longest := len(leftRunes)
	if len(rightRunes) > longest {
		longest = len(rightRunes)
	}
	if longest == 0 {
		return 100
	}
	distance := levenshteinDistance(leftRunes, rightRunes)
	return (longest - distance) * 100 / longest
}

func levenshteinDistance(left, right []rune) int {
	if len(left) < len(right) {
		left, right = right, left
	}
	previous := make([]int, len(right)+1)
	for i := range previous {
		previous[i] = i
	}
	for i, leftRune := range left {
		current := make([]int, len(right)+1)
		current[0] = i + 1
		for j, rightRune := range right {
			cost := 0
			if leftRune != rightRune {
				cost = 1
			}
			current[j+1] = min(current[j]+1, min(previous[j+1]+1, previous[j]+cost))
		}
		previous = current
	}
	return previous[len(right)]
}

func min(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func max(left, right int) int {
	if left > right {
		return left
	}
	return right
}
