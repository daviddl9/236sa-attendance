package handlers

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/davidlivingston/go-nextjs-starter/backend/internal/database"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/models"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/services/attendance"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/services/reports"
	sessionservice "github.com/davidlivingston/go-nextjs-starter/backend/internal/services/sessions"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/sse"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/telegram"
	"github.com/jackc/pgx/v5"
)

var (
	ErrInvalidAdminDraft            = errors.New("invalid Telegram admin event draft")
	ErrAdminUnavailable             = telegram.ErrAdminUnavailable
	ErrTelegramAdminUnavailable     = telegram.ErrAdminUnavailable
	ErrAdminContextConflict         = telegram.ErrAdminContextConflict
	ErrTelegramAdminContextConflict = telegram.ErrAdminContextConflict
)

// TelegramAdminStore is the PostgreSQL adapter for telegram.AdminStore. It
// intentionally shares the existing TelegramPairingStore so the pairing,
// soldier attendance, and admin paths all use one database capability and one
// optional SSE hub. The alias below keeps both names available to callers that
// construct the admin adapter directly.
type TelegramAdminStore = TelegramPairingStore

// NewTelegramAdminStore constructs the admin adapter. The optional hub is
// variadic for compatibility with deployments that do not expose SSE.
func NewTelegramAdminStore(db *database.DB, hubs ...*sse.Hub) *TelegramAdminStore {
	var hub *sse.Hub
	if len(hubs) > 0 {
		hub = hubs[0]
	}
	return NewTelegramAdminStoreWithHub(db, hub)
}

func NewTelegramAdminStoreWithHub(db *database.DB, hub *sse.Hub) *TelegramAdminStore {
	config := telegram.LoadConfig()
	return &TelegramAdminStore{db: db, hub: hub, botUsername: config.BotUsername}
}

func (s *TelegramPairingStore) adminDB() error {
	if s == nil || s.db == nil || s.db.Pool == nil {
		return errors.New("telegram admin store is not configured")
	}
	return nil
}

// Actor resolves a pairing against the current verified roster row. The
// display name, rank-derived tier, override, battery, and superadmin flag are
// all loaded here; values in Pairing are treated only as lookup hints.
func (s *TelegramPairingStore) Actor(ctx context.Context, pairing telegram.Pairing) (telegram.AdminActor, bool, error) {
	if err := s.adminDB(); err != nil {
		return telegram.AdminActor{}, false, err
	}
	if pairing.TelegramID == 0 {
		return telegram.AdminActor{}, false, nil
	}

	var (
		telegramID   int64
		userID       string
		fullName     string
		rank         string
		battery      string
		tierOverride *int16
		isSuperadmin bool
		verified     bool
	)
	err := s.db.Pool.QueryRow(ctx, `
		SELECT p.telegram_id, p.user_id,
		       COALESCE(u."full_name", ''), COALESCE(u.rank, ''),
		       COALESCE(u.battery, ''), u.tier_override,
		       u.is_superadmin, u.verified
		FROM telegram_pairing p
		JOIN "user" u ON u.id = p.user_id
		WHERE p.telegram_id = $1
	`, pairing.TelegramID).Scan(
		&telegramID, &userID, &fullName, &rank, &battery,
		&tierOverride, &isSuperadmin, &verified,
	)
	if errors.Is(err, pgx.ErrNoRows) || !verified {
		return telegram.AdminActor{}, false, nil
	}
	if err != nil {
		return telegram.AdminActor{}, false, err
	}
	// A pairing returned by the authenticated lookup must continue to refer to
	// the same roster row. This also prevents a caller from mixing a Telegram
	// capability with an arbitrary user ID.
	if pairing.UserID != "" && pairing.UserID != userID {
		return telegram.AdminActor{}, false, nil
	}

	user := &models.User{
		ID: userID, FullName: &fullName, Rank: &rank, Battery: &battery,
		TierOverride: tierOverride, Verified: verified, IsSuperadmin: isSuperadmin,
	}
	return telegram.AdminActor{
		Pairing:  telegram.Pairing{TelegramID: telegramID, UserID: userID, FullName: fullName},
		FullName: fullName, Tier: user.GetTier(), Battery: battery, IsSuperadmin: isSuperadmin,
	}, true, nil
}

// reloadAdminActor deliberately resolves the pairing before every operation.
// The Telegram callback can outlive a roster edit, so a stale AdminActor must
// never retain authority after its tier, battery, or verification changes.
func (s *TelegramPairingStore) reloadAdminActor(ctx context.Context, supplied telegram.AdminActor, minimum models.AccessTier) (telegram.AdminActor, *models.User, error) {
	actor, found, err := s.Actor(ctx, supplied.Pairing)
	if err != nil {
		return telegram.AdminActor{}, nil, err
	}
	if !found || actor.Tier < minimum || strings.TrimSpace(actor.Pairing.UserID) == "" {
		return telegram.AdminActor{}, nil, adminUnavailable()
	}
	return actor, adminUser(actor), nil
}

// adminUser creates the model shape required by the shared services. The
// effective tier has already been calculated from the current roster row, so a
// minimal rank/tier representation is sufficient for those services' scope
// predicates while avoiding a second authorization query.
func adminUser(actor telegram.AdminActor) *models.User {
	fullName := actor.FullName
	battery := actor.Battery
	rank := models.RankPTE
	switch {
	case actor.IsSuperadmin || actor.Tier >= models.TierSuperadmin:
		rank = models.RankCPT
	case actor.Tier >= models.TierUnitCommander:
		rank = models.RankSSG
	case actor.Tier >= models.TierBatteryNCO:
		rank = models.Rank3SG
	}
	return &models.User{
		ID: actor.Pairing.UserID, FullName: &fullName, Rank: &rank, Battery: &battery,
		Verified: true, IsSuperadmin: actor.IsSuperadmin || actor.Tier >= models.TierSuperadmin,
	}
}

func (s *TelegramPairingStore) sessionService() *sessionservice.Service {
	username := s.botUsername
	if strings.TrimSpace(username) == "" {
		username = telegram.LoadConfig().BotUsername
	}
	return sessionservice.NewService(s.db, username)
}

func (s *TelegramPairingStore) reportService() *reports.Service {
	return reports.NewService(s.db)
}

// adminUnavailable intentionally ignores all lookup details. Callers use the
// same error for an unknown, unauthorized, closed, or expired resource.
func adminUnavailable(_ ...string) error {
	return telegram.ErrAdminUnavailable
}

func isExpired(endTime *time.Time, now time.Time) bool {
	if endTime == nil {
		return false
	}
	// The schema intentionally uses PostgreSQL TIMESTAMP (without time zone).
	// pgx scans that value with UTC while callers commonly write a local
	// wall-clock time, so compare wall-clock components in the caller's
	// location rather than treating the scan location as an instant.
	normalized := time.Date(
		endTime.Year(), endTime.Month(), endTime.Day(),
		endTime.Hour(), endTime.Minute(), endTime.Second(), endTime.Nanosecond(),
		now.Location(),
	)
	// Prefer the interpretation nearest to now. This handles both values
	// written by the application (which preserve the caller's wall clock) and
	// values produced by PostgreSQL expressions such as NOW() + interval.
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

func activeSessionForActor(ctx context.Context, service *sessionservice.Service, actor *models.User, sessionID string) (models.AttendanceSession, error) {
	if strings.TrimSpace(sessionID) == "" {
		return models.AttendanceSession{}, adminUnavailable()
	}
	sessions, err := service.ListActive(ctx, actor)
	if err != nil {
		return models.AttendanceSession{}, err
	}
	now := time.Now()
	for _, session := range sessions {
		if session.ID == sessionID && !isExpired(session.EndTime, now) {
			return session, nil
		}
	}
	return models.AttendanceSession{}, adminUnavailable()
}

func (s *TelegramPairingStore) ActiveEvents(ctx context.Context, supplied telegram.AdminActor) ([]telegram.ActiveEvent, error) {
	_, actor, err := s.reloadAdminActor(ctx, supplied, models.TierBatteryNCO)
	if err != nil {
		return nil, err
	}
	sessions, err := s.sessionService().ListActive(ctx, actor)
	if err != nil {
		return nil, fmt.Errorf("list Telegram admin events: %w", err)
	}
	now := time.Now()
	events := make([]telegram.ActiveEvent, 0, len(sessions))
	for _, session := range sessions {
		if isExpired(session.EndTime, now) {
			continue
		}
		events = append(events, activeEvent(session))
	}
	return events, nil
}

func activeEvent(session models.AttendanceSession) telegram.ActiveEvent {
	batteries := append([]string(nil), session.Batteries...)
	return telegram.ActiveEvent{
		ID: session.ID, Name: session.Name, Scope: session.Scope,
		Batteries: batteries, EndTime: session.EndTime, TelegramLink: session.TelegramLink,
	}
}

func validateAdminDraft(draft telegram.AdminDraft) error {
	name := strings.TrimSpace(draft.Name)
	if name == "" || len([]rune(name)) > 80 {
		return fmt.Errorf("%w: event name must be between 1 and 80 characters", ErrInvalidAdminDraft)
	}
	if draft.EndTime.IsZero() || !draft.EndTime.After(time.Now()) {
		return fmt.Errorf("%w: event end time must be in the future", ErrInvalidAdminDraft)
	}
	switch draft.Scope {
	case models.SessionScopeUnitWide:
		return nil
	case models.SessionScopeBatterySpecific:
		if !validAdminBattery(draft.Battery) {
			return fmt.Errorf("%w: battery is required", ErrInvalidAdminDraft)
		}
		return nil
	default:
		return fmt.Errorf("%w: unsupported event scope", ErrInvalidAdminDraft)
	}
}

func validAdminBattery(battery string) bool {
	switch battery {
	case models.BatteryHQ, models.BatteryAlpha, models.BatteryBravo:
		return true
	default:
		return false
	}
}

func (s *TelegramPairingStore) CreateEvent(ctx context.Context, supplied telegram.AdminActor, draft telegram.AdminDraft) (telegram.ActiveEvent, error) {
	actor, _, err := s.reloadAdminActor(ctx, supplied, models.TierUnitCommander)
	if err != nil {
		return telegram.ActiveEvent{}, err
	}
	if err := validateAdminDraft(draft); err != nil {
		return telegram.ActiveEvent{}, err
	}

	name := strings.TrimSpace(draft.Name)
	batteries := []string(nil)
	if draft.Scope == models.SessionScopeBatterySpecific {
		batteries = []string{draft.Battery}
	}
	endTime := draft.EndTime
	session, err := s.sessionService().Create(ctx, sessionservice.CreateRequest{
		Name: name, Scope: draft.Scope, Batteries: batteries,
		EndTime: &endTime, CreatedBy: actor.Pairing.UserID,
	})
	if err != nil {
		if errors.Is(err, sessionservice.ErrInvalidRequest) {
			return telegram.ActiveEvent{}, fmt.Errorf("%w: event request rejected", ErrInvalidAdminDraft)
		}
		return telegram.ActiveEvent{}, fmt.Errorf("create Telegram admin event: %w", err)
	}
	return activeEvent(session), nil
}

func mapSessionOperationError(err error) error {
	if errors.Is(err, sessionservice.ErrSessionNotFound) ||
		errors.Is(err, sessionservice.ErrSessionNotOwner) ||
		errors.Is(err, sessionservice.ErrSessionClosed) {
		return adminUnavailable()
	}
	return err
}

func (s *TelegramPairingStore) CloseEvent(ctx context.Context, supplied telegram.AdminActor, sessionID string) error {
	actor, user, err := s.reloadAdminActor(ctx, supplied, models.TierUnitCommander)
	if err != nil {
		return err
	}
	if _, err := activeSessionForActor(ctx, s.sessionService(), user, sessionID); err != nil {
		return mapSessionOperationError(err)
	}
	if err := s.sessionService().Close(ctx, sessionID, user); err != nil {
		return mapSessionOperationError(err)
	}

	// Closing commits in the shared service before the selected Telegram
	// context is cleared. Broadcast even if the best-effort cleanup encounters
	// a separate database failure: the session close itself is authoritative.
	clearErr := s.ClearContext(ctx, actor.Pairing.TelegramID)
	if s.hub != nil {
		s.hub.Broadcast(sessionID, sse.Event{
			Type:    sse.EventTypeSessionClosed,
			Payload: sse.SessionClosedPayload{SessionID: sessionID},
		})
	}
	if clearErr != nil {
		return fmt.Errorf("clear Telegram admin context after close: %w", clearErr)
	}
	return nil
}

func (s *TelegramPairingStore) Status(ctx context.Context, supplied telegram.AdminActor, sessionID, query string, page int) (telegram.AttendancePage, error) {
	_, actor, err := s.reloadAdminActor(ctx, supplied, models.TierBatteryNCO)
	if err != nil {
		return telegram.AttendancePage{}, err
	}
	if _, err := activeSessionForActor(ctx, s.sessionService(), actor, sessionID); err != nil {
		return telegram.AttendancePage{}, mapSessionOperationError(err)
	}

	report := s.reportService()
	summary, err := report.Summary(ctx, sessionID, actor)
	if err != nil {
		return telegram.AttendancePage{}, mapReportOperationError(err)
	}
	missing, err := report.Missing(ctx, sessionID, actor, query, page, 10)
	if err != nil {
		return telegram.AttendancePage{}, mapReportOperationError(err)
	}
	rows := make([]telegram.AdminUser, 0, len(missing.Rows))
	for _, row := range missing.Rows {
		rows = append(rows, telegram.AdminUser{ID: row.ID, Name: row.Name, Rank: row.Rank, Battery: row.Battery})
	}
	return telegram.AttendancePage{
		SessionName: summary.SessionName, Total: summary.Total, Present: summary.Present,
		Missing: summary.Missing, Percentage: summary.Percentage, Rows: rows,
		Page: missing.Page, PageCount: missing.PageCount,
		HasPrevious: missing.HasPrevious, HasNext: missing.HasNext,
	}, nil
}

func mapReportOperationError(err error) error {
	if errors.Is(err, reports.ErrSessionNotFound) || errors.Is(err, reports.ErrUnsupportedScope) {
		return adminUnavailable()
	}
	return err
}

func (s *TelegramPairingStore) eligibleAdminUsers(ctx context.Context, sessionID string, actor *models.User) (map[string]reports.UserRow, error) {
	rows, err := s.reportService().EligibleUsers(ctx, sessionID, actor)
	if err != nil {
		return nil, mapReportOperationError(err)
	}
	eligible := make(map[string]reports.UserRow, len(rows))
	for _, row := range rows {
		eligible[row.ID] = row
	}
	return eligible, nil
}

func (s *TelegramPairingStore) MarkManual(ctx context.Context, supplied telegram.AdminActor, sessionID, targetUserID string) error {
	actor, user, err := s.reloadAdminActor(ctx, supplied, models.TierBatteryNCO)
	if err != nil {
		return err
	}
	if _, err := activeSessionForActor(ctx, s.sessionService(), user, sessionID); err != nil {
		return mapSessionOperationError(err)
	}
	eligible, err := s.eligibleAdminUsers(ctx, sessionID, user)
	if err != nil {
		return err
	}
	target, ok := eligible[targetUserID]
	if !ok || strings.TrimSpace(targetUserID) == "" {
		return adminUnavailable()
	}

	tx, err := s.db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin Telegram manual mark: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// Recheck the target's current roster state under the write transaction so
	// a verification or superadmin edit between the report query and this
	// mutation cannot widen the mark authority.
	var verified, isSuperadmin bool
	if err := tx.QueryRow(ctx, `
		SELECT verified, is_superadmin FROM "user" WHERE id = $1 FOR SHARE
	`, targetUserID).Scan(&verified, &isSuperadmin); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return adminUnavailable()
		}
		return fmt.Errorf("load Telegram manual-mark target: %w", err)
	}
	if !verified || isSuperadmin {
		return adminUnavailable()
	}

	markedBy := actor.Pairing.UserID
	outcome, err := attendance.Mark(ctx, tx, attendance.MarkRequest{
		SessionID: sessionID, UserID: targetUserID,
		Method: models.MarkingMethodManual, MarkedBy: &markedBy,
	})
	if err != nil {
		return fmt.Errorf("mark Telegram manual attendance: %w", err)
	}
	if outcome != attendance.Marked {
		return adminUnavailable()
	}
	var markedAt time.Time
	if err := tx.QueryRow(ctx, `
		SELECT marked_at FROM attendance_record WHERE session_id = $1 AND user_id = $2
	`, sessionID, targetUserID).Scan(&markedAt); err != nil {
		return fmt.Errorf("read Telegram manual attendance mark: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit Telegram manual attendance mark: %w", err)
	}

	if s.hub != nil {
		targetModel := adminUserRow(target)
		(&AttendanceHandler{db: s.db, hub: s.hub}).broadcastAttendanceMarked(
			context.Background(), sessionID, &targetModel, models.MarkingMethodManual, markedAt,
		)
	}
	return nil
}

func adminUserRow(row reports.UserRow) models.User {
	name, rank, battery := row.Name, row.Rank, row.Battery
	return models.User{ID: row.ID, FullName: &name, Rank: &rank, Battery: &battery, Verified: true}
}

func (s *TelegramPairingStore) OwnManualMarks(ctx context.Context, supplied telegram.AdminActor, sessionID string, page int) ([]telegram.AdminUser, error) {
	_, actor, err := s.reloadAdminActor(ctx, supplied, models.TierBatteryNCO)
	if err != nil {
		return nil, err
	}
	if _, err := activeSessionForActor(ctx, s.sessionService(), actor, sessionID); err != nil {
		return nil, mapSessionOperationError(err)
	}
	eligible, err := s.eligibleAdminUsers(ctx, sessionID, actor)
	if err != nil {
		return nil, err
	}
	if page < 1 {
		page = 1
	}
	const pageSize = 10
	offset := (page - 1) * pageSize
	rows, err := s.db.Pool.Query(ctx, `
		SELECT u.id, COALESCE(u."full_name", ''), COALESCE(u.rank, ''), COALESCE(u.battery, '')
		FROM attendance_record ar
		JOIN "user" u ON u.id = ar.user_id
		WHERE ar.session_id = $1
		  AND ar.marking_method = $2
		  AND ar.marked_by = $3
		ORDER BY ar.marked_at DESC, u.id ASC
	`, sessionID, models.MarkingMethodManual, actor.ID)
	if err != nil {
		return nil, fmt.Errorf("list Telegram manual attendance marks: %w", err)
	}
	defer rows.Close()
	allMarks := make([]telegram.AdminUser, 0)
	for rows.Next() {
		var row telegram.AdminUser
		if err := rows.Scan(&row.ID, &row.Name, &row.Rank, &row.Battery); err != nil {
			return nil, fmt.Errorf("scan Telegram manual attendance mark: %w", err)
		}
		if _, ok := eligible[row.ID]; ok {
			allMarks = append(allMarks, row)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate Telegram manual attendance marks: %w", err)
	}
	if offset >= len(allMarks) {
		return []telegram.AdminUser{}, nil
	}
	end := offset + pageSize
	if end > len(allMarks) {
		end = len(allMarks)
	}
	return allMarks[offset:end], nil
}

func (s *TelegramPairingStore) UndoManual(ctx context.Context, supplied telegram.AdminActor, sessionID, targetUserID string) error {
	actor, user, err := s.reloadAdminActor(ctx, supplied, models.TierBatteryNCO)
	if err != nil {
		return err
	}
	if _, err := activeSessionForActor(ctx, s.sessionService(), user, sessionID); err != nil {
		return mapSessionOperationError(err)
	}
	eligible, err := s.eligibleAdminUsers(ctx, sessionID, user)
	if err != nil {
		return err
	}
	if _, ok := eligible[targetUserID]; !ok || strings.TrimSpace(targetUserID) == "" {
		return adminUnavailable()
	}

	tx, err := s.db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin Telegram manual attendance undo: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var status string
	var endTime *time.Time
	if err := tx.QueryRow(ctx, `
		SELECT status, end_time FROM attendance_session WHERE id = $1 FOR SHARE
	`, sessionID).Scan(&status, &endTime); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return adminUnavailable()
		}
		return fmt.Errorf("load Telegram session for undo: %w", err)
	}
	if status != models.SessionStatusActive || isExpired(endTime, time.Now()) {
		return adminUnavailable()
	}

	outcome, err := attendance.UndoManual(ctx, tx, attendance.UndoRequest{
		SessionID: sessionID, UserID: targetUserID, MarkedBy: actor.Pairing.UserID,
	})
	if err != nil {
		return fmt.Errorf("undo Telegram manual attendance: %w", err)
	}
	if outcome != attendance.Undone {
		return adminUnavailable()
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit Telegram manual attendance undo: %w", err)
	}
	if s.hub != nil {
		(&AttendanceHandler{db: s.db, hub: s.hub}).broadcastAttendanceRemoved(
			context.Background(), sessionID, targetUserID,
		)
	}
	return nil
}

func (s *TelegramPairingStore) LoadContext(ctx context.Context, telegramID int64) (telegram.AdminContext, error) {
	if err := s.adminDB(); err != nil {
		return telegram.AdminContext{}, err
	}
	if telegramID == 0 {
		return telegram.AdminContext{}, errors.New("telegram admin context requires a Telegram account")
	}

	var result telegram.AdminContext
	var sessionID, state, draftName, draftScope, draftBattery string
	var draftEndTime, expiresAt *time.Time
	err := s.db.Pool.QueryRow(ctx, `
		SELECT COALESCE(session_id, ''), state,
		       COALESCE(draft_name, ''), COALESCE(draft_scope, ''),
		       COALESCE(draft_battery, ''), draft_end_time, expires_at, version
		FROM telegram_chat_context WHERE telegram_id = $1
	`, telegramID).Scan(
		&sessionID, &state, &draftName, &draftScope, &draftBattery,
		&draftEndTime, &expiresAt, &result.Version,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return telegram.AdminContext{TelegramID: telegramID, State: "idle"}, nil
	}
	if err != nil {
		return telegram.AdminContext{}, fmt.Errorf("load Telegram admin context: %w", err)
	}
	result.TelegramID = telegramID
	result.SessionID = sessionID
	result.State = state
	if result.State == "" {
		result.State = "idle"
	}
	result.Draft = telegram.AdminDraft{Name: draftName, Scope: draftScope, Battery: draftBattery}
	if draftEndTime != nil {
		result.Draft.EndTime = *draftEndTime
	}
	result.ExpiresAt = expiresAt

	if result.SessionID != "" {
		var status string
		var endTime *time.Time
		checkErr := s.db.Pool.QueryRow(ctx, `
			SELECT status, end_time FROM attendance_session WHERE id = $1
		`, result.SessionID).Scan(&status, &endTime)
		if errors.Is(checkErr, pgx.ErrNoRows) || checkErr == nil && (status != models.SessionStatusActive || isExpired(endTime, time.Now())) {
			if clearErr := s.ClearContext(ctx, telegramID); clearErr != nil {
				return telegram.AdminContext{}, clearErr
			}
			return telegram.AdminContext{TelegramID: telegramID, State: "idle"}, nil
		}
		if checkErr != nil {
			return telegram.AdminContext{}, fmt.Errorf("check Telegram selected session: %w", checkErr)
		}
	}

	// Draft expiration is intentionally applied only to contexts without a
	// selected session. A selected event has its own end-time availability and
	// must not be confused with the short-lived wizard draft timeout.
	if result.SessionID == "" && isExpired(result.ExpiresAt, time.Now()) && result.State != "idle" {
		updated, clearErr := s.db.Pool.Exec(ctx, `
			UPDATE telegram_chat_context
			SET state = 'idle', session_id = NULL,
			    draft_name = NULL, draft_scope = NULL, draft_battery = NULL,
			    draft_end_time = NULL, expires_at = NULL,
			    version = version + 1, "updatedAt" = NOW()
			WHERE telegram_id = $1 AND version = $2
		`, telegramID, result.Version)
		if clearErr != nil {
			return telegram.AdminContext{}, fmt.Errorf("expire Telegram admin draft: %w", clearErr)
		}
		if updated.RowsAffected() == 1 {
			return telegram.AdminContext{TelegramID: telegramID, State: "idle", Version: result.Version + 1}, nil
		}
		// A concurrent callback won the optimistic write. Reloading gives the
		// caller the current state without overwriting that newer draft.
		return s.LoadContext(ctx, telegramID)
	}
	return result, nil
}

func (s *TelegramPairingStore) SaveContext(ctx context.Context, next telegram.AdminContext) error {
	if err := s.adminDB(); err != nil {
		return err
	}
	if next.TelegramID == 0 {
		return errors.New("telegram admin context requires a Telegram account")
	}
	state := strings.TrimSpace(next.State)
	if state == "" {
		state = "idle"
	}
	var sessionID, draftName, draftScope, draftBattery, draftEndTime, expiresAt any
	if strings.TrimSpace(next.SessionID) != "" {
		sessionID = strings.TrimSpace(next.SessionID)
	}
	if strings.TrimSpace(next.Draft.Name) != "" {
		draftName = next.Draft.Name
	}
	if strings.TrimSpace(next.Draft.Scope) != "" {
		draftScope = next.Draft.Scope
	}
	if strings.TrimSpace(next.Draft.Battery) != "" {
		draftBattery = next.Draft.Battery
	}
	if !next.Draft.EndTime.IsZero() {
		draftEndTime = next.Draft.EndTime
	}
	if next.ExpiresAt != nil {
		expiresAt = *next.ExpiresAt
	}

	result, err := s.db.Pool.Exec(ctx, `
		INSERT INTO telegram_chat_context (
			telegram_id, session_id, state, draft_name, draft_scope,
			draft_battery, draft_end_time, expires_at, version, "updatedAt"
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW())
		ON CONFLICT (telegram_id) DO UPDATE SET
			session_id = EXCLUDED.session_id,
			state = EXCLUDED.state,
			draft_name = EXCLUDED.draft_name,
			draft_scope = EXCLUDED.draft_scope,
			draft_battery = EXCLUDED.draft_battery,
			draft_end_time = EXCLUDED.draft_end_time,
			expires_at = EXCLUDED.expires_at,
			version = telegram_chat_context.version + 1,
			"updatedAt" = NOW()
		WHERE telegram_chat_context.version = EXCLUDED.version
	`, next.TelegramID, sessionID, state, draftName, draftScope, draftBattery, draftEndTime, expiresAt, next.Version)
	if err != nil {
		return fmt.Errorf("save Telegram admin context: %w", err)
	}
	if result.RowsAffected() == 0 {
		return telegram.ErrAdminContextConflict
	}
	return nil
}

func (s *TelegramPairingStore) ClearContext(ctx context.Context, telegramID int64) error {
	if err := s.adminDB(); err != nil {
		return err
	}
	if telegramID == 0 {
		return errors.New("telegram admin context requires a Telegram account")
	}
	if _, err := s.db.Pool.Exec(ctx, `DELETE FROM telegram_chat_context WHERE telegram_id = $1`, telegramID); err != nil {
		return fmt.Errorf("clear Telegram admin context: %w", err)
	}
	return nil
}
