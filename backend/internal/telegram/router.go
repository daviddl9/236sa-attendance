package telegram

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

const UnlinkedReply = "Your account is not yet linked. Pairing arrives soon."

// Pairing is the only identity data the bot needs for this task. FullName is
// read for the Telegram account identified by TelegramID, never supplied by a
// message sender.
type Pairing struct {
	TelegramID int64
	UserID     string
	FullName   string
}

// PairingLookup reads a confirmed pairing for one Telegram account. This task
// intentionally has no write method and never reads pairing requests.
type PairingLookup interface {
	FindPairing(ctx context.Context, telegramID int64) (Pairing, bool, error)
}

// Action is an outbound operation produced by routing one update.
type Action struct {
	Kind            ActionKind
	ChatID          int64
	Text            string
	CallbackQueryID string
}

type ActionKind int

const (
	SendMessage ActionKind = iota
	AnswerCallbackQuery
)

// Bot routes updates and composes replies. It does not mark attendance, pair
// accounts, resolve deep-link codes, or write to the database.
type Bot struct {
	pairings PairingLookup
}

func NewBot(pairings PairingLookup) *Bot {
	return &Bot{pairings: pairings}
}

// HandleUpdate turns one decoded update into outbound actions. An empty action
// list means the update was deliberately ignored.
func (b *Bot) HandleUpdate(ctx context.Context, update Update) ([]Action, error) {
	if update.Message != nil {
		return b.handleMessage(ctx, update.Message)
	}
	if update.CallbackQuery != nil {
		return handleCallbackQuery(update.CallbackQuery), nil
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

	pairing, found, err := b.pairings.FindPairing(ctx, message.From.ID)
	if err != nil {
		return nil, fmt.Errorf("find Telegram pairing: %w", err)
	}
	if !found {
		return []Action{{Kind: SendMessage, ChatID: message.Chat.ID, Text: UnlinkedReply}}, nil
	}
	return []Action{{Kind: SendMessage, ChatID: message.Chat.ID, Text: linkedReply(pairing.FullName)}}, nil
}

func handleCallbackQuery(query *CallbackQuery) []Action {
	if query.ID == "" || query.Message == nil || query.Message.Chat.Type != "private" || query.Message.Chat.ID == 0 {
		return nil
	}
	return []Action{{Kind: AnswerCallbackQuery, ChatID: query.Message.Chat.ID, CallbackQueryID: query.ID}}
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

// ActionSink accepts outbound actions without making the webhook wait for the
// Telegram API.
type ActionSink interface {
	Enqueue(actions []Action)
}

// Dispatcher sends actions from a bounded background queue. Enqueue is
// intentionally non-blocking: Telegram expects a quick webhook acknowledgement
// and will retry if a slow API call holds the response open.
type Dispatcher struct {
	sender Sender
	queue  chan Action
	done   chan struct{}

	mu     sync.RWMutex
	closed bool
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

func (d *Dispatcher) Enqueue(actions []Action) {
	if len(actions) == 0 {
		return
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.closed {
		return
	}
	for _, action := range actions {
		select {
		case d.queue <- action:
		default:
			// Dropping an outbound reply is preferable to holding a webhook
			// request open. The update has already been acknowledged.
		}
	}
}

func (d *Dispatcher) run() {
	defer close(d.done)
	for action := range d.queue {
		ctx, cancel := context.WithTimeout(context.Background(), deliveryTimeout)
		switch action.Kind {
		case SendMessage:
			if err := d.sender.SendMessage(ctx, action.ChatID, action.Text); err != nil {
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
