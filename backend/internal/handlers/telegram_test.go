package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/davidlivingston/go-nextjs-starter/backend/internal/telegram"
)

type telegramActionSink struct {
	actions []telegram.Action
}

func (s *telegramActionSink) Enqueue(actions []telegram.Action) {
	s.actions = append(s.actions, actions...)
}

type telegramPairingLookup struct {
	pairing telegram.Pairing
	found   bool
}

func (l telegramPairingLookup) FindPairing(context.Context, int64) (telegram.Pairing, bool, error) {
	return l.pairing, l.found, nil
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
			if tc.wantOK {
				if len(sink.actions) != 1 {
					t.Fatalf("accepted request queued %d actions, want 1", len(sink.actions))
				}
			} else if len(sink.actions) != 0 {
				t.Fatalf("rejected request queued actions: %#v", sink.actions)
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
	if len(sink.actions) != 0 {
		t.Fatalf("group message queued actions: %#v", sink.actions)
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
	if len(sink.actions) != 1 || sink.actions[0].Text != telegram.UnlinkedReply {
		t.Fatalf("actions = %#v, want unlinked reply", sink.actions)
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
			if len(sink.actions) != 0 {
				t.Fatalf("malformed or missing update queued actions: %#v", sink.actions)
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

func jsonBody(body string) *strings.Reader {
	return strings.NewReader(body)
}
