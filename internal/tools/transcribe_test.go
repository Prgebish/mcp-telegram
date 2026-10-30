package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Prgebish/mcp-telegram/internal/acl"
	"github.com/Prgebish/mcp-telegram/internal/config"
	"github.com/gotd/td/bin"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func init() {
	transcribePollInitial = 10 * time.Millisecond
	transcribePollMax = 20 * time.Millisecond
}

func voiceMessage(id int, peer tg.PeerClass) *tg.Message {
	return &tg.Message{
		ID:     id,
		PeerID: peer,
		Media: &tg.MessageMediaDocument{
			Document: &tg.Document{
				Attributes: []tg.DocumentAttributeClass{&tg.DocumentAttributeAudio{Voice: true}},
			},
		},
	}
}

// transcribeAPI mocks the Telegram API: getMessages returns msg, and each
// transcribeAudio call is answered by the next element of replies (the last
// one repeats). It counts transcribeAudio calls.
type transcribeAPI struct {
	msg     tg.MessageClass
	replies []func(*tg.MessagesTranscribedAudio) error
	calls   int
	last    *tg.MessagesTranscribeAudioRequest
}

func (m *transcribeAPI) client() *tg.Client {
	return mockTgClient(func(_ context.Context, input bin.Encoder, output bin.Decoder) error {
		switch req := input.(type) {
		case *tg.MessagesGetMessagesRequest, *tg.ChannelsGetMessagesRequest:
			output.(*tg.MessagesMessagesBox).Messages = &tg.MessagesMessages{
				Messages: []tg.MessageClass{m.msg},
			}
			return nil
		case *tg.MessagesTranscribeAudioRequest:
			m.last = req
			i := min(m.calls, len(m.replies)-1)
			m.calls++
			return m.replies[i](output.(*tg.MessagesTranscribedAudio))
		}
		return nil
	})
}

func reply(pending bool, text string) func(*tg.MessagesTranscribedAudio) error {
	return func(out *tg.MessagesTranscribedAudio) error {
		*out = tg.MessagesTranscribedAudio{Pending: pending, TranscriptionID: 1, Text: text}
		return nil
	}
}

func replyErr(code int, msg string) func(*tg.MessagesTranscribedAudio) error {
	return func(*tg.MessagesTranscribedAudio) error {
		return tgerr.New(code, msg)
	}
}

func newTranscribeDeps(api *tg.Client, timeoutSeconds int) *Deps {
	deps := newTestDeps(newAllowedResolver(), api)
	deps.ACL = newTestChecker("user:100", config.PermRead)
	deps.Transcribe = config.TranscribeConfig{Enabled: true, TimeoutSeconds: timeoutSeconds}
	return deps
}

func decodeTranscribe(t *testing.T, r *mcp.CallToolResult) transcribeOutput {
	t.Helper()
	if r.IsError {
		t.Fatalf("unexpected error: %s", resultText(r))
	}
	var out transcribeOutput
	if err := json.Unmarshal([]byte(resultText(r)), &out); err != nil {
		t.Fatalf("decode %q: %v", resultText(r), err)
	}
	return out
}

func noTelegramClient(t *testing.T) *tg.Client {
	return mockTgClient(func(_ context.Context, input bin.Encoder, _ bin.Decoder) error {
		t.Fatalf("unexpected Telegram request %T", input)
		return nil
	})
}

func TestHandleTranscribe_OutsideACL(t *testing.T) {
	resolver := &dynamicMockResolver{
		fn: func(_ context.Context, ref string) (Peer, acl.PeerIdentity, error) {
			t.Fatalf("resolver called for %q", ref)
			return nil, acl.PeerIdentity{}, nil
		},
	}
	deps := newTranscribeDeps(noTelegramClient(t), 30)
	deps.Resolver = resolver

	for _, ref := range []string{"user:999", "chat:100", "channel:5"} {
		result := handleTranscribe(context.Background(), deps, transcribeInput{Peer: ref, MessageID: 1})
		if !result.IsError || !strings.Contains(resultText(result), "access denied") {
			t.Errorf("%s: expected access denied, got %q", ref, resultText(result))
		}
	}
}

func TestHandleTranscribe_NoReadPermission(t *testing.T) {
	deps := newTranscribeDeps(noTelegramClient(t), 30)
	deps.ACL = newTestChecker("user:100", config.PermSend, config.PermDraft)

	result := handleTranscribe(context.Background(), deps, transcribeInput{Peer: "user:100", MessageID: 1})
	if !result.IsError || !strings.Contains(resultText(result), "'read' permission") {
		t.Errorf("expected read permission denied, got %q", resultText(result))
	}
}

func TestHandleTranscribe_InvalidMessageID(t *testing.T) {
	deps := newTranscribeDeps(noTelegramClient(t), 30)
	result := handleTranscribe(context.Background(), deps, transcribeInput{Peer: "user:100", MessageID: 0})
	if !result.IsError || !strings.Contains(resultText(result), "message_id") {
		t.Errorf("expected message_id error, got %q", resultText(result))
	}
}

func TestHandleTranscribe_NotVoice(t *testing.T) {
	tests := []struct {
		name string
		msg  *tg.Message
	}{
		{"text", &tg.Message{ID: 5, PeerID: &tg.PeerUser{UserID: 100}, Message: "hello"}},
		{"photo", &tg.Message{ID: 5, PeerID: &tg.PeerUser{UserID: 100}, Media: &tg.MessageMediaPhoto{}}},
		{"music", &tg.Message{ID: 5, PeerID: &tg.PeerUser{UserID: 100}, Media: &tg.MessageMediaDocument{
			Document: &tg.Document{Attributes: []tg.DocumentAttributeClass{&tg.DocumentAttributeAudio{Title: "song"}}},
		}}},
		{"regular video", &tg.Message{ID: 5, PeerID: &tg.PeerUser{UserID: 100}, Media: &tg.MessageMediaDocument{
			Document: &tg.Document{Attributes: []tg.DocumentAttributeClass{&tg.DocumentAttributeVideo{}}},
		}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := &transcribeAPI{msg: tt.msg, replies: []func(*tg.MessagesTranscribedAudio) error{reply(false, "x")}}
			deps := newTranscribeDeps(api.client(), 30)
			result := handleTranscribe(context.Background(), deps, transcribeInput{Peer: "user:100", MessageID: 5})
			if !result.IsError || !strings.Contains(resultText(result), "not a voice message") {
				t.Errorf("expected not-voice error, got %q", resultText(result))
			}
			if api.calls != 0 {
				t.Errorf("transcribeAudio called %d times, want 0", api.calls)
			}
		})
	}
}

func TestHandleTranscribe_MessageFromOtherChat(t *testing.T) {
	// messages.getMessages ignores the peer for private chats, so a message
	// ID from a chat outside the ACL must not be transcribed.
	api := &transcribeAPI{
		msg:     voiceMessage(5, &tg.PeerUser{UserID: 999}),
		replies: []func(*tg.MessagesTranscribedAudio) error{reply(false, "secret")},
	}
	deps := newTranscribeDeps(api.client(), 30)
	result := handleTranscribe(context.Background(), deps, transcribeInput{Peer: "user:100", MessageID: 5})
	if !result.IsError || !strings.Contains(resultText(result), "not found") {
		t.Errorf("expected not found, got %q", resultText(result))
	}
	if api.calls != 0 {
		t.Errorf("transcribeAudio called %d times, want 0", api.calls)
	}
}

func TestHandleTranscribe_Done(t *testing.T) {
	api := &transcribeAPI{
		msg:     voiceMessage(5, &tg.PeerUser{UserID: 100}),
		replies: []func(*tg.MessagesTranscribedAudio) error{reply(false, "привет")},
	}
	deps := newTranscribeDeps(api.client(), 30)
	out := decodeTranscribe(t, handleTranscribe(context.Background(), deps, transcribeInput{Peer: "user:100", MessageID: 5}))
	want := transcribeOutput{Peer: "user:100", MessageID: 5, Status: "done", Text: "привет"}
	if out != want {
		t.Errorf("result = %+v, want %+v", out, want)
	}
	if api.last == nil || api.last.MsgID != 5 {
		t.Errorf("transcribeAudio request = %+v, want MsgID 5", api.last)
	}
}

func TestHandleTranscribe_RoundVideoInChannel(t *testing.T) {
	msg := &tg.Message{
		ID:     8,
		PeerID: &tg.PeerChannel{ChannelID: 200},
		Media: &tg.MessageMediaDocument{
			Document: &tg.Document{
				Attributes: []tg.DocumentAttributeClass{&tg.DocumentAttributeVideo{RoundMessage: true}},
			},
		},
	}
	api := &transcribeAPI{msg: msg, replies: []func(*tg.MessagesTranscribedAudio) error{reply(false, "кружок")}}
	deps := newTranscribeDeps(api.client(), 30)
	deps.Resolver = newChannelResolver()
	deps.ACL = newTestChecker("channel:200", config.PermRead)

	out := decodeTranscribe(t, handleTranscribe(context.Background(), deps, transcribeInput{Peer: "channel:200", MessageID: 8}))
	if out.Status != "done" || out.Text != "кружок" {
		t.Errorf("result = %+v", out)
	}
}

func TestHandleTranscribe_PendingThenDone(t *testing.T) {
	api := &transcribeAPI{
		msg: voiceMessage(5, &tg.PeerUser{UserID: 100}),
		replies: []func(*tg.MessagesTranscribedAudio) error{
			reply(true, ""),
			reply(true, "при"),
			reply(false, "привет"),
		},
	}
	deps := newTranscribeDeps(api.client(), 5)
	out := decodeTranscribe(t, handleTranscribe(context.Background(), deps, transcribeInput{Peer: "user:100", MessageID: 5}))
	if out.Status != "done" || out.Text != "привет" {
		t.Errorf("result = %+v, want done/привет", out)
	}
	if api.calls != 3 {
		t.Errorf("transcribeAudio called %d times, want 3", api.calls)
	}
}

func TestHandleTranscribe_PendingUntilTimeout(t *testing.T) {
	api := &transcribeAPI{
		msg:     voiceMessage(5, &tg.PeerUser{UserID: 100}),
		replies: []func(*tg.MessagesTranscribedAudio) error{reply(true, "частично")},
	}
	deps := newTranscribeDeps(api.client(), 1)

	start := time.Now()
	result := handleTranscribe(context.Background(), deps, transcribeInput{Peer: "user:100", MessageID: 5})
	elapsed := time.Since(start)

	out := decodeTranscribe(t, result)
	if out.Status != "pending" || out.Text != "частично" {
		t.Errorf("result = %+v, want pending with partial text", out)
	}
	// No upper bound: on a loaded runner wall-clock time is unreliable, and
	// a handler that never stops polling is caught by the test timeout.
	if elapsed < time.Second {
		t.Errorf("elapsed = %s, want at least the 1s timeout", elapsed)
	}
	if api.calls < 2 {
		t.Errorf("transcribeAudio called %d times, want repeated polling", api.calls)
	}
}

func TestHandleTranscribe_Errors(t *testing.T) {
	tests := []struct {
		name string
		err  func(*tg.MessagesTranscribedAudio) error
		want string
	}{
		{"premium", replyErr(403, "PREMIUM_ACCOUNT_REQUIRED"), "Telegram Premium is required"},
		{"bad id", replyErr(400, "MSG_ID_INVALID"), "invalid message ID"},
		{"too long", replyErr(400, "MSG_VOICE_TOO_LONG"), "too long"},
		{"not voice", replyErr(400, "MSG_VOICE_MISSING"), "not a voice message"},
		{"flood wait beyond timeout", replyErr(420, "FLOOD_WAIT_60"), "FLOOD_WAIT"},
		{"other", replyErr(500, "INTERNAL"), "transcription failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := &transcribeAPI{
				msg:     voiceMessage(5, &tg.PeerUser{UserID: 100}),
				replies: []func(*tg.MessagesTranscribedAudio) error{tt.err},
			}
			deps := newTranscribeDeps(api.client(), 1)
			result := handleTranscribe(context.Background(), deps, transcribeInput{Peer: "user:100", MessageID: 5})
			if !result.IsError || !strings.Contains(resultText(result), tt.want) {
				t.Errorf("result = %q, want error containing %q", resultText(result), tt.want)
			}
			if api.calls != 1 {
				t.Errorf("transcribeAudio called %d times, want 1", api.calls)
			}
		})
	}
}

func TestHandleTranscribe_TranscriptionFailed(t *testing.T) {
	api := &transcribeAPI{
		msg:     voiceMessage(5, &tg.PeerUser{UserID: 100}),
		replies: []func(*tg.MessagesTranscribedAudio) error{replyErr(400, "TRANSCRIPTION_FAILED")},
	}
	deps := newTranscribeDeps(api.client(), 30)
	out := decodeTranscribe(t, handleTranscribe(context.Background(), deps, transcribeInput{Peer: "user:100", MessageID: 5}))
	if out.Status != "failed" || out.Text != "" {
		t.Errorf("result = %+v, want failed", out)
	}
}

func TestHandleTranscribe_FloodWaitRetried(t *testing.T) {
	api := &transcribeAPI{
		msg: voiceMessage(5, &tg.PeerUser{UserID: 100}),
		replies: []func(*tg.MessagesTranscribedAudio) error{
			replyErr(420, "FLOOD_WAIT_1"),
			reply(false, "готово"),
		},
	}
	deps := newTranscribeDeps(api.client(), 5)

	start := time.Now()
	out := decodeTranscribe(t, handleTranscribe(context.Background(), deps, transcribeInput{Peer: "user:100", MessageID: 5}))
	if out.Status != "done" || out.Text != "готово" {
		t.Errorf("result = %+v, want done", out)
	}
	if elapsed := time.Since(start); elapsed < time.Second {
		t.Errorf("elapsed = %s, want at least the 1s FLOOD_WAIT", elapsed)
	}
}

func TestRegister_TranscribeFlag(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		deps := newTranscribeDeps(nil, 30)
		deps.Transcribe.Enabled = enabled

		server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
		Register(server, deps)

		ctx := context.Background()
		clientTransport, serverTransport := mcp.NewInMemoryTransports()
		if _, err := server.Connect(ctx, serverTransport, nil); err != nil {
			t.Fatal(err)
		}
		session, err := mcp.NewClient(&mcp.Implementation{Name: "client", Version: "0"}, nil).Connect(ctx, clientTransport, nil)
		if err != nil {
			t.Fatal(err)
		}
		tools, err := session.ListTools(ctx, nil)
		session.Close()
		if err != nil {
			t.Fatal(err)
		}

		found := false
		for _, tool := range tools.Tools {
			if tool.Name == "tg_transcribe" {
				found = true
			}
		}
		if found != enabled {
			t.Errorf("transcribe.enabled=%v: tg_transcribe registered = %v", enabled, found)
		}
	}
}
