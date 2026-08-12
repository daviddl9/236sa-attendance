package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type fakePairingLookup struct {
	pairing Pairing
	found   bool
	seenID  int64
	err     error
}

func (f *fakePairingLookup) FindPairing(_ context.Context, telegramID int64) (Pairing, bool, error) {
	f.seenID = telegramID
	return f.pairing, f.found, f.err
}

func TestBotIgnoresNonPrivateMessages(t *testing.T) {
	lookup := &fakePairingLookup{found: true, pairing: Pairing{FullName: "PRIVATE PERSON"}}
	bot := NewBot(lookup)

	for _, chatType := range []string{"group", "supergroup"} {
		actions, err := bot.HandleUpdate(context.Background(), Update{Message: &Message{
			From: &User{ID: 42},
			Chat: Chat{ID: -42, Type: chatType},
			Text: "/start",
		}})
		if err != nil {
			t.Fatalf("%s message: %v", chatType, err)
		}
		if len(actions) != 0 {
			t.Fatalf("%s actions = %#v, want none", chatType, actions)
		}
	}
	if lookup.seenID != 0 {
		t.Fatalf("group message looked up Telegram ID %d", lookup.seenID)
	}
}

func TestBotRepliesToUnknownPrivateAccount(t *testing.T) {
	lookup := &fakePairingLookup{}
	bot := NewBot(lookup)
	actions, err := bot.HandleUpdate(context.Background(), Update{Message: &Message{
		From: &User{ID: 9001},
		Chat: Chat{ID: 9001, Type: "private"},
		Text: "/start session-code",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if lookup.seenID != 9001 {
		t.Fatalf("lookup Telegram ID = %d, want 9001", lookup.seenID)
	}
	if len(actions) != 1 || actions[0].Kind != SendMessage || actions[0].Text != UnlinkedReply {
		t.Fatalf("actions = %#v, want unlinked reply", actions)
	}
}

func TestBotRepliesWithOnlyPairedAccountName(t *testing.T) {
	lookup := &fakePairingLookup{
		found:   true,
		pairing: Pairing{TelegramID: 9002, UserID: "user-1", FullName: "PTE OWN PERSON"},
	}
	bot := NewBot(lookup)
	actions, err := bot.HandleUpdate(context.Background(), Update{Message: &Message{
		From: &User{ID: 9002},
		Chat: Chat{ID: 9002, Type: "private"},
		Text: "hello",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(actions) != 1 {
		t.Fatalf("actions = %#v, want one action", actions)
	}
	if got := actions[0].Text; got != "I recognise you as PTE OWN PERSON." {
		t.Fatalf("reply = %q", got)
	}
	if strings.Contains(actions[0].Text, "OTHER PERSON") {
		t.Fatalf("reply disclosed another person's name: %q", actions[0].Text)
	}
}

func TestBotAnswersPrivateCallbackOnly(t *testing.T) {
	bot := NewBot(nil)
	actions, err := bot.HandleUpdate(context.Background(), Update{CallbackQuery: &CallbackQuery{
		ID:      "callback-1",
		Message: &Message{Chat: Chat{ID: 7, Type: "private"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(actions) != 1 || actions[0].Kind != AnswerCallbackQuery || actions[0].CallbackQueryID != "callback-1" {
		t.Fatalf("actions = %#v, want callback acknowledgement", actions)
	}

	actions, err = bot.HandleUpdate(context.Background(), Update{CallbackQuery: &CallbackQuery{
		ID:      "callback-group",
		Message: &Message{Chat: Chat{ID: -7, Type: "supergroup"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(actions) != 0 {
		t.Fatalf("group callback actions = %#v, want none", actions)
	}
}

func TestDecodeUpdateVariants(t *testing.T) {
	tests := []struct {
		name  string
		body  string
		check func(t *testing.T, update Update)
	}{
		{
			name: "private message",
			body: `{"update_id":1,"message":{"message_id":2,"from":{"id":3},"chat":{"id":3,"type":"private"},"text":"hello"}}`,
			check: func(t *testing.T, update Update) {
				if update.Message == nil || update.Message.Text != "hello" || update.Message.Chat.Type != "private" {
					t.Fatalf("decoded update = %#v", update)
				}
			},
		},
		{
			name: "start payload",
			body: `{"message":{"from":{"id":3},"chat":{"id":3,"type":"private"},"text":"/start abc123"}}`,
			check: func(t *testing.T, update Update) {
				if update.Message == nil || update.Message.Text != "/start abc123" {
					t.Fatalf("start text = %#v", update.Message)
				}
			},
		},
		{
			name: "callback query",
			body: `{"callback_query":{"id":"callback-1","from":{"id":3},"message":{"chat":{"id":3,"type":"private"}},"data":"confirm"}}`,
			check: func(t *testing.T, update Update) {
				if update.CallbackQuery == nil || update.CallbackQuery.ID != "callback-1" || update.CallbackQuery.Data != "confirm" {
					t.Fatalf("callback = %#v", update.CallbackQuery)
				}
			},
		},
		{
			name: "missing fields",
			body: `{}`,
			check: func(t *testing.T, update Update) {
				if update.Message != nil || update.CallbackQuery != nil {
					t.Fatalf("missing update fields = %#v", update)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			update, err := DecodeUpdate(strings.NewReader(tt.body))
			if err != nil {
				t.Fatal(err)
			}
			tt.check(t, update)
		})
	}
}

func TestDecodeUpdateRejectsMalformedJSON(t *testing.T) {
	if _, err := DecodeUpdate(strings.NewReader(`{"message":`)); err == nil {
		t.Fatal("malformed JSON decoded without an error")
	}
	if _, err := DecodeUpdate(strings.NewReader(`{} {}`)); err == nil {
		t.Fatal("multiple JSON values decoded without an error")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestClientUsesJSONAPIWithoutLeakingErrors(t *testing.T) {
	const testToken = "redacted-token"
	calls := 0
	client, err := NewClientWithTransport(testToken, &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if !strings.HasSuffix(req.URL.Path, "/sendMessage") {
			t.Fatalf("request path = %q", req.URL.Path)
		}
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatal(err)
		}
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("request body: %v", err)
		}
		if payload["chat_id"] != float64(42) || payload["text"] != "plain text" {
			t.Fatalf("payload = %#v", payload)
		}
		return jsonResponse(`{"ok":true}`), nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.SendMessage(context.Background(), 42, "plain text"); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}

func TestClientDoesNotReturnTransportURL(t *testing.T) {
	const testToken = "redacted-token"
	client, err := NewClientWithTransport(testToken, &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("transport failed")
	})})
	if err != nil {
		t.Fatal(err)
	}
	err = client.SendMessage(context.Background(), 1, "text")
	if err == nil || strings.Contains(err.Error(), testToken) {
		t.Fatalf("error = %v, want token-free error", err)
	}
}

func jsonResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}

func TestSetWebhookAndCallbackMethodsUseExpectedAPI(t *testing.T) {
	var paths []string
	var payloads []map[string]any
	client, err := NewClientWithTransport("redacted-token", &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		paths = append(paths, req.URL.Path)
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			return nil, err
		}
		payloads = append(payloads, payload)
		return jsonResponse(`{"ok":true}`), nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.AnswerCallbackQuery(context.Background(), "callback-1"); err != nil {
		t.Fatal(err)
	}
	if err := client.SetWebhook(context.Background(), "https://attendance.example/webhook/path", "header-secret"); err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 || !strings.HasSuffix(paths[0], "/answerCallbackQuery") || !strings.HasSuffix(paths[1], "/setWebhook") {
		t.Fatalf("paths = %#v", paths)
	}
	if payloads[0]["callback_query_id"] != "callback-1" {
		t.Fatalf("callback payload = %#v", payloads[0])
	}
	if payloads[1]["url"] != "https://attendance.example/webhook/path" || payloads[1]["secret_token"] != "header-secret" {
		t.Fatalf("webhook payload = %#v", payloads[1])
	}
}

func TestConfigUsesIndependentWebhookPath(t *testing.T) {
	config := Config{BotToken: "redacted-token", WebhookSecret: "header-secret", WebhookPath: "independent-path"}
	if err := config.Validate(); err != nil {
		t.Fatal(err)
	}
	if got := config.WebhookPathSegment(); got != "independent-path" {
		t.Fatalf("path segment = %q, want configured path", got)
	}
	config.WebhookSecret = "another-secret"
	if got := config.WebhookPathSegment(); got != "independent-path" {
		t.Fatalf("path segment changed with secret = %q", got)
	}
	if got := config.WebhookURL("https://attendance.example/"); got != "https://attendance.example/api/telegram/webhook/independent-path" {
		t.Fatalf("webhook URL = %q", got)
	}
}

func TestConfigRequiresWebhookPathWhenEnabled(t *testing.T) {
	if err := (Config{}).Validate(); err != nil {
		t.Fatalf("disabled config validation error = %v", err)
	}
	if err := (Config{BotToken: "redacted-token"}).Validate(); err == nil {
		t.Fatal("enabled config without webhook path passed validation")
	}
}

func TestDispatcherDoesNotBlockOnSender(t *testing.T) {
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	sender := &blockingSender{started: started, release: release}
	dispatcher := NewDispatcher(sender, 1)
	dispatcher.Enqueue([]Action{{Kind: SendMessage, ChatID: 1, Text: "text"}})
	<-started
	second := make(chan struct{})
	go func() {
		dispatcher.Enqueue([]Action{{Kind: SendMessage, ChatID: 2, Text: "text"}})
		close(second)
	}()
	select {
	case <-second:
	case <-timeAfter():
		t.Fatal("enqueue waited for slow Telegram sender")
	}
	close(release)
	dispatcher.Close()
}

type blockingSender struct {
	started chan<- struct{}
	release <-chan struct{}
}

func (s *blockingSender) SendMessage(context.Context, int64, string) error {
	s.started <- struct{}{}
	<-s.release
	return nil
}

func (s *blockingSender) AnswerCallbackQuery(context.Context, string) error { return nil }

func timeAfter() <-chan time.Time {
	return time.After(100 * time.Millisecond)
}
