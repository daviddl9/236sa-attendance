package telegram

import (
	"context"
	"errors"
	"time"

	"github.com/davidlivingston/go-nextjs-starter/backend/internal/models"
)

// AdminActor is the current, database-backed authority associated with one
// confirmed Telegram pairing. Pairing remains the authenticated capability;
// the remaining fields are reloaded from the verified roster for every
// operation and must not be treated as durable authorization claims.
type AdminActor struct {
	Pairing      Pairing
	FullName     string
	Tier         models.AccessTier
	Battery      string
	IsSuperadmin bool
}

type AdminDraft struct {
	Name    string
	Scope   string
	Battery string
	EndTime time.Time
}

type AdminContext struct {
	TelegramID int64
	SessionID  string
	State      string
	Draft      AdminDraft
	Version    int64
	ExpiresAt  *time.Time
}

type ActiveEvent struct {
	ID           string
	Name         string
	Scope        string
	Batteries    []string
	EndTime      *time.Time
	TelegramLink string
	// CreatedBy is optional in the Telegram projection. The PostgreSQL store
	// populates it for local ownership presentation; CloseEvent remains the
	// authoritative recheck for every close mutation.
	CreatedBy string
}

type AttendancePage struct {
	SessionName string
	Total       int
	Present     int
	Missing     int
	// Extras counts walk-ins (marked users outside the roster), shown as a
	// "+N walk-ins" suffix so Present/Total stay anchored to the roster.
	Extras      int
	Percentage  float64
	Rows        []AdminUser
	Page        int
	PageCount   int
	HasPrevious bool
	HasNext     bool
}

type AdminUser struct {
	ID      string
	Name    string
	Rank    string
	Battery string
}

// ErrAdminUnavailable is intentionally deliberately vague. Telegram callers
// use it for missing, closed, expired, and unauthorized sessions or people so
// an account cannot probe roster or attendance existence.
var ErrAdminUnavailable = errors.New("telegram admin resource unavailable")

// ErrAdminContextConflict indicates that a context write used an older
// optimistic version than the row currently stored for the chat.
var ErrAdminContextConflict = errors.New("telegram admin context version conflict")

// ErrContextConflict is kept as a short alias for adapters that expose the
// context persistence primitive directly.
var ErrContextConflict = ErrAdminContextConflict

// AdminStore is the database boundary used by the Telegram admin flow. It is
// intentionally independent of Telegram transport types so the wizard can be
// tested with an in-memory fake in a later task.
type AdminStore interface {
	Actor(ctx context.Context, pairing Pairing) (AdminActor, bool, error)
	LoadContext(ctx context.Context, telegramID int64) (AdminContext, error)
	SaveContext(ctx context.Context, next AdminContext) error
	ClearContext(ctx context.Context, telegramID int64) error
	ClearContextForSession(ctx context.Context, telegramID int64, sessionID string, expectedVersion int64) error
	ActiveEvents(ctx context.Context, actor AdminActor) ([]ActiveEvent, error)
	CreateEvent(ctx context.Context, actor AdminActor, draft AdminDraft) (ActiveEvent, error)
	CloseEvent(ctx context.Context, actor AdminActor, sessionID string) error
	Status(ctx context.Context, actor AdminActor, sessionID, query string, page int) (AttendancePage, error)
	MarkManual(ctx context.Context, actor AdminActor, sessionID, targetUserID string) error
	OwnManualMarks(ctx context.Context, actor AdminActor, sessionID string, page int) ([]AdminUser, error)
	UndoManual(ctx context.Context, actor AdminActor, sessionID, targetUserID string) error
}
