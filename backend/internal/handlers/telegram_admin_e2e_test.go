package handlers

import (
	"bytes"
	"context"
	"fmt"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/davidlivingston/go-nextjs-starter/backend/internal/database"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/models"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/telegram"
)

// telegramAdminE2ESender exercises the same transport interfaces used by the
// production dispatcher without contacting Telegram. In addition to Sender,
// it implements the optional markupSender and PhotoSender methods so this E2E
// observes callback acknowledgements, inline keyboards, and QR PNG delivery.
type telegramAdminE2ESender struct {
	mu        sync.Mutex
	messages  []telegramAdminE2EMessage
	photos    []telegramAdminE2EPhoto
	callbacks []string
}

type telegramAdminE2EMessage struct {
	ChatID int64
	Text   string
	Markup *telegram.InlineKeyboardMarkup
}

type telegramAdminE2EPhoto struct {
	ChatID  int64
	Photo   []byte
	Caption string
	Markup  *telegram.InlineKeyboardMarkup
}

var _ telegram.Sender = (*telegramAdminE2ESender)(nil)
var _ telegram.PhotoSender = (*telegramAdminE2ESender)(nil)

func (s *telegramAdminE2ESender) SendMessage(_ context.Context, chatID int64, text string) error {
	return s.recordMessage(chatID, text, nil)
}

// SendMessageWithMarkup is the optional markupSender method selected by the
// dispatcher when an action contains an inline keyboard.
func (s *telegramAdminE2ESender) SendMessageWithMarkup(_ context.Context, chatID int64, text string, markup *telegram.InlineKeyboardMarkup) error {
	return s.recordMessage(chatID, text, markup)
}

func (s *telegramAdminE2ESender) recordMessage(chatID int64, text string, markup *telegram.InlineKeyboardMarkup) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = append(s.messages, telegramAdminE2EMessage{ChatID: chatID, Text: text, Markup: markup})
	return nil
}

func (s *telegramAdminE2ESender) AnswerCallbackQuery(_ context.Context, callbackQueryID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.callbacks = append(s.callbacks, callbackQueryID)
	return nil
}

func (s *telegramAdminE2ESender) SendPhoto(_ context.Context, chatID int64, photo []byte, caption string, markup *telegram.InlineKeyboardMarkup) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.photos = append(s.photos, telegramAdminE2EPhoto{
		ChatID: chatID, Photo: append([]byte(nil), photo...), Caption: caption, Markup: markup,
	})
	return nil
}

func (s *telegramAdminE2ESender) deliveryCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.messages) + len(s.photos) + len(s.callbacks)
}

func (s *telegramAdminE2ESender) callbackCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.callbacks)
}

func (s *telegramAdminE2ESender) photoSnapshot() []telegramAdminE2EPhoto {
	s.mu.Lock()
	defer s.mu.Unlock()
	photos := make([]telegramAdminE2EPhoto, len(s.photos))
	copy(photos, s.photos)
	return photos
}

func TestTelegramAdminE2EThroughBotHandleUpdate(t *testing.T) {
	t.Setenv("TELEGRAM_BOT_USERNAME", "synthetic_tg_admin_bot")
	db, prefix := openTelegramAdminDB(t)
	ctx := context.Background()

	creatorID := prefix + "-c"
	tierTwoID := prefix + "-2"
	otherCreatorID := prefix + "-o"
	superadminID := prefix + "-s"
	soldierID := prefix + "-x"
	alphaTargetID := prefix + "-a"
	creatorMarkID := prefix + "-b"
	bravoTargetID := prefix + "-v"

	seedUser(t, db, creatorID, "T3 E2E CREATOR", models.RankSSG, models.BatteryAlpha, "", true)
	seedUser(t, db, tierTwoID, "T2 E2E NCO", models.Rank3SG, models.BatteryAlpha, "", true)
	seedUser(t, db, otherCreatorID, "T3 E2E OTHER", models.RankSSG, models.BatteryBravo, "", true)
	seedUser(t, db, superadminID, "SUPER E2E ADMIN", models.RankCPT, models.BatteryHQ, "", true)
	seedUser(t, db, soldierID, "PTE E2E SOLDIER", models.RankPTE, models.BatteryAlpha, "", true)
	seedUser(t, db, alphaTargetID, "PTE E2E ALPHA TARGET", models.RankPTE, models.BatteryAlpha, "", true)
	seedUser(t, db, creatorMarkID, "PTE E2E CREATOR MARK", models.RankPTE, models.BatteryAlpha, "", true)
	seedUser(t, db, bravoTargetID, "PTE E2E BRAVO TARGET", models.RankPTE, models.BatteryBravo, "", true)
	if _, err := db.Pool.Exec(ctx, `UPDATE "user" SET is_superadmin = true WHERE id = $1`, superadminID); err != nil {
		t.Fatal(err)
	}

	creatorTelegramID := telegramAdminID(prefix, 30)
	tierTwoTelegramID := telegramAdminID(prefix, 31)
	otherCreatorTelegramID := telegramAdminID(prefix, 32)
	superadminTelegramID := telegramAdminID(prefix, 33)
	soldierTelegramID := telegramAdminID(prefix, 34)
	seedTelegramAdminPairing(t, db, prefix+"-pc", creatorTelegramID, creatorID)
	seedTelegramAdminPairing(t, db, prefix+"-p2", tierTwoTelegramID, tierTwoID)
	seedTelegramAdminPairing(t, db, prefix+"-po", otherCreatorTelegramID, otherCreatorID)
	seedTelegramAdminPairing(t, db, prefix+"-ps", superadminTelegramID, superadminID)
	seedTelegramAdminPairing(t, db, prefix+"-px", soldierTelegramID, soldierID)

	// These independent active fixtures exercise Tier 2's event and roster
	// boundaries instead of relying only on the event created by the wizard.
	bravoOnlyEventID := fmt.Sprintf("%d-b", telegramAdminID(prefix, 101))
	unitWideEventID := fmt.Sprintf("%d-u", telegramAdminID(prefix, 102))
	seedAdminSession(t, db, bravoOnlyEventID, "TG E2E Bravo-only Event", models.SessionScopeBatterySpecific, []string{models.BatteryBravo}, otherCreatorID, time.Hour)
	seedAdminSession(t, db, unitWideEventID, "TG E2E Unit-wide Event", models.SessionScopeUnitWide, nil, otherCreatorID, time.Hour)

	store := NewTelegramAdminStore(db, nil)
	sender := &telegramAdminE2ESender{}
	dispatcher := telegram.NewDispatcher(sender, 128)
	defer dispatcher.Close()
	bot := telegram.NewBotWithAdmin(store, store, telegram.NewAdminRouter(store))

	// Tier 3 creation: one wizard uses one database session, persists its
	// selected context, and delivers both a URL button and a PNG photo.
	menu := driveTelegramAdminE2E(t, bot, dispatcher, sender, telegramAdminE2EMessageUpdate(creatorTelegramID, "/menu"))
	if !telegramAdminE2EHasButton(menu, "Create event") {
		t.Fatalf("creator menu = %#v, want Create event", menu)
	}
	driveTelegramAdminE2E(t, bot, dispatcher, sender, telegramAdminE2ECallbackUpdate(creatorTelegramID, "a:create"))
	driveTelegramAdminE2E(t, bot, dispatcher, sender, telegramAdminE2EMessageUpdate(creatorTelegramID, "  TG E2E Battery Parade  "))
	driveTelegramAdminE2E(t, bot, dispatcher, sender, telegramAdminE2ECallbackUpdate(creatorTelegramID, "a:scope:"+models.SessionScopeBatterySpecific))
	driveTelegramAdminE2E(t, bot, dispatcher, sender, telegramAdminE2ECallbackUpdate(creatorTelegramID, "a:battery:"+models.BatteryAlpha))
	review := driveTelegramAdminE2E(t, bot, dispatcher, sender, telegramAdminE2ECallbackUpdate(creatorTelegramID, "a:duration:1h"))
	confirm := telegramAdminE2EButton(t, review, "Confirm")
	beforeCreate := countTelegramAdminSessions(t, db, creatorID)
	createdActions := driveTelegramAdminE2E(t, bot, dispatcher, sender, telegramAdminE2ECallbackUpdate(creatorTelegramID, confirm.CallbackData))
	afterCreate := countTelegramAdminSessions(t, db, creatorID)
	if afterCreate != beforeCreate+1 {
		t.Fatalf("session count after confirmation = %d, want %d", afterCreate, beforeCreate+1)
	}
	createdID := latestTelegramAdminSession(t, db, creatorID)
	if !telegramAdminE2EHasButton(createdActions, "Open attendance link") {
		t.Fatalf("created actions = %#v, want URL button", createdActions)
	}
	photoAction := telegramAdminE2EAction(t, createdActions, telegram.SendPhoto)
	if len(photoAction.Photo) == 0 {
		t.Fatal("created event did not produce a QR photo action")
	}
	if _, err := png.DecodeConfig(bytes.NewReader(photoAction.Photo)); err != nil {
		t.Fatalf("created QR is not a PNG: %v", err)
	}
	link := telegramAdminE2EURL(t, createdActions)
	parsedLink, err := url.Parse(link)
	if err != nil || parsedLink.Query().Get("start") == "" {
		t.Fatalf("created Telegram link = %q, want opaque start code", link)
	}
	var (
		persistedName      string
		persistedScope     string
		persistedBatteries []string
		persistedStatus    string
		persistedCreator   string
		persistedEndTime   *time.Time
		persistedDeepLink  *string
	)
	if err := db.Pool.QueryRow(ctx, `
		SELECT name, scope, batteries, status, created_by, end_time, deeplink_code
		FROM attendance_session WHERE id = $1
	`, createdID).Scan(
		&persistedName, &persistedScope, &persistedBatteries, &persistedStatus,
		&persistedCreator, &persistedEndTime, &persistedDeepLink,
	); err != nil {
		t.Fatalf("load persisted created session: %v", err)
	}
	if persistedName != "TG E2E Battery Parade" {
		t.Fatalf("persisted session name = %q, want trimmed event name", persistedName)
	}
	if persistedScope != models.SessionScopeBatterySpecific {
		t.Fatalf("persisted session scope = %q, want battery_specific", persistedScope)
	}
	if len(persistedBatteries) != 1 || persistedBatteries[0] != models.BatteryAlpha {
		t.Fatalf("persisted session batteries = %v, want exactly [%s]", persistedBatteries, models.BatteryAlpha)
	}
	if persistedStatus != models.SessionStatusActive {
		t.Fatalf("persisted session status = %q, want active", persistedStatus)
	}
	if persistedCreator != creatorID {
		t.Fatalf("persisted session creator = %q, want %q", persistedCreator, creatorID)
	}
	if persistedEndTime == nil || persistedEndTime.IsZero() || !persistedEndTime.After(time.Now()) {
		t.Fatalf("persisted session end_time = %v, want nonzero future time", persistedEndTime)
	}
	if persistedDeepLink == nil || strings.TrimSpace(*persistedDeepLink) == "" {
		t.Fatalf("persisted session deeplink_code = %v, want nonempty code", persistedDeepLink)
	}
	if parsedLink.Query().Get("start") != *persistedDeepLink {
		t.Fatalf("Telegram URL start payload = %q, want stored deeplink_code %q", parsedLink.Query().Get("start"), *persistedDeepLink)
	}
	if photoAction.ReplyMarkup == nil || telegramAdminE2EURLFromMarkup(photoAction.ReplyMarkup) != link {
		t.Fatalf("photo markup = %#v, want URL button %q", photoAction.ReplyMarkup, link)
	}
	deliveredPhotos := sender.photoSnapshot()
	if len(deliveredPhotos) == 0 || !bytes.Equal(deliveredPhotos[len(deliveredPhotos)-1].Photo, photoAction.Photo) {
		t.Fatalf("fake Telegram photo deliveries = %d, want the created QR PNG", len(deliveredPhotos))
	}
	if telegramAdminE2EURLFromMarkup(deliveredPhotos[len(deliveredPhotos)-1].Markup) != link {
		t.Fatalf("delivered photo markup = %#v, want URL button %q", deliveredPhotos[len(deliveredPhotos)-1].Markup, link)
	}
	if telegramAdminE2EAction(t, createdActions, telegram.SendMessage).ReplyMarkup == nil {
		t.Fatal("link message did not carry inline URL markup")
	}

	// A repeated confirmation is a no-op after the durable state reservation;
	// it cannot create a second session row.
	repeated := driveTelegramAdminE2E(t, bot, dispatcher, sender, telegramAdminE2ECallbackUpdate(creatorTelegramID, confirm.CallbackData))
	if len(repeated) != 1 || repeated[0].Kind != telegram.AnswerCallbackQuery {
		t.Fatalf("repeated creation confirmation actions = %#v, want acknowledgement only", repeated)
	}
	if got := countTelegramAdminSessions(t, db, creatorID); got != afterCreate {
		t.Fatalf("session count after repeated confirmation = %d, want %d", got, afterCreate)
	}
	creatorContext, err := store.LoadContext(ctx, creatorTelegramID)
	if err != nil {
		t.Fatal(err)
	}
	if creatorContext.SessionID != createdID || creatorContext.State != "selected_session" {
		t.Fatalf("creator context = %+v, want selected session %s", creatorContext, createdID)
	}

	// A new store and router instance must recover the selected session from
	// PostgreSQL rather than process memory.
	reloadedStore := NewTelegramAdminStore(db, nil)
	reloadedBot := telegram.NewBotWithAdmin(reloadedStore, reloadedStore, telegram.NewAdminRouter(reloadedStore))
	reloadedEvents := driveTelegramAdminE2E(t, reloadedBot, dispatcher, sender, telegramAdminE2ECallbackUpdate(creatorTelegramID, "a:events"))
	reloadedSelect := telegramAdminE2EButton(t, reloadedEvents, "TG E2E Battery Parade")
	reloadedSelected := driveTelegramAdminE2E(t, reloadedBot, dispatcher, sender, telegramAdminE2ECallbackUpdate(creatorTelegramID, reloadedSelect.CallbackData))
	statusButton := telegramAdminE2EButton(t, reloadedSelected, "View status")
	reloadedStatus := driveTelegramAdminE2E(t, reloadedBot, dispatcher, sender, telegramAdminE2ECallbackUpdate(creatorTelegramID, statusButton.CallbackData))
	if !strings.Contains(telegramAdminE2EAction(t, reloadedStatus, telegram.SendMessage).Text, "TG E2E Battery Parade") {
		t.Fatalf("reloaded status = %#v, want selected event name", reloadedStatus)
	}
	reloadedContext, err := reloadedStore.LoadContext(ctx, creatorTelegramID)
	if err != nil {
		t.Fatal(err)
	}
	if reloadedContext.SessionID != createdID || reloadedContext.State != "viewing_status" {
		t.Fatalf("reloaded context = %+v, want viewing_status for %s", reloadedContext, createdID)
	}

	// Tier 2 has the active-event/read/manual surface but cannot create or
	// close, and its status rows are restricted to its own battery.
	creatorActor, found, err := reloadedStore.Actor(ctx, telegram.Pairing{TelegramID: creatorTelegramID, UserID: creatorID})
	if err != nil || !found {
		t.Fatalf("creator actor = %+v found=%v err=%v", creatorActor, found, err)
	}
	if err := reloadedStore.MarkManual(ctx, creatorActor, createdID, creatorMarkID); err != nil {
		t.Fatalf("seed creator-owned manual mark: %v", err)
	}
	tierTwoMenu := driveTelegramAdminE2E(t, reloadedBot, dispatcher, sender, telegramAdminE2EMessageUpdate(tierTwoTelegramID, "/menu"))
	if telegramAdminE2EHasButton(tierTwoMenu, "Create event") {
		t.Fatalf("tier-two menu = %#v, must not expose Create event", tierTwoMenu)
	}
	tierTwoSessionCount := countTelegramAdminSessions(t, db, creatorID)
	tierTwoForbiddenCreate := driveTelegramAdminE2E(t, reloadedBot, dispatcher, sender, telegramAdminE2ECallbackUpdate(tierTwoTelegramID, "a:create"))
	if !strings.Contains(telegramAdminE2EActionText(tierTwoForbiddenCreate), "That action is no longer available.") || strings.Contains(telegramAdminE2EActionText(tierTwoForbiddenCreate), "TG E2E Battery Parade") {
		t.Fatalf("tier-two forbidden create = %#v, want safe response without event data", tierTwoForbiddenCreate)
	}
	if got := countTelegramAdminSessions(t, db, creatorID); got != tierTwoSessionCount {
		t.Fatalf("tier-two forbidden create changed session count from %d to %d", tierTwoSessionCount, got)
	}
	tierTwoEvents := driveTelegramAdminE2E(t, reloadedBot, dispatcher, sender, telegramAdminE2ECallbackUpdate(tierTwoTelegramID, "a:events"))
	if !telegramAdminE2EHasButton(tierTwoEvents, "TG E2E Battery Parade") || !telegramAdminE2EHasButton(tierTwoEvents, "TG E2E Unit-wide Event") || telegramAdminE2EHasButton(tierTwoEvents, "TG E2E Bravo-only Event") {
		t.Fatalf("tier-two active events = %#v, want created/Unit-wide but not Bravo-only", tierTwoEvents)
	}
	tierTwoSelect := telegramAdminE2EButton(t, tierTwoEvents, "TG E2E Battery Parade")
	selectParts := strings.Split(tierTwoSelect.CallbackData, ":")
	if len(selectParts) != 3 {
		t.Fatalf("tier-two event selection callback = %q", tierTwoSelect.CallbackData)
	}
	forgedTierTwoClose := driveTelegramAdminE2E(t, reloadedBot, dispatcher, sender, telegramAdminE2ECallbackUpdate(tierTwoTelegramID, "a:close:"+selectParts[2]))
	if !strings.Contains(telegramAdminE2EActionText(forgedTierTwoClose), "That action is no longer available.") || strings.Contains(telegramAdminE2EActionText(forgedTierTwoClose), "TG E2E Battery Parade") {
		t.Fatalf("tier-two forged close = %#v, want safe response without event data", forgedTierTwoClose)
	}
	assertTelegramAdminSessionStatus(t, db, createdID, models.SessionStatusActive)

	unitWideSelect := telegramAdminE2EButton(t, tierTwoEvents, "TG E2E Unit-wide Event")
	unitWideSelected := driveTelegramAdminE2E(t, reloadedBot, dispatcher, sender, telegramAdminE2ECallbackUpdate(tierTwoTelegramID, unitWideSelect.CallbackData))
	unitWideStatusButton := telegramAdminE2EButton(t, unitWideSelected, "View status")
	unitWideStatus := driveTelegramAdminE2E(t, reloadedBot, dispatcher, sender, telegramAdminE2ECallbackUpdate(tierTwoTelegramID, unitWideStatusButton.CallbackData))
	unitWideStatusText := telegramAdminE2EActionText(unitWideStatus)
	if !strings.Contains(unitWideStatusText, "PTE E2E ALPHA TARGET") || strings.Contains(unitWideStatusText, "PTE E2E BRAVO TARGET") {
		t.Fatalf("tier-two unit-wide status = %q, want Alpha but not Bravo rows", unitWideStatusText)
	}

	tierTwoSelected := driveTelegramAdminE2E(t, reloadedBot, dispatcher, sender, telegramAdminE2ECallbackUpdate(tierTwoTelegramID, tierTwoSelect.CallbackData))
	if telegramAdminE2EHasButton(tierTwoSelected, "Close event") {
		t.Fatalf("tier-two selected event = %#v, must not expose Close event", tierTwoSelected)
	}
	tierTwoStatusButton := telegramAdminE2EButton(t, tierTwoSelected, "View status")
	tierTwoStatus := driveTelegramAdminE2E(t, reloadedBot, dispatcher, sender, telegramAdminE2ECallbackUpdate(tierTwoTelegramID, tierTwoStatusButton.CallbackData))
	tierTwoStatusText := telegramAdminE2EActionText(tierTwoStatus)
	if !strings.Contains(tierTwoStatusText, "PTE E2E ALPHA TARGET") || strings.Contains(tierTwoStatusText, "PTE E2E BRAVO TARGET") {
		t.Fatalf("tier-two status = %q, want only Alpha missing rows", tierTwoStatusText)
	}
	if countTelegramAdminRows(t, db, createdID, alphaTargetID) != 0 {
		t.Fatal("Alpha target unexpectedly marked before Tier 2 action")
	}
	if countTelegramAdminRows(t, db, createdID, bravoTargetID) != 0 {
		t.Fatal("Bravo target unexpectedly marked before Tier 2 action")
	}

	alphaMark := telegramAdminE2EButton(t, tierTwoStatus, "Mark PTE E2E ALPHA TARGET")
	markPrompt := driveTelegramAdminE2E(t, reloadedBot, dispatcher, sender, telegramAdminE2ECallbackUpdate(tierTwoTelegramID, alphaMark.CallbackData))
	markConfirm := telegramAdminE2EButton(t, markPrompt, "Mark present")
	marked := driveTelegramAdminE2E(t, reloadedBot, dispatcher, sender, telegramAdminE2ECallbackUpdate(tierTwoTelegramID, markConfirm.CallbackData))
	if !strings.Contains(telegramAdminE2EActionText(marked), "Soldier marked present") {
		t.Fatalf("manual mark result = %#v", marked)
	}
	assertTelegramAdminAttendance(t, db, createdID, alphaTargetID, models.MarkingMethodManual, tierTwoID)
	if countTelegramAdminRows(t, db, createdID, bravoTargetID) != 0 {
		t.Fatal("Tier 2 marked a Bravo target")
	}

	tierTwoMarks := driveTelegramAdminE2E(t, reloadedBot, dispatcher, sender, telegramAdminE2ECallbackUpdate(tierTwoTelegramID, telegramAdminE2EButton(t, tierTwoSelected, "My manual marks").CallbackData))
	if !strings.Contains(telegramAdminE2EActionText(tierTwoMarks), "PTE E2E ALPHA TARGET") || strings.Contains(telegramAdminE2EActionText(tierTwoMarks), "PTE E2E CREATOR MARK") {
		t.Fatalf("Tier 2 own marks = %#v, want only own manual row", tierTwoMarks)
	}
	ownUndo := telegramAdminE2EButton(t, tierTwoMarks, "Undo PTE E2E ALPHA TARGET")
	parts := strings.Split(ownUndo.CallbackData, ":")
	if len(parts) != 4 {
		t.Fatalf("own undo callback = %q", ownUndo.CallbackData)
	}
	otherUndo := strings.Join([]string{parts[0], parts[1], parts[2], creatorMarkID}, ":")
	forgedUndo := driveTelegramAdminE2E(t, reloadedBot, dispatcher, sender, telegramAdminE2ECallbackUpdate(tierTwoTelegramID, otherUndo))
	if !strings.Contains(telegramAdminE2EActionText(forgedUndo), "no longer available") {
		t.Fatalf("Tier 2 forged undo = %#v, want safe response", forgedUndo)
	}
	assertTelegramAdminAttendance(t, db, createdID, creatorMarkID, models.MarkingMethodManual, creatorID)
	if countTelegramAdminRows(t, db, createdID, alphaTargetID) != 1 {
		t.Fatal("forged undo removed another commander's manual mark or own mark unexpectedly")
	}
	undone := driveTelegramAdminE2E(t, reloadedBot, dispatcher, sender, telegramAdminE2ECallbackUpdate(tierTwoTelegramID, ownUndo.CallbackData))
	if !strings.Contains(telegramAdminE2EActionText(undone), "Manual mark undone") {
		t.Fatalf("own undo = %#v", undone)
	}
	if countTelegramAdminRows(t, db, createdID, alphaTargetID) != 0 {
		t.Fatal("own manual mark remained after undo")
	}
	assertTelegramAdminAttendance(t, db, createdID, creatorMarkID, models.MarkingMethodManual, creatorID)

	// A paired soldier remains on the existing /start deep-link path while the
	// same Bot wiring carries the admin router.
	soldierCode := parsedLink.Query().Get("start")
	soldierFirst := driveTelegramAdminE2E(t, bot, dispatcher, sender, telegramAdminE2EMessageUpdate(soldierTelegramID, "/start "+soldierCode))
	if !strings.Contains(telegramAdminE2EActionText(soldierFirst), "Attendance marked for TG E2E Battery Parade.") {
		t.Fatalf("paired soldier first /start = %#v", soldierFirst)
	}
	soldierSecond := driveTelegramAdminE2E(t, bot, dispatcher, sender, telegramAdminE2EMessageUpdate(soldierTelegramID, "/start "+soldierCode))
	if !strings.Contains(telegramAdminE2EActionText(soldierSecond), "already marked for TG E2E Battery Parade.") {
		t.Fatalf("paired soldier repeated /start = %#v", soldierSecond)
	}
	assertTelegramAdminAttendance(t, db, createdID, soldierID, models.MarkingMethodTelegramScan, "")
	if countTelegramAdminRows(t, db, createdID, soldierID) != 1 {
		t.Fatal("paired soldier /start created duplicate attendance rows")
	}

	// A different Tier 3 creator can select the event but cannot close another
	// creator's event. A superadmin can close it.
	otherEvents := driveTelegramAdminE2E(t, reloadedBot, dispatcher, sender, telegramAdminE2ECallbackUpdate(otherCreatorTelegramID, "a:events"))
	otherSelect := telegramAdminE2EButton(t, otherEvents, "TG E2E Battery Parade")
	otherSelected := driveTelegramAdminE2E(t, reloadedBot, dispatcher, sender, telegramAdminE2ECallbackUpdate(otherCreatorTelegramID, otherSelect.CallbackData))
	if telegramAdminE2EHasButton(otherSelected, "Close event") {
		t.Fatalf("other creator selected menu = %#v, must not expose Close event", otherSelected)
	}
	closeParts := strings.Split(otherSelect.CallbackData, ":")
	if len(closeParts) != 3 {
		t.Fatalf("event selection callback = %q", otherSelect.CallbackData)
	}
	otherClose := driveTelegramAdminE2E(t, reloadedBot, dispatcher, sender, telegramAdminE2ECallbackUpdate(otherCreatorTelegramID, "a:close:"+closeParts[2]))
	if !strings.Contains(telegramAdminE2EActionText(otherClose), "no longer available") {
		t.Fatalf("other creator close = %#v, want safe response", otherClose)
	}
	assertTelegramAdminSessionStatus(t, db, createdID, models.SessionStatusActive)

	superEvents := driveTelegramAdminE2E(t, reloadedBot, dispatcher, sender, telegramAdminE2ECallbackUpdate(superadminTelegramID, "a:events"))
	superSelect := telegramAdminE2EButton(t, superEvents, "TG E2E Battery Parade")
	superSelected := driveTelegramAdminE2E(t, reloadedBot, dispatcher, sender, telegramAdminE2ECallbackUpdate(superadminTelegramID, superSelect.CallbackData))
	superClose := telegramAdminE2EButton(t, superSelected, "Close event")
	closePrompt := driveTelegramAdminE2E(t, reloadedBot, dispatcher, sender, telegramAdminE2ECallbackUpdate(superadminTelegramID, superClose.CallbackData))
	closeConfirm := telegramAdminE2EButton(t, closePrompt, "Confirm close")
	closed := driveTelegramAdminE2E(t, reloadedBot, dispatcher, sender, telegramAdminE2ECallbackUpdate(superadminTelegramID, closeConfirm.CallbackData))
	if !strings.Contains(telegramAdminE2EActionText(closed), "Event closed") {
		t.Fatalf("superadmin close = %#v", closed)
	}
	assertTelegramAdminSessionStatus(t, db, createdID, models.SessionStatusClosed)
	closedContextStore := NewTelegramAdminStore(db, nil)
	superadminContext, err := closedContextStore.LoadContext(ctx, superadminTelegramID)
	if err != nil {
		t.Fatalf("reload superadmin context after close: %v", err)
	}
	if superadminContext.State != "idle" || superadminContext.SessionID != "" {
		t.Fatalf("superadmin context after close = %+v, want idle state with empty session", superadminContext)
	}

	// The old Tier 2 mark callback is stale after the superadmin close. It is
	// acknowledged safely without presenting a confirmation or exposing names.
	stale := driveTelegramAdminE2E(t, reloadedBot, dispatcher, sender, telegramAdminE2ECallbackUpdate(tierTwoTelegramID, alphaMark.CallbackData))
	if !strings.Contains(telegramAdminE2EActionText(stale), "no longer available") || telegramAdminE2EFindButton(stale, "Mark present") != "" {
		t.Fatalf("stale callback response = %#v, want safe response without confirmation", stale)
	}
	if strings.Contains(telegramAdminE2EActionText(stale), "PTE E2E ALPHA TARGET") || strings.Contains(telegramAdminE2EActionText(stale), "TG E2E Battery Parade") {
		t.Fatalf("stale callback leaked resource names: %#v", stale)
	}
	if countTelegramAdminRows(t, db, createdID, alphaTargetID) != 0 {
		t.Fatal("stale callback mutated attendance after closure")
	}

	// Malformed, unknown-account, and group updates must not enter the admin
	// surface. A wrong webhook secret is also rejected before Bot.HandleUpdate.
	if actions := driveTelegramAdminE2E(t, reloadedBot, dispatcher, sender, telegram.Update{}); len(actions) != 0 {
		t.Fatalf("malformed empty update actions = %#v", actions)
	}
	unknown := driveTelegramAdminE2E(t, reloadedBot, dispatcher, sender, telegramAdminE2EMessageUpdate(telegramAdminID(prefix, 40), "/menu"))
	if telegramAdminE2EHasButton(unknown, "Create event") || strings.Contains(telegramAdminE2EActionText(unknown), "T3 E2E CREATOR") {
		t.Fatalf("unknown account admin actions/name leakage = %#v", unknown)
	}
	group := telegram.Update{Message: &telegram.Message{
		From: &telegram.User{ID: creatorTelegramID}, Chat: telegram.Chat{ID: -creatorTelegramID, Type: "group"}, Text: "/menu",
	}}
	if actions := driveTelegramAdminE2E(t, reloadedBot, dispatcher, sender, group); len(actions) != 0 {
		t.Fatalf("group admin actions = %#v", actions)
	}
	malformedCallback := driveTelegramAdminE2E(t, reloadedBot, dispatcher, sender, telegramAdminE2ECallbackUpdate(creatorTelegramID, "a:%%%"))
	if len(malformedCallback) != 1 || malformedCallback[0].Kind != telegram.AnswerCallbackQuery {
		t.Fatalf("malformed callback actions = %#v, want acknowledgement only", malformedCallback)
	}

	unauthenticatedBefore := sender.deliveryCount()
	unauthenticatedHandler := NewTelegramHandler(reloadedBot, "synthetic-admin-webhook-secret", dispatcher)
	unauthenticatedRequest := httptest.NewRequest(http.MethodPost, "/api/telegram/webhook/synthetic", strings.NewReader(`{"message":{"from":{"id":`+fmt.Sprint(creatorTelegramID)+`},"chat":{"id":`+fmt.Sprint(creatorTelegramID)+`,"type":"private"},"text":"/menu"}}`))
	unauthenticatedRequest.Header.Set(telegramSecretHeader, "wrong-secret")
	unauthenticatedResponse := httptest.NewRecorder()
	unauthenticatedHandler.Webhook(unauthenticatedResponse, unauthenticatedRequest)
	if unauthenticatedResponse.Code == http.StatusOK || sender.deliveryCount() != unauthenticatedBefore {
		t.Fatalf("wrong-secret webhook status=%d deliveries=%d, want rejection without action", unauthenticatedResponse.Code, sender.deliveryCount()-unauthenticatedBefore)
	}
	if sender.callbackCount() == 0 || len(sender.photoSnapshot()) == 0 {
		t.Fatal("fake Telegram sender did not observe callback acknowledgements and QR photo delivery")
	}
}

func driveTelegramAdminE2E(t *testing.T, bot *telegram.Bot, dispatcher *telegram.Dispatcher, sender *telegramAdminE2ESender, update telegram.Update) []telegram.Action {
	t.Helper()
	actions, err := bot.HandleUpdate(context.Background(), update)
	if err != nil {
		t.Fatalf("HandleUpdate: %v", err)
	}
	before := sender.deliveryCount()
	if !dispatcher.Enqueue(actions) {
		t.Fatalf("dispatcher rejected %d actions", len(actions))
	}
	if len(actions) > 0 {
		sender.waitFor(t, before+len(actions))
	}
	return actions
}

func (s *telegramAdminE2ESender) waitFor(t *testing.T, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if s.deliveryCount() >= want {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("fake Telegram sender deliveries = %d, want at least %d", s.deliveryCount(), want)
}

func telegramAdminE2EMessageUpdate(telegramID int64, text string) telegram.Update {
	return telegram.Update{Message: &telegram.Message{
		From: &telegram.User{ID: telegramID}, Chat: telegram.Chat{ID: telegramID, Type: "private"}, Text: text,
	}}
}

func telegramAdminE2ECallbackUpdate(telegramID int64, data string) telegram.Update {
	return telegram.Update{CallbackQuery: &telegram.CallbackQuery{
		ID: fmt.Sprintf("tg-admin-e2e-callback-%d", time.Now().UnixNano()), From: &telegram.User{ID: telegramID}, Data: data,
		Message: &telegram.Message{From: &telegram.User{ID: telegramID}, Chat: telegram.Chat{ID: telegramID, Type: "private"}},
	}}
}

func telegramAdminE2EAction(t *testing.T, actions []telegram.Action, kind telegram.ActionKind) telegram.Action {
	t.Helper()
	for _, action := range actions {
		if action.Kind == kind {
			return action
		}
	}
	t.Fatalf("actions = %#v, want action kind %d", actions, kind)
	return telegram.Action{}
}

func telegramAdminE2EButton(t *testing.T, actions []telegram.Action, text string) telegram.InlineKeyboardButton {
	t.Helper()
	for _, action := range actions {
		if action.ReplyMarkup == nil {
			continue
		}
		for _, row := range action.ReplyMarkup.InlineKeyboard {
			for _, button := range row {
				if strings.Contains(button.Text, text) {
					return button
				}
			}
		}
	}
	t.Fatalf("actions = %#v (rendered=%q), want button containing %q", actions, telegramAdminE2EActionText(actions), text)
	return telegram.InlineKeyboardButton{}
}

func telegramAdminE2EFindButton(actions []telegram.Action, text string) string {
	for _, action := range actions {
		if action.ReplyMarkup == nil {
			continue
		}
		for _, row := range action.ReplyMarkup.InlineKeyboard {
			for _, button := range row {
				if strings.Contains(button.Text, text) {
					return button.CallbackData
				}
			}
		}
	}
	return ""
}

func telegramAdminE2EHasButton(actions []telegram.Action, text string) bool {
	for _, action := range actions {
		if action.ReplyMarkup == nil {
			continue
		}
		for _, row := range action.ReplyMarkup.InlineKeyboard {
			for _, button := range row {
				if strings.Contains(button.Text, text) {
					return true
				}
			}
		}
	}
	return false
}

func telegramAdminE2EActionText(actions []telegram.Action) string {
	var builder strings.Builder
	for _, action := range actions {
		builder.WriteString(action.Text)
		builder.WriteString("\n")
		builder.WriteString(action.Caption)
		builder.WriteString("\n")
		if action.ReplyMarkup != nil {
			for _, row := range action.ReplyMarkup.InlineKeyboard {
				for _, button := range row {
					builder.WriteString(button.Text)
					builder.WriteString("\n")
				}
			}
		}
	}
	return builder.String()
}

func telegramAdminE2EURL(t *testing.T, actions []telegram.Action) string {
	t.Helper()
	for _, action := range actions {
		if link := telegramAdminE2EURLFromMarkup(action.ReplyMarkup); link != "" {
			return link
		}
	}
	t.Fatal("actions contained no URL button")
	return ""
}

func telegramAdminE2EURLFromMarkup(markup *telegram.InlineKeyboardMarkup) string {
	if markup == nil {
		return ""
	}
	for _, row := range markup.InlineKeyboard {
		for _, button := range row {
			if button.URL != "" {
				return button.URL
			}
		}
	}
	return ""
}

func countTelegramAdminSessions(t *testing.T, db *database.DB, creatorID string) int {
	t.Helper()
	var count int
	if err := db.Pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM attendance_session WHERE created_by = $1`, creatorID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func latestTelegramAdminSession(t *testing.T, db *database.DB, creatorID string) string {
	t.Helper()
	var id string
	if err := db.Pool.QueryRow(context.Background(), `
		SELECT id FROM attendance_session WHERE created_by = $1 ORDER BY "createdAt" DESC, id DESC LIMIT 1
	`, creatorID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func countTelegramAdminRows(t *testing.T, db *database.DB, sessionID, userID string) int {
	t.Helper()
	var count int
	if err := db.Pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM attendance_record WHERE session_id = $1 AND user_id = $2`, sessionID, userID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func assertTelegramAdminAttendance(t *testing.T, db *database.DB, sessionID, userID, method, markedBy string) {
	t.Helper()
	var gotMethod string
	var gotMarkedBy *string
	if err := db.Pool.QueryRow(context.Background(), `
		SELECT marking_method, marked_by FROM attendance_record WHERE session_id = $1 AND user_id = $2
	`, sessionID, userID).Scan(&gotMethod, &gotMarkedBy); err != nil {
		t.Fatal(err)
	}
	if gotMethod != method {
		t.Fatalf("attendance %s/%s marking_method = %q, want %q", sessionID, userID, gotMethod, method)
	}
	if markedBy == "" {
		if gotMarkedBy != nil {
			t.Fatalf("attendance %s/%s marked_by = %q, want NULL", sessionID, userID, *gotMarkedBy)
		}
		return
	}
	if gotMarkedBy == nil || *gotMarkedBy != markedBy {
		t.Fatalf("attendance %s/%s marked_by = %v, want %q", sessionID, userID, gotMarkedBy, markedBy)
	}
}

func assertTelegramAdminSessionStatus(t *testing.T, db *database.DB, sessionID, want string) {
	t.Helper()
	var got string
	if err := db.Pool.QueryRow(context.Background(), `SELECT status FROM attendance_session WHERE id = $1`, sessionID).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("session %s status = %q, want %q", sessionID, got, want)
	}
}
