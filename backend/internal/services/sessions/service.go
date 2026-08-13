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
	// ErrSessionExpired indicates that an end-time bounded operation arrived
	// after the session's end time.
	ErrSessionExpired = errors.New("attendance session has expired")
	// ErrSessionUnauthorized indicates that the actor is outside the session's
	// current scope. Adapters map this to their safe unavailable response.
	ErrSessionUnauthorized = errors.New("actor is not authorized for attendance session")
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

// IsExpired compares PostgreSQL TIMESTAMP values using wall-clock semantics.
// The schema intentionally stores end_time without a timezone, while pgx may
// scan it as UTC and callers may construct it in a local location. Choosing
// the interpretation nearest to the operation time handles both forms.
func IsExpired(endTime *time.Time, now time.Time) bool {
	if endTime == nil {
		return false
	}
	normalized := time.Date(
		endTime.Year(), endTime.Month(), endTime.Day(),
		endTime.Hour(), endTime.Minute(), endTime.Second(), endTime.Nanosecond(),
		now.Location(),
	)
	directDelta := endTime.Sub(now)
	normalizedDelta := normalized.Sub(now)
	if directDelta < 0 {
		directDelta = -directDelta
	}
	if normalizedDelta < 0 {
		normalizedDelta = -normalizedDelta
	}
	if normalizedDelta <= directDelta {
		return !normalized.After(now)
	}
	return !endTime.After(now)
}

// Create validates and atomically inserts a standard attendance session.
func (s *Service) Create(ctx context.Context, req CreateRequest) (models.AttendanceSession, error) {
	if err := validateCreateRequest(req); err != nil {
		return models.AttendanceSession{}, err
	}
	if s == nil || s.db == nil || s.db.Pool == nil {
		return models.AttendanceSession{}, errors.New("session service is not configured")
	}
	tx, err := s.db.Pool.Begin(ctx)
	if err != nil {
		return models.AttendanceSession{}, fmt.Errorf("begin session creation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	session, err := s.CreateInTx(ctx, tx, req)
	if err != nil {
		return models.AttendanceSession{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return models.AttendanceSession{}, fmt.Errorf("commit attendance session: %w", err)
	}
	return session, nil
}

// CreateInTx inserts a standard session using the caller's transaction. It
// shares the same schema contract as Create so Telegram can authorize and
// create an event under one transaction.
func (s *Service) CreateInTx(ctx context.Context, tx pgx.Tx, req CreateRequest) (models.AttendanceSession, error) {
	if err := validateCreateRequest(req); err != nil {
		return models.AttendanceSession{}, err
	}
	if s == nil || tx == nil {
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
		&session.ID, &session.Name, &session.QRCode, &session.QRCodeSecret,
		&session.Scope, &session.Batteries, &session.Status, &session.CreatedBy,
		&session.StartTime, &session.EndTime, &closedAt, &returnedCode,
		&session.CreatedAt, &session.UpdatedAt,
	)
	if err != nil {
		return models.AttendanceSession{}, fmt.Errorf("insert attendance session: %w", err)
	}
	session.ClosedAt = closedAt
	if returnedCode != nil {
		session.DeepLinkCode = *returnedCode
	}
	session.TelegramLink = s.telegramLink(session.DeepLinkCode)
	return session, nil
}

// sessionQueryer is implemented by both pgxpool.Pool and pgx.Tx.
type sessionQueryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

// ListActive returns active sessions usable by actor. End-time expiry is
// evaluated by PostgreSQL at query time so a stale web or Telegram preflight
// cannot keep an expired event visible.
func (s *Service) ListActive(ctx context.Context, actor *models.User) ([]models.AttendanceSession, error) {
	if s == nil || s.db == nil || s.db.Pool == nil {
		return nil, errors.New("session service is not configured")
	}
	return s.listActive(ctx, s.db.Pool, actor)
}

// ListActiveTx runs the same active-session scope query in a caller-owned
// transaction, allowing Telegram to bind actor and session authorization.
func (s *Service) ListActiveTx(ctx context.Context, tx pgx.Tx, actor *models.User) ([]models.AttendanceSession, error) {
	if s == nil || tx == nil {
		return nil, errors.New("session service is not configured")
	}
	return s.listActive(ctx, tx, actor)
}

func (s *Service) listActive(ctx context.Context, q sessionQueryer, actor *models.User) ([]models.AttendanceSession, error) {
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

	rows, err := q.Query(ctx, query, args...)
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
		if IsExpired(session.EndTime, time.Now()) {
			continue
		}
		session.TelegramLink = s.telegramLink(session.DeepLinkCode)
		sessions = append(sessions, session)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate active attendance sessions: %w", err)
	}
	return sessions, nil
}

// LoadAuthorizedTx locks and reloads one session in the caller's transaction.
// Tier 2 scope is evaluated against the actor's current battery, including the
// participant intersection for custom-list sessions.
func (s *Service) LoadAuthorizedTx(ctx context.Context, tx pgx.Tx, sessionID string, actor *models.User, requireActive bool) (models.AttendanceSession, error) {
	if s == nil || tx == nil {
		return models.AttendanceSession{}, errors.New("session service is not configured")
	}
	if strings.TrimSpace(sessionID) == "" {
		return models.AttendanceSession{}, fmt.Errorf("%w: session ID is required", ErrInvalidRequest)
	}

	var session models.AttendanceSession
	var closedAt *time.Time
	var deeplinkCode *string
	err := tx.QueryRow(ctx, `
		SELECT id, name, qr_code, qr_code_secret, scope, batteries,
		       status, created_by, start_time, end_time, closed_at, deeplink_code,
		       "createdAt", "updatedAt"
		FROM attendance_session
		WHERE id = $1
		FOR UPDATE
	`, sessionID).Scan(
		&session.ID, &session.Name, &session.QRCode, &session.QRCodeSecret,
		&session.Scope, &session.Batteries, &session.Status, &session.CreatedBy,
		&session.StartTime, &session.EndTime, &closedAt, &deeplinkCode,
		&session.CreatedAt, &session.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return models.AttendanceSession{}, fmt.Errorf("%w: %s", ErrSessionNotFound, sessionID)
	}
	if err != nil {
		return models.AttendanceSession{}, fmt.Errorf("load authorized attendance session: %w", err)
	}
	session.ClosedAt = closedAt
	if deeplinkCode != nil {
		session.DeepLinkCode = *deeplinkCode
	}
	if session.Status != models.SessionStatusActive {
		return models.AttendanceSession{}, ErrSessionClosed
	}
	if requireActive && IsExpired(session.EndTime, time.Now()) {
		return models.AttendanceSession{}, ErrSessionExpired
	}
	allowed, err := actorCanUseSession(ctx, tx, session, actor)
	if err != nil {
		return models.AttendanceSession{}, fmt.Errorf("check attendance session authority: %w", err)
	}
	if !allowed {
		return models.AttendanceSession{}, ErrSessionUnauthorized
	}
	return session, nil
}

func actorCanUseSession(ctx context.Context, tx pgx.Tx, session models.AttendanceSession, actor *models.User) (bool, error) {
	if actor == nil {
		return false, nil
	}
	if actor.GetTier() >= models.TierUnitCommander {
		return true, nil
	}
	if actor.Battery == nil || strings.TrimSpace(*actor.Battery) == "" {
		return false, nil
	}
	battery := strings.TrimSpace(*actor.Battery)
	switch session.Scope {
	case models.SessionScopeUnitWide:
		return true, nil
	case models.SessionScopeBatterySpecific:
		for _, allowed := range session.Batteries {
			if allowed == battery {
				return true, nil
			}
		}
		return false, nil
	case models.SessionScopeCustomList:
		var participant bool
		err := tx.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1
				FROM session_participants sp
				JOIN "user" participant ON participant.id = sp.user_id
				WHERE sp.session_id = $1
				  AND participant.battery = $2
				  AND participant.verified = true
			)
		`, session.ID, battery).Scan(&participant)
		return participant, err
	default:
		return false, nil
	}
}

// CloseInTx closes a session in the caller's transaction. When requireActive
// is true, an end-time-expired session is unavailable even if its status still
// says active. The web Close adapter continues to use its historical
// status-only contract; Telegram passes true.
func (s *Service) CloseInTx(ctx context.Context, tx pgx.Tx, sessionID string, actor *models.User, requireActive bool) error {
	if s == nil || tx == nil {
		return errors.New("session service is not configured")
	}
	if strings.TrimSpace(sessionID) == "" {
		return fmt.Errorf("%w: session ID is required", ErrInvalidRequest)
	}
	var createdBy, status string
	var endTime *time.Time
	err := tx.QueryRow(ctx, `
		SELECT created_by, status, end_time
		FROM attendance_session
		WHERE id = $1
		FOR UPDATE
	`, sessionID).Scan(&createdBy, &status, &endTime)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: %s", ErrSessionNotFound, sessionID)
	}
	if err != nil {
		return fmt.Errorf("load attendance session for close: %w", err)
	}
	if status != models.SessionStatusActive {
		return ErrSessionClosed
	}
	if requireActive && IsExpired(endTime, time.Now()) {
		return ErrSessionExpired
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
	return nil
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
	if err := s.CloseInTx(ctx, tx, sessionID, actor, false); err != nil {
		return err
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
