package telegram

import (
	"bytes"
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

func TestInlineKeyboardButtonJSONSupportsURLAndCallbackButtons(t *testing.T) {
	urlOnly, err := json.Marshal(InlineKeyboardButton{Text: "Open", URL: "https://attendance.example/start"})
	if err != nil {
		t.Fatal(err)
	}
	var urlPayload map[string]json.RawMessage
	if err := json.Unmarshal(urlOnly, &urlPayload); err != nil {
		t.Fatal(err)
	}
	if _, ok := urlPayload["callback_data"]; ok {
		t.Fatalf("URL-only button contains callback_data: %s", urlOnly)
	}
	if got := string(urlPayload["url"]); got != `"https://attendance.example/start"` {
		t.Fatalf("URL-only button URL = %s", got)
	}

	callbackOnly, err := json.Marshal(InlineKeyboardButton{Text: "Confirm", CallbackData: "p:y:attempt-1"})
	if err != nil {
		t.Fatal(err)
	}
	var callbackPayload map[string]json.RawMessage
	if err := json.Unmarshal(callbackOnly, &callbackPayload); err != nil {
		t.Fatal(err)
	}
	if _, ok := callbackPayload["url"]; ok {
		t.Fatalf("callback-only button contains url: %s", callbackOnly)
	}
	if got := string(callbackPayload["callback_data"]); got != `"p:y:attempt-1"` {
		t.Fatalf("callback-only button callback_data = %s", got)
	}
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

func TestClientSendPhotoUsesMultipartForm(t *testing.T) {
	const testToken = "photo-token"
	wantPhoto := []byte("synthetic PNG bytes")
	wantMarkup := &InlineKeyboardMarkup{InlineKeyboard: [][]InlineKeyboardButton{{
		{Text: "Open", URL: "https://attendance.example/start"},
	}}}
	client, err := NewClientWithTransport(testToken, &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if !strings.HasSuffix(req.URL.Path, "/sendPhoto") {
			t.Fatalf("request path = %q", req.URL.Path)
		}
		if !strings.HasPrefix(req.Header.Get("Content-Type"), "multipart/form-data;") {
			t.Fatalf("content type = %q, want multipart/form-data", req.Header.Get("Content-Type"))
		}
		reader, err := req.MultipartReader()
		if err != nil {
			t.Fatal(err)
		}
		fields := make(map[string]string)
		var gotPhoto []byte
		var gotFilename string
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			value, err := io.ReadAll(part)
			if err != nil {
				t.Fatal(err)
			}
			if part.FormName() == "photo" {
				gotPhoto = value
				gotFilename = part.FileName()
			} else {
				fields[part.FormName()] = string(value)
			}
		}
		if fields["chat_id"] != "42" || fields["caption"] != "QR caption" {
			t.Fatalf("multipart fields = %#v", fields)
		}
		var gotMarkup InlineKeyboardMarkup
		if err := json.Unmarshal([]byte(fields["reply_markup"]), &gotMarkup); err != nil {
			t.Fatalf("reply_markup = %q: %v", fields["reply_markup"], err)
		}
		if !bytes.Equal(gotPhoto, wantPhoto) || gotFilename != "qr.png" {
			t.Fatalf("photo = %q filename=%q", gotPhoto, gotFilename)
		}
		if got := gotMarkup.InlineKeyboard[0][0].URL; got != wantMarkup.InlineKeyboard[0][0].URL {
			t.Fatalf("markup URL = %q", got)
		}
		return jsonResponse(`{"ok":true}`), nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.SendPhoto(context.Background(), 42, wantPhoto, "QR caption", wantMarkup); err != nil {
		t.Fatal(err)
	}
}

func TestClientSendPhotoErrorsAreTokenSafe(t *testing.T) {
	const testToken = "photo-secret-token"
	requestURL := apiBaseURL + "/bot" + testToken + "/sendPhoto"
	for _, test := range []struct {
		name      string
		transport roundTripFunc
	}{
		{name: "transport", transport: func(*http.Request) (*http.Response, error) {
			return nil, errors.New("request failed: " + requestURL)
		}},
		{name: "malformed response", transport: func(*http.Request) (*http.Response, error) {
			return jsonResponse("not-json"), nil
		}},
		{name: "non-2xx", transport: func(*http.Request) (*http.Response, error) {
			response := jsonResponse(`{"ok":false,"description":"` + requestURL + `"}`)
			response.StatusCode = http.StatusBadGateway
			return response, nil
		}},
		{name: "telegram failure", transport: func(*http.Request) (*http.Response, error) {
			return jsonResponse(`{"ok":false,"description":"` + requestURL + `"}`), nil
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			client, err := NewClientWithTransport(testToken, &http.Client{Transport: test.transport})
			if err != nil {
				t.Fatal(err)
			}
			err = client.SendPhoto(context.Background(), 42, []byte("photo"), "caption", nil)
			if err == nil {
				t.Fatal("SendPhoto succeeded unexpectedly")
			}
			if strings.Contains(err.Error(), testToken) || strings.Contains(err.Error(), requestURL) {
				t.Fatalf("error leaked token or URL: %v", err)
			}
		})
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
	config := Config{BotToken: "redacted-token", WebhookSecret: "header-secret", BotUsername: "synthetic_attendance_bot", WebhookPath: "independent-path"}
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

func TestConfigRequiresAllTelegramValuesWhenPartiallyConfigured(t *testing.T) {
	complete := Config{
		BotToken:      "redacted-token",
		WebhookSecret: "header-secret",
		BotUsername:   "synthetic_attendance_bot",
		WebhookPath:   "independent-path",
	}
	if !complete.Enabled() {
		t.Fatal("complete configuration is disabled")
	}
	if err := complete.Validate(); err != nil {
		t.Fatalf("complete config validation error = %v", err)
	}

	for _, tc := range []struct {
		name   string
		config Config
	}{
		{name: "bot token", config: Config{WebhookSecret: complete.WebhookSecret, BotUsername: complete.BotUsername, WebhookPath: complete.WebhookPath}},
		{name: "webhook secret", config: Config{BotToken: complete.BotToken, BotUsername: complete.BotUsername, WebhookPath: complete.WebhookPath}},
		{name: "bot username", config: Config{BotToken: complete.BotToken, WebhookSecret: complete.WebhookSecret, WebhookPath: complete.WebhookPath}},
		{name: "webhook path", config: Config{BotToken: complete.BotToken, WebhookSecret: complete.WebhookSecret, BotUsername: complete.BotUsername}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if !tc.config.Enabled() {
				t.Fatal("partially configured Telegram settings were treated as disabled")
			}
			if err := tc.config.Validate(); err == nil {
				t.Fatal("partially configured Telegram settings passed validation")
			}
		})
	}
}

func TestConfigStaysDisabledWhenNoTelegramValuesAreConfigured(t *testing.T) {
	config := Config{}
	if config.Enabled() {
		t.Fatal("empty Telegram configuration was enabled")
	}
	if err := config.Validate(); err != nil {
		t.Fatalf("disabled config validation error = %v", err)
	}
}

func TestConfigDoesNotSerializeSensitiveSettings(t *testing.T) {
	config := Config{
		BotToken:      "synthetic-bot-token",
		WebhookSecret: "synthetic-webhook-secret",
		BotUsername:   "synthetic_attendance_bot",
		WebhookPath:   "synthetic-webhook-path",
	}
	encoded, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"synthetic-bot-token", "synthetic-webhook-secret", "synthetic-webhook-path"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("sensitive Telegram setting %q appeared in JSON: %s", secret, encoded)
		}
	}
}

type photoTestSender struct {
	photoCalled   chan struct{}
	messageCalled chan struct{}
	photoErr      error
	photo         []byte
	caption       string
	markup        *InlineKeyboardMarkup
	messages      []string
}

func (s *photoTestSender) SendMessage(_ context.Context, _ int64, text string) error {
	s.messages = append(s.messages, text)
	if s.messageCalled != nil {
		s.messageCalled <- struct{}{}
	}
	return nil
}

func (s *photoTestSender) AnswerCallbackQuery(context.Context, string) error { return nil }

func (s *photoTestSender) SendPhoto(_ context.Context, _ int64, photo []byte, caption string, markup *InlineKeyboardMarkup) error {
	s.photo = append([]byte(nil), photo...)
	s.caption = caption
	s.markup = markup
	if s.photoCalled != nil {
		s.photoCalled <- struct{}{}
	}
	return s.photoErr
}

type timeoutPhotoSender struct {
	fallbackContextErr error
	messages           []string
}

func (s *timeoutPhotoSender) SendMessage(ctx context.Context, _ int64, text string) error {
	s.fallbackContextErr = ctx.Err()
	s.messages = append(s.messages, text)
	return nil
}

func (s *timeoutPhotoSender) AnswerCallbackQuery(context.Context, string) error { return nil }

func (s *timeoutPhotoSender) SendPhoto(ctx context.Context, _ int64, _ []byte, _ string, _ *InlineKeyboardMarkup) error {
	<-ctx.Done()
	return ctx.Err()
}

type fallbackOnlySender struct {
	messageCalled chan struct{}
	messages      []string
}

func (s *fallbackOnlySender) SendMessage(_ context.Context, _ int64, text string) error {
	s.messages = append(s.messages, text)
	s.messageCalled <- struct{}{}
	return nil
}

func (s *fallbackOnlySender) AnswerCallbackQuery(context.Context, string) error { return nil }

func TestDispatcherSendsPhotoToPhotoSender(t *testing.T) {
	sender := &photoTestSender{photoCalled: make(chan struct{}, 1), messageCalled: make(chan struct{}, 1)}
	dispatcher := NewDispatcher(sender, 1)
	if !dispatcher.Enqueue([]Action{{Kind: SendPhoto, ChatID: 42, Photo: []byte("png"), Caption: "caption", ReplyMarkup: &InlineKeyboardMarkup{}}}) {
		t.Fatal("photo action was not enqueued")
	}
	<-sender.photoCalled
	dispatcher.Close()
	if string(sender.photo) != "png" || sender.caption != "caption" || sender.markup == nil {
		t.Fatalf("photo delivery = photo %q caption %q markup %#v", sender.photo, sender.caption, sender.markup)
	}
	if len(sender.messages) != 0 {
		t.Fatalf("messages = %#v, want no fallback after successful photo", sender.messages)
	}
}

func TestDispatcherFallsBackWhenPhotoSendFails(t *testing.T) {
	sender := &photoTestSender{photoCalled: make(chan struct{}, 1), messageCalled: make(chan struct{}, 1), photoErr: errors.New("photo failed")}
	dispatcher := NewDispatcher(sender, 1)
	if !dispatcher.Enqueue([]Action{{Kind: SendPhoto, ChatID: 42, Photo: []byte("png"), FallbackText: "open link"}}) {
		t.Fatal("photo action was not enqueued")
	}
	<-sender.messageCalled
	dispatcher.Close()
	if len(sender.messages) != 1 || sender.messages[0] != "open link" {
		t.Fatalf("fallback messages = %#v", sender.messages)
	}
}

func TestDispatcherUsesFreshContextForTimedOutPhotoFallback(t *testing.T) {
	sender := &timeoutPhotoSender{}
	dispatcher := &Dispatcher{sender: sender}
	photoCtx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	dispatcher.deliverPhoto(photoCtx, Action{Kind: SendPhoto, ChatID: 42, Photo: []byte("png"), FallbackText: "open link"})

	if sender.fallbackContextErr != nil {
		t.Fatalf("fallback context error = %v, want nil", sender.fallbackContextErr)
	}
	if len(sender.messages) != 1 || sender.messages[0] != "open link" {
		t.Fatalf("fallback messages = %#v", sender.messages)
	}
}

func TestDispatcherFallsBackWhenSenderLacksPhotoSupport(t *testing.T) {
	sender := &fallbackOnlySender{messageCalled: make(chan struct{}, 1)}
	dispatcher := NewDispatcher(sender, 1)
	if !dispatcher.Enqueue([]Action{{Kind: SendPhoto, ChatID: 42, Photo: []byte("png"), FallbackText: "open link"}}) {
		t.Fatal("photo action was not enqueued")
	}
	<-sender.messageCalled
	dispatcher.Close()
	if len(sender.messages) != 1 || sender.messages[0] != "open link" {
		t.Fatalf("fallback messages = %#v", sender.messages)
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

type fakePairingFlow struct {
	fakePairingLookup
	proposal     PairingProposal
	confirmation PairingConfirmation
	confirmErr   error
	discardErr   error
	seenName     string
	seenAttempt  string
	discarded    string
}

func (f *fakePairingFlow) ProposePairing(_ context.Context, _ int64, name string) (PairingProposal, error) {
	f.seenName = name
	return f.proposal, nil
}

func (f *fakePairingFlow) ConfirmPairing(_ context.Context, _ int64, attemptID string) (PairingConfirmation, error) {
	f.seenAttempt = attemptID
	if f.confirmErr != nil {
		return PairingConfirmation{}, f.confirmErr
	}
	return f.confirmation, nil
}

func (f *fakePairingFlow) DiscardPairing(_ context.Context, _ int64, attemptID string) error {
	f.discarded = attemptID
	return f.discardErr
}

func TestBotProposesExactlyOneStrongCandidateWithKeyboard(t *testing.T) {
	flow := &fakePairingFlow{proposal: PairingProposal{
		AttemptID: "attempt-1", UserID: "user-1", Name: "TAN WEI MING", Rank: "CPL", Battery: "Bravo", Score: 96,
	}}
	bot := NewBot(flow)
	actions, err := bot.HandleUpdate(context.Background(), Update{Message: &Message{
		From: &User{ID: 42}, Chat: Chat{ID: 42, Type: "private"}, Text: "TAN WEI MIMG",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if flow.seenName != "TAN WEI MIMG" || len(actions) != 1 {
		t.Fatalf("name=%q actions=%#v", flow.seenName, actions)
	}
	if actions[0].Text != "Are you CPL TAN WEI MING, Bravo?" {
		t.Fatalf("proposal text = %q", actions[0].Text)
	}
	if actions[0].ReplyMarkup == nil || len(actions[0].ReplyMarkup.InlineKeyboard) != 1 || len(actions[0].ReplyMarkup.InlineKeyboard[0]) != 2 {
		t.Fatalf("keyboard = %#v", actions[0].ReplyMarkup)
	}
	if actions[0].ReplyMarkup.InlineKeyboard[0][0].CallbackData != "p:y:attempt-1" {
		t.Fatalf("yes callback = %q", actions[0].ReplyMarkup.InlineKeyboard[0][0].CallbackData)
	}
}

func TestBotWeakMatchNamesNobody(t *testing.T) {
	flow := &fakePairingFlow{proposal: PairingProposal{NoMatch: true}}
	bot := NewBot(flow)
	actions, err := bot.HandleUpdate(context.Background(), Update{Message: &Message{
		From: &User{ID: 42}, Chat: Chat{ID: 42, Type: "private"}, Text: "unrelated",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(actions) != 1 || actions[0].Text != NoMatchReply || strings.Contains(actions[0].Text, "TAN WEI MING") {
		t.Fatalf("weak-match reply = %#v", actions)
	}
}

func TestBotConfirmationUsesCallbackAndDoesNotRevealConflict(t *testing.T) {
	flow := &fakePairingFlow{confirmation: PairingConfirmation{
		Outcome: PairingConflict,
		Pairing: Pairing{FullName: "HELD PERSON"},
	}}
	bot := NewBot(flow)
	actions, err := bot.HandleUpdate(context.Background(), Update{CallbackQuery: &CallbackQuery{
		ID: "callback-1", From: &User{ID: 42}, Data: "p:y:attempt-1",
		Message: &Message{Chat: Chat{ID: 42, Type: "private"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if flow.seenAttempt != "attempt-1" || len(actions) != 2 {
		t.Fatalf("attempt=%q actions=%#v", flow.seenAttempt, actions)
	}
	if actions[1].Text != PairingConflictReply || strings.Contains(actions[1].Text, "HELD PERSON") {
		t.Fatalf("conflict reply = %q", actions[1].Text)
	}
}

func TestBotUnpairedStartWithoutPayloadUsesTelegramDisplayName(t *testing.T) {
	flow := &fakePairingFlow{proposal: PairingProposal{
		AttemptID: "attempt-start", UserID: "user-start", Name: "SYNTHETIC DISPLAY SOLDIER", Rank: "PTE", Battery: "Alpha", Score: 100,
	}}
	bot := NewBot(flow)
	actions, err := bot.HandleUpdate(context.Background(), Update{Message: &Message{
		From: &User{ID: 42, FirstName: "Synthetic", LastName: "Display Soldier"},
		Chat: Chat{ID: 42, Type: "private"}, Text: "/start",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if flow.seenName != "Synthetic Display Soldier" {
		t.Fatalf("pairing name = %q, want Telegram display name", flow.seenName)
	}
	if len(actions) != 1 || actions[0].Text != "Are you PTE SYNTHETIC DISPLAY SOLDIER, Alpha?" {
		t.Fatalf("actions = %#v", actions)
	}
}

func TestBotUnpairedStartWithEmptyDisplayNameAsksForName(t *testing.T) {
	flow := &fakePairingFlow{}
	bot := NewBot(flow)
	actions, err := bot.HandleUpdate(context.Background(), Update{Message: &Message{
		From: &User{ID: 42}, Chat: Chat{ID: 42, Type: "private"}, Text: "/start",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(actions) != 1 || actions[0].Text != NamePromptReply {
		t.Fatalf("prompt = %#v", actions)
	}
	if flow.seenName != "" {
		t.Fatalf("pairing name = %q, want no pairing attempt", flow.seenName)
	}
}

func TestBotUnpairedMalformedStartUsesTelegramDisplayName(t *testing.T) {
	flow := &fakePairingFlow{proposal: PairingProposal{
		AttemptID: "attempt-malformed", UserID: "user-malformed", Name: "SYNTHETIC DISPLAY SOLDIER", Rank: "PTE", Battery: "Alpha",
	}}
	bot := NewBot(flow)
	_, err := bot.HandleUpdate(context.Background(), Update{Message: &Message{
		From: &User{ID: 42, FirstName: "Synthetic", LastName: "Display Soldier"},
		Chat: Chat{ID: 42, Type: "private"}, Text: "/start opaque payload",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if flow.seenName != "Synthetic Display Soldier" {
		t.Fatalf("pairing name = %q, want Telegram display name for malformed /start", flow.seenName)
	}
}

func TestParseStartCommandPreservesExplicitCommandState(t *testing.T) {
	for _, tc := range []struct {
		text       string
		isStart    bool
		hasPayload bool
		payload    string
	}{
		{text: "/start", isStart: true},
		{text: "/start opaque-code", isStart: true, hasPayload: true, payload: "opaque-code"},
		{text: "/start opaque code", isStart: true, hasPayload: true},
		{text: "/start@synthetic_bot", isStart: true},
		{text: "hello", isStart: false},
	} {
		t.Run(tc.text, func(t *testing.T) {
			got := parseStartCommand(tc.text)
			if got.isStartCommand != tc.isStart || got.hasPayload != tc.hasPayload || got.payload != tc.payload {
				t.Fatalf("parseStartCommand(%q) = %+v, want isStart=%v hasPayload=%v payload=%q", tc.text, got, tc.isStart, tc.hasPayload, tc.payload)
			}
		})
	}
}

func TestBotDeclineDiscardsProposal(t *testing.T) {
	flow := &fakePairingFlow{}
	bot := NewBot(flow)
	actions, err := bot.HandleUpdate(context.Background(), Update{CallbackQuery: &CallbackQuery{
		ID: "callback-1", From: &User{ID: 42}, Data: "p:n:attempt-1",
		Message: &Message{Chat: Chat{ID: 42, Type: "private"}},
	}})
	if err != nil || flow.discarded != "attempt-1" || len(actions) != 2 || actions[1].Text != PairingDeclinedReply {
		t.Fatalf("discarded=%q actions=%#v err=%v", flow.discarded, actions, err)
	}
}

func TestBotCallbackServiceErrorsStillReturnAcknowledgement(t *testing.T) {
	serviceErr := errors.New("pairing service unavailable")
	for _, tc := range []struct {
		name string
		data string
		flow func(*fakePairingFlow)
	}{
		{name: "discard", data: "p:n:attempt-1", flow: func(flow *fakePairingFlow) { flow.discardErr = serviceErr }},
		{name: "confirm", data: "p:y:attempt-1", flow: func(flow *fakePairingFlow) { flow.confirmErr = serviceErr }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			flow := &fakePairingFlow{}
			tc.flow(flow)
			actions, err := NewBot(flow).HandleUpdate(context.Background(), Update{CallbackQuery: &CallbackQuery{
				ID: "callback-1", From: &User{ID: 42}, Data: tc.data,
				Message: &Message{Chat: Chat{ID: 42, Type: "private"}},
			}})
			if !errors.Is(err, serviceErr) {
				t.Fatalf("error = %v, want service error", err)
			}
			if len(actions) != 1 || actions[0].Kind != AnswerCallbackQuery || actions[0].CallbackQueryID != "callback-1" {
				t.Fatalf("actions = %#v, want acknowledgement only", actions)
			}
		})
	}
}
