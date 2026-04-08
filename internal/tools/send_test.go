package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/Prgebish/mcp-telegram/internal/acl"
	"github.com/Prgebish/mcp-telegram/internal/config"
	"github.com/gotd/td/bin"
	"github.com/gotd/td/tg"
)

func TestHandleSend_EmptyInput(t *testing.T) {
	deps := newTestDeps(newAllowedResolver(), nil, config.PermSend)
	result := handleSend(context.Background(), deps, sendInput{Chat: "@testchat"})
	if !result.IsError {
		t.Fatal("expected error")
	}
	if text := resultText(result); !strings.Contains(text, "either text or file") {
		t.Errorf("error = %q, want 'either text or file'", text)
	}
}

func TestHandleSend_ResolveError(t *testing.T) {
	deps := newTestDeps(newErrorResolver("peer not found"), nil, config.PermSend)
	result := handleSend(context.Background(), deps, sendInput{Chat: "@unknown", Text: "hi"})
	if !result.IsError {
		t.Fatal("expected error")
	}
	if text := resultText(result); !strings.Contains(text, "peer not found") {
		t.Errorf("error = %q, want 'peer not found'", text)
	}
}

func TestHandleSend_ACLDenied(t *testing.T) {
	checker, err := acl.NewChecker(config.ACLConfig{
		Chats: []config.ChatRule{
			{Match: "@alice", Permissions: []config.Permission{config.PermRead}},
		},
	})
	if err != nil {
		t.Fatalf("NewChecker: %v", err)
	}

	peer := acl.PeerIdentity{Kind: acl.KindUser, ID: 1, Username: "alice"}

	if checker.Allowed(peer, config.PermSend) {
		t.Error("@alice should not have send permission (only read)")
	}
	if !checker.Allowed(peer, config.PermRead) {
		t.Error("@alice should have read permission")
	}
}

func TestHandleSend_ACLAllow(t *testing.T) {
	checker, err := acl.NewChecker(config.ACLConfig{
		Chats: []config.ChatRule{
			{Match: "@bob", Permissions: []config.Permission{config.PermRead, config.PermSend}},
		},
	})
	if err != nil {
		t.Fatalf("NewChecker: %v", err)
	}

	peer := acl.PeerIdentity{Kind: acl.KindUser, ID: 2, Username: "bob"}

	if !checker.Allowed(peer, config.PermSend) {
		t.Error("@bob should have send permission")
	}
}

func TestHandleSend_ACLDeniedHandler(t *testing.T) {
	deps := newTestDeps(newAllowedResolver(), nil, config.PermRead) // no send perm
	result := handleSend(context.Background(), deps, sendInput{Chat: "@testchat", Text: "hi"})
	if !result.IsError {
		t.Fatal("expected error")
	}
	if text := resultText(result); !strings.Contains(text, "access denied") {
		t.Errorf("error = %q, want 'access denied'", text)
	}
}

func TestHandleSend_InvalidReplyTo(t *testing.T) {
	api := mockTgClient(func(_ context.Context, _ bin.Encoder, _ bin.Decoder) error {
		return nil
	})
	deps := newTestDeps(newAllowedResolver(), api, config.PermSend)
	result := handleSend(context.Background(), deps, sendInput{Chat: "@testchat", Text: "hi", ReplyTo: "abc"})
	if !result.IsError {
		t.Fatal("expected error")
	}
	if text := resultText(result); !strings.Contains(text, "invalid reply_to") {
		t.Errorf("error = %q, want 'invalid reply_to'", text)
	}
}

func TestHandleSend_APIError(t *testing.T) {
	deps := newTestDeps(newAllowedResolver(), errorTgClient("flood wait"), config.PermSend)
	result := handleSend(context.Background(), deps, sendInput{Chat: "@testchat", Text: "hi"})
	if !result.IsError {
		t.Fatal("expected error")
	}
	if text := resultText(result); !strings.Contains(text, "flood wait") {
		t.Errorf("error = %q, want 'flood wait'", text)
	}
}

func TestHandleSend_Success(t *testing.T) {
	var sentMsg string
	api := mockTgClient(func(_ context.Context, input bin.Encoder, output bin.Decoder) error {
		if req, ok := input.(*tg.MessagesSendMessageRequest); ok {
			sentMsg = req.Message
		}
		if v, ok := output.(*tg.UpdatesBox); ok {
			v.Updates = &tg.Updates{}
		}
		return nil
	})
	deps := newTestDeps(newAllowedResolver(), api, config.PermSend)
	result := handleSend(context.Background(), deps, sendInput{Chat: "@testchat", Text: "hello world"})
	if result.IsError {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	if sentMsg != "hello world" {
		t.Errorf("sent message = %q, want %q", sentMsg, "hello world")
	}
	if text := resultText(result); !strings.Contains(text, "Message sent to @testchat") {
		t.Errorf("result = %q, want 'Message sent to @testchat'", text)
	}
}

func TestHandleSend_SuccessWithReply(t *testing.T) {
	var replyID int
	api := mockTgClient(func(_ context.Context, input bin.Encoder, output bin.Decoder) error {
		if req, ok := input.(*tg.MessagesSendMessageRequest); ok {
			if reply, ok := req.ReplyTo.(*tg.InputReplyToMessage); ok {
				replyID = reply.ReplyToMsgID
			}
		}
		if v, ok := output.(*tg.UpdatesBox); ok {
			v.Updates = &tg.Updates{}
		}
		return nil
	})
	deps := newTestDeps(newAllowedResolver(), api, config.PermSend)
	result := handleSend(context.Background(), deps, sendInput{Chat: "@testchat", Text: "reply", ReplyTo: "42"})
	if result.IsError {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	if replyID != 42 {
		t.Errorf("reply_to = %d, want 42", replyID)
	}
	text := resultText(result)
	if !strings.Contains(text, "reply to 42") {
		t.Errorf("result = %q, want 'reply to 42'", text)
	}
}
