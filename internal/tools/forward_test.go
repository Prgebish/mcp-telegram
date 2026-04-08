package tools

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/Prgebish/mcp-telegram/internal/acl"
	"github.com/Prgebish/mcp-telegram/internal/config"
	"github.com/gotd/td/bin"
	"github.com/gotd/td/tg"
)

func TestHandleForward_InvalidMessageIDs(t *testing.T) {
	deps := newTestDeps(newAllowedResolver(), nil, config.PermRead, config.PermSend)
	tests := []struct {
		name string
		ids  string
	}{
		{"empty", ""},
		{"spaces only", "   "},
		{"not a number", "abc"},
		{"mixed", "123,abc,456"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handleForward(context.Background(), deps, forwardInput{
				FromChat: "@testchat", ToChat: "@testchat", MessageIDs: tt.ids,
			})
			if !result.IsError {
				t.Fatal("expected error")
			}
			if text := resultText(result); !strings.Contains(text, "invalid message_ids") {
				t.Errorf("error = %q, want 'invalid message_ids'", text)
			}
		})
	}
}

func TestHandleForward_FromResolveError(t *testing.T) {
	deps := newTestDeps(newErrorResolver("peer not found"), nil, config.PermRead, config.PermSend)
	result := handleForward(context.Background(), deps, forwardInput{
		FromChat: "@unknown", ToChat: "@testchat", MessageIDs: "123",
	})
	if !result.IsError {
		t.Fatal("expected error")
	}
	if text := resultText(result); !strings.Contains(text, "cannot resolve from_chat") {
		t.Errorf("error = %q, want 'cannot resolve from_chat'", text)
	}
}

func TestHandleForward_FromACLDenied(t *testing.T) {
	deps := newTestDeps(newAllowedResolver(), nil, config.PermSend) // no read
	result := handleForward(context.Background(), deps, forwardInput{
		FromChat: "@testchat", ToChat: "@testchat", MessageIDs: "123",
	})
	if !result.IsError {
		t.Fatal("expected error")
	}
	if text := resultText(result); !strings.Contains(text, "'read' permission") {
		t.Errorf("error = %q, want read permission denied", text)
	}
}

func TestHandleForward_ToResolveError(t *testing.T) {
	callCount := 0
	resolver := &dynamicMockResolver{
		fn: func(_ context.Context, ref string) (Peer, acl.PeerIdentity, error) {
			callCount++
			if callCount == 1 {
				return newAllowedResolver().peer, newAllowedResolver().identity, nil
			}
			return nil, acl.PeerIdentity{}, fmt.Errorf("destination not found")
		},
	}
	deps := newTestDeps(resolver, nil, config.PermRead, config.PermSend)
	result := handleForward(context.Background(), deps, forwardInput{
		FromChat: "@testchat", ToChat: "@unknown", MessageIDs: "123",
	})
	if !result.IsError {
		t.Fatal("expected error")
	}
	if text := resultText(result); !strings.Contains(text, "cannot resolve to_chat") {
		t.Errorf("error = %q, want 'cannot resolve to_chat'", text)
	}
}

func TestHandleForward_ToACLDenied(t *testing.T) {
	deps := newTestDeps(newAllowedResolver(), nil, config.PermRead) // no send
	result := handleForward(context.Background(), deps, forwardInput{
		FromChat: "@testchat", ToChat: "@testchat", MessageIDs: "123",
	})
	if !result.IsError {
		t.Fatal("expected error")
	}
	if text := resultText(result); !strings.Contains(text, "'send' permission") {
		t.Errorf("error = %q, want send permission denied", text)
	}
}

func TestHandleForward_APIError(t *testing.T) {
	deps := newTestDeps(newAllowedResolver(), errorTgClient("forward blocked"), config.PermRead, config.PermSend)
	result := handleForward(context.Background(), deps, forwardInput{
		FromChat: "@testchat", ToChat: "@testchat", MessageIDs: "123",
	})
	if !result.IsError {
		t.Fatal("expected error")
	}
	if text := resultText(result); !strings.Contains(text, "forward blocked") {
		t.Errorf("error = %q, want 'forward blocked'", text)
	}
}

func TestHandleForward_Success(t *testing.T) {
	var capturedIDs []int
	api := mockTgClient(func(_ context.Context, input bin.Encoder, output bin.Decoder) error {
		if req, ok := input.(*tg.MessagesForwardMessagesRequest); ok {
			capturedIDs = req.ID
		}
		if v, ok := output.(*tg.UpdatesBox); ok {
			v.Updates = &tg.Updates{}
		}
		return nil
	})
	deps := newTestDeps(newAllowedResolver(), api, config.PermRead, config.PermSend)
	result := handleForward(context.Background(), deps, forwardInput{
		FromChat: "@testchat", ToChat: "@testchat", MessageIDs: "100, 200, 300",
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	if len(capturedIDs) != 3 || capturedIDs[0] != 100 || capturedIDs[1] != 200 || capturedIDs[2] != 300 {
		t.Errorf("forwarded IDs = %v, want [100 200 300]", capturedIDs)
	}
	text := resultText(result)
	if !strings.Contains(text, "Forwarded 3 message(s)") {
		t.Errorf("result = %q, want 'Forwarded 3 message(s)'", text)
	}
}

func TestParseMessageIDs(t *testing.T) {
	tests := []struct {
		input   string
		want    []int
		wantErr bool
	}{
		{"123", []int{123}, false},
		{"1,2,3", []int{1, 2, 3}, false},
		{" 10 , 20 , 30 ", []int{10, 20, 30}, false},
		{"", nil, true},
		{"abc", nil, true},
		{"1,,2", []int{1, 2}, false}, // empty parts skipped
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseMessageIDs(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("got[%d] = %d, want %d", i, got[i], tt.want[i])
				}
			}
		})
	}
}
