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

func TestHandleSearch_EmptyQuery(t *testing.T) {
	deps := newTestDeps(newAllowedResolver(), nil, config.PermRead)
	result := handleSearch(context.Background(), deps, searchInput{Chat: "@testchat", Query: ""})
	if !result.IsError {
		t.Fatal("expected error")
	}
	if text := resultText(result); !strings.Contains(text, "query is required") {
		t.Errorf("error = %q, want 'query is required'", text)
	}
}

func TestHandleSearch_ResolveError(t *testing.T) {
	deps := newTestDeps(newErrorResolver("peer not found"), nil, config.PermRead)
	result := handleSearch(context.Background(), deps, searchInput{Chat: "@unknown", Query: "test"})
	if !result.IsError {
		t.Fatal("expected error")
	}
	if text := resultText(result); !strings.Contains(text, "peer not found") {
		t.Errorf("error = %q, want 'peer not found'", text)
	}
}

func TestHandleSearch_ACLDenied(t *testing.T) {
	deps := newTestDeps(newAllowedResolver(), nil, config.PermSend) // no read
	result := handleSearch(context.Background(), deps, searchInput{Chat: "@testchat", Query: "test"})
	if !result.IsError {
		t.Fatal("expected error")
	}
	if text := resultText(result); !strings.Contains(text, "access denied") {
		t.Errorf("error = %q, want 'access denied'", text)
	}
}

func TestHandleSearch_APIError(t *testing.T) {
	deps := newTestDeps(newAllowedResolver(), errorTgClient("search failed"), config.PermRead)
	result := handleSearch(context.Background(), deps, searchInput{Chat: "@testchat", Query: "test"})
	if !result.IsError {
		t.Fatal("expected error")
	}
	if text := resultText(result); !strings.Contains(text, "search failed") {
		t.Errorf("error = %q, want 'search failed'", text)
	}
}

func TestHandleSearch_NoResults(t *testing.T) {
	api := mockTgClient(func(_ context.Context, _ bin.Encoder, output bin.Decoder) error {
		if v, ok := output.(*tg.MessagesMessagesBox); ok {
			v.Messages = &tg.MessagesMessages{
				Messages: nil,
				Users:    nil,
				Chats:    nil,
			}
		}
		return nil
	})
	deps := newTestDeps(newAllowedResolver(), api, config.PermRead)
	result := handleSearch(context.Background(), deps, searchInput{Chat: "@testchat", Query: "nonexistent"})
	if result.IsError {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "No messages found") {
		t.Errorf("result = %q, want 'No messages found'", text)
	}
	if !strings.Contains(text, "nonexistent") {
		t.Errorf("result = %q, should include the query", text)
	}
}

func TestHandleSearch_FromResolveError(t *testing.T) {
	// Chat resolves fine but "from" fails.
	callCount := 0
	resolver := &mockResolver{}
	resolver.peer = &mockPeer{inputPeer: &tg.InputPeerUser{UserID: 100}}
	resolver.identity = acl.PeerIdentity{Kind: acl.KindUser, ID: 100, Username: "testchat"}

	dynamicResolver := &dynamicMockResolver{
		fn: func(_ context.Context, ref string) (Peer, acl.PeerIdentity, error) {
			callCount++
			if callCount == 1 {
				// First call: resolve chat — success.
				return resolver.peer, resolver.identity, nil
			}
			// Second call: resolve from — fail.
			return nil, acl.PeerIdentity{}, fmt.Errorf("unknown sender")
		},
	}

	deps := newTestDeps(dynamicResolver, nil, config.PermRead)
	result := handleSearch(context.Background(), deps, searchInput{Chat: "@testchat", Query: "test", From: "@nobody"})
	if !result.IsError {
		t.Fatal("expected error")
	}
	if text := resultText(result); !strings.Contains(text, "unknown sender") {
		t.Errorf("error = %q, want 'unknown sender'", text)
	}
}

func TestHandleSearch_FromAndOffsetPassthrough(t *testing.T) {
	var capturedReq *tg.MessagesSearchRequest
	api := mockTgClient(func(_ context.Context, input bin.Encoder, output bin.Decoder) error {
		if req, ok := input.(*tg.MessagesSearchRequest); ok {
			capturedReq = req
		}
		if v, ok := output.(*tg.MessagesMessagesBox); ok {
			v.Messages = &tg.MessagesMessages{}
		}
		return nil
	})
	deps := newTestDeps(newAllowedResolver(), api, config.PermRead)
	result := handleSearch(context.Background(), deps, searchInput{
		Chat:     "@testchat",
		Query:    "deploy",
		OffsetID: 999,
		From:     "@testchat", // resolves to the same mock peer
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	if capturedReq == nil {
		t.Fatal("MessagesSearchRequest not captured")
	}
	if capturedReq.OffsetID != 999 {
		t.Errorf("OffsetID = %d, want 999", capturedReq.OffsetID)
	}
	fromID, ok := capturedReq.GetFromID()
	if !ok {
		t.Error("FromID not set in request")
	} else if fromID == nil {
		t.Error("FromID is nil")
	}
}

func TestHandleSearch_Success(t *testing.T) {
	var capturedQuery string
	api := mockTgClient(func(_ context.Context, input bin.Encoder, output bin.Decoder) error {
		if req, ok := input.(*tg.MessagesSearchRequest); ok {
			capturedQuery = req.Q
		}
		if v, ok := output.(*tg.MessagesMessagesBox); ok {
			v.Messages = &tg.MessagesMessages{
				Messages: []tg.MessageClass{
					&tg.Message{
						ID:      42,
						Message: "found this message",
						Date:    1700000000,
						FromID:  &tg.PeerUser{UserID: 1},
					},
				},
				Users: []tg.UserClass{
					&tg.User{ID: 1, FirstName: "Alice"},
				},
				Chats: nil,
			}
		}
		return nil
	})
	deps := newTestDeps(newAllowedResolver(), api, config.PermRead)
	result := handleSearch(context.Background(), deps, searchInput{Chat: "@testchat", Query: "hello"})
	if result.IsError {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	if capturedQuery != "hello" {
		t.Errorf("query sent to API = %q, want %q", capturedQuery, "hello")
	}
	text := resultText(result)
	if !strings.Contains(text, "[42]") {
		t.Errorf("result = %q, want message ID [42]", text)
	}
	if !strings.Contains(text, "Alice") {
		t.Errorf("result = %q, want sender 'Alice'", text)
	}
	if !strings.Contains(text, "found this message") {
		t.Errorf("result = %q, want message text", text)
	}
}
