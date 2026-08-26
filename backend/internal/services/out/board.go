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

// outSessionColumns is the SELECT list shared by every Out session lookup.
const outSessionColumns = `id, name, qr_code, qr_code_secret, status, created_by,
	       start_time, out_subtype, expected_return_at, closed_at, "createdAt", "updatedAt"`

type scanner interface{ Scan(dest ...any) error }

// scanOutSession reads outSessionColumns into an OutSession.
func scanOutSession(row scanner) (models.OutSession, error) {
	var s models.OutSession
	var closedAt *time.Time
	if err := row.Scan(
		&s.ID, &s.Name, &s.QRCode, &s.QRCodeSecret, &s.Status, &s.CreatedBy,
		&s.StartTime, &s.Subtype, &s.ExpectedReturnAt, &closedAt,
		&s.CreatedAt, &s.UpdatedAt,
	); err != nil {
		return models.OutSession{}, err
	}
	s.ClosedAt = closedAt
	s.SubtypeLabel = models.OutSubtypeDisplayName(s.Subtype)
	s.ExpectedReturnAt = s.ExpectedReturnAt.In(timeutil.SGT)
	return s, nil
}

// loadOutSessionTx loads one Out session for update inside a transaction.
// Sessions of any other type report as not found, so an attendance session ID
// can never be driven through the Out endpoints.
func loadOutSessionTx(ctx context.Context, tx pgx.Tx, sessionID string) (models.OutSession, error) {
	row := tx.QueryRow(ctx, `
		SELECT `+outSessionColumns+`
		FROM attendance_session
		WHERE id = $1 AND session_type = $2
		FOR UPDATE
	`, sessionID, models.SessionTypeOut)
	session, err := scanOutSession(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return models.OutSession{}, ErrSessionNotFound
	}
	if err != nil {
		return models.OutSession{}, fmt.Errorf("load out session: %w", err)
	}
	return session, nil
}

// List returns Out sessions, newest first. Closed sessions are included so a
// commander can review the previous night.
func (s *Service) List(ctx context.Context, includeClosed bool) ([]models.OutSession, error) {
	if s == nil || s.db == nil || s.db.Pool == nil {
		return nil, errors.New("out session service is not configured")
	}
	query := `SELECT ` + outSessionColumns + `
		FROM attendance_session
		WHERE session_type = $1`
	if !includeClosed {
		query += ` AND status = 'active'`
	}
	query += ` ORDER BY start_time DESC LIMIT 200`

	rows, err := s.db.Pool.Query(ctx, query, models.SessionTypeOut)
	if err != nil {
		return nil, fmt.Errorf("list out sessions: %w", err)
	}
	defer rows.Close()

	sessions := make([]models.OutSession, 0)
	for rows.Next() {
		session, err := scanOutSession(rows)
		if err != nil {
			return nil, fmt.Errorf("scan out session: %w", err)
		}
		sessions = append(sessions, session)
	}
	return sessions, rows.Err()
}

// GetBoard returns the session with everyone currently enrolled and their
// standing. Overdue is derived at read time from the session's expected return,
// so no background job is needed to keep the board truthful.
func (s *Service) GetBoard(ctx context.Context, sessionID string) (Board, error) {
	if s == nil || s.db == nil || s.db.Pool == nil {
		return Board{}, errors.New("out session service is not configured")
	}
	session, err := s.Get(ctx, sessionID)
	if err != nil {
		return Board{}, err
	}

	rows, err := s.db.Pool.Query(ctx, `
		WITH latest AS (
			SELECT DISTINCT ON (m.user_id)
			       m.user_id, m.direction, m.occurred_at, m.method
			FROM out_movement m
			WHERE m.session_id = $1 AND m.voided_at IS NULL
			ORDER BY m.user_id, m.occurred_at DESC, m.id DESC
		),
		first_out AS (
			SELECT user_id, MIN(occurred_at) AS first_out_at
			FROM out_movement
			WHERE session_id = $1 AND direction = 'out' AND voided_at IS NULL
			GROUP BY user_id
		)
		SELECT l.user_id, u."full_name", u.rank, u.battery,
		       l.direction, l.occurred_at, l.method,
		       (l.direction = 'out' AND NOW() > $2) AS overdue,
		       f.first_out_at
		FROM latest l
		JOIN "user" u ON u.id = l.user_id
		LEFT JOIN first_out f ON f.user_id = l.user_id
		ORDER BY overdue DESC, l.occurred_at DESC
	`, sessionID, session.ExpectedReturnAt)
	if err != nil {
		return Board{}, fmt.Errorf("load out board: %w", err)
	}
	defer rows.Close()

	board := Board{Session: session, Members: make([]MovementState, 0)}
	for rows.Next() {
		var m MovementState
		var firstOut *time.Time
		if err := rows.Scan(&m.UserID, &m.FullName, &m.Rank, &m.Battery,
			&m.Direction, &m.OccurredAt, &m.Method, &m.Overdue, &firstOut); err != nil {
			return Board{}, fmt.Errorf("scan out board row: %w", err)
		}
		m.OccurredAt = m.OccurredAt.In(timeutil.SGT)
		if firstOut != nil {
			local := firstOut.In(timeutil.SGT)
			m.FirstOutAt = &local
		}
		if m.Direction == models.DirectionOut {
			board.OutCount++
			if m.Overdue {
				board.OverdueCount++
			}
		} else {
			board.ReturnedCount++
		}
		board.Members = append(board.Members, m)
	}
	if err := rows.Err(); err != nil {
		return Board{}, err
	}
	return board, nil
}

// Close ends an Out session. Anyone still out at that moment is reported so the
// commander sees who never scanned back in; the close is refused until they
// acknowledge it, unless the session is already empty of people out.
func (s *Service) Close(ctx context.Context, sessionID, actorID string, acknowledgeStillOut bool) (Board, error) {
	if s == nil || s.db == nil || s.db.Pool == nil {
		return Board{}, errors.New("out session service is not configured")
	}
	board, err := s.GetBoard(ctx, sessionID)
	if err != nil {
		return Board{}, err
	}
	if board.Session.Status != models.SessionStatusActive {
		return board, ErrSessionNotActive
	}
	if board.OutCount > 0 && !acknowledgeStillOut {
		return board, ErrStillOut
	}

	tag, err := s.db.Pool.Exec(ctx, `
		UPDATE attendance_session
		SET status = 'closed', closed_at = NOW(), "updatedAt" = NOW()
		WHERE id = $1 AND session_type = $2 AND status = 'active'
	`, sessionID, models.SessionTypeOut)
	if err != nil {
		return board, fmt.Errorf("close out session: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return board, ErrSessionNotActive
	}
	return s.GetBoard(ctx, sessionID)
}

var (
	// ErrSessionNotActive indicates a close was attempted on a session that is
	// already closed.
	ErrSessionNotActive = errors.New("out session is not active")
	// ErrStillOut indicates people are still out and the caller has not
	// acknowledged closing anyway.
	ErrStillOut = errors.New("personnel are still out")
)
