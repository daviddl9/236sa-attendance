package handlers

import (
	"context"
	"errors"
	"fmt"
	"sort"
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
	// Check the database error before inspecting scan destinations. A failed
	// scan can leave verified at its zero value; treating that as an unpaired
	// account would hide outages and violate the actor-resolution contract.
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return telegram.AdminActor{}, false, nil
		}
		return telegram.AdminActor{}, false, err
	}
	if !verified {
		return telegram.AdminActor{}, false, nil
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
// reloadAdminActorTx resolves the Telegram pairing and current verified roster
// authority while holding the actor rows in the operation transaction. The
// supplied AdminActor is only a lookup hint; none of its role fields are
// trusted.
func (s *TelegramPairingStore) reloadAdminActorTx(ctx context.Context, tx pgx.Tx, supplied telegram.AdminActor, minimum models.AccessTier) (telegram.AdminActor, *models.User, error) {
	if s == nil || tx == nil {
		return telegram.AdminActor{}, nil, errors.New("telegram admin store is not configured")
	}
	telegramID := supplied.Pairing.TelegramID
	if telegramID == 0 {
		return telegram.AdminActor{}, nil, adminUnavailable()
	}
	var (
		currentTelegramID int64
		userID            string
		fullName          string
		rank              string
		battery           string
		tierOverride      *int16
		isSuperadmin      bool
		verified          bool
	)
	err := tx.QueryRow(ctx, `
		SELECT p.telegram_id, p.user_id,
		       COALESCE(u."full_name", ''), COALESCE(u.rank, ''),
		       COALESCE(u.battery, ''), u.tier_override,
		       u.is_superadmin, u.verified
		FROM telegram_pairing p
		JOIN "user" u ON u.id = p.user_id
		WHERE p.telegram_id = $1
		FOR UPDATE
	`, telegramID).Scan(
		&currentTelegramID, &userID, &fullName, &rank, &battery,
		&tierOverride, &isSuperadmin, &verified,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return telegram.AdminActor{}, nil, adminUnavailable()
		}
		return telegram.AdminActor{}, nil, fmt.Errorf("reload Telegram admin actor: %w", err)
	}
	if !verified || (supplied.Pairing.UserID != "" && supplied.Pairing.UserID != userID) {
		return telegram.AdminActor{}, nil, adminUnavailable()
	}
	fullNameCopy, rankCopy, batteryCopy := fullName, rank, battery
	user := &models.User{
		ID: userID, FullName: &fullNameCopy, Rank: &rankCopy, Battery: &batteryCopy,
		TierOverride: tierOverride, Verified: verified, IsSuperadmin: isSuperadmin,
	}
	actor := telegram.AdminActor{
		Pairing:  telegram.Pairing{TelegramID: currentTelegramID, UserID: userID, FullName: fullName},
		FullName: fullName, Tier: user.GetTier(), Battery: battery, IsSuperadmin: isSuperadmin,
	}
	if actor.Tier < minimum {
		return telegram.AdminActor{}, nil, adminUnavailable()
	}
	return actor, user, nil
}

func (s *TelegramPairingStore) loadAdminSessionTx(ctx context.Context, tx pgx.Tx, sessionID string, actor *models.User, requireActive bool) (models.AttendanceSession, error) {
	session, err := s.sessionService().LoadAuthorizedTx(ctx, tx, sessionID, actor, requireActive)
	if err != nil {
		return models.AttendanceSession{}, mapSessionOperationError(err)
	}
	return session, nil
}

func loadSelectedContextTx(ctx context.Context, tx pgx.Tx, telegramID int64) (string, int64, bool, error) {
	if telegramID == 0 {
		return "", 0, false, nil
	}
	var sessionID string
	var version int64
	err := tx.QueryRow(ctx, `
		SELECT COALESCE(session_id, ''), version
		FROM telegram_chat_context
		WHERE telegram_id = $1
		FOR UPDATE
	`, telegramID).Scan(&sessionID, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", 0, false, nil
	}
	if err != nil {
		return "", 0, false, fmt.Errorf("load Telegram admin context for close: %w", err)
	}
	return sessionID, version, true, nil
}

// loadAuthorizedAdminTargetTx reloads and locks the target roster row before a
// Telegram manual mutation. It applies the same participant/battery boundary
// used by reports, plus the explicit verified/non-superadmin target contract.
func loadAuthorizedAdminTargetTx(ctx context.Context, tx pgx.Tx, session models.AttendanceSession, actor *models.User, targetUserID string) (reports.UserRow, error) {
	if strings.TrimSpace(targetUserID) == "" || actor == nil {
		return reports.UserRow{}, adminUnavailable()
	}
	var row reports.UserRow
	var verified, isSuperadmin bool
	err := tx.QueryRow(ctx, `
		SELECT id, COALESCE("full_name", ''), COALESCE(rank, ''), COALESCE(battery, ''),
		       verified, is_superadmin
		FROM "user"
		WHERE id = $1
		FOR UPDATE
	`, targetUserID).Scan(&row.ID, &row.Name, &row.Rank, &row.Battery, &verified, &isSuperadmin)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return reports.UserRow{}, adminUnavailable()
		}
		return reports.UserRow{}, fmt.Errorf("load Telegram target roster row: %w", err)
	}
	if !verified || isSuperadmin {
		return reports.UserRow{}, adminUnavailable()
	}

	if actor.GetTier() < models.TierUnitCommander {
		if actor.Battery == nil || strings.TrimSpace(*actor.Battery) == "" || row.Battery != strings.TrimSpace(*actor.Battery) {
			return reports.UserRow{}, adminUnavailable()
		}
	}
	switch session.Scope {
	case models.SessionScopeUnitWide:
		return row, nil
	case models.SessionScopeBatterySpecific:
		if !containsString(session.Batteries, row.Battery) {
			return reports.UserRow{}, adminUnavailable()
		}
		return row, nil
	case models.SessionScopeCustomList:
		var participant bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM session_participants
				WHERE session_id = $1 AND user_id = $2
			)
		`, session.ID, targetUserID).Scan(&participant); err != nil {
			return reports.UserRow{}, fmt.Errorf("check Telegram custom-session participant: %w", err)
		}
		if !participant {
			return reports.UserRow{}, adminUnavailable()
		}
		return row, nil
	default:
		return reports.UserRow{}, adminUnavailable()
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

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
	return sessionservice.IsExpired(endTime, now)
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
	if err := s.adminDB(); err != nil {
		return nil, err
	}
	tx, err := s.db.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin Telegram active-event read: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	_, actor, err := s.reloadAdminActorTx(ctx, tx, supplied, models.TierBatteryNCO)
	if err != nil {
		return nil, err
	}
	sessions, err := s.sessionService().ListActiveTx(ctx, tx, actor)
	if err != nil {
		return nil, fmt.Errorf("list Telegram admin events: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit Telegram active-event read: %w", err)
	}
	events := make([]telegram.ActiveEvent, 0, len(sessions))
	for _, session := range sessions {
		events = append(events, activeEvent(session))
	}
	return events, nil
}

func activeEvent(session models.AttendanceSession) telegram.ActiveEvent {
	batteries := append([]string(nil), session.Batteries...)
	return telegram.ActiveEvent{
		ID: session.ID, Name: session.Name, Scope: session.Scope,
		Batteries: batteries, EndTime: session.EndTime, TelegramLink: session.TelegramLink,
		CreatedBy: session.CreatedBy,
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
	if err := s.adminDB(); err != nil {
		return telegram.ActiveEvent{}, err
	}
	tx, err := s.db.Pool.Begin(ctx)
	if err != nil {
		return telegram.ActiveEvent{}, fmt.Errorf("begin Telegram admin event creation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	actor, _, err := s.reloadAdminActorTx(ctx, tx, supplied, models.TierUnitCommander)
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
	session, err := s.sessionService().CreateInTx(ctx, tx, sessionservice.CreateRequest{
		Name: name, Scope: draft.Scope, Batteries: batteries,
		EndTime: &endTime, CreatedBy: actor.Pairing.UserID,
	})
	if err != nil {
		if errors.Is(err, sessionservice.ErrInvalidRequest) {
			return telegram.ActiveEvent{}, fmt.Errorf("%w: event request rejected", ErrInvalidAdminDraft)
		}
		return telegram.ActiveEvent{}, fmt.Errorf("create Telegram admin event: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return telegram.ActiveEvent{}, fmt.Errorf("commit Telegram admin event: %w", err)
	}
	return activeEvent(session), nil
}

func mapSessionOperationError(err error) error {
	if errors.Is(err, sessionservice.ErrSessionNotFound) ||
		errors.Is(err, sessionservice.ErrSessionNotOwner) ||
		errors.Is(err, sessionservice.ErrSessionClosed) ||
		errors.Is(err, sessionservice.ErrSessionExpired) ||
		errors.Is(err, sessionservice.ErrSessionUnauthorized) {
		return adminUnavailable()
	}
	return err
}

func (s *TelegramPairingStore) CloseEvent(ctx context.Context, supplied telegram.AdminActor, sessionID string) error {
	if err := s.adminDB(); err != nil {
		return err
	}

	// Capture the callback's context version before any close locks are taken.
	// The later row lock and conditional clear ensure cleanup never erases a
	// context written after this close operation began.
	contextSnapshot, err := s.LoadContext(ctx, supplied.Pairing.TelegramID)
	if err != nil {
		return err
	}

	tx, err := s.db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin Telegram event close: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// Lock the context row before actor/session authorization. The selected
	// session and version captured here remain the cleanup precondition after
	// commit, so a concurrent callback cannot replace the context unnoticed.
	selectedSessionID, selectedVersion, contextExists, err := loadSelectedContextTx(ctx, tx, supplied.Pairing.TelegramID)
	if err != nil {
		return err
	}
	cleanupContext := contextExists &&
		selectedSessionID == contextSnapshot.SessionID &&
		selectedVersion == contextSnapshot.Version &&
		selectedSessionID == sessionID
	actor, user, err := s.reloadAdminActorTx(ctx, tx, supplied, models.TierUnitCommander)
	if err != nil {
		return err
	}
	if _, err := s.loadAdminSessionTx(ctx, tx, sessionID, user, true); err != nil {
		return err
	}
	if err := s.sessionService().CloseInTx(ctx, tx, sessionID, user, true); err != nil {
		return mapSessionOperationError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit Telegram event close: %w", err)
	}

	// The close is authoritative once committed. Conditionally clear only the
	// context version observed before the close began; a conflict means another
	// callback won the context race and must be preserved.
	if cleanupContext {
		if err := s.ClearContextForSession(ctx, actor.Pairing.TelegramID, sessionID, selectedVersion); err != nil && !errors.Is(err, telegram.ErrAdminContextConflict) {
			return fmt.Errorf("clear Telegram admin context after close: %w", err)
		}
	}
	if s.hub != nil {
		s.hub.Broadcast(sessionID, sse.Event{
			Type:    sse.EventTypeSessionClosed,
			Payload: sse.SessionClosedPayload{SessionID: sessionID},
		})
	}
	return nil
}

func (s *TelegramPairingStore) Status(ctx context.Context, supplied telegram.AdminActor, sessionID, query string, page int) (telegram.AttendancePage, error) {
	if err := s.adminDB(); err != nil {
		return telegram.AttendancePage{}, err
	}
	tx, err := s.db.Pool.Begin(ctx)
	if err != nil {
		return telegram.AttendancePage{}, fmt.Errorf("begin Telegram status read: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	_, actor, err := s.reloadAdminActorTx(ctx, tx, supplied, models.TierBatteryNCO)
	if err != nil {
		return telegram.AttendancePage{}, err
	}
	if _, err := s.loadAdminSessionTx(ctx, tx, sessionID, actor, true); err != nil {
		return telegram.AttendancePage{}, err
	}

	report := s.reportService()
	eligibleRows, err := report.EligibleUsersTx(ctx, tx, sessionID, actor)
	if err != nil {
		return telegram.AttendancePage{}, mapReportOperationError(err)
	}
	if err := lockReportRosterTx(ctx, tx, eligibleRows); err != nil {
		return telegram.AttendancePage{}, err
	}
	summary, err := report.SummaryTx(ctx, tx, sessionID, actor)
	if err != nil {
		return telegram.AttendancePage{}, mapReportOperationError(err)
	}
	missing, err := report.MissingTx(ctx, tx, sessionID, actor, query, page, 10)
	if err != nil {
		return telegram.AttendancePage{}, mapReportOperationError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return telegram.AttendancePage{}, fmt.Errorf("commit Telegram status read: %w", err)
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

// lockReportRosterTx freezes the roster rows used by a status read after the
// session and actor have been authorized. Without this second lock phase, a
// roster edit between the summary and missing-list statements could produce
// counts and rows from different authority snapshots.
func lockReportRosterTx(ctx context.Context, tx pgx.Tx, rows []reports.UserRow) error {
	if len(rows) == 0 {
		return nil
	}
	ids := make([]string, 0, len(rows))
	seen := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		if row.ID == "" {
			continue
		}
		if _, ok := seen[row.ID]; ok {
			continue
		}
		seen[row.ID] = struct{}{}
		ids = append(ids, row.ID)
	}
	if len(ids) == 0 {
		return nil
	}
	sort.Strings(ids)
	lockedRows, err := tx.Query(ctx, `
		SELECT id FROM "user"
		WHERE id = ANY($1::text[])
		ORDER BY id
		FOR UPDATE
	`, ids)
	if err != nil {
		return fmt.Errorf("lock Telegram report roster: %w", err)
	}
	defer lockedRows.Close()
	for lockedRows.Next() {
		var id string
		if err := lockedRows.Scan(&id); err != nil {
			return fmt.Errorf("scan Telegram report roster lock: %w", err)
		}
	}
	if err := lockedRows.Err(); err != nil {
		return fmt.Errorf("iterate Telegram report roster locks: %w", err)
	}
	return nil
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
	if err := s.adminDB(); err != nil {
		return err
	}
	tx, err := s.db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin Telegram manual mark: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	actor, user, err := s.reloadAdminActorTx(ctx, tx, supplied, models.TierBatteryNCO)
	if err != nil {
		return err
	}
	session, err := s.loadAdminSessionTx(ctx, tx, sessionID, user, true)
	if err != nil {
		return err
	}
	target, err := loadAuthorizedAdminTargetTx(ctx, tx, session, user, targetUserID)
	if err != nil {
		return err
	}

	markedBy := actor.Pairing.UserID
	outcome, err := attendance.Mark(ctx, tx, attendance.MarkRequest{
		SessionID: sessionID, UserID: targetUserID,
		Method: models.MarkingMethodManual, MarkedBy: &markedBy,
	})
	if err != nil {
		return fmt.Errorf("mark Telegram manual attendance: %w", err)
	}
	if outcome != attendance.Marked && outcome != attendance.MarkedOutOfScope {
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

	// Counts are computed after commit with the shared report roster rules;
	// this includes complete custom-list participant sets. Target metadata is
	// retained from the locked authorization row for the SSE payload.
	s.broadcastAdminAttendanceMarked(sessionID, user, target, markedAt)
	return nil
}

func adminUserRow(row reports.UserRow) models.User {
	name, rank, battery := row.Name, row.Rank, row.Battery
	return models.User{ID: row.ID, FullName: &name, Rank: &rank, Battery: &battery, Verified: true}
}

// broadcastAdminAttendanceMarked computes post-commit counts through the
// shared report service rather than AttendanceHandler.getSessionCounts. The
// latter intentionally preserves an older dashboard query that cannot count
// custom-list participants.
func (s *TelegramPairingStore) broadcastAdminAttendanceMarked(sessionID string, actor *models.User, target reports.UserRow, markedAt time.Time) {
	if s.hub == nil {
		return
	}
	summary, err := s.reportService().Summary(context.Background(), sessionID, actor)
	if err != nil {
		return
	}
	s.hub.Broadcast(sessionID, sse.Event{
		Type: sse.EventTypeAttendanceMarked,
		Payload: sse.AttendanceMarkedPayload{
			UserID: target.ID, UserName: target.Name, UserRank: target.Rank,
			UserBattery: target.Battery, MarkingMethod: models.MarkingMethodManual,
			MarkedAt: markedAt, PresentCount: summary.Present, TotalUsers: summary.Total,
		},
	})
}

func (s *TelegramPairingStore) broadcastAdminAttendanceRemoved(sessionID string, actor *models.User, targetUserID string) {
	if s.hub == nil {
		return
	}
	summary, err := s.reportService().Summary(context.Background(), sessionID, actor)
	if err != nil {
		return
	}
	s.hub.Broadcast(sessionID, sse.Event{
		Type: sse.EventTypeAttendanceRemoved,
		Payload: sse.AttendanceRemovedPayload{
			UserID: targetUserID, PresentCount: summary.Present, TotalUsers: summary.Total,
		},
	})
}

func (s *TelegramPairingStore) OwnManualMarks(ctx context.Context, supplied telegram.AdminActor, sessionID string, page int) ([]telegram.AdminUser, error) {
	if err := s.adminDB(); err != nil {
		return nil, err
	}
	tx, err := s.db.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin Telegram own-mark read: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	_, actor, err := s.reloadAdminActorTx(ctx, tx, supplied, models.TierBatteryNCO)
	if err != nil {
		return nil, err
	}
	session, err := s.loadAdminSessionTx(ctx, tx, sessionID, actor, true)
	if err != nil {
		return nil, err
	}

	// Start from the shared report roster, then lock/recheck every candidate
	// before reading owned marks. This keeps verification, superadmin, battery,
	// and custom participant decisions in the same transaction as the read.
	eligibleRows, err := s.reportService().EligibleUsersTx(ctx, tx, sessionID, actor)
	if err != nil {
		return nil, mapReportOperationError(err)
	}
	eligible := make(map[string]telegram.AdminUser, len(eligibleRows))
	for _, candidate := range eligibleRows {
		current, authErr := loadAuthorizedAdminTargetTx(ctx, tx, session, actor, candidate.ID)
		if authErr != nil {
			if errors.Is(authErr, telegram.ErrAdminUnavailable) {
				continue
			}
			return nil, authErr
		}
		eligible[current.ID] = telegram.AdminUser{ID: current.ID, Name: current.Name, Rank: current.Rank, Battery: current.Battery}
	}

	rows, err := tx.Query(ctx, `
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
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit Telegram own-mark read: %w", err)
	}
	if page < 1 {
		page = 1
	}
	const pageSize = 10
	offset := (page - 1) * pageSize
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
	if err := s.adminDB(); err != nil {
		return err
	}
	tx, err := s.db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin Telegram manual attendance undo: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	actor, user, err := s.reloadAdminActorTx(ctx, tx, supplied, models.TierBatteryNCO)
	if err != nil {
		return err
	}
	session, err := s.loadAdminSessionTx(ctx, tx, sessionID, user, true)
	if err != nil {
		return err
	}
	if _, err := loadAuthorizedAdminTargetTx(ctx, tx, session, user, targetUserID); err != nil {
		return err
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
	s.broadcastAdminAttendanceRemoved(sessionID, user, targetUserID)
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
			clearErr := s.ClearContextForSession(ctx, telegramID, result.SessionID, result.Version)
			if errors.Is(clearErr, telegram.ErrAdminContextConflict) {
				// A concurrent callback won the conditional clear. Reload without
				// overwriting that newer draft or selected session.
				return s.LoadContext(ctx, telegramID)
			}
			if clearErr != nil {
				return telegram.AdminContext{}, clearErr
			}
			return telegram.AdminContext{TelegramID: telegramID, State: "idle", Version: result.Version + 1}, nil
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

	tx, err := s.db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin Telegram admin context save: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Probe before taking the per-chat advisory lock. If two callbacks both
	// observe a missing row, the loser is kept as a conflict even when the
	// winner commits before the loser reaches its insert statement; otherwise a
	// concurrent first save could silently become an update at version zero.
	var existedBeforeLock bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM telegram_chat_context WHERE telegram_id = $1)
	`, next.TelegramID).Scan(&existedBeforeLock); err != nil {
		return fmt.Errorf("check Telegram admin context for save: %w", err)
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, next.TelegramID); err != nil {
		return fmt.Errorf("lock Telegram admin context for save: %w", err)
	}

	var currentVersion int64
	err = tx.QueryRow(ctx, `
		SELECT version FROM telegram_chat_context WHERE telegram_id = $1 FOR UPDATE
	`, next.TelegramID).Scan(&currentVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		// A missing row represents only the initial version. A non-zero version
		// is necessarily stale and must not recreate a context after a clear or
		// an earlier callback.
		if next.Version != 0 || existedBeforeLock {
			return telegram.ErrAdminContextConflict
		}
		// Both callers that observed a missing row reach this insert path. The
		// unique key makes exactly one initial save win; the loser gets the
		// explicit optimistic conflict rather than updating the winner.
		result, insertErr := tx.Exec(ctx, `
			INSERT INTO telegram_chat_context (
				telegram_id, session_id, state, draft_name, draft_scope,
				draft_battery, draft_end_time, expires_at, version, "updatedAt"
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW())
		`, next.TelegramID, sessionID, state, draftName, draftScope, draftBattery, draftEndTime, expiresAt, next.Version+1)
		if insertErr != nil {
			return fmt.Errorf("save Telegram admin context: %w", insertErr)
		}
		if result.RowsAffected() != 1 {
			return telegram.ErrAdminContextConflict
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit Telegram admin context save: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("load Telegram admin context for save: %w", err)
	}
	// A row that appeared after the pre-lock snapshot belongs to a concurrent
	// initial save (or tombstone). Never reinterpret that race as an update to
	// version zero.
	if !existedBeforeLock {
		return telegram.ErrAdminContextConflict
	}
	if currentVersion != next.Version {
		return telegram.ErrAdminContextConflict
	}
	result, err := tx.Exec(ctx, `
		UPDATE telegram_chat_context
		SET session_id = $2, state = $3, draft_name = $4, draft_scope = $5,
		    draft_battery = $6, draft_end_time = $7, expires_at = $8,
		    version = version + 1, "updatedAt" = NOW()
		WHERE telegram_id = $1 AND version = $9
	`, next.TelegramID, sessionID, state, draftName, draftScope, draftBattery, draftEndTime, expiresAt, next.Version)
	if err != nil {
		return fmt.Errorf("save Telegram admin context: %w", err)
	}
	if result.RowsAffected() != 1 {
		return telegram.ErrAdminContextConflict
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit Telegram admin context save: %w", err)
	}
	return nil
}

// ClearContext resets a context row to an idle tombstone. It intentionally
// leaves the row in place and advances version so stale callbacks cannot
// recreate a prior draft or selected event.
func (s *TelegramPairingStore) ClearContext(ctx context.Context, telegramID int64) error {
	if err := s.adminDB(); err != nil {
		return err
	}
	if telegramID == 0 {
		return errors.New("telegram admin context requires a Telegram account")
	}
	tx, err := s.db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin Telegram admin context clear: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, telegramID); err != nil {
		return fmt.Errorf("lock Telegram admin context for clear: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO telegram_chat_context (
			telegram_id, session_id, state, draft_name, draft_scope,
			draft_battery, draft_end_time, expires_at, version, "updatedAt"
		)
		VALUES ($1, NULL, 'idle', NULL, NULL, NULL, NULL, NULL, 1, NOW())
		ON CONFLICT (telegram_id) DO UPDATE SET
			session_id = NULL, state = 'idle', draft_name = NULL,
			draft_scope = NULL, draft_battery = NULL, draft_end_time = NULL,
			expires_at = NULL, version = telegram_chat_context.version + 1,
			"updatedAt" = NOW()
	`, telegramID); err != nil {
		return fmt.Errorf("clear Telegram admin context: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit Telegram admin context clear: %w", err)
	}
	return nil
}

// ClearContextForSession conditionally resets only the selected session and
// optimistic version observed by the caller. Any mismatch is a conflict and
// leaves the newer draft/selection untouched.
func (s *TelegramPairingStore) ClearContextForSession(ctx context.Context, telegramID int64, sessionID string, expectedVersion int64) error {
	if err := s.adminDB(); err != nil {
		return err
	}
	if telegramID == 0 || strings.TrimSpace(sessionID) == "" {
		return telegram.ErrAdminContextConflict
	}
	tx, err := s.db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin Telegram admin session context clear: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, telegramID); err != nil {
		return fmt.Errorf("lock Telegram admin session context clear: %w", err)
	}
	result, err := tx.Exec(ctx, `
		UPDATE telegram_chat_context
		SET session_id = NULL, state = 'idle', draft_name = NULL,
		    draft_scope = NULL, draft_battery = NULL, draft_end_time = NULL,
		    expires_at = NULL, version = version + 1, "updatedAt" = NOW()
		WHERE telegram_id = $1 AND session_id = $2 AND version = $3
	`, telegramID, sessionID, expectedVersion)
	if err != nil {
		return fmt.Errorf("clear Telegram admin context for session: %w", err)
	}
	if result.RowsAffected() != 1 {
		return telegram.ErrAdminContextConflict
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit Telegram admin session context clear: %w", err)
	}
	return nil
}
