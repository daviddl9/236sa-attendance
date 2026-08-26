// Package out contains the application service for "Out" sessions: the
// self-enrolling gate log used for Nights Out, Stay Out and Off Pass.
package out

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
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/timeutil"
	"github.com/jackc/pgx/v5"
)

var (
	// ErrInvalidRequest indicates a malformed Out session request.
	ErrInvalidRequest = errors.New("invalid out session request")
	// ErrSessionNotFound indicates that the requested Out session does not exist.
	ErrSessionNotFound = errors.New("out session not found")
)

// Default return times, in SGT. Nights Out and Off Pass are same-day evening
// returns; Stay Out is overnight and returns for first parade.
const (
	sameDayReturnHour   = 23
	overnightReturnHour = 7
)

// CreateRequest contains the values needed to open an Out session. There is no
// scope and no participant list: soldiers enrol themselves by scanning.
type CreateRequest struct {
	Name             string
	Subtype          string
	ExpectedReturnAt *time.Time // nil selects the sub-type's SGT default
	CreatedBy        string
}

// Service owns Out session persistence.
type Service struct {
	db *database.DB
}

// NewService constructs an Out session service.
func NewService(db *database.DB) *Service {
	return &Service{db: db}
}

// DefaultReturnAt returns the expected return time for a sub-type, expressed
// in SGT so the value never depends on the commander's device clock.
//
// Nights Out and Off Pass default to 23:00 the same day; Stay Out defaults to
// 07:00 the next morning. A same-day default that has already passed — a
// session opened at 23:30 — rolls forward a day, because a session whose
// enrollees are overdue the instant they scan is never what was meant.
func DefaultReturnAt(subtype string, now time.Time) time.Time {
	local := now.In(timeutil.SGT)
	year, month, day := local.Date()

	if subtype == models.OutSubtypeStayOut {
		return time.Date(year, month, day+1, overnightReturnHour, 0, 0, 0, timeutil.SGT)
	}

	candidate := time.Date(year, month, day, sameDayReturnHour, 0, 0, 0, timeutil.SGT)
	if !candidate.After(local) {
		candidate = candidate.AddDate(0, 0, 1)
	}
	return candidate
}

// Create validates and inserts an Out session, returning the stored row.
func (s *Service) Create(ctx context.Context, req CreateRequest) (models.OutSession, error) {
	normalized, err := validateCreateRequest(req)
	if err != nil {
		return models.OutSession{}, err
	}
	if s == nil || s.db == nil || s.db.Pool == nil {
		return models.OutSession{}, errors.New("out session service is not configured")
	}

	tx, err := s.db.Pool.Begin(ctx)
	if err != nil {
		return models.OutSession{}, fmt.Errorf("begin out session creation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	session, err := s.CreateInTx(ctx, tx, normalized)
	if err != nil {
		return models.OutSession{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return models.OutSession{}, fmt.Errorf("commit out session: %w", err)
	}
	return session, nil
}

// CreateInTx inserts an Out session using the caller's transaction.
func (s *Service) CreateInTx(ctx context.Context, tx pgx.Tx, req CreateRequest) (models.OutSession, error) {
	normalized, err := validateCreateRequest(req)
	if err != nil {
		return models.OutSession{}, err
	}
	if tx == nil {
		return models.OutSession{}, errors.New("out session service is not configured")
	}

	sessionID, err := randomHex(16)
	if err != nil {
		return models.OutSession{}, fmt.Errorf("generate out session ID: %w", err)
	}
	qrSecret, err := randomHex(32)
	if err != nil {
		return models.OutSession{}, fmt.Errorf("generate QR secret: %w", err)
	}

	now := time.Now()
	qrCode := sessionID + ":" + qrSecret

	// end_time and deeplink_code are deliberately left NULL. An Out session
	// must survive overnight, so it must never carry an end time the expiry
	// path could act on, and Telegram scanning is not part of this flow.
	var session models.OutSession
	var closedAt *time.Time
	err = tx.QueryRow(ctx, `
		INSERT INTO attendance_session (
			id, name, qr_code, qr_code_secret, scope, batteries, status,
			created_by, start_time, session_type, out_subtype,
			expected_return_at, "createdAt", "updatedAt"
		)
		VALUES ($1, $2, $3, $4, $5, '{}', $6, $7, $8, $9, $10, $11, $12, $12)
		RETURNING id, name, qr_code, qr_code_secret, status, created_by,
		          start_time, out_subtype, expected_return_at, closed_at,
		          "createdAt", "updatedAt"
	`,
		sessionID, normalized.Name, qrCode, qrSecret, models.SessionScopeUnitWide,
		models.SessionStatusActive, normalized.CreatedBy, now,
		models.SessionTypeOut, normalized.Subtype, *normalized.ExpectedReturnAt, now,
	).Scan(
		&session.ID, &session.Name, &session.QRCode, &session.QRCodeSecret,
		&session.Status, &session.CreatedBy, &session.StartTime,
		&session.Subtype, &session.ExpectedReturnAt, &closedAt,
		&session.CreatedAt, &session.UpdatedAt,
	)
	if err != nil {
		return models.OutSession{}, fmt.Errorf("insert out session: %w", err)
	}
	session.ClosedAt = closedAt
	session.SubtypeLabel = models.OutSubtypeDisplayName(session.Subtype)
	session.ExpectedReturnAt = session.ExpectedReturnAt.In(timeutil.SGT)
	return session, nil
}

// Get returns one Out session by ID. Sessions of any other type are reported
// as not found, so an attendance session ID can never be driven through the
// Out endpoints.
func (s *Service) Get(ctx context.Context, sessionID string) (models.OutSession, error) {
	if s == nil || s.db == nil || s.db.Pool == nil {
		return models.OutSession{}, errors.New("out session service is not configured")
	}
	row := s.db.Pool.QueryRow(ctx, `
		SELECT `+outSessionColumns+`
		FROM attendance_session
		WHERE id = $1 AND session_type = $2
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

// validateCreateRequest normalizes and checks a create request, filling the
// sub-type's default return time when the caller did not supply one.
func validateCreateRequest(req CreateRequest) (CreateRequest, error) {
	req.Name = strings.TrimSpace(req.Name)
	req.Subtype = strings.TrimSpace(req.Subtype)

	if strings.TrimSpace(req.CreatedBy) == "" {
		return req, fmt.Errorf("%w: creator is required", ErrInvalidRequest)
	}
	if req.Name == "" {
		return req, fmt.Errorf("%w: name is required", ErrInvalidRequest)
	}
	if !models.IsValidOutSubtype(req.Subtype) {
		return req, fmt.Errorf("%w: unsupported out sub-type %q", ErrInvalidRequest, req.Subtype)
	}

	now := time.Now()
	if req.ExpectedReturnAt == nil {
		defaulted := DefaultReturnAt(req.Subtype, now)
		req.ExpectedReturnAt = &defaulted
		return req, nil
	}
	// A return time already in the past would mark every enrollee overdue the
	// moment they scan, which is always a mistake rather than an intent.
	if !req.ExpectedReturnAt.After(now) {
		return req, fmt.Errorf("%w: expected return time must be in the future", ErrInvalidRequest)
	}
	return req, nil
}

func randomHex(size int) (string, error) {
	bytes := make([]byte, size)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
