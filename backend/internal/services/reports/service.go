// Package reports contains scoped attendance reporting for web and Telegram
// adapters. It returns roster data without personnel-status records.
package reports

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/davidlivingston/go-nextjs-starter/backend/internal/database"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/models"
	"github.com/jackc/pgx/v5"
)

var (
	// ErrSessionNotFound indicates that no attendance session exists for the ID.
	ErrSessionNotFound = errors.New("attendance session not found")
	// ErrUnsupportedScope indicates corrupt or obsolete session scope data.
	ErrUnsupportedScope = errors.New("unsupported attendance session scope")
)

// UserRow is the status-safe roster shape used by Telegram. It intentionally
// contains no active personnel-status fields.
type UserRow struct {
	ID      string
	Name    string
	Rank    string
	Battery string
}

// Summary is the scoped attendance summary for one session.
type Summary struct {
	SessionName string
	Total       int
	Present     int
	Missing     int
	Percentage  float64
}

// MissingPage contains a deterministic, scoped page of absent users.
type MissingPage struct {
	Rows        []UserRow
	Page        int
	PageCount   int
	Total       int
	HasPrevious bool
	HasNext     bool
}

// Service owns shared, authorization-aware attendance reports.
type Service struct {
	db *database.DB
}

// NewService constructs a report service.
func NewService(db *database.DB) *Service {
	return &Service{db: db}
}

// Summary returns attendance counts over the session's authorized roster. The
// roster rules mirror the dashboard: custom lists use their participants,
// unit-wide/battery-specific sessions exclude superadmins, and Tier 1-2
// actors are restricted to their own battery.
func (s *Service) Summary(ctx context.Context, sessionID string, actor *models.User) (Summary, error) {
	if s == nil || s.db == nil || s.db.Pool == nil {
		return Summary{}, errors.New("report service is not configured")
	}

	meta, rosterSQL, args, err := s.scopedRoster(ctx, sessionID, actor)
	if err != nil {
		return Summary{}, err
	}
	attendanceArg := len(args) + 1
	args = append(args, sessionID)

	var total, present int
	err = s.db.Pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT COUNT(*)::int, COUNT(ar.user_id)::int
		FROM (%s) AS eligible
		LEFT JOIN attendance_record ar
		  ON ar.session_id = $%d AND ar.user_id = eligible.user_id
	`, rosterSQL, attendanceArg), args...).Scan(&total, &present)
	if err != nil {
		return Summary{}, fmt.Errorf("summarize attendance session: %w", err)
	}

	percentage := 0.0
	if total > 0 {
		percentage = float64(present) / float64(total) * 100
	}
	// Avoid propagating a non-finite value if a future database type change
	// causes an unexpected count conversion.
	if math.IsNaN(percentage) || math.IsInf(percentage, 0) {
		percentage = 0
	}
	return Summary{
		SessionName: meta.name,
		Total:       total,
		Present:     present,
		Missing:     total - present,
		Percentage:  percentage,
	}, nil
}

// EligibleUsers returns the scoped roster for web adapters that need to
// preserve the dashboard's detailed response shape. It intentionally exposes
// the same status-free UserRow type used by Telegram.
func (s *Service) EligibleUsers(ctx context.Context, sessionID string, actor *models.User) ([]UserRow, error) {
	if s == nil || s.db == nil || s.db.Pool == nil {
		return nil, errors.New("report service is not configured")
	}
	_, rosterSQL, args, err := s.scopedRoster(ctx, sessionID, actor)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Pool.Query(ctx, fmt.Sprintf(`
		SELECT eligible.user_id, eligible.user_name, eligible.user_rank, eligible.user_battery
		FROM (%s) AS eligible
		ORDER BY lower(eligible.user_name), eligible.user_id
	`, rosterSQL), args...)
	if err != nil {
		return nil, fmt.Errorf("list eligible attendance users: %w", err)
	}
	defer rows.Close()

	users := make([]UserRow, 0)
	for rows.Next() {
		var user UserRow
		if err := rows.Scan(&user.ID, &user.Name, &user.Rank, &user.Battery); err != nil {
			return nil, fmt.Errorf("scan eligible attendance user: %w", err)
		}
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate eligible attendance users: %w", err)
	}
	return users, nil
}

// Missing returns an authorized, name-searchable page. Telegram callers use
// this service directly, so every request is bounded to ten rows regardless of
// a larger requested page size.
func (s *Service) Missing(ctx context.Context, sessionID string, actor *models.User, query string, page, pageSize int) (MissingPage, error) {
	if s == nil || s.db == nil || s.db.Pool == nil {
		return MissingPage{}, errors.New("report service is not configured")
	}

	meta, rosterSQL, args, err := s.scopedRoster(ctx, sessionID, actor)
	if err != nil {
		return MissingPage{}, err
	}
	_ = meta // scopedRoster validates and loads the session; rows need no metadata.

	search := strings.TrimSpace(query)
	if search != "" {
		placeholder := len(args) + 1
		rosterSQL += fmt.Sprintf(` AND u."full_name" ILIKE $%d`, placeholder)
		args = append(args, "%"+search+"%")
	}

	attendanceArg := len(args) + 1
	countArgs := appendCopy(args, sessionID)
	var total int
	if err := s.db.Pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT COUNT(*)::int
		FROM (%s) AS eligible
		WHERE NOT EXISTS (
			SELECT 1 FROM attendance_record ar
			WHERE ar.session_id = $%d AND ar.user_id = eligible.user_id
		)
	`, rosterSQL, attendanceArg), countArgs...).Scan(&total); err != nil {
		return MissingPage{}, fmt.Errorf("count missing attendance users: %w", err)
	}

	if pageSize <= 0 {
		pageSize = 10
	}
	if pageSize > 10 {
		pageSize = 10
	}
	if page < 1 {
		page = 1
	}
	pageCount := 0
	if total > 0 {
		pageCount = (total + pageSize - 1) / pageSize
		if page > pageCount {
			page = pageCount
		}
	} else {
		page = 1
	}

	rows := make([]UserRow, 0)
	if total > 0 {
		rowArgs := appendCopy(args, sessionID)
		limitArg := len(rowArgs) + 1
		offsetArg := limitArg + 1
		rowArgs = append(rowArgs, pageSize, (page-1)*pageSize)
		queryRows, err := s.db.Pool.Query(ctx, fmt.Sprintf(`
			SELECT eligible.user_id, eligible.user_name, eligible.user_rank, eligible.user_battery
			FROM (%s) AS eligible
			WHERE NOT EXISTS (
				SELECT 1 FROM attendance_record ar
				WHERE ar.session_id = $%d AND ar.user_id = eligible.user_id
			)
			ORDER BY lower(eligible.user_name), eligible.user_id
			LIMIT $%d OFFSET $%d
		`, rosterSQL, attendanceArg, limitArg, offsetArg), rowArgs...)
		if err != nil {
			return MissingPage{}, fmt.Errorf("list missing attendance users: %w", err)
		}
		defer queryRows.Close()
		for queryRows.Next() {
			var row UserRow
			if err := queryRows.Scan(&row.ID, &row.Name, &row.Rank, &row.Battery); err != nil {
				return MissingPage{}, fmt.Errorf("scan missing attendance user: %w", err)
			}
			rows = append(rows, row)
		}
		if err := queryRows.Err(); err != nil {
			return MissingPage{}, fmt.Errorf("iterate missing attendance users: %w", err)
		}
	}

	return MissingPage{
		Rows:        rows,
		Page:        page,
		PageCount:   pageCount,
		Total:       total,
		HasPrevious: pageCount > 0 && page > 1,
		HasNext:     pageCount > 0 && page < pageCount,
	}, nil
}

type sessionMeta struct {
	name      string
	scope     string
	batteries []string
}

// scopedRoster returns a query whose selected columns are user_id, user_name,
// user_rank, and user_battery. It deliberately retains the table alias u so a
// caller can safely append a SQL name predicate before wrapping the query.
func (s *Service) scopedRoster(ctx context.Context, sessionID string, actor *models.User) (sessionMeta, string, []any, error) {
	var meta sessionMeta
	err := s.db.Pool.QueryRow(ctx, `
		SELECT name, scope, batteries
		FROM attendance_session
		WHERE id = $1
	`, sessionID).Scan(&meta.name, &meta.scope, &meta.batteries)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return sessionMeta{}, "", nil, fmt.Errorf("%w: %s", ErrSessionNotFound, sessionID)
		}
		return sessionMeta{}, "", nil, fmt.Errorf("load attendance session report scope: %w", err)
	}

	args := make([]any, 0, 2)
	where := []string{}
	from := `FROM "user" u`
	if meta.scope == models.SessionScopeCustomList {
		from += ` JOIN session_participants sp ON sp.user_id = u.id`
		args = append(args, sessionID)
		where = append(where, "sp.session_id = $1", `u.verified = true`)
	} else {
		where = append(where, `u."is_superadmin" = false`, `u.verified = true`)
		switch meta.scope {
		case models.SessionScopeUnitWide:
		case models.SessionScopeBatterySpecific:
			// The battery predicate is added below so Tier 2 is an
			// intersection with the session's batteries, not an authority
			// bypass when the actor is outside the event scope.
		default:
			return sessionMeta{}, "", nil, fmt.Errorf("%w: %s", ErrUnsupportedScope, meta.scope)
		}
	}

	if restricted, battery := restrictedBattery(actor); restricted {
		if battery == "" {
			where = append(where, "FALSE")
		} else if meta.scope == models.SessionScopeBatterySpecific {
			if !contains(meta.batteries, battery) {
				where = append(where, "FALSE")
			} else {
				placeholder := len(args) + 1
				where = append(where, fmt.Sprintf("u.battery = $%d", placeholder))
				args = append(args, battery)
			}
		} else {
			placeholder := len(args) + 1
			where = append(where, fmt.Sprintf("u.battery = $%d", placeholder))
			args = append(args, battery)
		}
	} else if meta.scope == models.SessionScopeBatterySpecific {
		placeholder := len(args) + 1
		where = append(where, fmt.Sprintf("u.battery = ANY($%d)", placeholder))
		args = append(args, meta.batteries)
	}

	return meta, fmt.Sprintf(`
		SELECT u.id AS user_id,
		       COALESCE(u."full_name", '') AS user_name,
		       COALESCE(u.rank, '') AS user_rank,
		       COALESCE(u.battery, '') AS user_battery
		%s
		WHERE %s
	`, from, strings.Join(where, " AND ")), args, nil
}

func restrictedBattery(actor *models.User) (bool, string) {
	if actor == nil || actor.GetTier() >= models.TierUnitCommander {
		return false, ""
	}
	if actor.Battery == nil {
		return true, ""
	}
	return true, strings.TrimSpace(*actor.Battery)
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func appendCopy(values []any, extra ...any) []any {
	result := make([]any, 0, len(values)+len(extra))
	result = append(result, values...)
	return append(result, extra...)
}
