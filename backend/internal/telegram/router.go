package telegram

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	UnlinkedReply             = "Your account is not yet linked. Pairing arrives soon."
	NamePromptReply           = "Please send your full name so I can find your roster record."
	UnpairedAttendanceReply   = "Please pair your Telegram account before scanning an attendance QR code."
	AttendanceClosedReply     = "This attendance session is closed."
	AttendanceOutOfScopeReply = "You are not eligible for this attendance session."
	AttendanceInvalidReply    = "This attendance link is invalid or unavailable."
	NoMatchReply              = "I couldn't find a close match. Please see a commander to complete pairing."
	PairingConflictReply      = "Pairing could not be completed. Please see a commander."
	PairingDeclinedReply      = "Okay. Send your full name again if you want to try another match."
	PairingStaleReply         = "That proposal is no longer available. Please send your full name again."
)

// Pairing is the identity data the bot needs. FullName is read for the
// Telegram account identified by TelegramID, never supplied by a message sender.
type Pairing struct {
	TelegramID int64
	UserID     string
	FullName   string
}

// PairingLookup reads a confirmed pairing for one Telegram account.
type PairingLookup interface {
	FindPairing(ctx context.Context, telegramID int64) (Pairing, bool, error)
}

// AttendanceOutcome is the result of resolving one Telegram deep link and
// applying the shared attendance marking rules.
type AttendanceOutcome int

const (
	AttendanceMarked AttendanceOutcome = iota
	AttendanceAlreadyMarked
	AttendanceSessionClosed
	AttendanceOutOfScope
	AttendanceUnknownCode
	AttendanceUnpaired
)

// AttendanceResult contains only the session name needed in a successful
// soldier-facing reply. Unknown and rejected outcomes leave it empty.
type AttendanceResult struct {
	Outcome     AttendanceOutcome
	SessionName string
}

// AttendanceMarker resolves a per-session Telegram deep-link code and marks
// the paired account. The production adapter owns the transaction and calls
// services/attendance.Mark; tests can inject a real-DB adapter or a narrow
// fake without replacing the pairing lookup.
type AttendanceMarker interface {
	MarkAttendance(ctx context.Context, telegramID int64, deeplinkCode string) (AttendanceResult, error)
}

// PairingProposal is the one roster row the system may propose to a soldier.
// A proposal is created only for a strong match; no-match and rate-limited
// attempts return an empty proposal with the corresponding flag set.
type PairingProposal struct {
	AttemptID   string
	UserID      string
	Name        string
	Rank        string
	Battery     string
	Score       int
	NoMatch     bool
	RateLimited bool
}

// PairingConfirmation describes the result of tapping a proposal button.
type PairingConfirmation struct {
	Outcome PairingConfirmationOutcome
	Pairing Pairing
}

type PairingConfirmationOutcome int

const (
	PairingConfirmed PairingConfirmationOutcome = iota
	PairingConflict
	PairingStale
)

// PairingFlow is optional so the task-3 read-only fakes remain valid. The
// production database store implements both PairingLookup and PairingFlow.
type PairingFlow interface {
	ProposePairing(ctx context.Context, telegramID int64, name string) (PairingProposal, error)
	ConfirmPairing(ctx context.Context, telegramID int64, attemptID string) (PairingConfirmation, error)
}

// PairingDiscarder is optional for test doubles and invalidates a declined
// proposal without deleting its audit/rate-limit attempt row.
type PairingDiscarder interface {
	DiscardPairing(ctx context.Context, telegramID int64, attemptID string) error
}

// Action is an outbound operation produced by routing one update.
type Action struct {
	Kind            ActionKind
	ChatID          int64
	Text            string
	CallbackQueryID string
	ReplyMarkup     *InlineKeyboardMarkup
}

// InlineKeyboardMarkup is the subset of Telegram's reply markup used for a
// tap-to-confirm pairing proposal.
type InlineKeyboardMarkup struct {
	InlineKeyboard [][]InlineKeyboardButton `json:"inline_keyboard"`
}

type InlineKeyboardButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data"`
}

type ActionKind int

const (
	SendMessage ActionKind = iota
	AnswerCallbackQuery
)

// Bot routes updates and composes replies. Attendance and pairing writes are
// delegated to optional service adapters implemented by the database store.
type Bot struct {
	pairings   PairingLookup
	flow       PairingFlow
	attendance AttendanceMarker
}

func NewBot(pairings PairingLookup) *Bot {
	return NewBotWithAttendance(pairings, nil)
}

// NewBotWithAttendance keeps the pairing lookup and attendance marker
// independently injectable. NewBot remains the compatibility constructor for
// read-only pairing tests and automatically discovers adapters implemented by
// the same production store.
func NewBotWithAttendance(pairings PairingLookup, marker AttendanceMarker) *Bot {
	bot := &Bot{pairings: pairings, attendance: marker}
	if flow, ok := pairings.(PairingFlow); ok {
		bot.flow = flow
	}
	if bot.attendance == nil {
		if marker, ok := pairings.(AttendanceMarker); ok {
			bot.attendance = marker
		}
	}
	return bot
}

// HandleUpdate turns one decoded update into outbound actions. An empty action
// list means the update was deliberately ignored.
func (b *Bot) HandleUpdate(ctx context.Context, update Update) ([]Action, error) {
	if update.Message != nil {
		return b.handleMessage(ctx, update.Message)
	}
	if update.CallbackQuery != nil {
		return b.handleCallbackQuery(ctx, update.CallbackQuery)
	}
	return nil, nil
}

func (b *Bot) handleMessage(ctx context.Context, message *Message) ([]Action, error) {
	if message.Chat.Type != "private" || message.Chat.ID == 0 || message.From == nil || message.From.ID == 0 {
		return nil, nil
	}
	if b.pairings == nil {
		return nil, errors.New("telegram pairing lookup is not configured")
	}

	start := parseStartCommand(message.Text)
	pairing, found, err := b.pairings.FindPairing(ctx, message.From.ID)
	if err != nil {
		return nil, fmt.Errorf("find Telegram pairing: %w", err)
	}
	if found {
		if start.hasPayload {
			if start.payload == "" || b.attendance == nil {
				// A malformed payload is not a name and must never enter the
				// pairing matcher. Keep the old linked response when the
				// attendance adapter is not configured in a read-only test bot.
				if b.attendance == nil {
					return []Action{{Kind: SendMessage, ChatID: message.Chat.ID, Text: linkedReply(pairing.FullName)}}, nil
				}
				return []Action{{Kind: SendMessage, ChatID: message.Chat.ID, Text: AttendanceInvalidReply}}, nil
			}
			result, err := b.attendance.MarkAttendance(ctx, message.From.ID, start.payload)
			if err != nil {
				return nil, fmt.Errorf("mark Telegram attendance: %w", err)
			}
			return []Action{{Kind: SendMessage, ChatID: message.Chat.ID, Text: attendanceReply(result)}}, nil
		}
		return []Action{{Kind: SendMessage, ChatID: message.Chat.ID, Text: linkedReply(pairing.FullName)}}, nil
	}
	if start.hasPayload {
		// The deep-link payload is an opaque capability, never identity input.
		// An unpaired account must pair first and must not learn anything about
		// the code or session.
		if b.flow == nil && b.attendance == nil {
			return []Action{{Kind: SendMessage, ChatID: message.Chat.ID, Text: UnlinkedReply}}, nil
		}
		return []Action{{Kind: SendMessage, ChatID: message.Chat.ID, Text: UnpairedAttendanceReply}}, nil
	}
	if b.flow == nil {
		return []Action{{Kind: SendMessage, ChatID: message.Chat.ID, Text: UnlinkedReply}}, nil
	}

	name := pairingNameInput(message.Text)
	if name == "" {
		return []Action{{Kind: SendMessage, ChatID: message.Chat.ID, Text: NamePromptReply}}, nil
	}
	proposal, err := b.flow.ProposePairing(ctx, message.From.ID, name)
	if err != nil {
		return nil, fmt.Errorf("propose Telegram pairing: %w", err)
	}
	if proposal.NoMatch || proposal.RateLimited {
		return []Action{{Kind: SendMessage, ChatID: message.Chat.ID, Text: NoMatchReply}}, nil
	}
	return []Action{{
		Kind:        SendMessage,
		ChatID:      message.Chat.ID,
		Text:        proposalText(proposal),
		ReplyMarkup: pairingKeyboard(proposal.AttemptID),
	}}, nil
}

func (b *Bot) handleCallbackQuery(ctx context.Context, query *CallbackQuery) ([]Action, error) {
	if query.ID == "" || query.Message == nil || query.Message.Chat.Type != "private" || query.Message.Chat.ID == 0 {
		return nil, nil
	}
	actions := []Action{{Kind: AnswerCallbackQuery, ChatID: query.Message.Chat.ID, CallbackQueryID: query.ID}}
	if b.flow == nil || query.From == nil || query.From.ID == 0 {
		return actions, nil
	}

	kind, attemptID, ok := parsePairingCallback(query.Data)
	if !ok {
		return actions, nil
	}
	if kind == "n" {
		if discarder, ok := b.flow.(PairingDiscarder); ok {
			if err := discarder.DiscardPairing(ctx, query.From.ID, attemptID); err != nil {
				return nil, fmt.Errorf("discard Telegram pairing: %w", err)
			}
		}
		actions = append(actions, Action{Kind: SendMessage, ChatID: query.Message.Chat.ID, Text: PairingDeclinedReply})
		return actions, nil
	}

	confirmation, err := b.flow.ConfirmPairing(ctx, query.From.ID, attemptID)
	if err != nil {
		return nil, fmt.Errorf("confirm Telegram pairing: %w", err)
	}
	switch confirmation.Outcome {
	case PairingConfirmed:
		actions = append(actions, Action{Kind: SendMessage, ChatID: query.Message.Chat.ID, Text: linkedReply(confirmation.Pairing.FullName) + " You are ready to mark attendance."})
	case PairingConflict:
		actions = append(actions, Action{Kind: SendMessage, ChatID: query.Message.Chat.ID, Text: PairingConflictReply})
	default:
		actions = append(actions, Action{Kind: SendMessage, ChatID: query.Message.Chat.ID, Text: PairingStaleReply})
	}
	return actions, nil
}

type startCommand struct {
	hasPayload bool
	payload    string
}

func parseStartCommand(text string) startCommand {
	fields := strings.Fields(strings.TrimSpace(text))
	if len(fields) == 0 {
		return startCommand{}
	}
	command := fields[0]
	if at := strings.IndexByte(command, '@'); at >= 0 {
		command = command[:at]
	}
	if !strings.EqualFold(command, "/start") {
		return startCommand{}
	}
	if len(fields) == 1 {
		return startCommand{}
	}
	if len(fields) != 2 {
		return startCommand{hasPayload: true}
	}
	return startCommand{hasPayload: true, payload: fields[1]}
}

func attendanceReply(result AttendanceResult) string {
	sessionName := strings.TrimSpace(result.SessionName)
	switch result.Outcome {
	case AttendanceMarked:
		if sessionName == "" {
			return "Attendance marked."
		}
		return "Attendance marked for " + sessionName + "."
	case AttendanceAlreadyMarked:
		if sessionName == "" {
			return "You are already marked for this attendance session."
		}
		return "You are already marked for " + sessionName + "."
	case AttendanceSessionClosed:
		return AttendanceClosedReply
	case AttendanceOutOfScope:
		return AttendanceOutOfScopeReply
	case AttendanceUnpaired:
		return UnpairedAttendanceReply
	default:
		// Unknown and malformed capabilities deliberately share one response.
		return AttendanceInvalidReply
	}
}

func pairingNameInput(text string) string {
	text = strings.TrimSpace(text)
	if text == "" || strings.HasPrefix(text, "/") {
		return ""
	}
	return text
}

func proposalText(proposal PairingProposal) string {
	identity := strings.TrimSpace(strings.Join([]string{proposal.Rank, proposal.Name}, " "))
	if proposal.Battery == "" {
		return "Are you " + identity + "?"
	}
	return "Are you " + identity + ", " + proposal.Battery + "?"
}

func pairingKeyboard(attemptID string) *InlineKeyboardMarkup {
	return &InlineKeyboardMarkup{InlineKeyboard: [][]InlineKeyboardButton{{
		{Text: "Yes, that's me", CallbackData: "p:y:" + attemptID},
		{Text: "No, try again", CallbackData: "p:n:" + attemptID},
	}}}
}

func parsePairingCallback(data string) (string, string, bool) {
	parts := strings.Split(data, ":")
	if len(parts) != 3 || parts[0] != "p" || (parts[1] != "y" && parts[1] != "n") || parts[2] == "" {
		return "", "", false
	}
	return parts[1], parts[2], true
}

func linkedReply(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "Your account is linked."
	}
	return "I recognise you as " + name + "."
}

// Sender is implemented by Client and can be replaced by a fake in tests.
type Sender interface {
	SendMessage(ctx context.Context, chatID int64, text string) error
	AnswerCallbackQuery(ctx context.Context, callbackQueryID string) error
}

type markupSender interface {
	SendMessageWithMarkup(ctx context.Context, chatID int64, text string, replyMarkup *InlineKeyboardMarkup) error
}

// ActionSink accepts outbound actions without making the webhook wait for the
// Telegram API. It returns false when the action cannot be accepted.
type ActionSink interface {
	Enqueue(actions []Action) bool
}

// Dispatcher sends actions from a bounded background queue. Enqueue is
// intentionally non-blocking: Telegram expects a quick webhook acknowledgement
// and will retry if a slow API call holds the response open.
type Dispatcher struct {
	sender Sender
	queue  chan Action
	done   chan struct{}

	mu              sync.Mutex
	closed          bool
	saturationCount uint64
}

const (
	defaultQueueSize = 512
	deliveryTimeout  = 10 * time.Second
)

func NewDispatcher(sender Sender, queueSize int) *Dispatcher {
	if queueSize <= 0 {
		queueSize = defaultQueueSize
	}
	d := &Dispatcher{
		sender: sender,
		queue:  make(chan Action, queueSize),
		done:   make(chan struct{}),
	}
	go d.run()
	return d
}

func (d *Dispatcher) Enqueue(actions []Action) bool {
	if len(actions) == 0 {
		return true
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return false
	}
	if len(actions) > cap(d.queue)-len(d.queue) {
		count := atomic.AddUint64(&d.saturationCount, 1)
		log.Printf("Telegram reply queue saturated; rejecting update (count=%d)", count)
		return false
	}
	for _, action := range actions {
		d.queue <- action
	}
	return true
}

func (d *Dispatcher) run() {
	defer close(d.done)
	for action := range d.queue {
		ctx, cancel := context.WithTimeout(context.Background(), deliveryTimeout)
		switch action.Kind {
		case SendMessage:
			var err error
			if action.ReplyMarkup != nil {
				if sender, ok := d.sender.(markupSender); ok {
					err = sender.SendMessageWithMarkup(ctx, action.ChatID, action.Text, action.ReplyMarkup)
				} else {
					err = d.sender.SendMessage(ctx, action.ChatID, action.Text)
				}
			} else {
				err = d.sender.SendMessage(ctx, action.ChatID, action.Text)
			}
			if err != nil {
				// Do not log action.Text: a linked reply may contain a person's name.
				// The client also guarantees that its token is absent from errors.
				logDeliveryError("send message", err)
			}
		case AnswerCallbackQuery:
			if err := d.sender.AnswerCallbackQuery(ctx, action.CallbackQueryID); err != nil {
				logDeliveryError("answer callback query", err)
			}
		}
		cancel()
	}
}

func (d *Dispatcher) Close() {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return
	}
	d.closed = true
	close(d.queue)
	d.mu.Unlock()
	<-d.done
}

func logDeliveryError(operation string, _ error) {
	log.Printf("Telegram %s failed", operation)
}
