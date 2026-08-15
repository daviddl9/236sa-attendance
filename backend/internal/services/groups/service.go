// Package groups provides persistence for named, reusable participant lists.
package groups

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
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	// ErrInvalidRequest indicates a malformed group request.
	ErrInvalidRequest = errors.New("invalid participant group request")
	// ErrNotFound indicates the requested group does not exist.
	ErrNotFound = errors.New("participant group not found")
	// ErrDuplicateName indicates the creator already has a group with this name.
	ErrDuplicateName = errors.New("a group with this name already exists")
)

// Service owns participant-group persistence.
type Service struct {
	db *database.DB
}

// NewService constructs a group service.
func NewService(db *database.DB) *Service {
	return &Service{db: db}
}

// Create validates and atomically inserts a group with its members.
func (s *Service) Create(ctx context.Context, name string, participantIDs []string, createdBy string) (models.ParticipantGroup, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return models.ParticipantGroup{}, ErrInvalidRequest
	}
	if len(participantIDs) == 0 {
		return models.ParticipantGroup{}, ErrInvalidRequest
	}
	if s == nil || s.db == nil || s.db.Pool == nil {
		return models.ParticipantGroup{}, errors.New("group service is not configured")
	}

	tx, err := s.db.Pool.Begin(ctx)
	if err != nil {
		return models.ParticipantGroup{}, fmt.Errorf("begin group creation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	id, err := randomHex(16)
	if err != nil {
		return models.ParticipantGroup{}, err
	}
	now := time.Now()

	var group models.ParticipantGroup
	err = tx.QueryRow(ctx, `
		INSERT INTO participant_group (id, name, created_by, "createdAt", "updatedAt")
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, name, created_by, "createdAt", "updatedAt"
	`, id, name, createdBy, now, now).
		Scan(&group.ID, &group.Name, &group.CreatedBy, &group.CreatedAt, &group.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return models.ParticipantGroup{}, ErrDuplicateName
		}
		return models.ParticipantGroup{}, fmt.Errorf("insert participant group: %w", err)
	}

	for _, uid := range participantIDs {
		if _, err := tx.Exec(ctx, `
			INSERT INTO participant_group_member (group_id, user_id) VALUES ($1, $2)
			ON CONFLICT DO NOTHING
		`, id, uid); err != nil {
			return models.ParticipantGroup{}, fmt.Errorf("insert group member: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return models.ParticipantGroup{}, fmt.Errorf("commit group creation: %w", err)
	}
	count := len(participantIDs)
	group.MemberCount = &count
	return group, nil
}

// List returns all groups with their member counts, ordered by name.
func (s *Service) List(ctx context.Context) ([]models.ParticipantGroup, error) {
	if s == nil || s.db == nil || s.db.Pool == nil {
		return nil, errors.New("group service is not configured")
	}
	rows, err := s.db.Pool.Query(ctx, `
		SELECT g.id, g.name, g.created_by, g."createdAt", g."updatedAt",
		       COUNT(m.user_id)::int AS member_count
		FROM participant_group g
		LEFT JOIN participant_group_member m ON m.group_id = g.id
		GROUP BY g.id
		ORDER BY lower(g.name) ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("list participant groups: %w", err)
	}
	defer rows.Close()

	groups := make([]models.ParticipantGroup, 0)
	for rows.Next() {
		var g models.ParticipantGroup
		if err := rows.Scan(&g.ID, &g.Name, &g.CreatedBy, &g.CreatedAt, &g.UpdatedAt, &g.MemberCount); err != nil {
			return nil, fmt.Errorf("scan participant group: %w", err)
		}
		groups = append(groups, g)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return groups, nil
}

// Get returns a group and its member IDs, or ErrNotFound.
func (s *Service) Get(ctx context.Context, id string) (models.ParticipantGroup, []string, error) {
	if s == nil || s.db == nil || s.db.Pool == nil {
		return models.ParticipantGroup{}, nil, errors.New("group service is not configured")
	}
	var g models.ParticipantGroup
	var count int
	err := s.db.Pool.QueryRow(ctx, `
		SELECT g.id, g.name, g.created_by, g."createdAt", g."updatedAt",
		       (SELECT COUNT(*)::int FROM participant_group_member m WHERE m.group_id = g.id)
		FROM participant_group g
		WHERE g.id = $1
	`, id).Scan(&g.ID, &g.Name, &g.CreatedBy, &g.CreatedAt, &g.UpdatedAt, &count)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.ParticipantGroup{}, nil, ErrNotFound
		}
		return models.ParticipantGroup{}, nil, fmt.Errorf("get participant group: %w", err)
	}
	g.MemberCount = &count

	rows, err := s.db.Pool.Query(ctx, `
		SELECT user_id FROM participant_group_member WHERE group_id = $1 ORDER BY user_id
	`, id)
	if err != nil {
		return models.ParticipantGroup{}, nil, fmt.Errorf("list group members: %w", err)
	}
	defer rows.Close()

	members := make([]string, 0, count)
	for rows.Next() {
		var uid string
		if err := rows.Scan(&uid); err != nil {
			return models.ParticipantGroup{}, nil, fmt.Errorf("scan group member: %w", err)
		}
		members = append(members, uid)
	}
	if err := rows.Err(); err != nil {
		return models.ParticipantGroup{}, nil, err
	}
	return g, members, nil
}

// Members returns the member user IDs of a group, or ErrNotFound.
func (s *Service) Members(ctx context.Context, id string) ([]string, error) {
	_, members, err := s.Get(ctx, id)
	return members, err
}

// Delete removes a group (members cascade).
func (s *Service) Delete(ctx context.Context, id string) error {
	if s == nil || s.db == nil || s.db.Pool == nil {
		return errors.New("group service is not configured")
	}
	tag, err := s.db.Pool.Exec(ctx, `DELETE FROM participant_group WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete participant group: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// SaveSessionAsGroup snapshots a session's participants into a new named group.
// It returns ErrInvalidRequest when the session has no participants.
func (s *Service) SaveSessionAsGroup(ctx context.Context, sessionID, name, createdBy string) (models.ParticipantGroup, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return models.ParticipantGroup{}, ErrInvalidRequest
	}
	if s == nil || s.db == nil || s.db.Pool == nil {
		return models.ParticipantGroup{}, errors.New("group service is not configured")
	}

	rows, err := s.db.Pool.Query(ctx, `
		SELECT user_id FROM session_participants WHERE session_id = $1 ORDER BY user_id
	`, sessionID)
	if err != nil {
		return models.ParticipantGroup{}, fmt.Errorf("read session participants: %w", err)
	}
	defer rows.Close()

	var memberIDs []string
	for rows.Next() {
		var uid string
		if err := rows.Scan(&uid); err != nil {
			return models.ParticipantGroup{}, fmt.Errorf("scan session participant: %w", err)
		}
		memberIDs = append(memberIDs, uid)
	}
	if err := rows.Err(); err != nil {
		return models.ParticipantGroup{}, err
	}
	if len(memberIDs) == 0 {
		return models.ParticipantGroup{}, ErrInvalidRequest
	}
	return s.Create(ctx, name, memberIDs, createdBy)
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
