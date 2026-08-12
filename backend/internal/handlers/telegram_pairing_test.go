package handlers

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/davidlivingston/go-nextjs-starter/backend/internal/database"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/telegram"
)

func TestTelegramPairingStrongMatchConfirmAndRecognition(t *testing.T) {
	db, prefix, telegramID := openTelegramPairingDB(t)
	targetID := prefix + "-target"
	seedUser(t, db, targetID, "TAN WEI MING", "CPL", "Bravo", "", true)
	store := &TelegramPairingStore{db: db}

	proposal, err := store.ProposePairing(context.Background(), telegramID, "TAN WEI MIMG")
	if err != nil {
		t.Fatal(err)
	}
	if proposal.NoMatch || proposal.AttemptID == "" || proposal.UserID != targetID {
		t.Fatalf("proposal = %+v", proposal)
	}
	confirmation, err := store.ConfirmPairing(context.Background(), telegramID, proposal.AttemptID)
	if err != nil || confirmation.Outcome != telegram.PairingConfirmed {
		t.Fatalf("confirmation = %+v, err=%v", confirmation, err)
	}

	var selfConfirmed bool
	var confirmedBy *string
	if err := db.Pool.QueryRow(context.Background(), `
		SELECT self_confirmed, confirmed_by FROM telegram_pairing WHERE telegram_id = $1
	`, telegramID).Scan(&selfConfirmed, &confirmedBy); err != nil {
		t.Fatal(err)
	}
	if !selfConfirmed || confirmedBy != nil {
		t.Fatalf("pairing fields = self_confirmed %v, confirmed_by %v", selfConfirmed, confirmedBy)
	}
	var attemptCount int
	var outcome string
	if err := db.Pool.QueryRow(context.Background(), `
		SELECT COUNT(*), max(outcome) FROM telegram_pairing_attempt WHERE telegram_id = $1
	`, telegramID).Scan(&attemptCount, &outcome); err != nil {
		t.Fatal(err)
	}
	if attemptCount != 1 || outcome != "confirmed" {
		t.Fatalf("attempts = (%d, %q), want one confirmed attempt", attemptCount, outcome)
	}
	repeat, err := store.ConfirmPairing(context.Background(), telegramID, proposal.AttemptID)
	if err != nil || repeat.Outcome != telegram.PairingConflict {
		t.Fatalf("repeated confirmation = %+v, err=%v", repeat, err)
	}
	declinedID := telegramID + 10
	declined, err := store.ProposePairing(context.Background(), declinedID, "TAN WEI MING")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.DiscardPairing(context.Background(), declinedID, declined.AttemptID); err != nil {
		t.Fatal(err)
	}
	if result, err := store.ConfirmPairing(context.Background(), declinedID, declined.AttemptID); err != nil || result.Outcome != telegram.PairingStale {
		t.Fatalf("declined confirmation = %+v, err=%v", result, err)
	}

	bot := telegram.NewBot(store)
	actions, err := bot.HandleUpdate(context.Background(), telegram.Update{Message: &telegram.Message{
		From: &telegram.User{ID: telegramID}, Chat: telegram.Chat{ID: telegramID, Type: "private"}, Text: "hello",
	}})
	if err != nil || len(actions) != 1 || actions[0].Text != "I recognise you as TAN WEI MING." {
		t.Fatalf("recognition actions = %#v, err=%v", actions, err)
	}
}

func TestTelegramPairingWeakMatchCreatesOneRequestAndNoNameReply(t *testing.T) {
	db, prefix, telegramID := openTelegramPairingDB(t)
	seedUser(t, db, prefix+"-target", "TAN WEI MING", "CPL", "Bravo", "", true)
	store := &TelegramPairingStore{db: db}

	proposal, err := store.ProposePairing(context.Background(), telegramID, "UNRELATED PERSON")
	if err != nil || !proposal.NoMatch || proposal.RateLimited {
		t.Fatalf("proposal = %+v, err=%v", proposal, err)
	}
	if _, err := store.ProposePairing(context.Background(), telegramID, "ANOTHER UNKNOWN"); err != nil {
		t.Fatal(err)
	}
	var requestCount, attemptCount int
	var displayName, outcome string
	ctx := context.Background()
	if err := db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM telegram_pairing_request WHERE telegram_id = $1`, telegramID).Scan(&requestCount); err != nil {
		t.Fatal(err)
	}
	if err := db.Pool.QueryRow(ctx, `SELECT COUNT(*), max(outcome) FROM telegram_pairing_attempt WHERE telegram_id = $1`, telegramID).Scan(&attemptCount, &outcome); err != nil {
		t.Fatal(err)
	}
	if err := db.Pool.QueryRow(ctx, `SELECT display_name FROM telegram_pairing_request WHERE telegram_id = $1`, telegramID).Scan(&displayName); err != nil {
		t.Fatal(err)
	}
	if requestCount != 1 || attemptCount != 2 || outcome != "no_match" || displayName != "ANOTHER UNKNOWN" {
		t.Fatalf("request=(%d,%q), attempts=(%d,%q)", requestCount, displayName, attemptCount, outcome)
	}
}

func TestTelegramPairingConflictIsAnOutcome(t *testing.T) {
	db, prefix, telegramID := openTelegramPairingDB(t)
	targetID := prefix + "-target"
	seedUser(t, db, targetID, "TAN WEI MING", "CPL", "Bravo", "", true)
	store := &TelegramPairingStore{db: db}
	first, err := store.ProposePairing(context.Background(), telegramID, "TAN WEI MING")
	if err != nil {
		t.Fatal(err)
	}
	if result, err := store.ConfirmPairing(context.Background(), telegramID, first.AttemptID); err != nil || result.Outcome != telegram.PairingConfirmed {
		t.Fatalf("first confirmation = %+v, err=%v", result, err)
	}
	secondTelegramID := telegramID + 1
	second, err := store.ProposePairing(context.Background(), secondTelegramID, "TAN WEI MING")
	if err != nil {
		t.Fatal(err)
	}
	result, err := store.ConfirmPairing(context.Background(), secondTelegramID, second.AttemptID)
	if err != nil || result.Outcome != telegram.PairingConflict {
		t.Fatalf("conflict confirmation = %+v, err=%v", result, err)
	}
	var pairings int
	var outcome string
	ctx := context.Background()
	if err := db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM telegram_pairing WHERE user_id = $1`, targetID).Scan(&pairings); err != nil {
		t.Fatal(err)
	}
	if err := db.Pool.QueryRow(ctx, `SELECT outcome FROM telegram_pairing_attempt WHERE id = $1`, second.AttemptID).Scan(&outcome); err != nil {
		t.Fatal(err)
	}
	if pairings != 1 || outcome != "refused_conflict" {
		t.Fatalf("pairings=%d outcome=%q", pairings, outcome)
	}
}

func TestTelegramPairingOneAccountCannotHoldTwoRows(t *testing.T) {
	db, prefix, telegramID := openTelegramPairingDB(t)
	seedUser(t, db, prefix+"-one", "ONE PERSON", "PTE", "Alpha", "", true)
	seedUser(t, db, prefix+"-two", "TWO PERSON", "PTE", "Alpha", "", true)
	store := &TelegramPairingStore{db: db}
	first, err := store.ProposePairing(context.Background(), telegramID, "ONE PERSON")
	if err != nil {
		t.Fatal(err)
	}
	if result, err := store.ConfirmPairing(context.Background(), telegramID, first.AttemptID); err != nil || result.Outcome != telegram.PairingConfirmed {
		t.Fatalf("first = %+v, err=%v", result, err)
	}
	second, err := store.ProposePairing(context.Background(), telegramID, "TWO PERSON")
	if err != nil {
		t.Fatal(err)
	}
	result, err := store.ConfirmPairing(context.Background(), telegramID, second.AttemptID)
	if err != nil || result.Outcome != telegram.PairingConflict {
		t.Fatalf("second = %+v, err=%v", result, err)
	}
}

func TestTelegramPairingRateLimitRecordsDecline(t *testing.T) {
	db, prefix, telegramID := openTelegramPairingDB(t)
	seedUser(t, db, prefix+"-target", "TAN WEI MING", "CPL", "Bravo", "", true)
	store := &TelegramPairingStore{db: db}
	for i := 0; i < pairingProposalLimit; i++ {
		proposal, err := store.ProposePairing(context.Background(), telegramID, "TAN WEI MING")
		if err != nil || proposal.RateLimited || proposal.AttemptID == "" {
			t.Fatalf("proposal %d = %+v, err=%v", i, proposal, err)
		}
	}
	proposal, err := store.ProposePairing(context.Background(), telegramID, "TAN WEI MING")
	if err != nil || !proposal.RateLimited {
		t.Fatalf("limited proposal = %+v, err=%v", proposal, err)
	}
	var count int
	var noMatch int
	if err := db.Pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM telegram_pairing_attempt WHERE telegram_id = $1`, telegramID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if err := db.Pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM telegram_pairing_attempt WHERE telegram_id = $1 AND outcome = 'no_match'`, telegramID).Scan(&noMatch); err != nil {
		t.Fatal(err)
	}
	if count != pairingProposalLimit+1 || noMatch != 1 {
		t.Fatalf("attempt counts = total %d, no_match %d", count, noMatch)
	}
}

func TestTelegramPairingConcurrentConfirmationsProduceOnePairingAndOneConflict(t *testing.T) {
	db, prefix, telegramID := openTelegramPairingDB(t)
	targetID := prefix + "-target"
	seedUser(t, db, targetID, "TAN WEI MING", "CPL", "Bravo", "", true)
	store := &TelegramPairingStore{db: db}
	first, err := store.ProposePairing(context.Background(), telegramID, "TAN WEI MING")
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.ProposePairing(context.Background(), telegramID+1, "TAN WEI MING")
	if err != nil {
		t.Fatal(err)
	}

	results := make(chan telegram.PairingConfirmation, 2)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, input := range []struct {
		id      int64
		attempt string
	}{{telegramID, first.AttemptID}, {telegramID + 1, second.AttemptID}} {
		wg.Add(1)
		go func(input struct {
			id      int64
			attempt string
		}) {
			defer wg.Done()
			result, err := store.ConfirmPairing(context.Background(), input.id, input.attempt)
			results <- result
			errs <- err
		}(input)
	}
	wg.Wait()
	close(results)
	close(errs)
	confirmed, conflicts := 0, 0
	for result := range results {
		switch result.Outcome {
		case telegram.PairingConfirmed:
			confirmed++
		case telegram.PairingConflict:
			conflicts++
		}
	}
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var pairings int
	if err := db.Pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM telegram_pairing WHERE user_id = $1`, targetID).Scan(&pairings); err != nil {
		t.Fatal(err)
	}
	if confirmed != 1 || conflicts != 1 || pairings != 1 {
		t.Fatalf("confirmed=%d conflicts=%d pairings=%d", confirmed, conflicts, pairings)
	}
}

func openTelegramPairingDB(t *testing.T) (*database.DB, string, int64) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to run the database integration test")
	}
	db, err := database.NewPostgresDB(url)
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	prefix := fmt.Sprintf("tg-pair-%d", time.Now().UnixNano())
	telegramID := time.Now().UnixNano() % 1_000_000_000_000
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = db.Pool.Exec(ctx, `DELETE FROM telegram_pairing_attempt WHERE telegram_id BETWEEN $1 AND $2`, telegramID, telegramID+10)
		_, _ = db.Pool.Exec(ctx, `DELETE FROM telegram_pairing_request WHERE telegram_id BETWEEN $1 AND $2`, telegramID, telegramID+10)
		_, _ = db.Pool.Exec(ctx, `DELETE FROM telegram_pairing WHERE telegram_id BETWEEN $1 AND $2`, telegramID, telegramID+10)
		_, _ = db.Pool.Exec(ctx, `DELETE FROM "user" WHERE id LIKE $1`, prefix+"-%")
		db.Close()
	})
	return db, prefix, telegramID
}
