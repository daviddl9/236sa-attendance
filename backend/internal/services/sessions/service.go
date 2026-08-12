// Package sessions contains the shared attendance-session application service.
package sessions

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/davidlivingston/go-nextjs-starter/backend/internal/database"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/models"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/services/deeplink"
	"github.com/jackc/pgx/v5"
)

var (
	// ErrInvalidRequest indicates a malformed standard-session request.
	ErrInvalidRequest = errors.New("invalid attendance session request")
	// ErrSessionNotFound indicates that the requested session does not exist.
	ErrSessionNotFound = errors.New("attendance session not found")
	// ErrSessionNotOwner indicates that an actor cannot close the session.
	ErrSessionNotOwner = errors.New("actor does not own attendance session")
	// ErrSessionClosed indicates that a close was attempted after the session
	// was no longer active.
	ErrSessionClosed = errors.New("attendance session is not active")
)

// CreateRequest contains the values needed to create a standard attendance
// session. Custom participant sessions remain a dashboard-only handler flow.
type CreateRequest struct {
	Name      string
	Scope     string
	Batteries []string
	EndTime   *time.Time
	CreatedBy string
}

// Service owns shared attendance-session persistence and authorization rules.
type Service struct {
	db          *database.DB
	botUsername string
}

// NewService constructs a session service. botUsername is kept separate from
// Telegram transport credentials; only the public configured username is used
// to build links.
func NewService(db *database.DB, botUsername string) *Service {
	return &Service{
		db:          db,
		botUsername: strings.TrimPrefix(strings.TrimSpace(botUsername), "@"),
	}
}

// Create validates and atomically inserts a standard attendance session.
func (s *Service) Create(ctx context.Context, req CreateRequest) (models.AttendanceSession, error) {
	if err := validateCreateRequest(req); err != nil {
		return models.AttendanceSession{}, err
	}
	if s == nil || s.db == nil || s.db.Pool == nil {
		return models.AttendanceSession{}, errors.New("session service is not configured")
	}

	sessionID, err := randomHex(16)
	if err != nil {
		return models.AttendanceSession{}, fmt.Errorf("generate session ID: %w", err)
	}
	qrSecret, err := randomHex(32)
	if err != nil {
		return models.AttendanceSession{}, fmt.Errorf("generate QR secret: %w", err)
	}
	deeplinkCode, err := deeplink.GenerateCode()
	if err != nil {
		return models.AttendanceSession{}, fmt.Errorf("generate session deep-link code: %w", err)
	}

	batteries := req.Batteries
	if batteries == nil {
		batteries = []string{}
	}
	now := time.Now()
	qrCode := sessionID + ":" + qrSecret

	tx, err := s.db.Pool.Begin(ctx)
	if err != nil {
		return models.AttendanceSession{}, fmt.Errorf("begin session creation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var session models.AttendanceSession
	var closedAt *time.Time
	var returnedCode *string
	err = tx.QueryRow(ctx, `
		INSERT INTO attendance_session (
			id, name, qr_code, qr_code_secret, scope, batteries,
			status, created_by, start_time, end_time, deeplink_code, "createdAt", "updatedAt"
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING id, name, qr_code, qr_code_secret, scope, batteries,
		          status, created_by, start_time, end_time, closed_at, deeplink_code,
		          "createdAt", "updatedAt"
	`, sessionID, req.Name, qrCode, qrSecret, req.Scope, batteries,
		models.SessionStatusActive, req.CreatedBy, now, req.EndTime, deeplinkCode, now, now,
	).Scan(
		&session.ID,
		&session.Name,
		&session.QRCode,
		&session.QRCodeSecret,
		&session.Scope,
		&session.Batteries,
		&session.Status,
		&session.CreatedBy,
		&session.StartTime,
		&session.EndTime,
		&closedAt,
		&returnedCode,
		&session.CreatedAt,
		&session.UpdatedAt,
	)
	if err != nil {
		return models.AttendanceSession{}, fmt.Errorf("insert attendance session: %w", err)
	}
	session.ClosedAt = closedAt
	if returnedCode != nil {
		session.DeepLinkCode = *returnedCode
	}
	if err := tx.Commit(ctx); err != nil {
		return models.AttendanceSession{}, fmt.Errorf("commit attendance session: %w", err)
	}
	session.TelegramLink = s.telegramLink(session.DeepLinkCode)
	return session, nil
}

// ListActive returns active sessions usable by actor. Tier 2 (and lower) is
// restricted to unit-wide sessions and battery-specific sessions containing
// the actor's battery. Tier 3+ uses the unit-wide authority boundary and can
// see every active session in the unit, including existing custom sessions.
func (s *Service) ListActive(ctx context.Context, actor *models.User) ([]models.AttendanceSession, error) {
	if s == nil || s.db == nil || s.db.Pool == nil {
		return nil, errors.New("session service is not configured")
	}

	query := `
		SELECT id, name, qr_code, qr_code_secret, scope, batteries,
		       status, created_by, start_time, end_time, closed_at, deeplink_code,
		       "createdAt", "updatedAt"
		FROM attendance_session
		WHERE status = $1
	`
	args := []any{models.SessionStatusActive}
	if actor != nil && actor.GetTier() < models.TierUnitCommander {
		if actor.Battery == nil || strings.TrimSpace(*actor.Battery) == "" {
			query += " AND FALSE\n"
		} else {
			query += ` AND (
				scope = $2 OR
				(scope = $3 AND $4 = ANY(batteries)) OR
				(scope = $5 AND EXISTS (
					SELECT 1 FROM session_participants sp
					JOIN "user" participant ON participant.id = sp.user_id
					WHERE sp.session_id = attendance_session.id
					  AND participant.battery = $4
					  AND participant.verified = true
				))
			)
			`
			args = append(args,
				models.SessionScopeUnitWide,
				models.SessionScopeBatterySpecific,
				*actor.Battery,
				models.SessionScopeCustomList,
			)
		}
	}
	query += ` ORDER BY start_time DESC, id ASC`

	rows, err := s.db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list active attendance sessions: %w", err)
	}
	defer rows.Close()

	sessions := make([]models.AttendanceSession, 0)
	for rows.Next() {
		session, err := scanSession(rows)
		if err != nil {
			return nil, fmt.Errorf("scan active attendance session: %w", err)
		}
		session.TelegramLink = s.telegramLink(session.DeepLinkCode)
		sessions = append(sessions, session)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate active attendance sessions: %w", err)
	}
	return sessions, nil
}

// Close closes an active session when actor created it or is a superadmin.
func (s *Service) Close(ctx context.Context, sessionID string, actor *models.User) error {
	if s == nil || s.db == nil || s.db.Pool == nil {
		return errors.New("session service is not configured")
	}
	if strings.TrimSpace(sessionID) == "" {
		return fmt.Errorf("%w: session ID is required", ErrInvalidRequest)
	}

	tx, err := s.db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin session close: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var createdBy, status string
	err = tx.QueryRow(ctx, `
		SELECT created_by, status
		FROM attendance_session
		WHERE id = $1
		FOR UPDATE
	`, sessionID).Scan(&createdBy, &status)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: %s", ErrSessionNotFound, sessionID)
		}
		return fmt.Errorf("load attendance session for close: %w", err)
	}
	if status != models.SessionStatusActive {
		return ErrSessionClosed
	}

	isSuperadmin := actor != nil && (actor.IsSuperadmin || actor.GetTier() >= models.TierSuperadmin)
	if actor == nil || (createdBy != actor.ID && !isSuperadmin) {
		return ErrSessionNotOwner
	}

	now := time.Now()
	result, err := tx.Exec(ctx, `
		UPDATE attendance_session
		SET status = $1, closed_at = $2, "updatedAt" = $2
		WHERE id = $3 AND status = $4
	`, models.SessionStatusClosed, now, sessionID, models.SessionStatusActive)
	if err != nil {
		return fmt.Errorf("close attendance session: %w", err)
	}
	if result.RowsAffected() != 1 {
		return ErrSessionClosed
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit attendance session close: %w", err)
	}
	return nil
}

// Get returns a session by ID, including internal capabilities for trusted
// application adapters. Model JSON tags keep those capabilities out of web
// responses unless the handler explicitly derives the public Telegram link.
func (s *Service) Get(ctx context.Context, sessionID string) (models.AttendanceSession, error) {
	if s == nil || s.db == nil || s.db.Pool == nil {
		return models.AttendanceSession{}, errors.New("session service is not configured")
	}
	if strings.TrimSpace(sessionID) == "" {
		return models.AttendanceSession{}, fmt.Errorf("%w: session ID is required", ErrInvalidRequest)
	}

	session, err := scanSession(s.db.Pool.QueryRow(ctx, `
		SELECT id, name, qr_code, qr_code_secret, scope, batteries,
		       status, created_by, start_time, end_time, closed_at, deeplink_code,
		       "createdAt", "updatedAt"
		FROM attendance_session
		WHERE id = $1
	`, sessionID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.AttendanceSession{}, fmt.Errorf("%w: %s", ErrSessionNotFound, sessionID)
		}
		return models.AttendanceSession{}, fmt.Errorf("get attendance session: %w", err)
	}
	session.TelegramLink = s.telegramLink(session.DeepLinkCode)
	return session, nil
}

func validateCreateRequest(req CreateRequest) error {
	if strings.TrimSpace(req.CreatedBy) == "" {
		return fmt.Errorf("%w: creator is required", ErrInvalidRequest)
	}
	switch req.Scope {
	case models.SessionScopeUnitWide:
	case models.SessionScopeBatterySpecific:
		if len(req.Batteries) == 0 {
			return fmt.Errorf("%w: batteries are required for battery-specific sessions", ErrInvalidRequest)
		}
	default:
		return fmt.Errorf("%w: unsupported session scope %q", ErrInvalidRequest, req.Scope)
	}
	for _, battery := range req.Batteries {
		if !isAllowedBattery(battery) {
			return fmt.Errorf("%w: invalid battery %q", ErrInvalidRequest, battery)
		}
	}
	return nil
}

func isAllowedBattery(battery string) bool {
	switch battery {
	case models.BatteryHQ, models.BatteryAlpha, models.BatteryBravo:
		return true
	default:
		return false
	}
}

func (s *Service) telegramLink(code string) string {
	if s == nil || s.botUsername == "" || !deeplink.IsValidCode(code) {
		return ""
	}
	return fmt.Sprintf("https://t.me/%s?start=%s", s.botUsername, code)
}

func randomHex(size int) (string, error) {
	bytes := make([]byte, size)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanSession(row scanner) (models.AttendanceSession, error) {
	var session models.AttendanceSession
	var closedAt *time.Time
	var deeplinkCode *string
	if err := row.Scan(
		&session.ID,
		&session.Name,
		&session.QRCode,
		&session.QRCodeSecret,
		&session.Scope,
		&session.Batteries,
		&session.Status,
		&session.CreatedBy,
		&session.StartTime,
		&session.EndTime,
		&closedAt,
		&deeplinkCode,
		&session.CreatedAt,
		&session.UpdatedAt,
	); err != nil {
		return models.AttendanceSession{}, err
	}
	session.ClosedAt = closedAt
	if deeplinkCode != nil {
		session.DeepLinkCode = *deeplinkCode
	}
	return session, nil
}
