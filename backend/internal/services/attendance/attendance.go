// Package attendance contains the transaction-bound attendance marking rules.
package attendance

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/davidlivingston/go-nextjs-starter/backend/internal/models"
	"github.com/jackc/pgx/v5"
)

// MarkRequest contains the values needed to mark one user for one session.
type MarkRequest struct {
	SessionID string
	UserID    string
	Method    string     // qr_scan | telegram_scan | manual
	MarkedBy  *string    // set for manual marks
	MarkedAt  *time.Time // set when a caller needs one timestamp for a batch
}

// MarkOutcome is the result of evaluating a mark request.
type MarkOutcome int

const (
	Marked MarkOutcome = iota
	AlreadyMarked
	SessionClosed
	OutOfScope
)

// UndoRequest contains the values needed to remove one actor-owned manual
// attendance mark.
type UndoRequest struct {
	SessionID string
	UserID    string
	MarkedBy  string
}

// UndoOutcome is the result of evaluating a strict manual-undo request.
type UndoOutcome int

const (
	Undone UndoOutcome = iota
	UndoNotFound
	UndoNotOwned
	UndoNotManual
)

// Mark applies the attendance rules using the caller's transaction. It does
// not begin, commit, or roll back tx.
func Mark(ctx context.Context, tx pgx.Tx, req MarkRequest) (MarkOutcome, error) {
	status, scope, batteries, err := loadSession(ctx, tx, req.SessionID)
	if err != nil {
		return Marked, fmt.Errorf("load attendance session: %w", err)
	}
	if status != models.SessionStatusActive {
		return SessionClosed, nil
	}

	inScope, err := userInScope(ctx, tx, req.SessionID, req.UserID, scope, batteries)
	if err != nil {
		return Marked, fmt.Errorf("check attendance scope: %w", err)
	}
	if !inScope {
		return OutOfScope, nil
	}

	recordID, err := newID()
	if err != nil {
		return Marked, fmt.Errorf("generate attendance record ID: %w", err)
	}
	markedAt := time.Now()
	if req.MarkedAt != nil {
		markedAt = *req.MarkedAt
	}
	result, err := tx.Exec(ctx, `
		INSERT INTO attendance_record (
			id, session_id, user_id, marked_at, marking_method, marked_by, "createdAt", "updatedAt"
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (session_id, user_id) DO NOTHING
	`, recordID, req.SessionID, req.UserID, markedAt, req.Method, req.MarkedBy, markedAt, markedAt)
	if err != nil {
		return Marked, fmt.Errorf("insert attendance record: %w", err)
	}
	if result.RowsAffected() == 0 {
		return AlreadyMarked, nil
	}
	return Marked, nil
}

// UndoManual removes an attendance record only when it is a manual mark made
// by the requested actor. The caller owns the transaction and decides whether
// to commit or roll it back.
func UndoManual(ctx context.Context, tx pgx.Tx, req UndoRequest) (UndoOutcome, error) {
	var method string
	var markedBy *string
	err := tx.QueryRow(ctx, `
		SELECT marking_method, marked_by
		FROM attendance_record
		WHERE session_id = $1 AND user_id = $2
		FOR UPDATE
	`, req.SessionID, req.UserID).Scan(&method, &markedBy)
	if err != nil {
		if err == pgx.ErrNoRows {
			return UndoNotFound, nil
		}
		return UndoNotFound, fmt.Errorf("load attendance record for undo: %w", err)
	}
	if method != models.MarkingMethodManual {
		return UndoNotManual, nil
	}
	if markedBy == nil || *markedBy != req.MarkedBy {
		return UndoNotOwned, nil
	}

	result, err := tx.Exec(ctx, `
		DELETE FROM attendance_record
		WHERE session_id = $1 AND user_id = $2
		  AND marking_method = $3 AND marked_by = $4
	`, req.SessionID, req.UserID, models.MarkingMethodManual, req.MarkedBy)
	if err != nil {
		return UndoNotFound, fmt.Errorf("delete attendance record for undo: %w", err)
	}
	if result.RowsAffected() == 0 {
		return UndoNotFound, nil
	}
	return Undone, nil
}

func loadSession(ctx context.Context, tx pgx.Tx, sessionID string) (string, string, []string, error) {
	var status, scope string
	var batteries []string
	err := tx.QueryRow(ctx, `
		SELECT status, scope, batteries FROM attendance_session WHERE id = $1
	`, sessionID).Scan(&status, &scope, &batteries)
	return status, scope, batteries, err
}

func userInScope(ctx context.Context, tx pgx.Tx, sessionID, userID, scope string, batteries []string) (bool, error) {
	switch scope {
	case models.SessionScopeUnitWide:
		return true, nil
	case models.SessionScopeBatterySpecific:
		var inScope bool
		err := tx.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM "user" WHERE id = $1 AND battery = ANY($2)
			)
		`, userID, batteries).Scan(&inScope)
		return inScope, err
	case models.SessionScopeCustomList:
		var inScope bool
		err := tx.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM session_participants
				WHERE session_id = $1 AND user_id = $2
			)
		`, sessionID, userID).Scan(&inScope)
		return inScope, err
	default:
		return false, nil
	}
}

func newID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
