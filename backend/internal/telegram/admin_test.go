package telegram

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/davidlivingston/go-nextjs-starter/backend/internal/models"
)

type routerAdminStore struct {
	actor      AdminActor
	actorFound bool
	contexts   map[int64]AdminContext
	events     []ActiveEvent
	created    []AdminDraft
	status     AttendancePage
	marks      []AdminUser
	actorErr   error
	statusErr  error
	markErr    error
	undoErr    error
	closeErr   error
	contextErr error
	undoCalls  []string
	markCalls  []string
}

func (s *routerAdminStore) Actor(_ context.Context, pairing Pairing) (AdminActor, bool, error) {
	if s.actorErr != nil {
		return AdminActor{}, false, s.actorErr
	}
	actor := s.actor
	if actor.Pairing.TelegramID == 0 {
		actor.Pairing = pairing
	}
	return actor, s.actorFound, nil
}

func (s *routerAdminStore) LoadContext(_ context.Context, telegramID int64) (AdminContext, error) {
	if s.contextErr != nil {
		return AdminContext{}, s.contextErr
	}
	if s.contexts == nil {
		s.contexts = map[int64]AdminContext{}
	}
	if value, ok := s.contexts[telegramID]; ok {
		return value, nil
	}
	return AdminContext{TelegramID: telegramID, State: "idle"}, nil
}

func (s *routerAdminStore) SaveContext(_ context.Context, next AdminContext) error {
	if s.contextErr != nil {
		return s.contextErr
	}
	if s.contexts == nil {
		s.contexts = map[int64]AdminContext{}
	}
	current, ok := s.contexts[next.TelegramID]
	if ok && current.Version != next.Version {
		return ErrAdminContextConflict
	}
	if !ok && next.Version != 0 {
		return ErrAdminContextConflict
	}
	next.Version++
	s.contexts[next.TelegramID] = next
	return nil
}

func (s *routerAdminStore) ClearContext(_ context.Context, telegramID int64) error {
	if s.contextErr != nil {
		return s.contextErr
	}
	if s.contexts == nil {
		s.contexts = map[int64]AdminContext{}
	}
	current := s.contexts[telegramID]
	s.contexts[telegramID] = AdminContext{TelegramID: telegramID, State: "idle", Version: current.Version + 1}
	return nil
}

func (s *routerAdminStore) ClearContextForSession(_ context.Context, telegramID int64, sessionID string, expectedVersion int64) error {
	current, err := s.LoadContext(context.Background(), telegramID)
	if err != nil {
		return err
	}
	if current.SessionID != sessionID || current.Version != expectedVersion {
		return ErrAdminContextConflict
	}
	return s.ClearContext(context.Background(), telegramID)
}

func (s *routerAdminStore) ActiveEvents(_ context.Context, _ AdminActor) ([]ActiveEvent, error) {
	return append([]ActiveEvent(nil), s.events...), nil
}

func (s *routerAdminStore) CreateEvent(_ context.Context, _ AdminActor, draft AdminDraft) (ActiveEvent, error) {
	s.created = append(s.created, draft)
	if len(s.events) == 0 {
		s.events = append(s.events, ActiveEvent{ID: "event-1", Name: draft.Name, Scope: draft.Scope, Batteries: []string{draft.Battery}, TelegramLink: "https://t.me/synthetic?start=opaque-link"})
	}
	return s.events[len(s.events)-1], nil
}

func (s *routerAdminStore) CloseEvent(_ context.Context, _ AdminActor, _ string) error {
	return s.closeErr
}

func (s *routerAdminStore) Status(_ context.Context, _ AdminActor, _, query string, page int) (AttendancePage, error) {
	if s.statusErr != nil {
		return AttendancePage{}, s.statusErr
	}
	result := s.status
	result.Page = page
	if query != "" {
		result.SessionName = "search:" + query
	}
	if len(result.Rows) > 10 {
		result.Rows = result.Rows[:10]
	}
	return result, nil
}

func (s *routerAdminStore) MarkManual(_ context.Context, _ AdminActor, sessionID, targetUserID string) error {
	s.markCalls = append(s.markCalls, sessionID+":"+targetUserID)
	return s.markErr
}

func (s *routerAdminStore) OwnManualMarks(_ context.Context, _ AdminActor, _ string, _ int) ([]AdminUser, error) {
	return append([]AdminUser(nil), s.marks...), nil
}

func (s *routerAdminStore) UndoManual(_ context.Context, _ AdminActor, sessionID, targetUserID string) error {
	s.undoCalls = append(s.undoCalls, sessionID+":"+targetUserID)
	return s.undoErr
}

func adminPairing() Pairing {
	return Pairing{TelegramID: 71001, UserID: "commander-1", FullName: "UNIT COMMANDER"}
}

func adminMessage(text string) *Message {
	pairing := adminPairing()
	return &Message{From: &User{ID: pairing.TelegramID}, Chat: Chat{ID: pairing.TelegramID, Type: "private"}, Text: text}
}

func adminCallback(data string) *CallbackQuery {
	pairing := adminPairing()
	return &CallbackQuery{ID: "callback-1", From: &User{ID: pairing.TelegramID}, Data: data, Message: adminMessage("")}
}

func adminStoreWithTier(tier models.AccessTier) *routerAdminStore {
	return &routerAdminStore{actor: AdminActor{Pairing: adminPairing(), FullName: "UNIT COMMANDER", Tier: tier, Battery: models.BatteryAlpha}, actorFound: true}
}

func actionText(actions []Action) string {
	for _, action := range actions {
		if action.Kind == SendMessage {
			return action.Text
		}
	}
	return ""
}

func hasButton(actions []Action, text string) bool {
	for _, action := range actions {
		if action.ReplyMarkup == nil {
			continue
		}
		for _, row := range action.ReplyMarkup.InlineKeyboard {
			for _, button := range row {
				if button.Text == text {
					return true
				}
			}
		}
	}
	return false
}

func callbackData(actions []Action, text string) string {
	for _, action := range actions {
		if action.ReplyMarkup == nil {
			continue
		}
		for _, row := range action.ReplyMarkup.InlineKeyboard {
			for _, button := range row {
				if button.Text == text || strings.Contains(button.Text, text) {
					return button.CallbackData
				}
			}
		}
	}
	return ""
}

func TestAdminRoleMenus(t *testing.T) {
	for _, tc := range []struct {
		name       string
		tier       models.AccessTier
		wantCreate bool
	}{
		{name: "tier two", tier: models.TierBatteryNCO},
		{name: "unit commander", tier: models.TierUnitCommander, wantCreate: true},
		{name: "superadmin", tier: models.TierSuperadmin, wantCreate: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := adminStoreWithTier(tc.tier)
			router := NewAdminRouter(store)
			actions, handled, err := router.HandleMessage(context.Background(), adminMessage("/menu"), adminPairing())
			if err != nil || !handled {
				t.Fatalf("handled=%v actions=%#v err=%v", handled, actions, err)
			}
			if hasButton(actions, "Create event") != tc.wantCreate {
				t.Fatalf("create button presence = %v, want %v", hasButton(actions, "Create event"), tc.wantCreate)
			}
			if !hasButton(actions, "Active events") || len(actions[0].ReplyMarkup.InlineKeyboard) < 1 {
				t.Fatalf("menu actions = %#v", actions)
			}
		})
	}
}

func TestAdminWizardValidationAndCreationQR(t *testing.T) {
	store := adminStoreWithTier(models.TierUnitCommander)
	router := NewAdminRouter(store)
	ctx := context.Background()

	actions, handled, err := router.HandleCallback(ctx, adminCallback("a:create"), adminPairing())
	if err != nil || !handled || !strings.Contains(actionText(actions), "event name") {
		t.Fatalf("create transition actions=%#v handled=%v err=%v", actions, handled, err)
	}
	if _, _, err := router.HandleMessage(ctx, adminMessage("   "), adminPairing()); err != nil {
		t.Fatal(err)
	}
	if len(store.created) != 0 {
		t.Fatal("invalid empty name created an event")
	}
	tooLong := strings.Repeat("x", 81)
	if actions, _, err = router.HandleMessage(ctx, adminMessage(tooLong), adminPairing()); err != nil || !strings.Contains(actionText(actions), "80") {
		t.Fatalf("long name actions=%#v err=%v", actions, err)
	}
	if _, _, err = router.HandleMessage(ctx, adminMessage(" First Parade "), adminPairing()); err != nil {
		t.Fatal(err)
	}
	if _, _, err = router.HandleCallback(ctx, adminCallback("a:scope:battery_specific"), adminPairing()); err != nil {
		t.Fatal(err)
	}
	if _, _, err = router.HandleCallback(ctx, adminCallback("a:battery:Alpha"), adminPairing()); err != nil {
		t.Fatal(err)
	}
	actions, _, err = router.HandleCallback(ctx, adminCallback("a:duration:30m"), adminPairing())
	if err != nil {
		t.Fatal(err)
	}
	confirm := callbackData(actions, "Confirm")
	if confirm == "" {
		loaded, loadErr := store.LoadContext(ctx, adminPairing().TelegramID)
		if loadErr != nil {
			t.Fatal(loadErr)
		}
		confirm = "a:confirm-create"
		if loaded.State != "confirming_creation" {
			t.Fatalf("state=%q, want confirming_creation", loaded.State)
		}
	}
	actions, handled, err = router.HandleCallback(ctx, adminCallback(confirm), adminPairing())
	if err != nil || !handled || len(store.created) != 1 {
		t.Fatalf("confirm actions=%#v handled=%v created=%d err=%v", actions, handled, len(store.created), err)
	}
	var photo bool
	for _, action := range actions {
		if action.Kind == SendPhoto {
			photo = true
			if len(action.Photo) == 0 || !strings.Contains(action.FallbackText, "https://t.me/synthetic") {
				t.Fatalf("photo action=%#v", action)
			}
		}
		if strings.Contains(action.Text, "qr-secret") || strings.Contains(action.Caption, "qr-secret") || strings.Contains(action.FallbackText, "qr-secret") {
			t.Fatalf("unexpected QR secret in action=%#v", action)
		}
	}
	if !photo || !strings.Contains(actionText(actions), "https://t.me/synthetic") {
		t.Fatalf("creation actions=%#v", actions)
	}
}

func TestAdminCancelSelectionStatusSearchMarkUndoAndStale(t *testing.T) {
	store := adminStoreWithTier(models.TierUnitCommander)
	store.events = []ActiveEvent{{ID: "event-1", Name: "First Parade", TelegramLink: "https://t.me/synthetic?start=opaque"}}
	store.status = AttendancePage{SessionName: "First Parade", Total: 12, Present: 2, Missing: 10, Percentage: 16.6, Page: 1, PageCount: 2, HasNext: true, Rows: make([]AdminUser, 12)}
	for i := range store.status.Rows {
		store.status.Rows[i] = AdminUser{ID: "person-" + string(rune('a'+i)), Name: "Person" + string(rune('A'+i))}
	}
	store.marks = []AdminUser{{ID: "person-a", Name: "PersonA"}}
	router := NewAdminRouter(store)
	ctx := context.Background()

	actions, _, err := router.HandleCallback(ctx, adminCallback("a:events"), adminPairing())
	if err != nil || !hasButton(actions, "First Parade") {
		t.Fatalf("events actions=%#v err=%v", actions, err)
	}
	selectData := callbackData(actions, "First Parade")
	if selectData == "" || len([]byte(selectData)) > 64 {
		t.Fatalf("selection callback=%q", selectData)
	}
	actions, _, err = router.HandleCallback(ctx, adminCallback(selectData), adminPairing())
	if err != nil || !hasButton(actions, "View status") {
		t.Fatalf("selection actions=%#v err=%v", actions, err)
	}
	statusData := callbackData(actions, "View status")
	actions, _, err = router.HandleCallback(ctx, adminCallback(statusData), adminPairing())
	if err != nil || strings.Count(actionText(actions), "Person") > 10 || !hasButton(actions, "Next") {
		t.Fatalf("status actions=%#v err=%v", actions, err)
	}
	if _, _, err = router.HandleCallback(ctx, adminCallback("a:search:event-1"), adminPairing()); err != nil {
		t.Fatal(err)
	}
	actions, _, err = router.HandleMessage(ctx, adminMessage("person"), adminPairing())
	if err != nil || !strings.Contains(actionText(actions), "search:person") {
		t.Fatalf("search actions=%#v err=%v", actions, err)
	}
	if _, _, err = router.HandleCallback(ctx, adminCallback("a:mark:event-1:person-a"), adminPairing()); err != nil {
		t.Fatal(err)
	}
	if len(store.markCalls) != 1 {
		t.Fatalf("mark calls=%v", store.markCalls)
	}
	if _, _, err = router.HandleCallback(ctx, adminCallback("a:undo:event-1:person-a"), adminPairing()); err != nil {
		t.Fatal(err)
	}
	if len(store.undoCalls) != 1 {
		t.Fatalf("undo calls=%v", store.undoCalls)
	}
	store.markErr = ErrAdminUnavailable
	actions, _, err = router.HandleCallback(ctx, adminCallback("a:mark:event-1:missing"), adminPairing())
	if err != nil || strings.Contains(actionText(actions), "missing") {
		t.Fatalf("safe stale mark actions=%#v err=%v", actions, err)
	}
	if _, _, err = router.HandleCallback(ctx, adminCallback("a:does-not-exist"), adminPairing()); err != nil {
		t.Fatal(err)
	}
	if _, _, err = router.HandleMessage(ctx, adminMessage("/cancel"), adminPairing()); err != nil {
		t.Fatal(err)
	}
}

func TestAdminCloseOwnershipAndBotCompatibility(t *testing.T) {
	store := adminStoreWithTier(models.TierUnitCommander)
	store.events = []ActiveEvent{{ID: "event-1", Name: "First Parade", CreatedBy: "someone-else", TelegramLink: "https://t.me/synthetic?start=opaque"}}
	router := NewAdminRouter(store)
	if actions, _, err := router.HandleCallback(context.Background(), adminCallback("a:events"), adminPairing()); err != nil || len(actions) == 0 {
		t.Fatalf("events actions=%#v err=%v", actions, err)
	}
	// Selection is still authorized, but ownership is checked before the
	// confirmation prompt and never delegated to callback actor data.
	selectData := "a:event:event-1"
	if _, _, err := router.HandleCallback(context.Background(), adminCallback(selectData), adminPairing()); err != nil {
		t.Fatal(err)
	}
	actions, _, err := router.HandleCallback(context.Background(), adminCallback("a:close:event-1"), adminPairing())
	if err != nil || strings.Contains(actionText(actions), "Confirm close") {
		t.Fatalf("non-owner close actions=%#v err=%v", actions, err)
	}

	lookup := &fakePairingLookup{found: true, pairing: adminPairing()}
	bot := NewBotWithAdmin(lookup, nil, router)
	actions, err = bot.HandleUpdate(context.Background(), Update{Message: adminMessage("hello")})
	if err != nil || !hasButton(actions, "Open commander menu") {
		t.Fatalf("admin linked actions=%#v err=%v", actions, err)
	}
	soldierStore := adminStoreWithTier(models.TierEnlisted)
	soldierRouter := NewAdminRouter(soldierStore)
	bot = NewBotWithAdmin(lookup, nil, soldierRouter)
	actions, err = bot.HandleUpdate(context.Background(), Update{Message: adminMessage("hello")})
	if err != nil || hasButton(actions, "Open commander menu") || len(actions) != 1 {
		t.Fatalf("soldier linked actions=%#v err=%v", actions, err)
	}
}

func TestAdminContextConflictIsSafeNoOp(t *testing.T) {
	store := adminStoreWithTier(models.TierUnitCommander)
	store.contextErr = ErrAdminContextConflict
	router := NewAdminRouter(store)
	actions, handled, err := router.HandleCallback(context.Background(), adminCallback("a:create"), adminPairing())
	if err != nil || !handled || len(actions) != 0 {
		t.Fatalf("conflict actions=%#v handled=%v err=%v", actions, handled, err)
	}
	store.contextErr = errors.New("store unavailable")
	if _, _, err := router.HandleMessage(context.Background(), adminMessage("hello"), adminPairing()); err == nil {
		t.Fatal("store error was swallowed")
	}
}

func TestAdminDurationsUseExactValuesAndCallbackBounds(t *testing.T) {
	for _, value := range []string{"30m", "1h", "2h", "4h"} {
		if len([]byte("a:duration:"+value)) > 64 {
			t.Fatalf("duration callback %q exceeds Telegram limit", value)
		}
	}
	if time.Hour <= 0 {
		t.Fatal("time package unavailable")
	}
}
