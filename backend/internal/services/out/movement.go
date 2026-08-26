package out

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/davidlivingston/go-nextjs-starter/backend/internal/models"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/timeutil"
	"github.com/jackc/pgx/v5"
)

// MinMovementInterval is how long a repeat scan is treated as a duplicate
// rather than a direction change. A soldier who scans twice at the gate —
// a slow confirmation screen, a stray second tap — must not be recorded as
// back inside camp while walking out of it.
//
// It is deliberately short. Accidental repeats happen within seconds, while a
// genuine turnaround — stepping out and coming straight back for something
// forgotten — is a real movement worth recording. A minute separates the two
// without making anyone wait long.
const MinMovementInterval = time.Minute

// NextMovementAt returns the instant at which a further movement will be
// accepted, or nil when one can be recorded now. The scan screen uses it to
// say when someone may scan again rather than offering a button the server
// would refuse.
func NextMovementAt(latest *models.OutMovement, now time.Time) *time.Time {
	if latest == nil {
		return nil
	}
	ready := latest.OccurredAt.Add(MinMovementInterval)
	if !ready.After(now) {
		return nil
	}
	return &ready
}

// RecordOutcome is the result of evaluating a movement request.
type RecordOutcome int

const (
	// Recorded means a new movement row was written.
	Recorded RecordOutcome = iota
	// Duplicate means the scan arrived within MinMovementInterval of the
	// previous one and was deliberately not recorded.
	Duplicate
	// DirectionChanged means the caller's expected direction no longer matches
	// what the log implies — a stale page, or a correction in between.
	DirectionChanged
	// SessionUnavailable means the session is missing, closed, or not an Out session.
	SessionUnavailable
	// NotVerified means the actor is not an approved roster member.
	NotVerified
)

// RecordRequest contains the values needed to record one gate movement.
type RecordRequest struct {
	SessionID string
	UserID    string
	Method    string // qr_scan | telegram_scan | manual
	// ExpectedDirection, when set, must match the direction the server derives.
	// The scan UI sends back what it displayed so a stale page cannot flip
	// someone the wrong way.
	ExpectedDirection string
	// RecordedBy is required for manual records and identifies the commander.
	RecordedBy *string
}

// RecordResult reports what happened and the state the caller should display.
type RecordResult struct {
	Outcome   RecordOutcome
	Direction string // the direction now in force for this user
	Movement  *models.OutMovement
	Previous  *models.OutMovement
}

// MovementState is one person's current standing in an Out session.
type MovementState struct {
	UserID     string     `json:"userId"`
	FullName   *string    `json:"fullName,omitempty"`
	Rank       *string    `json:"rank,omitempty"`
	Battery    *string    `json:"battery,omitempty"`
	Direction  string     `json:"direction"`
	OccurredAt time.Time  `json:"occurredAt"`
	Method     string     `json:"method"`
	Overdue    bool       `json:"overdue"`
	FirstOutAt *time.Time `json:"firstOutAt,omitempty"`
}

// Board is the commander's view of an Out session.
type Board struct {
	Session       models.OutSession `json:"session"`
	Members       []MovementState   `json:"members"`
	OutCount      int               `json:"outCount"`
	ReturnedCount int               `json:"returnedCount"`
	OverdueCount  int               `json:"overdueCount"`
}

// NextDirection returns the direction a scan should record given the person's
// latest movement. The first scan sends them out; the alternation continues
// from there, so someone who returns and goes out again needs no special case.
func NextDirection(latest *models.OutMovement) string {
	if latest == nil || latest.Direction == models.DirectionIn {
		return models.DirectionOut
	}
	return models.DirectionIn
}

// Record applies the movement rules inside the caller's transaction. It does
// not begin, commit or roll back tx.
func Record(ctx context.Context, tx pgx.Tx, req RecordRequest, now time.Time) (RecordResult, error) {
	session, err := loadOutSessionTx(ctx, tx, req.SessionID)
	if err != nil {
		if errors.Is(err, ErrSessionNotFound) {
			return RecordResult{Outcome: SessionUnavailable}, nil
		}
		return RecordResult{}, err
	}
	if session.Status != models.SessionStatusActive {
		return RecordResult{Outcome: SessionUnavailable}, nil
	}

	verified, err := userIsVerified(ctx, tx, req.UserID)
	if err != nil {
		return RecordResult{}, err
	}
	if !verified {
		return RecordResult{Outcome: NotVerified}, nil
	}

	latest, err := latestMovementTx(ctx, tx, req.SessionID, req.UserID)
	if err != nil {
		return RecordResult{}, err
	}

	// Duplicate guard runs before the direction check so a double tap reports
	// the state the person is actually in rather than a confusing conflict.
	if latest != nil && now.Sub(latest.OccurredAt) < MinMovementInterval {
		return RecordResult{
			Outcome:   Duplicate,
			Direction: latest.Direction,
			Previous:  latest,
		}, nil
	}

	direction := NextDirection(latest)
	if req.ExpectedDirection != "" && req.ExpectedDirection != direction {
		return RecordResult{
			Outcome:   DirectionChanged,
			Direction: direction,
			Previous:  latest,
		}, nil
	}

	id, err := randomHex(16)
	if err != nil {
		return RecordResult{}, fmt.Errorf("generate movement ID: %w", err)
	}
	var movement models.OutMovement
	err = tx.QueryRow(ctx, `
		INSERT INTO out_movement (id, session_id, user_id, direction, occurred_at, method, recorded_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, session_id, user_id, direction, occurred_at, method, recorded_by, "createdAt"
	`, id, req.SessionID, req.UserID, direction, now, req.Method, req.RecordedBy).Scan(
		&movement.ID, &movement.SessionID, &movement.UserID, &movement.Direction,
		&movement.OccurredAt, &movement.Method, &movement.RecordedBy, &movement.CreatedAt,
	)
	if err != nil {
		return RecordResult{}, fmt.Errorf("insert out movement: %w", err)
	}
	movement.OccurredAt = movement.OccurredAt.In(timeutil.SGT)
	return RecordResult{
		Outcome:   Recorded,
		Direction: direction,
		Movement:  &movement,
		Previous:  latest,
	}, nil
}

// Void marks a movement as voided so it stops counting toward current state.
// The row is retained: the gate timeline must survive a correction.
func (s *Service) Void(ctx context.Context, movementID, actorID, reason string) error {
	if s == nil || s.db == nil || s.db.Pool == nil {
		return errors.New("out session service is not configured")
	}
	var reasonArg *string
	if reason != "" {
		reasonArg = &reason
	}
	tag, err := s.db.Pool.Exec(ctx, `
		UPDATE out_movement
		SET voided_at = NOW(), voided_by = $1, void_reason = $2
		WHERE id = $3 AND voided_at IS NULL
	`, actorID, reasonArg, movementID)
	if err != nil {
		return fmt.Errorf("void out movement: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrMovementNotFound
	}
	return nil
}

// ErrMovementNotFound indicates the movement does not exist or is already voided.
var ErrMovementNotFound = errors.New("out movement not found")

// RecordMovement runs Record in its own transaction.
func (s *Service) RecordMovement(ctx context.Context, req RecordRequest) (RecordResult, error) {
	if s == nil || s.db == nil || s.db.Pool == nil {
		return RecordResult{}, errors.New("out session service is not configured")
	}
	tx, err := s.db.Pool.Begin(ctx)
	if err != nil {
		return RecordResult{}, fmt.Errorf("begin movement: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	result, err := Record(ctx, tx, req, time.Now())
	if err != nil {
		return RecordResult{}, err
	}
	if result.Outcome != Recorded {
		return result, nil
	}
	if err := tx.Commit(ctx); err != nil {
		return RecordResult{}, fmt.Errorf("commit movement: %w", err)
	}
	return result, nil
}

// LatestMovement returns a user's current movement in a session, or nil.
func (s *Service) LatestMovement(ctx context.Context, sessionID, userID string) (*models.OutMovement, error) {
	if s == nil || s.db == nil || s.db.Pool == nil {
		return nil, errors.New("out session service is not configured")
	}
	return latestMovement(ctx, s.db.Pool, sessionID, userID)
}

type rowQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

const latestMovementSQL = `
	SELECT id, session_id, user_id, direction, occurred_at, method, recorded_by, "createdAt"
	FROM out_movement
	WHERE session_id = $1 AND user_id = $2 AND voided_at IS NULL
	ORDER BY occurred_at DESC, id DESC
	LIMIT 1
`

func latestMovement(ctx context.Context, q rowQuerier, sessionID, userID string) (*models.OutMovement, error) {
	var m models.OutMovement
	err := q.QueryRow(ctx, latestMovementSQL, sessionID, userID).Scan(
		&m.ID, &m.SessionID, &m.UserID, &m.Direction, &m.OccurredAt,
		&m.Method, &m.RecordedBy, &m.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load latest out movement: %w", err)
	}
	m.OccurredAt = m.OccurredAt.In(timeutil.SGT)
	return &m, nil
}

func latestMovementTx(ctx context.Context, tx pgx.Tx, sessionID, userID string) (*models.OutMovement, error) {
	return latestMovement(ctx, tx, sessionID, userID)
}

func userIsVerified(ctx context.Context, tx pgx.Tx, userID string) (bool, error) {
	var verified bool
	err := tx.QueryRow(ctx, `SELECT verified FROM "user" WHERE id = $1`, userID).Scan(&verified)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check actor verification: %w", err)
	}
	return verified, nil
}
