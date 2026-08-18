// Command seed-groups creates the unit's operational participant groups from a
// roster Excel, idempotently. It matches roster rows to existing users by the
// NRIC last 5 (name as tie-breaker), reuses groups the creator already has
// under the same name, and only ever adds members — never removes.
//
// Usage:
//
//	seed-groups -file <roster.xlsx> -password <pwd> -created-by <user-id> [-db-url <dsn>]
//
// Groups seeded (rules confirmed with the unit owner):
//
//	RnS             Sub-Unit 1 = BN RECCE & SURVEY TM
//	FDC/BOC         Sub-Unit 1 = FIRE DIRECTION CEN
//	BCS             Sub-Unit 2 = MEDICAL PL
//	CSS             S1 CELL + MT PL + MEDICAL PL + HQ COMBAT TRAIN + S4/OC HQ + HQ-bty BSM
//	PSO             rank >= 1SG
//	PSP             Sub-Unit 2 = PERSONNEL SP PL
//	A Bty           Sub-Unit 1 = FIELD ARTY BTY A
//	B Bty           Sub-Unit 1 = FIELD ARTY BTY B
//	HQ Bty          Sub-Unit 1 = HQ BTY
//	MT Platoon      Sub-Unit 2 = MT PL
//	Technicians     vocation in {AUTO TECH, AUTO SPEC TECH, ARMT TECH, ARMT SPEC TECH}
//	CSS Commanders  CSS members with rank >= 1SG
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/davidlivingston/go-nextjs-starter/backend/internal/database"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/models"
	"github.com/xuri/excelize/v2"
)

// roster columns (0-indexed) in the "236 SA" sheet.
const (
	colNRIC     = 1
	colRank     = 2
	colName     = 3
	colPosDesc  = 5
	colSubUnit1 = 7
	colSubUnit2 = 8
	colVocation = 9
	headerRows  = 2
	rosterSheet = "236 SA"
	ps0Rank     = "1SG" // PSO and CSS Commanders include everyone at or above this rank
)

// row is one roster line.
type row struct {
	nric    string
	rank    string
	name    string
	posDesc string
	sub1    string
	sub2    string
	voc     string
}

// rule selects rows for one group.
type rule struct {
	group   string
	matches func(r row) bool
}

var rules = []rule{
	{group: "RnS", matches: func(r row) bool { return r.sub1 == "BN RECCE & SURVEY TM" }},
	{group: "FDC/BOC", matches: func(r row) bool { return r.sub1 == "FIRE DIRECTION CEN" }},
	{group: "BCS", matches: func(r row) bool { return r.sub2 == "MEDICAL PL" }},
	{group: "CSS", matches: matchesCSS},
	{group: "PSO", matches: func(r row) bool {
		order, ok := models.RankOrder[strings.ToUpper(r.rank)]
		return ok && order >= models.RankOrder[ps0Rank]
	}},
	{group: "PSP", matches: func(r row) bool { return r.sub2 == "PERSONNEL SP PL" }},
	{group: "A Bty", matches: func(r row) bool { return r.sub1 == "FIELD ARTY BTY A" }},
	{group: "B Bty", matches: func(r row) bool { return r.sub1 == "FIELD ARTY BTY B" }},
	{group: "HQ Bty", matches: func(r row) bool { return r.sub1 == "HQ BTY" }},
	{group: "MT Platoon", matches: func(r row) bool { return r.sub2 == "MT PL" }},
	{group: "Technicians", matches: func(r row) bool {
		switch r.voc {
		case "AUTO TECH", "AUTO SPEC TECH", "ARMT TECH", "ARMT SPEC TECH":
			return true
		}
		return false
	}},
	{group: "CSS Commanders", matches: matchesCSSCommander},
}

// matchesCSS reports whether a roster row belongs to Combat Service Support:
// the S1 cell, MT platoon, medical platoon, HQ combat train, S4/OC HQ, and the
// HQ-battery BSM. Battery-level elements are deliberately excluded.
func matchesCSS(r row) bool {
	if r.sub1 == "S1 BR" && r.sub2 == "S1 CELL" {
		return true
	}
	switch r.sub2 {
	case "MT PL", "MEDICAL PL", "HQ COMBAT TRAIN":
		return true
	}
	if r.posDesc == "S4/OC HQ" {
		return true
	}
	return r.sub2 == "BTY HQ" && r.posDesc == "BSM"
}

// matchesCSSCommander reports whether a roster row is a CSS commander — a CSS
// member at or above First Sergeant rank.
func matchesCSSCommander(r row) bool {
	order, ok := models.RankOrder[strings.ToUpper(r.rank)]
	return ok && matchesCSS(r) && order >= models.RankOrder[ps0Rank]
}

func main() {
	filePath := flag.String("file", "", "path to the roster Excel file (required)")
	password := flag.String("password", "", "password for the Excel file (required)")
	createdBy := flag.String("created-by", "", "id of the admin user who owns the seeded groups (required)")
	dbURL := flag.String("db-url", os.Getenv("DATABASE_URL"), "postgres DSN (defaults to DATABASE_URL)")
	flag.Parse()

	if *filePath == "" || *password == "" || *createdBy == "" {
		log.Fatal("flags -file, -password and -created-by are all required")
	}
	if *dbURL == "" {
		log.Fatal("DATABASE_URL is not set (pass -db-url or set the environment variable)")
	}

	rows, err := readRows(*filePath, *password)
	if err != nil {
		log.Fatalf("read roster: %v", err)
	}

	db, err := database.NewPostgresDB(*dbURL)
	if err != nil {
		log.Fatalf("connect to database: %v", err)
	}
	defer db.Close()

	if err := verifyCreatedBy(context.Background(), db, *createdBy); err != nil {
		log.Fatalf("verify -created-by: %v", err)
	}

	seed(context.Background(), db, rows, *createdBy)
}

// readRows opens the password-protected roster and returns its data rows.
func readRows(path, password string) ([]row, error) {
	f, err := excelize.OpenFile(path, excelize.Options{Password: password})
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	sheet := rosterSheet
	idx, err := f.GetSheetIndex(sheet)
	if err != nil || idx < 0 {
		return nil, fmt.Errorf("sheet %q not found", sheet)
	}
	raw, err := f.GetRows(sheet, excelize.Options{RawCellValue: true})
	if err != nil {
		return nil, fmt.Errorf("read sheet %q: %w", sheet, err)
	}

	var rows []row
	for i, cells := range raw {
		if i < headerRows || len(cells) <= colSubUnit2 {
			continue
		}
		name := strings.TrimSpace(cells[colName])
		if name == "" {
			continue
		}
		rows = append(rows, row{
			nric:    strings.ToUpper(strings.TrimSpace(cells[colNRIC])),
			rank:    strings.ToUpper(strings.TrimSpace(cells[colRank])),
			name:    name,
			posDesc: strings.TrimSpace(cells[colPosDesc]),
			sub1:    strings.TrimSpace(cells[colSubUnit1]),
			sub2:    strings.TrimSpace(cells[colSubUnit2]),
			voc:     strings.TrimSpace(cells[colVocation]),
		})
	}
	return rows, nil
}

func verifyCreatedBy(ctx context.Context, db *database.DB, userID string) error {
	var exists bool
	if err := db.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM "user" WHERE id = $1)`, userID).Scan(&exists); err != nil {
		return fmt.Errorf("query user: %w", err)
	}
	if !exists {
		return fmt.Errorf("user %q does not exist", userID)
	}
	return nil
}

// seed creates/updates each group and inserts its matched members.
func seed(ctx context.Context, db *database.DB, rows []row, createdBy string) {
	var totalMembers int
	for _, r := range rules {
		var matched []row
		for _, row := range rows {
			if r.matches(row) {
				matched = append(matched, row)
			}
		}

		userIDs, unmatched := resolveUsers(ctx, db, matched)

		groupID, created, err := ensureGroup(ctx, db, r.group, createdBy)
		if err != nil {
			log.Printf("%-8s ERROR: %v", r.group, err)
			continue
		}
		added, err := addMembers(ctx, db, groupID, userIDs)
		if err != nil {
			log.Printf("%-8s ERROR: %v", r.group, err)
			continue
		}

		status := "reused"
		if created {
			status = "created"
		}
		// Report the union of members newly added plus those already present.
		summary := fmt.Sprintf("%-8s %s · group %s · members %d (+%d new) · unmatched %d",
			r.group, status, groupID, len(userIDs), added, len(unmatched))
		if len(unmatched) > 0 {
			names := make([]string, 0, len(unmatched))
			for _, u := range unmatched {
				names = append(names, u.name)
			}
			summary += " · " + strings.Join(names, ", ")
		}
		fmt.Println(summary)
		totalMembers += len(userIDs)
	}
	fmt.Printf("Done. %d groups, %d memberships. Additive — re-running is safe.\n", len(rules), totalMembers)
}

// resolveUsers maps each matched roster row to an existing verified user,
// keyed by NRIC last 5 with the full name as a tie-breaker. Rows that cannot
// be resolved uniquely are returned under unmatched.
func resolveUsers(ctx context.Context, db *database.DB, rows []row) (userIDs []string, unmatched []row) {
	for _, r := range rows {
		uid, ok := resolveUser(ctx, db, r)
		if !ok {
			unmatched = append(unmatched, r)
			continue
		}
		userIDs = append(userIDs, uid)
	}
	return userIDs, unmatched
}

// resolveUser returns the verified user id for a roster row, or false when the
// row is ambiguous or matches no user.
func resolveUser(ctx context.Context, db *database.DB, r row) (string, bool) {
	var candidates []struct {
		id       string
		fullName string
	}
	rows, err := db.Pool.Query(ctx, `
		SELECT id, "full_name" FROM "user"
		WHERE upper("nric_last5") = $1 AND verified = true
	`, r.nric)
	if err != nil {
		log.Printf("query nric %s: %v", r.nric, err)
		return "", false
	}
	defer rows.Close()
	for rows.Next() {
		var c struct {
			id       string
			fullName string
		}
		if err := rows.Scan(&c.id, &c.fullName); err != nil {
			log.Printf("scan nric %s: %v", r.nric, err)
			return "", false
		}
		candidates = append(candidates, c)
	}
	if err := rows.Err(); err != nil {
		log.Printf("rows nric %s: %v", r.nric, err)
		return "", false
	}

	switch len(candidates) {
	case 0:
		return "", false
	case 1:
		return candidates[0].id, true
	}
	// Multiple users share an NRIC — prefer an exact name match.
	var exact []string
	for _, c := range candidates {
		if strings.EqualFold(strings.TrimSpace(c.fullName), r.name) {
			exact = append(exact, c.id)
		}
	}
	if len(exact) == 1 {
		return exact[0], true
	}
	return "", false
}

// ensureGroup returns the group id, creating the group when the creator does
// not already have one with that name.
func ensureGroup(ctx context.Context, db *database.DB, name, createdBy string) (id string, created bool, err error) {
	err = db.Pool.QueryRow(ctx, `
		SELECT id FROM participant_group
		WHERE created_by = $1 AND lower(name) = lower($2)
	`, createdBy, name).Scan(&id)
	if err == nil {
		return id, false, nil
	}

	_, err = db.Pool.Exec(ctx, `
		INSERT INTO participant_group (id, name, created_by, "createdAt", "updatedAt")
		VALUES ($1, $2, $3, NOW(), NOW())
		ON CONFLICT DO NOTHING
	`, newGroupID(), name, createdBy)
	if err != nil {
		return "", false, fmt.Errorf("insert group %q: %w", name, err)
	}
	err = db.Pool.QueryRow(ctx, `
		SELECT id FROM participant_group WHERE created_by = $1 AND lower(name) = lower($2)
	`, createdBy, name).Scan(&id)
	if err != nil {
		return "", false, fmt.Errorf("re-read group %q: %w", name, err)
	}
	return id, true, nil
}

// addMembers inserts each user id into the group, skipping existing rows.
func addMembers(ctx context.Context, db *database.DB, groupID string, userIDs []string) (added int, err error) {
	for _, uid := range userIDs {
		tag, err := db.Pool.Exec(ctx, `
			INSERT INTO participant_group_member (group_id, user_id)
			VALUES ($1, $2)
			ON CONFLICT DO NOTHING
		`, groupID, uid)
		if err != nil {
			return added, fmt.Errorf("add member %q: %w", uid, err)
		}
		if tag.RowsAffected() == 1 {
			added++
		}
	}
	return added, nil
}

// newGroupID returns a random hex group id, matching the service's scheme.
func newGroupID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		log.Fatalf("generate group id: %v", err)
	}
	return hex.EncodeToString(b)
}
