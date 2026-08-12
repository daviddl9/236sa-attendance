package handlers

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/davidlivingston/go-nextjs-starter/backend/internal/services/matching"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/telegram"
	"github.com/jackc/pgx/v5"
)

const (
	pairingProposalLimit  = 5
	pairingProposalWindow = 10 * time.Minute
	pairingProposalExpiry = 5 * time.Minute
)

// ProposePairing ranks a soldier's name, records exactly one attempt, and
// returns a proposal only when the top score is strong enough. An advisory
// lock makes the rolling-window check atomic for one Telegram account.
func (s *TelegramPairingStore) ProposePairing(ctx context.Context, telegramID int64, name string) (telegram.PairingProposal, error) {
	if s == nil || s.db == nil || s.db.Pool == nil {
		return telegram.PairingProposal{}, errors.New("telegram pairing store is not configured")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return telegram.PairingProposal{NoMatch: true}, nil
	}

	tx, err := s.db.Pool.Begin(ctx)
	if err != nil {
		return telegram.PairingProposal{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, telegramID); err != nil {
		return telegram.PairingProposal{}, err
	}

	var attempts int
	if err := tx.QueryRow(ctx, `
		SELECT COUNT(*) FROM telegram_pairing_attempt
		WHERE telegram_id = $1
		  AND "createdAt" >= NOW() - ($2 * INTERVAL '1 second')
	`, telegramID, int64(pairingProposalWindow/time.Second)).Scan(&attempts); err != nil {
		return telegram.PairingProposal{}, err
	}
	if attempts >= pairingProposalLimit {
		if err := insertPairingAttempt(ctx, tx, generateID(), telegramID, "no_match", nil); err != nil {
			return telegram.PairingProposal{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return telegram.PairingProposal{}, err
		}
		return telegram.PairingProposal{RateLimited: true}, nil
	}

	roster, err := loadMatchingRoster(ctx, tx)
	if err != nil {
		return telegram.PairingProposal{}, err
	}
	candidates := matching.RankCandidates(matching.Registration{Name: name}, roster)
	if !matching.Unambiguous(candidates) {
		attemptID := generateID()
		if err := insertPairingAttempt(ctx, tx, attemptID, telegramID, "no_match", nil); err != nil {
			return telegram.PairingProposal{}, err
		}
		if err := upsertPairingRequest(ctx, tx, telegramID, name, attemptID); err != nil {
			return telegram.PairingProposal{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return telegram.PairingProposal{}, err
		}
		return telegram.PairingProposal{NoMatch: true}, nil
	}

	candidate := candidates[0]
	attemptID := generateID()
	if err := insertPairingAttempt(ctx, tx, attemptID, telegramID, "proposed", &candidate.ID); err != nil {
		return telegram.PairingProposal{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return telegram.PairingProposal{}, err
	}
	return telegram.PairingProposal{
		AttemptID: attemptID, UserID: candidate.ID, Name: candidate.Name,
		Rank: candidate.Rank, Battery: candidate.Battery, Score: candidate.Score,
	}, nil
}

// ConfirmPairing performs the database-protected claim. ON CONFLICT turns
// either uniqueness constraint into a normal conflict outcome, including when
// two different Telegram accounts tap confirmation concurrently.
func (s *TelegramPairingStore) ConfirmPairing(ctx context.Context, telegramID int64, attemptID string) (telegram.PairingConfirmation, error) {
	if s == nil || s.db == nil || s.db.Pool == nil {
		return telegram.PairingConfirmation{}, errors.New("telegram pairing store is not configured")
	}
	if strings.TrimSpace(attemptID) == "" {
		return telegram.PairingConfirmation{Outcome: telegram.PairingStale}, nil
	}

	tx, err := s.db.Pool.Begin(ctx)
	if err != nil {
		return telegram.PairingConfirmation{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var targetID, attemptOutcome string
	err = tx.QueryRow(ctx, `
		SELECT COALESCE(user_id, ''), outcome FROM telegram_pairing_attempt
		WHERE id = $1 AND telegram_id = $2 FOR UPDATE
	`, attemptID, telegramID).Scan(&targetID, &attemptOutcome)
	if errors.Is(err, pgx.ErrNoRows) {
		return telegram.PairingConfirmation{Outcome: telegram.PairingStale}, nil
	}
	if err != nil {
		return telegram.PairingConfirmation{}, err
	}
	if attemptOutcome != "proposed" || targetID == "" {
		return pairingConfirmationState(telegramID, targetID), nil
	}

	// Unpairing and confirming a proposal for the same roster row use this
	// lock. The second attempt read below observes whichever action committed
	// first instead of reviving a proposal made while the row was held.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, targetID); err != nil {
		return telegram.PairingConfirmation{}, err
	}

	var current bool
	err = tx.QueryRow(ctx, `
		SELECT COALESCE(user_id, ''), outcome,
		       "createdAt" >= NOW() - ($3 * INTERVAL '1 second')
		FROM telegram_pairing_attempt
		WHERE id = $1 AND telegram_id = $2 FOR UPDATE
	`, attemptID, telegramID, int64(pairingProposalExpiry/time.Second)).Scan(&targetID, &attemptOutcome, &current)
	if errors.Is(err, pgx.ErrNoRows) {
		return telegram.PairingConfirmation{Outcome: telegram.PairingStale}, nil
	}
	if err != nil {
		return telegram.PairingConfirmation{}, err
	}
	if attemptOutcome != "proposed" || targetID == "" {
		return pairingConfirmationState(telegramID, targetID), nil
	}
	if !current {
		if _, err := tx.Exec(ctx, `UPDATE telegram_pairing_attempt SET user_id = NULL WHERE id = $1 AND outcome = 'proposed'`, attemptID); err != nil {
			return telegram.PairingConfirmation{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return telegram.PairingConfirmation{}, err
		}
		return telegram.PairingConfirmation{Outcome: telegram.PairingStale}, nil
	}

	result, err := tx.Exec(ctx, `
		INSERT INTO telegram_pairing (id, telegram_id, user_id, display_name, confirmed_by, self_confirmed)
		SELECT $1, $2, u.id, COALESCE(u."full_name", ''), NULL, true
		FROM "user" u WHERE u.id = $3 ON CONFLICT DO NOTHING
	`, generateID(), telegramID, targetID)
	if err != nil {
		return telegram.PairingConfirmation{}, err
	}
	if result.RowsAffected() == 0 {
		if err := markPairingAttempt(ctx, tx, attemptID, "refused_conflict"); err != nil {
			return telegram.PairingConfirmation{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return telegram.PairingConfirmation{}, err
		}
		return telegram.PairingConfirmation{Outcome: telegram.PairingConflict, Pairing: telegram.Pairing{
			TelegramID: telegramID, UserID: targetID,
		}}, nil
	}

	var fullName string
	if err := tx.QueryRow(ctx, `SELECT COALESCE("full_name", '') FROM "user" WHERE id = $1`, targetID).Scan(&fullName); err != nil {
		return telegram.PairingConfirmation{}, err
	}
	if err := markPairingAttempt(ctx, tx, attemptID, "confirmed"); err != nil {
		return telegram.PairingConfirmation{}, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM telegram_pairing_request WHERE telegram_id = $1`, telegramID); err != nil {
		return telegram.PairingConfirmation{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return telegram.PairingConfirmation{}, err
	}
	return telegram.PairingConfirmation{Outcome: telegram.PairingConfirmed, Pairing: telegram.Pairing{
		TelegramID: telegramID, UserID: targetID, FullName: fullName,
	}}, nil
}

// DiscardPairing invalidates a declined proposal but keeps its row so the
// rolling-window limit cannot be evaded by repeatedly tapping No.
func (s *TelegramPairingStore) DiscardPairing(ctx context.Context, telegramID int64, attemptID string) error {
	if s == nil || s.db == nil || s.db.Pool == nil {
		return errors.New("telegram pairing store is not configured")
	}
	_, err := s.db.Pool.Exec(ctx, `
		UPDATE telegram_pairing_attempt SET user_id = NULL
		WHERE id = $1 AND telegram_id = $2 AND outcome = 'proposed'
	`, attemptID, telegramID)
	return err
}

func pairingConfirmationState(telegramID int64, targetID string) telegram.PairingConfirmation {
	outcome := telegram.PairingConflict
	if targetID == "" {
		outcome = telegram.PairingStale
	}
	return telegram.PairingConfirmation{Outcome: outcome, Pairing: telegram.Pairing{
		TelegramID: telegramID, UserID: targetID,
	}}
}

func insertPairingAttempt(ctx context.Context, tx pgx.Tx, id string, telegramID int64, outcome string, userID *string) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO telegram_pairing_attempt (id, telegram_id, outcome, user_id)
		VALUES ($1, $2, $3, $4)
	`, id, telegramID, outcome, userID)
	return err
}

func markPairingAttempt(ctx context.Context, tx pgx.Tx, id, outcome string) error {
	_, err := tx.Exec(ctx, `UPDATE telegram_pairing_attempt SET outcome = $1 WHERE id = $2 AND outcome = 'proposed'`, outcome, id)
	return err
}

func upsertPairingRequest(ctx context.Context, tx pgx.Tx, telegramID int64, name, attemptID string) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO telegram_pairing_request (id, telegram_id, display_name, attempt_id)
		VALUES ($1, $2, $3, $4) ON CONFLICT (telegram_id) DO UPDATE
		SET display_name = EXCLUDED.display_name, attempt_id = EXCLUDED.attempt_id, "createdAt" = NOW()
	`, generateID(), telegramID, name, attemptID)
	return err
}

func loadMatchingRoster(ctx context.Context, tx pgx.Tx) ([]matching.RosterRow, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, COALESCE("full_name", ''), COALESCE(rank, ''), COALESCE(battery, '')
		FROM "user" WHERE verified = true
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	roster := make([]matching.RosterRow, 0)
	for rows.Next() {
		var row matching.RosterRow
		if err := rows.Scan(&row.ID, &row.Name, &row.Rank, &row.Battery); err != nil {
			return nil, err
		}
		roster = append(roster, row)
	}
	return roster, rows.Err()
}
