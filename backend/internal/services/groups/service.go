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

// SetMembers atomically replaces a group's full member list with the deduped
// user IDs. It returns the new member count and ErrNotFound when the group does
// not exist.
func (s *Service) SetMembers(ctx context.Context, groupID string, userIDs []string) (int, error) {
	if s == nil || s.db == nil || s.db.Pool == nil {
		return 0, errors.New("group service is not configured")
	}

	tx, err := s.db.Pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin set members: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM participant_group WHERE id = $1)`, groupID).Scan(&exists); err != nil {
		return 0, fmt.Errorf("check participant group: %w", err)
	}
	if !exists {
		return 0, ErrNotFound
	}

	if _, err := tx.Exec(ctx, `DELETE FROM participant_group_member WHERE group_id = $1`, groupID); err != nil {
		return 0, fmt.Errorf("clear group members: %w", err)
	}

	deduped := dedupeIDs(userIDs)
	for _, uid := range deduped {
		if _, err := tx.Exec(ctx, `
			INSERT INTO participant_group_member (group_id, user_id) VALUES ($1, $2)
			ON CONFLICT DO NOTHING
		`, groupID, uid); err != nil {
			return 0, fmt.Errorf("insert group member: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit set members: %w", err)
	}
	return len(deduped), nil
}

// RemoveMember deletes a single member from a group. Removing a user that is
// not a member is a no-op. It returns ErrNotFound when the group does not exist.
func (s *Service) RemoveMember(ctx context.Context, groupID, userID string) error {
	if s == nil || s.db == nil || s.db.Pool == nil {
		return errors.New("group service is not configured")
	}

	var exists bool
	if err := s.db.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM participant_group WHERE id = $1)`, groupID).Scan(&exists); err != nil {
		return fmt.Errorf("check participant group: %w", err)
	}
	if !exists {
		return ErrNotFound
	}

	if _, err := s.db.Pool.Exec(ctx, `
		DELETE FROM participant_group_member WHERE group_id = $1 AND user_id = $2
	`, groupID, userID); err != nil {
		return fmt.Errorf("delete group member: %w", err)
	}
	return nil
}

// MembersWithDetails returns the group's members joined with roster details,
// ordered by lower(full_name).
func (s *Service) MembersWithDetails(ctx context.Context, groupID string) ([]models.GroupMember, error) {
	if s == nil || s.db == nil || s.db.Pool == nil {
		return nil, errors.New("group service is not configured")
	}

	rows, err := s.db.Pool.Query(ctx, `
		SELECT m.user_id, u."full_name", u.rank, u.battery
		FROM participant_group_member m
		JOIN "user" u ON u.id = m.user_id
		WHERE m.group_id = $1
		ORDER BY lower(u."full_name") ASC
	`, groupID)
	if err != nil {
		return nil, fmt.Errorf("list group member details: %w", err)
	}
	defer rows.Close()

	members := make([]models.GroupMember, 0)
	for rows.Next() {
		var m models.GroupMember
		if err := rows.Scan(&m.UserID, &m.FullName, &m.Rank, &m.Battery); err != nil {
			return nil, fmt.Errorf("scan group member detail: %w", err)
		}
		members = append(members, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return members, nil
}

// dedupeIDs returns userIDs with duplicates removed, preserving first-seen
// order.
func dedupeIDs(userIDs []string) []string {
	seen := make(map[string]struct{}, len(userIDs))
	deduped := make([]string, 0, len(userIDs))
	for _, uid := range userIDs {
		if _, ok := seen[uid]; ok {
			continue
		}
		seen[uid] = struct{}{}
		deduped = append(deduped, uid)
	}
	return deduped
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

// Rename updates a group's name, returning the updated group.
func (s *Service) Rename(ctx context.Context, id, name string) (models.ParticipantGroup, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return models.ParticipantGroup{}, ErrInvalidRequest
	}
	if s == nil || s.db == nil || s.db.Pool == nil {
		return models.ParticipantGroup{}, errors.New("group service is not configured")
	}
	var group models.ParticipantGroup
	err := s.db.Pool.QueryRow(ctx, `
		UPDATE participant_group SET name = $1, "updatedAt" = $2 WHERE id = $3
		RETURNING id, name, created_by, "createdAt", "updatedAt"
	`, name, time.Now(), id).
		Scan(&group.ID, &group.Name, &group.CreatedBy, &group.CreatedAt, &group.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return models.ParticipantGroup{}, ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return models.ParticipantGroup{}, ErrDuplicateName
	}
	if err != nil {
		return models.ParticipantGroup{}, fmt.Errorf("rename participant group: %w", err)
	}
	return group, nil
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
