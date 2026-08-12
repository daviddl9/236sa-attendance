package telegram

import (
	"context"
	"testing"
)

type fakeAttendanceMarker struct {
	result   AttendanceResult
	seenID   int64
	seenCode string
	called   int
}

func (f *fakeAttendanceMarker) MarkAttendance(_ context.Context, telegramID int64, code string) (AttendanceResult, error) {
	f.called++
	f.seenID = telegramID
	f.seenCode = code
	return f.result, nil
}

type attendancePairingFlow struct {
	fakePairingLookup
	proposed bool
	seenName string
	proposal PairingProposal
}

func (f *attendancePairingFlow) ProposePairing(_ context.Context, _ int64, name string) (PairingProposal, error) {
	f.proposed = true
	f.seenName = name
	return f.proposal, nil
}

func (f *attendancePairingFlow) ConfirmPairing(context.Context, int64, string) (PairingConfirmation, error) {
	return PairingConfirmation{Outcome: PairingStale}, nil
}

func TestBotRoutesPairedStartPayloadToAttendanceMarker(t *testing.T) {
	marker := &fakeAttendanceMarker{result: AttendanceResult{Outcome: AttendanceMarked, SessionName: "Morning parade"}}
	bot := NewBotWithAttendance(&fakePairingLookup{
		found:   true,
		pairing: Pairing{TelegramID: 7001, UserID: "soldier-1", FullName: "PTE SYNTHETIC SOLDIER"},
	}, marker)

	actions, err := bot.HandleUpdate(context.Background(), Update{Message: &Message{
		From: &User{ID: 7001}, Chat: Chat{ID: 7001, Type: "private"}, Text: "/start abcDEF_123456789012345",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if marker.called != 1 || marker.seenID != 7001 || marker.seenCode != "abcDEF_123456789012345" {
		t.Fatalf("marker call = (%d, %d, %q)", marker.called, marker.seenID, marker.seenCode)
	}
	if len(actions) != 1 || actions[0].Text != "Attendance marked for Morning parade." {
		t.Fatalf("actions = %#v", actions)
	}
}

func TestBotDoesNotUseDeepLinkPayloadForPairing(t *testing.T) {
	flow := &attendancePairingFlow{proposal: PairingProposal{NoMatch: true}}
	marker := &fakeAttendanceMarker{}
	bot := NewBotWithAttendance(flow, marker)

	actions, err := bot.HandleUpdate(context.Background(), Update{Message: &Message{
		From: &User{ID: 7002, FirstName: "Synthetic", LastName: "Soldier"}, Chat: Chat{ID: 7002, Type: "private"}, Text: "/start opaque-code",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !flow.proposed || flow.seenName != "Synthetic Soldier" {
		t.Fatalf("pairing input = proposed %v name %q, want display name only", flow.proposed, flow.seenName)
	}
	if marker.called != 0 {
		t.Fatalf("attendance marker called %d times for unpaired account", marker.called)
	}
	if len(actions) != 1 || actions[0].Text != NoMatchReply {
		t.Fatalf("actions = %#v, want normal pairing reply", actions)
	}
}

func TestBotPromptsWhenDeepLinkSenderHasNoDisplayName(t *testing.T) {
	flow := &attendancePairingFlow{}
	bot := NewBotWithAttendance(flow, &fakeAttendanceMarker{})

	actions, err := bot.HandleUpdate(context.Background(), Update{Message: &Message{
		From: &User{ID: 7004}, Chat: Chat{ID: 7004, Type: "private"}, Text: "/start opaque-code",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if flow.proposed {
		t.Fatal("empty Telegram display name entered pairing matcher")
	}
	if len(actions) != 1 || actions[0].Text != NamePromptReply {
		t.Fatalf("actions = %#v, want name prompt", actions)
	}
}

func TestBotMapsAttendanceOutcomesToSafeReplies(t *testing.T) {
	for _, test := range []struct {
		name   string
		result AttendanceResult
		want   string
	}{
		{name: "already", result: AttendanceResult{Outcome: AttendanceAlreadyMarked, SessionName: "Morning parade"}, want: "You are already marked for Morning parade."},
		{name: "closed", result: AttendanceResult{Outcome: AttendanceSessionClosed}, want: AttendanceClosedReply},
		{name: "out of scope", result: AttendanceResult{Outcome: AttendanceOutOfScope}, want: AttendanceOutOfScopeReply},
		{name: "unknown", result: AttendanceResult{Outcome: AttendanceUnknownCode}, want: AttendanceInvalidReply},
	} {
		t.Run(test.name, func(t *testing.T) {
			marker := &fakeAttendanceMarker{result: test.result}
			bot := NewBotWithAttendance(&fakePairingLookup{
				found:   true,
				pairing: Pairing{TelegramID: 7003, UserID: "soldier-3"},
			}, marker)
			actions, err := bot.HandleUpdate(context.Background(), Update{Message: &Message{
				From: &User{ID: 7003}, Chat: Chat{ID: 7003, Type: "private"}, Text: "/start opaque-code",
			}})
			if err != nil {
				t.Fatal(err)
			}
			if len(actions) != 1 || actions[0].Text != test.want {
				t.Fatalf("actions = %#v, want %q", actions, test.want)
			}
		})
	}
}
