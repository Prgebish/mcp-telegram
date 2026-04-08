package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/Prgebish/mcp-telegram/internal/config"
	"github.com/gotd/td/bin"
	"github.com/gotd/td/tg"
)

func TestHandleMarkRead_ResolveError(t *testing.T) {
	deps := newTestDeps(newErrorResolver("peer not found"), nil, config.PermMarkRead)
	result := handleMarkRead(context.Background(), deps, markReadInput{Chat: "@unknown"})
	if !result.IsError {
		t.Fatal("expected error")
	}
	if text := resultText(result); !strings.Contains(text, "peer not found") {
		t.Errorf("error = %q, want 'peer not found'", text)
	}
}

func TestHandleMarkRead_ACLDenied(t *testing.T) {
	deps := newTestDeps(newAllowedResolver(), nil, config.PermRead) // no mark_read perm
	result := handleMarkRead(context.Background(), deps, markReadInput{Chat: "@testchat"})
	if !result.IsError {
		t.Fatal("expected error")
	}
	if text := resultText(result); !strings.Contains(text, "access denied") {
		t.Errorf("error = %q, want 'access denied'", text)
	}
}

func TestHandleMarkRead_APIError(t *testing.T) {
	deps := newTestDeps(newAllowedResolver(), errorTgClient("rpc error"), config.PermMarkRead)
	result := handleMarkRead(context.Background(), deps, markReadInput{Chat: "@testchat"})
	if !result.IsError {
		t.Fatal("expected error")
	}
	if text := resultText(result); !strings.Contains(text, "rpc error") {
		t.Errorf("error = %q, want 'rpc error'", text)
	}
}

func TestHandleMarkRead_SuccessNonChannel(t *testing.T) {
	api := mockTgClient(func(_ context.Context, input bin.Encoder, _ bin.Decoder) error {
		if _, ok := input.(*tg.MessagesReadHistoryRequest); !ok {
			t.Errorf("expected MessagesReadHistoryRequest, got %T", input)
		}
		return nil
	})
	deps := newTestDeps(newAllowedResolver(), api, config.PermMarkRead)
	result := handleMarkRead(context.Background(), deps, markReadInput{Chat: "@testchat"})
	if result.IsError {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	if text := resultText(result); !strings.Contains(text, "Marked as read: @testchat") {
		t.Errorf("result = %q, want 'Marked as read: @testchat'", text)
	}
}

func TestHandleMarkRead_SuccessChannel(t *testing.T) {
	var usedChannelAPI bool
	api := mockTgClient(func(_ context.Context, input bin.Encoder, output bin.Decoder) error {
		switch input.(type) {
		case *tg.ChannelsReadHistoryRequest:
			usedChannelAPI = true
			if v, ok := output.(*tg.BoolBox); ok {
				v.Bool = &tg.BoolTrue{}
			}
		case *tg.MessagesReadHistoryRequest:
			t.Error("should use ChannelsReadHistory for channel peers, not MessagesReadHistory")
		}
		return nil
	})
	deps := newTestDeps(newChannelResolver(), api, config.PermMarkRead)
	result := handleMarkRead(context.Background(), deps, markReadInput{Chat: "@testchat"})
	if result.IsError {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	if !usedChannelAPI {
		t.Error("expected ChannelsReadHistory to be called for channel peer")
	}
}
