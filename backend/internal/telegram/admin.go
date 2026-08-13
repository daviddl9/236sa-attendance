package telegram

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/davidlivingston/go-nextjs-starter/backend/internal/models"
)

const (
	adminStateIdle             = "idle"
	adminStateCreatingName     = "creating_name"
	adminStateChoosingScope    = "choosing_scope"
	adminStateChoosingBattery  = "choosing_battery"
	adminStateChoosingDuration = "choosing_duration"
	adminStateConfirmingCreate = "confirming_creation"
	adminStateCreating         = "creating"
	adminStateSelected         = "selected_session"
	adminStateViewingStatus    = "viewing_status"
	adminStateSearchingName    = "searching_name"
	adminStateConfirmingMark   = "confirming_mark"
	adminStateOwnMarks         = "reviewing_own_marks"
	adminStateConfirmingClose  = "confirming_close"

	adminDraftTTL = 15 * time.Minute
	adminPageSize = 10
)

const (
	adminDuration30m = "30m"
	adminDuration1h  = "1h"
	adminDuration2h  = "2h"
	adminDuration4h  = "4h"
)

// AdminFlow is the Telegram-facing adapter for the commander UI. Pairing is
// passed by Bot only after the update has been authenticated against the
// Telegram account, so implementations must never accept an actor identifier
// from callback data.
type AdminFlow interface {
	HandleMessage(ctx context.Context, message *Message, pairing Pairing) (actions []Action, handled bool, err error)
	HandleCallback(ctx context.Context, query *CallbackQuery, pairing Pairing) (actions []Action, handled bool, err error)
}

// AdminRouter implements the small, persistent Telegram commander state
// machine. All authority and resource lookups are delegated to AdminStore;
// callback data contains only bounded lookup hints.
type AdminRouter struct {
	store       AdminStore
	botUsername string
	now         func() time.Time
}

func NewAdminRouter(store AdminStore) *AdminRouter {
	return &AdminRouter{store: store, botUsername: LoadConfig().BotUsername, now: time.Now}
}

func (r *AdminRouter) HandleMessage(ctx context.Context, message *Message, pairing Pairing) ([]Action, bool, error) {
	if r == nil || r.store == nil || message == nil || message.Chat.Type != "private" || message.Chat.ID == 0 {
		return nil, false, nil
	}
	if message.From != nil && message.From.ID != 0 && pairing.TelegramID != 0 && message.From.ID != pairing.TelegramID {
		return nil, true, nil
	}
	actor, eligible, err := r.actor(ctx, pairing)
	if err != nil || !eligible {
		return nil, false, err
	}
	pairing = pairingForAdmin(pairing, actor, message.From)

	command := telegramCommand(message.Text)
	switch command {
	case "/menu":
		return r.menuAction(message.Chat.ID, actor), true, nil
	case "/cancel":
		if err := r.clearContext(ctx, pairing.TelegramID); err != nil {
			return nil, true, err
		}
		return r.menuAction(message.Chat.ID, actor), true, nil
	}

	adminContext, err := r.loadContext(ctx, pairing.TelegramID)
	if err != nil {
		return nil, true, err
	}
	if adminContext.ExpiresAt != nil && r.clock().After(*adminContext.ExpiresAt) && adminContext.SessionID == "" && adminContext.State != adminStateIdle {
		if err := r.clearContext(ctx, pairing.TelegramID); err != nil {
			return nil, true, err
		}
		return []Action{messageAction(message.Chat.ID, "That draft expired.\n\n"+adminMenuText(actor), adminMenuMarkup(actor))}, true, nil
	}

	switch adminContext.State {
	case adminStateCreatingName:
		return r.receiveName(ctx, message.Chat.ID, pairing, adminContext, message.Text)
	case adminStateSearchingName:
		return r.receiveSearch(ctx, message.Chat.ID, pairing, actor, adminContext, message.Text)
	case adminStateChoosingScope:
		return []Action{messageAction(message.Chat.ID, "Choose an event scope using the buttons below.", scopeMarkup())}, true, nil
	case adminStateChoosingBattery:
		return []Action{messageAction(message.Chat.ID, "Choose a battery using the buttons below.", batteryMarkup())}, true, nil
	case adminStateChoosingDuration:
		return []Action{messageAction(message.Chat.ID, "Choose how long the event should remain active.", durationMarkup())}, true, nil
	case adminStateConfirmingCreate:
		return []Action{messageAction(message.Chat.ID, creationReview(adminContext.Draft), creationConfirmMarkup())}, true, nil
	case adminStateConfirmingClose:
		return []Action{messageAction(message.Chat.ID, "Confirm closing this event using the button below.", closeConfirmMarkup(adminContext.SessionID))}, true, nil
	default:
		// An ordinary linked reply for a commander is the only entry point that
		// exposes the commander UI. Soldiers continue through the old text-only
		// response in Bot.
		return []Action{messageAction(message.Chat.ID, linkedReply(pairing.FullName), openMenuMarkup())}, true, nil
	}
}

func (r *AdminRouter) HandleCallback(ctx context.Context, query *CallbackQuery, pairing Pairing) ([]Action, bool, error) {
	if r == nil || r.store == nil || query == nil || query.Message == nil || query.Message.Chat.Type != "private" || query.Message.Chat.ID == 0 {
		return nil, false, nil
	}
	if query.From != nil && query.From.ID != 0 && pairing.TelegramID != 0 && query.From.ID != pairing.TelegramID {
		return nil, true, nil
	}
	if !strings.HasPrefix(query.Data, "a:") {
		return nil, false, nil
	}
	actor, eligible, err := r.actor(ctx, pairing)
	if err != nil || !eligible {
		return nil, true, err
	}
	pairing = pairingForAdmin(pairing, actor, query.From)
	chatID := query.Message.Chat.ID
	action, args, ok := parseAdminCallback(query.Data)
	if !ok {
		return nil, true, nil
	}

	switch action {
	case "menu":
		return r.menuAction(chatID, actor), true, nil
	case "cancel":
		if err := r.clearContext(ctx, pairing.TelegramID); err != nil {
			return nil, true, err
		}
		return r.menuAction(chatID, actor), true, nil
	case "create":
		if len(args) == 0 || args[0] == "" {
			return r.startCreate(ctx, chatID, pairing, actor)
		}
		if args[0] == "confirm" || args[0] == "confirm_creation" {
			return r.confirmCreation(ctx, chatID, pairing, actor)
		}
		return nil, true, nil
	case "confirm", "confirm-create", "confirm_creation", "confirm_create":
		if action == "confirm" && len(args) > 0 && args[0] != "create" && args[0] != "creation" {
			return nil, true, nil
		}
		return r.confirmCreation(ctx, chatID, pairing, actor)
	case "events", "active":
		return r.activeEventsAction(ctx, chatID, actor)
	case "event", "select":
		if len(args) != 1 {
			return nil, true, nil
		}
		return r.selectEvent(ctx, chatID, pairing, actor, args[0])
	case "scope":
		if len(args) != 1 {
			return nil, true, nil
		}
		return r.chooseScope(ctx, chatID, pairing, actor, args[0])
	case "battery":
		if len(args) != 1 {
			return nil, true, nil
		}
		return r.chooseBattery(ctx, chatID, pairing, actor, args[0])
	case "duration":
		if len(args) != 1 {
			return nil, true, nil
		}
		return r.chooseDuration(ctx, chatID, pairing, actor, args[0])
	case "qr":
		return r.sendSelectedQR(ctx, chatID, pairing, actor, callbackSession(args))
	case "status", "missing":
		sessionID, page, valid := callbackSessionPage(args)
		if !valid {
			return nil, true, nil
		}
		return r.statusAction(ctx, chatID, pairing, actor, sessionID, page, "")
	case "refresh":
		sessionID, page, valid := callbackSessionPage(args)
		if !valid {
			return nil, true, nil
		}
		return r.statusAction(ctx, chatID, pairing, actor, sessionID, page, "")
	case "search":
		return r.startSearch(ctx, chatID, pairing, actor, callbackSession(args))
	case "mark":
		sessionID, targetID, valid := callbackTarget(args)
		if !valid {
			return nil, true, nil
		}
		return r.mark(ctx, chatID, pairing, actor, sessionID, targetID)
	case "marks", "own-marks":
		sessionID, page, valid := callbackSessionPage(args)
		if !valid {
			return nil, true, nil
		}
		return r.ownMarks(ctx, chatID, pairing, actor, sessionID, page)
	case "undo":
		sessionID, targetID, valid := callbackTarget(args)
		if !valid {
			return nil, true, nil
		}
		return r.undo(ctx, chatID, pairing, actor, sessionID, targetID)
	case "close":
		if len(args) == 1 && (args[0] == "confirm" || args[0] == "yes") {
			return r.confirmClose(ctx, chatID, pairing, actor, "")
		}
		if len(args) == 1 && (args[0] == "no" || args[0] == "cancel") {
			return r.cancelClose(ctx, chatID, pairing, actor, "")
		}
		if len(args) == 2 && (args[0] == "confirm" || args[0] == "yes") {
			return r.confirmClose(ctx, chatID, pairing, actor, args[1])
		}
		if len(args) == 2 && (args[0] == "no" || args[0] == "cancel") {
			return r.cancelClose(ctx, chatID, pairing, actor, args[1])
		}
		if len(args) == 2 && (args[1] == "confirm" || args[1] == "yes") {
			return r.confirmClose(ctx, chatID, pairing, actor, args[0])
		}
		if len(args) == 2 && (args[1] == "no" || args[1] == "cancel") {
			return r.cancelClose(ctx, chatID, pairing, actor, args[0])
		}
		return r.askClose(ctx, chatID, pairing, actor, callbackSession(args))
	case "close-confirm", "confirm-close", "close_confirm", "confirm_close":
		return r.confirmClose(ctx, chatID, pairing, actor, callbackSession(args))
	case "close-cancel", "cancel-close", "close_cancel", "cancel_close":
		return r.cancelClose(ctx, chatID, pairing, actor, callbackSession(args))
	case "back":
		return r.back(ctx, chatID, pairing, actor, callbackSession(args))
	default:
		// A stale admin callback is deliberately indistinguishable from a
		// callback for an unknown resource. Bot acknowledges it, but the router
		// does not disclose session or roster existence.
		return nil, true, nil
	}
}

func (r *AdminRouter) actor(ctx context.Context, pairing Pairing) (AdminActor, bool, error) {
	if pairing.TelegramID == 0 {
		return AdminActor{}, false, nil
	}
	actor, found, err := r.store.Actor(ctx, pairing)
	if err != nil {
		return AdminActor{}, false, err
	}
	if !found || actor.Tier < models.TierBatteryNCO {
		return AdminActor{}, false, nil
	}
	if actor.Pairing.TelegramID == 0 {
		actor.Pairing.TelegramID = pairing.TelegramID
	}
	if actor.Pairing.UserID == "" {
		actor.Pairing.UserID = pairing.UserID
	}
	if actor.FullName == "" {
		actor.FullName = pairing.FullName
	}
	return actor, true, nil
}

func pairingForAdmin(pairing Pairing, actor AdminActor, user *User) Pairing {
	if pairing.TelegramID == 0 {
		pairing.TelegramID = actor.Pairing.TelegramID
	}
	if pairing.TelegramID == 0 && user != nil {
		pairing.TelegramID = user.ID
	}
	if pairing.UserID == "" {
		pairing.UserID = actor.Pairing.UserID
	}
	if pairing.FullName == "" {
		pairing.FullName = actor.FullName
	}
	return pairing
}

func (r *AdminRouter) clock() time.Time {
	if r != nil && r.now != nil {
		return r.now()
	}
	return time.Now()
}

func (r *AdminRouter) loadContext(ctx context.Context, telegramID int64) (AdminContext, error) {
	adminContext, err := r.store.LoadContext(ctx, telegramID)
	if errors.Is(err, ErrAdminContextConflict) {
		return AdminContext{TelegramID: telegramID, State: adminStateIdle}, nil
	}
	if err != nil {
		return AdminContext{}, err
	}
	if adminContext.TelegramID == 0 {
		adminContext.TelegramID = telegramID
	}
	if adminContext.State == "" {
		adminContext.State = adminStateIdle
	}
	return adminContext, nil
}

func (r *AdminRouter) saveContext(ctx context.Context, next AdminContext) error {
	if next.TelegramID == 0 {
		return nil
	}
	err := r.store.SaveContext(ctx, next)
	if errors.Is(err, ErrAdminContextConflict) {
		return ErrAdminContextConflict
	}
	return err
}

func (r *AdminRouter) clearContext(ctx context.Context, telegramID int64) error {
	err := r.store.ClearContext(ctx, telegramID)
	if errors.Is(err, ErrAdminContextConflict) {
		return nil
	}
	return err
}

func (r *AdminRouter) menuAction(chatID int64, actor AdminActor) []Action {
	return []Action{messageAction(chatID, adminMenuText(actor), adminMenuMarkup(actor))}
}

func adminMenuText(actor AdminActor) string {
	if isSuperadmin(actor) {
		return "Commander menu\nSuperadmin access"
	}
	if actor.Tier >= models.TierUnitCommander {
		return "Commander menu\nUnit commander access"
	}
	return "Commander menu\nBattery access"
}

func adminMenuMarkup(actor AdminActor) *InlineKeyboardMarkup {
	rows := make([][]InlineKeyboardButton, 0, 2)
	if actor.Tier >= models.TierUnitCommander {
		rows = append(rows, []InlineKeyboardButton{{Text: "Create event", CallbackData: "a:create"}})
	}
	rows = append(rows, []InlineKeyboardButton{{Text: "Active events", CallbackData: "a:events"}})
	return &InlineKeyboardMarkup{InlineKeyboard: rows}
}

func openMenuMarkup() *InlineKeyboardMarkup {
	return &InlineKeyboardMarkup{InlineKeyboard: [][]InlineKeyboardButton{{{Text: "Open commander menu", CallbackData: "a:menu"}}}}
}

func messageAction(chatID int64, text string, markup *InlineKeyboardMarkup) Action {
	return Action{Kind: SendMessage, ChatID: chatID, Text: text, ReplyMarkup: markup}
}

func (r *AdminRouter) startCreate(ctx context.Context, chatID int64, pairing Pairing, actor AdminActor) ([]Action, bool, error) {
	if actor.Tier < models.TierUnitCommander {
		return r.safeAction(chatID), true, nil
	}
	adminContext, err := r.loadContext(ctx, pairing.TelegramID)
	if err != nil {
		return nil, true, err
	}
	if adminContext.State != adminStateIdle || adminContext.SessionID != "" {
		return nil, true, nil
	}
	adminContext.State = adminStateCreatingName
	adminContext.Draft = AdminDraft{}
	expires := r.clock().Add(adminDraftTTL)
	adminContext.ExpiresAt = &expires
	if err := r.saveContext(ctx, adminContext); err != nil {
		if errors.Is(err, ErrAdminContextConflict) {
			return nil, true, nil
		}
		return nil, true, err
	}
	return []Action{messageAction(chatID, "Send the event name (1–80 characters).", nil)}, true, nil
}

func (r *AdminRouter) receiveName(ctx context.Context, chatID int64, pairing Pairing, adminContext AdminContext, input string) ([]Action, bool, error) {
	name := strings.TrimSpace(input)
	if name == "" {
		return []Action{messageAction(chatID, "Event name cannot be empty. Send the event name (1–80 characters).", nil)}, true, nil
	}
	if len([]rune(name)) > 80 {
		return []Action{messageAction(chatID, "Event name must be 80 characters or fewer. Send the event name again.", nil)}, true, nil
	}
	adminContext.Draft.Name = name
	adminContext.State = adminStateChoosingScope
	if err := r.saveContext(ctx, adminContext); err != nil {
		if errors.Is(err, ErrAdminContextConflict) {
			return nil, true, nil
		}
		return nil, true, err
	}
	return []Action{messageAction(chatID, "Choose an event scope.", scopeMarkup())}, true, nil
}

func (r *AdminRouter) chooseScope(ctx context.Context, chatID int64, pairing Pairing, actor AdminActor, scope string) ([]Action, bool, error) {
	if actor.Tier < models.TierUnitCommander || (scope != models.SessionScopeUnitWide && scope != models.SessionScopeBatterySpecific) {
		return r.safeAction(chatID), true, nil
	}
	adminContext, err := r.loadContext(ctx, pairing.TelegramID)
	if err != nil {
		return nil, true, err
	}
	if adminContext.State != adminStateChoosingScope || strings.TrimSpace(adminContext.Draft.Name) == "" {
		return nil, true, nil
	}
	adminContext.Draft.Scope = scope
	adminContext.Draft.Battery = ""
	if scope == models.SessionScopeBatterySpecific {
		adminContext.State = adminStateChoosingBattery
	} else {
		adminContext.State = adminStateChoosingDuration
	}
	if err := r.saveContext(ctx, adminContext); err != nil {
		if errors.Is(err, ErrAdminContextConflict) {
			return nil, true, nil
		}
		return nil, true, err
	}
	if scope == models.SessionScopeBatterySpecific {
		return []Action{messageAction(chatID, "Choose a battery.", batteryMarkup())}, true, nil
	}
	return []Action{messageAction(chatID, "Choose how long the event should remain active.", durationMarkup())}, true, nil
}

func (r *AdminRouter) chooseBattery(ctx context.Context, chatID int64, pairing Pairing, actor AdminActor, battery string) ([]Action, bool, error) {
	if actor.Tier < models.TierUnitCommander || !validAdminBatteryValue(battery) {
		return r.safeAction(chatID), true, nil
	}
	adminContext, err := r.loadContext(ctx, pairing.TelegramID)
	if err != nil {
		return nil, true, err
	}
	if adminContext.State != adminStateChoosingBattery || adminContext.Draft.Scope != models.SessionScopeBatterySpecific {
		return nil, true, nil
	}
	adminContext.Draft.Battery = battery
	adminContext.State = adminStateChoosingDuration
	if err := r.saveContext(ctx, adminContext); err != nil {
		if errors.Is(err, ErrAdminContextConflict) {
			return nil, true, nil
		}
		return nil, true, err
	}
	return []Action{messageAction(chatID, "Choose how long the event should remain active.", durationMarkup())}, true, nil
}

func (r *AdminRouter) chooseDuration(ctx context.Context, chatID int64, pairing Pairing, actor AdminActor, duration string) ([]Action, bool, error) {
	if actor.Tier < models.TierUnitCommander {
		return r.safeAction(chatID), true, nil
	}
	amount, ok := adminDuration(duration)
	if !ok {
		return r.safeAction(chatID), true, nil
	}
	adminContext, err := r.loadContext(ctx, pairing.TelegramID)
	if err != nil {
		return nil, true, err
	}
	if adminContext.State != adminStateChoosingDuration || adminContext.Draft.Name == "" || adminContext.Draft.Scope == "" {
		return nil, true, nil
	}
	if adminContext.Draft.Scope == models.SessionScopeBatterySpecific && !validAdminBatteryValue(adminContext.Draft.Battery) {
		return nil, true, nil
	}
	adminContext.Draft.EndTime = r.clock().Add(amount)
	adminContext.State = adminStateConfirmingCreate
	if err := r.saveContext(ctx, adminContext); err != nil {
		if errors.Is(err, ErrAdminContextConflict) {
			return nil, true, nil
		}
		return nil, true, err
	}
	return []Action{messageAction(chatID, creationReview(adminContext.Draft), creationConfirmMarkup())}, true, nil
}

func (r *AdminRouter) confirmCreation(ctx context.Context, chatID int64, pairing Pairing, actor AdminActor) ([]Action, bool, error) {
	if actor.Tier < models.TierUnitCommander {
		return r.safeAction(chatID), true, nil
	}
	adminContext, err := r.loadContext(ctx, pairing.TelegramID)
	if err != nil {
		return nil, true, err
	}
	if adminContext.State != adminStateConfirmingCreate || !validAdminDraftForRouter(adminContext.Draft, r.clock()) {
		return nil, true, nil
	}
	// Reserve the confirmation with an optimistic state transition before
	// invoking CreateEvent. Telegram redelivery therefore cannot create twice.
	adminContext.State = adminStateCreating
	reservedVersion := adminContext.Version
	if err := r.saveContext(ctx, adminContext); err != nil {
		if errors.Is(err, ErrAdminContextConflict) {
			return nil, true, nil
		}
		return nil, true, err
	}
	created, err := r.store.CreateEvent(ctx, actor, adminContext.Draft)
	if err != nil {
		// Restore the review state when the application operation failed. A
		// conflict here is safe: another callback owns the newer context.
		adminContext.Version = reservedVersion + 1
		adminContext.State = adminStateConfirmingCreate
		_ = r.saveContext(ctx, adminContext)
		if errors.Is(err, ErrAdminUnavailable) {
			return r.safeAction(chatID), true, nil
		}
		return nil, true, err
	}

	// Keep the newly-created event selected, which gives the commander an
	// immediate route to status/close while retaining the opaque link only in
	// the ActiveEvent returned by the store.
	adminContext.Version = reservedVersion + 1
	adminContext.State = adminStateSelected
	adminContext.SessionID = created.ID
	adminContext.Draft = AdminDraft{}
	adminContext.ExpiresAt = nil
	if saveErr := r.saveContext(ctx, adminContext); saveErr != nil && !errors.Is(saveErr, ErrAdminContextConflict) {
		return nil, true, saveErr
	}
	return r.eventDeliveryActions(chatID, created, "Event created"), true, nil
}

func validAdminDraftForRouter(draft AdminDraft, now time.Time) bool {
	name := strings.TrimSpace(draft.Name)
	if name == "" || len([]rune(name)) > 80 || draft.EndTime.IsZero() || !draft.EndTime.After(now) {
		return false
	}
	switch draft.Scope {
	case models.SessionScopeUnitWide:
		return true
	case models.SessionScopeBatterySpecific:
		return validAdminBatteryValue(draft.Battery)
	default:
		return false
	}
}

func (r *AdminRouter) activeEventsAction(ctx context.Context, chatID int64, actor AdminActor) ([]Action, bool, error) {
	events, err := r.store.ActiveEvents(ctx, actor)
	if err != nil {
		if errors.Is(err, ErrAdminUnavailable) {
			return r.safeAction(chatID), true, nil
		}
		return nil, true, err
	}
	return []Action{activeEventsMessage(chatID, events, actor)}, true, nil
}

func (r *AdminRouter) selectEvent(ctx context.Context, chatID int64, pairing Pairing, actor AdminActor, sessionID string) ([]Action, bool, error) {
	if strings.TrimSpace(sessionID) == "" {
		return nil, true, nil
	}
	event, found, err := r.findActiveEvent(ctx, actor, sessionID)
	if err != nil {
		if errors.Is(err, ErrAdminUnavailable) {
			return r.safeAction(chatID), true, nil
		}
		return nil, true, err
	}
	if !found {
		return r.safeAction(chatID), true, nil
	}
	adminContext, err := r.loadContext(ctx, pairing.TelegramID)
	if err != nil {
		return nil, true, err
	}
	adminContext.SessionID = event.ID
	adminContext.State = adminStateSelected
	adminContext.Draft = AdminDraft{}
	adminContext.ExpiresAt = nil
	if err := r.saveContext(ctx, adminContext); err != nil {
		if errors.Is(err, ErrAdminContextConflict) {
			return nil, true, nil
		}
		return nil, true, err
	}
	return []Action{selectedEventMessage(chatID, actor, event)}, true, nil
}

func (r *AdminRouter) findActiveEvent(ctx context.Context, actor AdminActor, sessionID string) (ActiveEvent, bool, error) {
	events, err := r.store.ActiveEvents(ctx, actor)
	if err != nil {
		return ActiveEvent{}, false, err
	}
	for _, event := range events {
		if event.ID == sessionID {
			return event, true, nil
		}
	}
	return ActiveEvent{}, false, nil
}

func (r *AdminRouter) selectedContext(ctx context.Context, pairing Pairing, expectedID string) (AdminContext, bool, error) {
	adminContext, err := r.loadContext(ctx, pairing.TelegramID)
	if err != nil {
		return AdminContext{}, false, err
	}
	if adminContext.SessionID == "" || (expectedID != "" && adminContext.SessionID != expectedID) {
		return adminContext, false, nil
	}
	return adminContext, true, nil
}

func (r *AdminRouter) selectedEvent(ctx context.Context, pairing Pairing, actor AdminActor, expectedID string) (AdminContext, ActiveEvent, bool, error) {
	adminContext, selected, err := r.selectedContext(ctx, pairing, expectedID)
	if err != nil || !selected {
		return adminContext, ActiveEvent{}, selected, err
	}
	event, found, err := r.findActiveEvent(ctx, actor, adminContext.SessionID)
	if err != nil {
		return adminContext, ActiveEvent{}, false, err
	}
	if !found {
		// A closed/expired event is treated as unavailable and its selected
		// context is cleared. ClearContext has no resource-specific disclosure.
		_ = r.clearContext(ctx, pairing.TelegramID)
		return adminContext, ActiveEvent{}, false, nil
	}
	return adminContext, event, true, nil
}

func (r *AdminRouter) sendSelectedQR(ctx context.Context, chatID int64, pairing Pairing, actor AdminActor, expectedID string) ([]Action, bool, error) {
	_, event, ok, err := r.selectedEvent(ctx, pairing, actor, expectedID)
	if err != nil {
		if errors.Is(err, ErrAdminUnavailable) {
			return r.safeAction(chatID), true, nil
		}
		return nil, true, err
	}
	if !ok {
		return r.safeAction(chatID), true, nil
	}
	return r.eventDeliveryActions(chatID, event, "Attendance link"), true, nil
}

func (r *AdminRouter) statusAction(ctx context.Context, chatID int64, pairing Pairing, actor AdminActor, expectedID string, page int, query string) ([]Action, bool, error) {
	adminContext, ok, err := r.selectedContext(ctx, pairing, expectedID)
	if err != nil {
		return nil, true, err
	}
	if !ok {
		return r.safeAction(chatID), true, nil
	}
	if page < 1 {
		page = 1
	}
	result, err := r.store.Status(ctx, actor, adminContext.SessionID, strings.TrimSpace(query), page)
	if err != nil {
		if errors.Is(err, ErrAdminUnavailable) {
			return r.safeAction(chatID), true, nil
		}
		return nil, true, err
	}
	if query != "" {
		adminContext.State = adminStateViewingStatus
		if saveErr := r.saveContext(ctx, adminContext); saveErr != nil && !errors.Is(saveErr, ErrAdminContextConflict) {
			return nil, true, saveErr
		}
	}
	return []Action{statusMessage(chatID, adminContext.SessionID, result, query)}, true, nil
}

func (r *AdminRouter) startSearch(ctx context.Context, chatID int64, pairing Pairing, actor AdminActor, expectedID string) ([]Action, bool, error) {
	adminContext, ok, err := r.selectedContext(ctx, pairing, expectedID)
	if err != nil {
		return nil, true, err
	}
	if !ok {
		return r.safeAction(chatID), true, nil
	}
	adminContext.State = adminStateSearchingName
	if err := r.saveContext(ctx, adminContext); err != nil {
		if errors.Is(err, ErrAdminContextConflict) {
			return nil, true, nil
		}
		return nil, true, err
	}
	return []Action{messageAction(chatID, "Send a soldier's name to search.", nil)}, true, nil
}

func (r *AdminRouter) receiveSearch(ctx context.Context, chatID int64, pairing Pairing, actor AdminActor, adminContext AdminContext, input string) ([]Action, bool, error) {
	query := strings.TrimSpace(input)
	if query == "" {
		return []Action{messageAction(chatID, "Send a name to search, or use /cancel.", nil)}, true, nil
	}
	result, err := r.store.Status(ctx, actor, adminContext.SessionID, query, 1)
	if err != nil {
		if errors.Is(err, ErrAdminUnavailable) {
			return r.safeAction(chatID), true, nil
		}
		return nil, true, err
	}
	adminContext.State = adminStateViewingStatus
	if saveErr := r.saveContext(ctx, adminContext); saveErr != nil && !errors.Is(saveErr, ErrAdminContextConflict) {
		return nil, true, saveErr
	}
	return []Action{statusMessage(chatID, adminContext.SessionID, result, query)}, true, nil
}

func (r *AdminRouter) mark(ctx context.Context, chatID int64, pairing Pairing, actor AdminActor, expectedSessionID, targetID string) ([]Action, bool, error) {
	adminContext, ok, err := r.selectedContext(ctx, pairing, expectedSessionID)
	if err != nil {
		return nil, true, err
	}
	if !ok || strings.TrimSpace(targetID) == "" {
		return r.safeAction(chatID), true, nil
	}
	if err := r.store.MarkManual(ctx, actor, adminContext.SessionID, targetID); err != nil {
		if errors.Is(err, ErrAdminUnavailable) {
			return r.safeAction(chatID), true, nil
		}
		return nil, true, err
	}
	return []Action{messageAction(chatID, "Soldier marked present.", markResultMarkup(adminContext.SessionID, targetID))}, true, nil
}

func (r *AdminRouter) ownMarks(ctx context.Context, chatID int64, pairing Pairing, actor AdminActor, expectedSessionID string, page int) ([]Action, bool, error) {
	adminContext, ok, err := r.selectedContext(ctx, pairing, expectedSessionID)
	if err != nil {
		return nil, true, err
	}
	if !ok {
		return r.safeAction(chatID), true, nil
	}
	if page < 1 {
		page = 1
	}
	rows, err := r.store.OwnManualMarks(ctx, actor, adminContext.SessionID, page)
	if err != nil {
		if errors.Is(err, ErrAdminUnavailable) {
			return r.safeAction(chatID), true, nil
		}
		return nil, true, err
	}
	return []Action{ownMarksMessage(chatID, adminContext.SessionID, rows, page)}, true, nil
}

func (r *AdminRouter) undo(ctx context.Context, chatID int64, pairing Pairing, actor AdminActor, expectedSessionID, targetID string) ([]Action, bool, error) {
	adminContext, ok, err := r.selectedContext(ctx, pairing, expectedSessionID)
	if err != nil {
		return nil, true, err
	}
	if !ok || strings.TrimSpace(targetID) == "" {
		return r.safeAction(chatID), true, nil
	}
	if err := r.store.UndoManual(ctx, actor, adminContext.SessionID, targetID); err != nil {
		if errors.Is(err, ErrAdminUnavailable) {
			return r.safeAction(chatID), true, nil
		}
		return nil, true, err
	}
	return []Action{messageAction(chatID, "Manual mark undone.", markResultMarkup(adminContext.SessionID, ""))}, true, nil
}

func (r *AdminRouter) askClose(ctx context.Context, chatID int64, pairing Pairing, actor AdminActor, expectedSessionID string) ([]Action, bool, error) {
	if actor.Tier < models.TierUnitCommander {
		return r.safeAction(chatID), true, nil
	}
	adminContext, event, ok, err := r.selectedEvent(ctx, pairing, actor, expectedSessionID)
	if err != nil {
		if errors.Is(err, ErrAdminUnavailable) {
			return r.safeAction(chatID), true, nil
		}
		return nil, true, err
	}
	if !ok || !canCloseEvent(actor, event) {
		return r.safeAction(chatID), true, nil
	}
	adminContext.State = adminStateConfirmingClose
	if err := r.saveContext(ctx, adminContext); err != nil {
		if errors.Is(err, ErrAdminContextConflict) {
			return nil, true, nil
		}
		return nil, true, err
	}
	return []Action{messageAction(chatID, "Close "+event.Name+"? This stops further attendance marking.", closeConfirmMarkup(event.ID))}, true, nil
}

func (r *AdminRouter) confirmClose(ctx context.Context, chatID int64, pairing Pairing, actor AdminActor, expectedSessionID string) ([]Action, bool, error) {
	if actor.Tier < models.TierUnitCommander {
		return r.safeAction(chatID), true, nil
	}
	adminContext, event, ok, err := r.selectedEvent(ctx, pairing, actor, expectedSessionID)
	if err != nil {
		if errors.Is(err, ErrAdminUnavailable) {
			return r.safeAction(chatID), true, nil
		}
		return nil, true, err
	}
	if !ok || adminContext.State != adminStateConfirmingClose || !canCloseEvent(actor, event) {
		return r.safeAction(chatID), true, nil
	}
	if err := r.store.CloseEvent(ctx, actor, adminContext.SessionID); err != nil {
		if errors.Is(err, ErrAdminUnavailable) {
			return r.safeAction(chatID), true, nil
		}
		return nil, true, err
	}
	if err := r.clearContext(ctx, pairing.TelegramID); err != nil {
		return nil, true, err
	}
	return []Action{messageAction(chatID, "Event closed.", adminMenuMarkup(actor))}, true, nil
}

func (r *AdminRouter) cancelClose(ctx context.Context, chatID int64, pairing Pairing, actor AdminActor, expectedSessionID string) ([]Action, bool, error) {
	adminContext, event, ok, err := r.selectedEvent(ctx, pairing, actor, expectedSessionID)
	if err != nil {
		if errors.Is(err, ErrAdminUnavailable) {
			return r.safeAction(chatID), true, nil
		}
		return nil, true, err
	}
	if !ok || adminContext.State != adminStateConfirmingClose {
		return r.safeAction(chatID), true, nil
	}
	adminContext.State = adminStateSelected
	if err := r.saveContext(ctx, adminContext); err != nil {
		if errors.Is(err, ErrAdminContextConflict) {
			return nil, true, nil
		}
		return nil, true, err
	}
	return []Action{selectedEventMessage(chatID, actor, event)}, true, nil
}

func (r *AdminRouter) back(ctx context.Context, chatID int64, pairing Pairing, actor AdminActor, expectedSessionID string) ([]Action, bool, error) {
	adminContext, _, ok, err := r.selectedEvent(ctx, pairing, actor, expectedSessionID)
	if err != nil {
		if errors.Is(err, ErrAdminUnavailable) {
			return r.safeAction(chatID), true, nil
		}
		return nil, true, err
	}
	if !ok {
		return r.safeAction(chatID), true, nil
	}
	if err := r.clearContext(ctx, adminContext.TelegramID); err != nil {
		return nil, true, err
	}
	return r.activeEventsAction(ctx, chatID, actor)
}

func (r *AdminRouter) safeAction(chatID int64) []Action {
	return []Action{messageAction(chatID, "That action is no longer available.", nil)}
}

func isSuperadmin(actor AdminActor) bool {
	return actor.IsSuperadmin || actor.Tier >= models.TierSuperadmin
}

func canCloseEvent(actor AdminActor, event ActiveEvent) bool {
	if isSuperadmin(actor) {
		return true
	}
	if actor.Tier < models.TierUnitCommander {
		return false
	}
	if event.CreatedBy == "" {
		// The store remains the authoritative ownership check at confirmation;
		// a blank optional field cannot be used to reject an otherwise valid
		// production event.
		return true
	}
	return event.CreatedBy == actor.Pairing.UserID
}

func activeEventsMessage(chatID int64, events []ActiveEvent, actor AdminActor) Action {
	text := "Active events"
	markup := &InlineKeyboardMarkup{}
	for _, event := range events {
		if strings.TrimSpace(event.ID) == "" || strings.TrimSpace(event.Name) == "" {
			continue
		}
		if end := eventEndLabel(event.EndTime); end != "" {
			text += "\n" + event.Name + " · ends " + end
		} else {
			text += "\n" + event.Name
		}
		if data, ok := adminCallbackData("a", "event", event.ID); ok {
			markup.InlineKeyboard = append(markup.InlineKeyboard, []InlineKeyboardButton{{Text: eventButtonText(event), CallbackData: data}})
		}
	}
	if len(events) == 0 {
		text += "\nNo active events."
	}
	if data, ok := adminCallbackData("a", "menu"); ok {
		markup.InlineKeyboard = append(markup.InlineKeyboard, []InlineKeyboardButton{{Text: "Back", CallbackData: data}})
	}
	return messageAction(chatID, text, markup)
}

func selectedEventMessage(chatID int64, actor AdminActor, event ActiveEvent) Action {
	text := event.Name
	if end := eventEndLabel(event.EndTime); end != "" {
		text += "\nEnds " + end
	}
	rows := [][]InlineKeyboardButton{}
	if data, ok := adminCallbackData("a", "qr", event.ID); ok {
		rows = append(rows, []InlineKeyboardButton{{Text: "Send QR + link", CallbackData: data}})
	}
	if data, ok := adminCallbackData("a", "status", event.ID, "1"); ok {
		rows = append(rows, []InlineKeyboardButton{{Text: "View status", CallbackData: data}})
	}
	if data, ok := adminCallbackData("a", "missing", event.ID, "1"); ok {
		rows = append(rows, []InlineKeyboardButton{{Text: "Missing soldiers", CallbackData: data}})
	}
	if data, ok := adminCallbackData("a", "search", event.ID); ok {
		rows = append(rows, []InlineKeyboardButton{{Text: "Search soldier", CallbackData: data}})
	}
	if data, ok := adminCallbackData("a", "marks", event.ID, "1"); ok {
		rows = append(rows, []InlineKeyboardButton{{Text: "My manual marks", CallbackData: data}})
	}
	if canCloseEvent(actor, event) {
		if data, ok := adminCallbackData("a", "close", event.ID); ok {
			rows = append(rows, []InlineKeyboardButton{{Text: "Close event", CallbackData: data}})
		}
	}
	if data, ok := adminCallbackData("a", "events"); ok {
		rows = append(rows, []InlineKeyboardButton{{Text: "Back", CallbackData: data}})
	}
	return messageAction(chatID, text, &InlineKeyboardMarkup{InlineKeyboard: rows})
}

func statusMessage(chatID int64, sessionID string, page AttendancePage, query string) Action {
	name := strings.TrimSpace(page.SessionName)
	if name == "" {
		name = "Attendance status"
	}
	text := name
	if query != "" {
		text += "\nSearch: " + query
	}
	percentage := page.Percentage
	if percentage < 0 {
		percentage = 0
	}
	text += fmt.Sprintf("\nPresent: %d / %d (%.1f%%)\nMissing: %d", page.Present, page.Total, percentage, page.Missing)
	if len(page.Rows) == 0 {
		text += "\n\nNo missing soldiers found."
	}
	rows := [][]InlineKeyboardButton{}
	limit := len(page.Rows)
	if limit > adminPageSize {
		limit = adminPageSize
	}
	for _, user := range page.Rows[:limit] {
		label := strings.TrimSpace(user.Name)
		if label == "" {
			label = user.ID
		}
		data, ok := adminCallbackData("a", "mark", sessionID, user.ID)
		if !ok {
			continue
		}
		rows = append(rows, []InlineKeyboardButton{{Text: "Mark " + label, CallbackData: data}})
	}
	if page.HasPrevious || page.Page > 1 {
		if data, ok := adminCallbackData("a", "status", sessionID, strconv.Itoa(maxAdminPage(page.Page-1))); ok {
			rows = append(rows, []InlineKeyboardButton{{Text: "Prev", CallbackData: data}})
		}
	}
	if page.HasNext {
		if data, ok := adminCallbackData("a", "status", sessionID, strconv.Itoa(maxAdminPage(page.Page+1))); ok {
			rows = append(rows, []InlineKeyboardButton{{Text: "Next", CallbackData: data}})
		}
	}
	if data, ok := adminCallbackData("a", "refresh", sessionID, strconv.Itoa(maxAdminPage(page.Page))); ok {
		rows = append(rows, []InlineKeyboardButton{{Text: "Refresh", CallbackData: data}})
	}
	if data, ok := adminCallbackData("a", "event", sessionID); ok {
		rows = append(rows, []InlineKeyboardButton{{Text: "Back", CallbackData: data}})
	}
	return Action{Kind: SendMessage, ChatID: chatID, Text: text, ReplyMarkup: &InlineKeyboardMarkup{InlineKeyboard: rows}}
}

func ownMarksMessage(chatID int64, sessionID string, users []AdminUser, page int) Action {
	text := "My manual marks"
	rows := [][]InlineKeyboardButton{}
	limit := len(users)
	if limit > adminPageSize {
		limit = adminPageSize
	}
	for _, user := range users[:limit] {
		label := strings.TrimSpace(user.Name)
		if label == "" {
			label = user.ID
		}
		if data, ok := adminCallbackData("a", "undo", sessionID, user.ID); ok {
			rows = append(rows, []InlineKeyboardButton{{Text: "Undo " + label, CallbackData: data}})
		}
	}
	if page > 1 {
		if data, ok := adminCallbackData("a", "marks", sessionID, strconv.Itoa(page-1)); ok {
			rows = append(rows, []InlineKeyboardButton{{Text: "Prev", CallbackData: data}})
		}
	}
	if len(users) == adminPageSize {
		if data, ok := adminCallbackData("a", "marks", sessionID, strconv.Itoa(page+1)); ok {
			rows = append(rows, []InlineKeyboardButton{{Text: "Next", CallbackData: data}})
		}
	}
	if data, ok := adminCallbackData("a", "status", sessionID, "1"); ok {
		rows = append(rows, []InlineKeyboardButton{{Text: "Back", CallbackData: data}})
	}
	if len(users) == 0 {
		text += "\nNo manual marks found."
	}
	return messageAction(chatID, text, &InlineKeyboardMarkup{InlineKeyboard: rows})
}

func (r *AdminRouter) eventDeliveryActions(chatID int64, event ActiveEvent, prefix string) []Action {
	link := strings.TrimSpace(event.TelegramLink)
	if link == "" {
		return r.safeAction(chatID)
	}
	linkMarkup := &InlineKeyboardMarkup{InlineKeyboard: [][]InlineKeyboardButton{{{Text: "Open attendance link", URL: link}}}}
	text := prefix + ": " + event.Name + "\nEvent link: " + link
	actions := []Action{{Kind: SendMessage, ChatID: chatID, Text: text, ReplyMarkup: linkMarkup}}
	photo, err := QRPNG(link)
	if err != nil {
		return actions
	}
	photoMarkup := &InlineKeyboardMarkup{InlineKeyboard: [][]InlineKeyboardButton{{{Text: "Open attendance link", URL: link}}}}
	actions = append(actions, Action{
		Kind:         SendPhoto,
		ChatID:       chatID,
		Photo:        photo,
		Caption:      event.Name,
		ReplyMarkup:  photoMarkup,
		FallbackText: "Event link: " + link,
	})
	return actions
}

func scopeMarkup() *InlineKeyboardMarkup {
	return &InlineKeyboardMarkup{InlineKeyboard: [][]InlineKeyboardButton{{
		{Text: "Unit-wide", CallbackData: "a:scope:" + models.SessionScopeUnitWide},
		{Text: "Battery-specific", CallbackData: "a:scope:" + models.SessionScopeBatterySpecific},
	}}}
}

func batteryMarkup() *InlineKeyboardMarkup {
	return &InlineKeyboardMarkup{InlineKeyboard: [][]InlineKeyboardButton{{
		{Text: models.BatteryHQ, CallbackData: "a:battery:" + models.BatteryHQ},
		{Text: models.BatteryAlpha, CallbackData: "a:battery:" + models.BatteryAlpha},
		{Text: models.BatteryBravo, CallbackData: "a:battery:" + models.BatteryBravo},
	}}}
}

func durationMarkup() *InlineKeyboardMarkup {
	return &InlineKeyboardMarkup{InlineKeyboard: [][]InlineKeyboardButton{{
		{Text: "30 min", CallbackData: "a:duration:" + adminDuration30m},
		{Text: "1 hour", CallbackData: "a:duration:" + adminDuration1h},
		{Text: "2 hours", CallbackData: "a:duration:" + adminDuration2h},
		{Text: "4 hours", CallbackData: "a:duration:" + adminDuration4h},
	}}}
}

func creationConfirmMarkup() *InlineKeyboardMarkup {
	return &InlineKeyboardMarkup{InlineKeyboard: [][]InlineKeyboardButton{{
		{Text: "Confirm", CallbackData: "a:confirm-create"},
		{Text: "Cancel", CallbackData: "a:cancel"},
	}}}
}

func closeConfirmMarkup(sessionID string) *InlineKeyboardMarkup {
	rows := [][]InlineKeyboardButton{}
	if data, ok := adminCallbackData("a", "close-confirm", sessionID); ok {
		rows = append(rows, []InlineKeyboardButton{{Text: "Confirm close", CallbackData: data}})
	}
	if data, ok := adminCallbackData("a", "close-cancel", sessionID); ok {
		rows = append(rows, []InlineKeyboardButton{{Text: "Cancel", CallbackData: data}})
	}
	return &InlineKeyboardMarkup{InlineKeyboard: rows}
}

func markResultMarkup(sessionID, targetID string) *InlineKeyboardMarkup {
	rows := [][]InlineKeyboardButton{}
	if targetID != "" {
		if data, ok := adminCallbackData("a", "undo", sessionID, targetID); ok {
			rows = append(rows, []InlineKeyboardButton{{Text: "Undo", CallbackData: data}})
		}
	}
	if data, ok := adminCallbackData("a", "marks", sessionID, "1"); ok {
		rows = append(rows, []InlineKeyboardButton{{Text: "My manual marks", CallbackData: data}})
	}
	return &InlineKeyboardMarkup{InlineKeyboard: rows}
}

func creationReview(draft AdminDraft) string {
	name := strings.TrimSpace(draft.Name)
	scope := draft.Scope
	if scope == models.SessionScopeUnitWide {
		scope = "unit-wide"
	} else if scope == models.SessionScopeBatterySpecific {
		scope = "battery-specific"
	}
	text := fmt.Sprintf("Review event\nName: %s\nScope: %s", name, scope)
	if draft.Battery != "" {
		text += "\nBattery: " + draft.Battery
	}
	if !draft.EndTime.IsZero() {
		text += "\nEnds: " + draft.EndTime.Format("2006-01-02 15:04")
	}
	return text
}

func eventButtonText(event ActiveEvent) string {
	label := event.Name
	if end := eventEndLabel(event.EndTime); end != "" {
		label += " · ends " + end
	}
	return label
}

func maxAdminPage(page int) int {
	if page < 1 {
		return 1
	}
	return page
}

func eventEndLabel(endTime *time.Time) string {
	if endTime == nil || endTime.IsZero() {
		return ""
	}
	return endTime.Format("15:04")
}

func validAdminBatteryValue(battery string) bool {
	switch battery {
	case models.BatteryHQ, models.BatteryAlpha, models.BatteryBravo:
		return true
	default:
		return false
	}
}

func adminDuration(value string) (time.Duration, bool) {
	switch value {
	case adminDuration30m:
		return 30 * time.Minute, true
	case adminDuration1h:
		return time.Hour, true
	case adminDuration2h:
		return 2 * time.Hour, true
	case adminDuration4h:
		return 4 * time.Hour, true
	default:
		return 0, false
	}
}

func telegramCommand(text string) string {
	fields := strings.Fields(strings.TrimSpace(text))
	if len(fields) == 0 {
		return ""
	}
	command := fields[0]
	if at := strings.IndexByte(command, '@'); at >= 0 {
		command = command[:at]
	}
	if command == "/menu" || command == "/cancel" {
		return strings.ToLower(command)
	}
	return ""
}

func parseAdminCallback(data string) (string, []string, bool) {
	parts := strings.Split(data, ":")
	if len(parts) < 2 || parts[0] != "a" || parts[1] == "" {
		return "", nil, false
	}
	for _, part := range parts[2:] {
		if part == "" {
			return "", nil, false
		}
	}
	return parts[1], parts[2:], true
}

func callbackSession(args []string) string {
	if len(args) == 0 {
		return ""
	}
	if len(args) == 1 {
		return args[0]
	}
	if args[0] == "confirm" || args[0] == "yes" || args[0] == "cancel" || args[0] == "no" {
		return args[1]
	}
	return args[0]
}

func callbackSessionPage(args []string) (string, int, bool) {
	if len(args) == 0 {
		return "", 1, true
	}
	if len(args) == 1 {
		page, err := strconv.Atoi(args[0])
		if err == nil {
			return "", page, true
		}
		return args[0], 1, true
	}
	page, err := strconv.Atoi(args[len(args)-1])
	if err != nil {
		return "", 0, false
	}
	return strings.Join(args[:len(args)-1], ":"), page, page > 0
}

func callbackTarget(args []string) (string, string, bool) {
	if len(args) < 1 || len(args) > 2 {
		return "", "", false
	}
	if len(args) == 1 {
		return "", args[0], args[0] != ""
	}
	return args[0], args[1], args[0] != "" && args[1] != ""
}

func adminCallbackData(parts ...string) (string, bool) {
	data := strings.Join(parts, ":")
	return data, len([]byte(data)) <= 64
}
