package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/davidlivingston/go-nextjs-starter/backend/internal/telegram"
)

type telegramActionSink struct {
	mu      sync.Mutex
	actions []telegram.Action
	calls   int
	reject  bool
}

func (s *telegramActionSink) Enqueue(actions []telegram.Action) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	s.actions = append(s.actions, actions...)
	return !s.reject
}

func (s *telegramActionSink) Actions() []telegram.Action {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]telegram.Action(nil), s.actions...)
}

type telegramPairingLookup struct {
	pairing telegram.Pairing
	found   bool
}

func (l telegramPairingLookup) FindPairing(context.Context, int64) (telegram.Pairing, bool, error) {
	return l.pairing, l.found, nil
}

type telegramCallbackFailureFlow struct {
	telegramPairingLookup
	confirmErr error
	discardErr error
}

func (f telegramCallbackFailureFlow) ProposePairing(context.Context, int64, string) (telegram.PairingProposal, error) {
	return telegram.PairingProposal{}, nil
}

func (f telegramCallbackFailureFlow) ConfirmPairing(context.Context, int64, string) (telegram.PairingConfirmation, error) {
	return telegram.PairingConfirmation{}, f.confirmErr
}

func (f telegramCallbackFailureFlow) DiscardPairing(context.Context, int64, string) error {
	return f.discardErr
}

type telegramFailingPairingLookup struct {
	err error
}

func (l telegramFailingPairingLookup) FindPairing(context.Context, int64) (telegram.Pairing, bool, error) {
	return telegram.Pairing{}, false, l.err
}

func newTelegramTestHandler(found bool) (*TelegramHandler, *telegramActionSink) {
	sink := &telegramActionSink{}
	bot := telegram.NewBot(telegramPairingLookup{
		found:   found,
		pairing: telegram.Pairing{FullName: "OWN PERSON"},
	})
	return NewTelegramHandler(bot, "header-secret", sink), sink
}

func TestTelegramWebhookAuthenticity(t *testing.T) {
	cases := []struct {
		name   string
		header string
		wantOK bool
	}{
		{name: "correct", header: "header-secret", wantOK: true},
		{name: "wrong", header: "wrong-secret"},
		{name: "missing"},
		{name: "empty", header: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler, sink := newTelegramTestHandler(false)
			req := httptest.NewRequest(http.MethodPost, "/api/telegram/webhook/path", jsonBody(`{"message":{"from":{"id":99},"chat":{"id":99,"type":"private"},"text":"hello"}}`))
			if tc.header != "" {
				req.Header.Set(telegramSecretHeader, tc.header)
			}
			rec := httptest.NewRecorder()
			handler.Webhook(rec, req)
			if tc.wantOK {
				if rec.Code != http.StatusOK {
					t.Fatalf("status = %d, want 200", rec.Code)
				}
			} else if rec.Code == http.StatusOK {
				t.Fatalf("status = 200 for rejected request")
			}
			actions := sink.Actions()
			if tc.wantOK {
				if len(actions) != 1 {
					t.Fatalf("accepted request queued %d actions, want 1", len(actions))
				}
			} else if len(actions) != 0 {
				t.Fatalf("rejected request queued actions: %#v", actions)
			}
		})
	}
}

func TestTelegramWebhookGroupMessageHasNoActionOrReply(t *testing.T) {
	handler, sink := newTelegramTestHandler(true)
	req := httptest.NewRequest(http.MethodPost, "/api/telegram/webhook/path", jsonBody(`{
		"message":{"from":{"id":99},"chat":{"id":-99,"type":"group"},"text":"/start"}
	}`))
	req.Header.Set(telegramSecretHeader, "header-secret")
	rec := httptest.NewRecorder()
	handler.Webhook(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if actions := sink.Actions(); len(actions) != 0 {
		t.Fatalf("group message queued actions: %#v", actions)
	}
}

func TestTelegramWebhookQueuesCallbackAcknowledgementOnProcessingError(t *testing.T) {
	serviceErr := errors.New("pairing service unavailable")
	flow := telegramCallbackFailureFlow{confirmErr: serviceErr}
	sink := &telegramActionSink{}
	handler := NewTelegramHandler(telegram.NewBot(flow), "header-secret", sink)
	req := httptest.NewRequest(http.MethodPost, "/api/telegram/webhook/path", jsonBody(`{
		"callback_query":{"id":"callback-1","from":{"id":42},"message":{"chat":{"id":42,"type":"private"}},"data":"p:y:attempt-1"}
	}`))
	req.Header.Set(telegramSecretHeader, "header-secret")
	rec := httptest.NewRecorder()
	handler.Webhook(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	actions := sink.Actions()
	if len(actions) != 1 || actions[0].Kind != telegram.AnswerCallbackQuery || actions[0].CallbackQueryID != "callback-1" {
		t.Fatalf("actions = %#v, want callback acknowledgement only", actions)
	}
}

func TestTelegramWebhookReturns503WhenCallbackAcknowledgementCannotQueue(t *testing.T) {
	sink := &telegramActionSink{reject: true}
	handler := NewTelegramHandler(telegram.NewBot(telegramCallbackFailureFlow{
		confirmErr: errors.New("pairing service unavailable"),
	}), "header-secret", sink)
	req := httptest.NewRequest(http.MethodPost, "/api/telegram/webhook/path", jsonBody(`{
		"callback_query":{"id":"callback-1","from":{"id":42},"message":{"chat":{"id":42,"type":"private"}},"data":"p:y:attempt-1"}
	}`))
	req.Header.Set(telegramSecretHeader, "header-secret")
	rec := httptest.NewRecorder()
	handler.Webhook(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	if actions := sink.Actions(); len(actions) != 1 || actions[0].Kind != telegram.AnswerCallbackQuery {
		t.Fatalf("actions = %#v, want rejected callback acknowledgement", actions)
	}
}

func TestTelegramWebhookDoesNotQueueMessageActionsOnProcessingError(t *testing.T) {
	sink := &telegramActionSink{}
	handler := NewTelegramHandler(telegram.NewBot(telegramFailingPairingLookup{
		err: errors.New("pairing service unavailable"),
	}), "header-secret", sink)
	req := httptest.NewRequest(http.MethodPost, "/api/telegram/webhook/path", jsonBody(`{
		"message":{"from":{"id":42},"chat":{"id":42,"type":"private"},"text":"hello"}
	}`))
	req.Header.Set(telegramSecretHeader, "header-secret")
	rec := httptest.NewRecorder()
	handler.Webhook(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if sink.calls != 0 || len(sink.Actions()) != 0 {
		t.Fatalf("sink calls=%d actions=%#v, want no message actions", sink.calls, sink.Actions())
	}
}

func TestTelegramWebhookRepliesToUnknownPrivateAccount(t *testing.T) {
	handler, sink := newTelegramTestHandler(false)
	req := httptest.NewRequest(http.MethodPost, "/api/telegram/webhook/path", jsonBody(`{
		"message":{"from":{"id":99},"chat":{"id":99,"type":"private"},"text":"hello"}
	}`))
	req.Header.Set(telegramSecretHeader, "header-secret")
	rec := httptest.NewRecorder()
	handler.Webhook(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if actions := sink.Actions(); len(actions) != 1 || actions[0].Text != telegram.UnlinkedReply {
		t.Fatalf("actions = %#v, want unlinked reply", actions)
	}
}

func TestTelegramWebhookAuthenticatedMalformedAndMissingUpdatesReturnOK(t *testing.T) {
	for _, body := range []string{`{"message":`, `{}`} {
		t.Run(body, func(t *testing.T) {
			handler, sink := newTelegramTestHandler(false)
			req := httptest.NewRequest(http.MethodPost, "/api/telegram/webhook/path", jsonBody(body))
			req.Header.Set(telegramSecretHeader, "header-secret")
			rec := httptest.NewRecorder()
			handler.Webhook(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rec.Code)
			}
			if actions := sink.Actions(); len(actions) != 0 {
				t.Fatalf("malformed or missing update queued actions: %#v", actions)
			}
		})
	}
}

func TestTelegramWebhookBodyLimitBoundary(t *testing.T) {
	const base = `{"message":{"from":{"id":99},"chat":{"id":99,"type":"private"},"text":"hello"}}`
	cases := []struct {
		name        string
		size        int
		wantActions int
	}{
		{name: "at limit", size: maxTelegramUpdate, wantActions: 1},
		{name: "beyond limit", size: maxTelegramUpdate + 1, wantActions: 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler, sink := newTelegramTestHandler(false)
			body := strings.Repeat(" ", tc.size-len(base))
			req := httptest.NewRequest(http.MethodPost, "/api/telegram/webhook/path", strings.NewReader(base+body))
			req.Header.Set(telegramSecretHeader, "header-secret")
			rec := httptest.NewRecorder()
			handler.Webhook(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rec.Code)
			}
			if actions := sink.Actions(); len(actions) != tc.wantActions {
				t.Fatalf("actions = %d, want %d", len(actions), tc.wantActions)
			}
		})
	}
}

func TestTelegramWebhookHandlesNullAndUnexpectedFieldTypes(t *testing.T) {
	bodies := map[string]string{
		"null update":           `null`,
		"null message":          `{"message":null}`,
		"null chat":             `{"message":{"chat":null}}`,
		"null sender":           `{"message":{"from":null,"chat":{"id":99,"type":"private"}}}`,
		"message array":         `{"message":[]}`,
		"chat array":            `{"message":{"chat":[]}}`,
		"sender array":          `{"message":{"from":[],"chat":{"id":99,"type":"private"}}}`,
		"chat ID string":        `{"message":{"chat":{"id":"wrong","type":"private"}}}`,
		"callback array":        `{"callback_query":[]}`,
		"callback ID number":    `{"callback_query":{"id":7}}`,
		"callback null message": `{"callback_query":{"id":"callback-1","message":null}}`,
		"callback null chat":    `{"callback_query":{"id":"callback-1","message":{"chat":null}}}`,
	}

	for name, body := range bodies {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recovered := recover(); recovered != nil {
					t.Fatalf("webhook panicked: %v", recovered)
				}
			}()
			handler, sink := newTelegramTestHandler(false)
			req := httptest.NewRequest(http.MethodPost, "/api/telegram/webhook/path", jsonBody(body))
			req.Header.Set(telegramSecretHeader, "header-secret")
			rec := httptest.NewRecorder()
			handler.Webhook(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rec.Code)
			}
			if actions := sink.Actions(); len(actions) != 0 {
				t.Fatalf("unexpected input queued actions: %#v", actions)
			}
		})
	}
}

func TestTelegramWebhookDisabledWithoutBotConfiguration(t *testing.T) {
	handler := NewTelegramHandler(nil, "", nil)
	req := httptest.NewRequest(http.MethodPost, "/api/telegram/webhook/path", jsonBody(`{"message":{}}`))
	req.Header.Set(telegramSecretHeader, "header-secret")
	rec := httptest.NewRecorder()
	handler.Webhook(rec, req)
	if rec.Code == http.StatusOK {
		t.Fatal("disabled Telegram webhook returned 200")
	}
}

func TestTelegramWebhookReturns503WhenReplyQueueIsFull(t *testing.T) {
	sender := &telegramWebhookSender{
		started: make(chan int64, 3),
		release: make(chan struct{}),
	}
	dispatcher := telegram.NewDispatcher(sender, 1)
	released := false
	defer func() {
		if !released {
			close(sender.release)
		}
		dispatcher.Close()
	}()

	handler := NewTelegramHandler(telegram.NewBot(telegramPairingLookup{}), "header-secret", dispatcher)
	first := handler.WebhookRecorder(1)
	if first.Code != http.StatusOK {
		t.Fatalf("first status = %d, want 200", first.Code)
	}
	if got := waitForTelegramAction(t, sender.started); got != 1 {
		t.Fatalf("first chat ID = %d, want 1", got)
	}

	second := handler.WebhookRecorder(2)
	if second.Code != http.StatusOK {
		t.Fatalf("second status = %d, want 200", second.Code)
	}
	third := handler.WebhookRecorder(3)
	if third.Code != http.StatusServiceUnavailable {
		t.Fatalf("full queue status = %d, want 503", third.Code)
	}

	close(sender.release)
	released = true
	if got := waitForTelegramAction(t, sender.started); got != 2 {
		t.Fatalf("queued chat ID = %d, want 2", got)
	}

	retry := handler.WebhookRecorder(3)
	if retry.Code != http.StatusOK {
		t.Fatalf("retried status = %d, want 200", retry.Code)
	}
	if got := waitForTelegramAction(t, sender.started); got != 3 {
		t.Fatalf("retried chat ID = %d, want 3", got)
	}
}

type telegramWebhookSender struct {
	started chan int64
	release chan struct{}
}

func (s *telegramWebhookSender) SendMessage(_ context.Context, chatID int64, _ string) error {
	s.started <- chatID
	<-s.release
	return nil
}

func (s *telegramWebhookSender) AnswerCallbackQuery(context.Context, string) error {
	return nil
}

func (h *TelegramHandler) WebhookRecorder(chatID int64) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/telegram/webhook/path", jsonBody(fmt.Sprintf(`{"message":{"from":{"id":%d},"chat":{"id":%d,"type":"private"},"text":"hello"}}`, chatID, chatID)))
	req.Header.Set(telegramSecretHeader, "header-secret")
	rec := httptest.NewRecorder()
	h.Webhook(rec, req)
	return rec
}

func waitForTelegramAction(t *testing.T, actions <-chan int64) int64 {
	t.Helper()
	select {
	case chatID := <-actions:
		return chatID
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for Telegram action")
		return 0
	}
}

func jsonBody(body string) *strings.Reader {
	return strings.NewReader(body)
}
