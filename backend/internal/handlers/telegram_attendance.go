package handlers

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/davidlivingston/go-nextjs-starter/backend/internal/models"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/services/attendance"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/services/deeplink"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/telegram"
	"github.com/jackc/pgx/v5"
)

// MarkAttendance resolves the persisted Telegram capability and applies the
// same transaction-bound attendance service used by the web scanner. The
// pairing row is read in the transaction as well, so a Telegram ID can never
// be supplied as a roster identity by the message payload.
func (s *TelegramPairingStore) MarkAttendance(ctx context.Context, telegramID int64, deeplinkCode string) (telegram.AttendanceResult, error) {
	if s == nil || s.db == nil || s.db.Pool == nil {
		return telegram.AttendanceResult{}, errors.New("telegram attendance store is not configured")
	}
	if !deeplink.IsValidCode(deeplinkCode) {
		return telegram.AttendanceResult{Outcome: telegram.AttendanceUnknownCode}, nil
	}

	tx, err := s.db.Pool.Begin(ctx)
	if err != nil {
		return telegram.AttendanceResult{}, fmt.Errorf("begin Telegram attendance mark: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Resolve the pairing before the session so Telegram attendance and admin
	// transactions acquire these row locks in one order.
	var userID string
	err = tx.QueryRow(ctx, `
		SELECT user_id FROM telegram_pairing
		WHERE telegram_id = $1
		FOR SHARE
	`, telegramID).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return telegram.AttendanceResult{Outcome: telegram.AttendanceUnpaired}, nil
	}
	if err != nil {
		return telegram.AttendanceResult{}, fmt.Errorf("resolve Telegram pairing for attendance: %w", err)
	}

	var sessionID, sessionName string
	err = tx.QueryRow(ctx, `
		SELECT id, name FROM attendance_session
		WHERE deeplink_code = $1
		FOR UPDATE
	`, deeplinkCode).Scan(&sessionID, &sessionName)
	if errors.Is(err, pgx.ErrNoRows) {
		return telegram.AttendanceResult{Outcome: telegram.AttendanceUnknownCode}, nil
	}
	if err != nil {
		return telegram.AttendanceResult{}, fmt.Errorf("resolve Telegram attendance code: %w", err)
	}

	var pairedUser models.User
	if s.hub != nil {
		if err := tx.QueryRow(ctx, `
			SELECT id, "full_name", rank, battery
			FROM "user" WHERE id = $1
		`, userID).Scan(&pairedUser.ID, &pairedUser.FullName, &pairedUser.Rank, &pairedUser.Battery); err != nil {
			return telegram.AttendanceResult{}, fmt.Errorf("load Telegram attendance user: %w", err)
		}
	}

	outcome, err := attendance.Mark(ctx, tx, attendance.MarkRequest{
		SessionID: sessionID,
		UserID:    userID,
		Method:    models.MarkingMethodTelegramScan,
	})
	if err != nil {
		return telegram.AttendanceResult{}, fmt.Errorf("mark Telegram attendance: %w", err)
	}

	result := telegram.AttendanceResult{SessionName: sessionName}
	switch outcome {
	case attendance.Marked:
		result.Outcome = telegram.AttendanceMarked
	case attendance.MarkedOutOfScope:
		result.Outcome = telegram.AttendanceMarkedOutOfScope
	case attendance.AlreadyMarked:
		result.Outcome = telegram.AttendanceAlreadyMarked
	case attendance.SessionClosed:
		result.Outcome = telegram.AttendanceSessionClosed
	default:
		return telegram.AttendanceResult{}, fmt.Errorf("unknown attendance service outcome %d", outcome)
	}

	marked := outcome == attendance.Marked || outcome == attendance.MarkedOutOfScope
	var markedAt time.Time
	if marked {
		if err := tx.QueryRow(ctx, `
			SELECT marked_at FROM attendance_record
			WHERE session_id = $1 AND user_id = $2
		`, sessionID, userID).Scan(&markedAt); err != nil {
			return telegram.AttendanceResult{}, fmt.Errorf("read Telegram attendance mark: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return telegram.AttendanceResult{}, fmt.Errorf("commit Telegram attendance mark: %w", err)
	}

	if marked && s.hub != nil {
		(&AttendanceHandler{db: s.db, hub: s.hub}).broadcastAttendanceMarked(
			context.Background(), sessionID, &pairedUser, models.MarkingMethodTelegramScan, markedAt,
		)
	}
	return result, nil
}
