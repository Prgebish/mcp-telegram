package tools

import (
	"context"
	"fmt"

	"github.com/Prgebish/mcp-telegram/internal/acl"
	"github.com/Prgebish/mcp-telegram/internal/config"
	"github.com/gotd/td/bin"
	"github.com/gotd/td/tg"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// --- mock peer types ---

type mockPeer struct {
	inputPeer tg.InputPeerClass
}

func (m *mockPeer) InputPeer() tg.InputPeerClass { return m.inputPeer }

type mockChannelPeer struct {
	inputPeer    tg.InputPeerClass
	inputChannel tg.InputChannelClass
}

func (m *mockChannelPeer) InputPeer() tg.InputPeerClass       { return m.inputPeer }
func (m *mockChannelPeer) InputChannel() tg.InputChannelClass { return m.inputChannel }

// --- mock resolver ---

type mockResolver struct {
	peer     Peer
	identity acl.PeerIdentity
	err      error
}

func (m *mockResolver) ResolvePeerForTool(_ context.Context, _ string) (Peer, acl.PeerIdentity, error) {
	return m.peer, m.identity, m.err
}

// --- mock invoker for *tg.Client ---

type funcInvoker func(ctx context.Context, input bin.Encoder, output bin.Decoder) error

func (f funcInvoker) Invoke(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
	return f(ctx, input, output)
}

func mockTgClient(handler func(ctx context.Context, input bin.Encoder, output bin.Decoder) error) *tg.Client {
	return tg.NewClient(funcInvoker(handler))
}

func errorTgClient(msg string) *tg.Client {
	return mockTgClient(func(_ context.Context, _ bin.Encoder, _ bin.Decoder) error {
		return fmt.Errorf("%s", msg)
	})
}

// --- test helpers ---

func newTestDeps(resolver PeerResolver, api *tg.Client, perms ...config.Permission) *Deps {
	checker := newTestChecker("@testchat", perms...)
	return &Deps{
		Resolver: resolver,
		API:      api,
		ACL:      checker,
		Limits: config.LimitsConfig{
			MaxMessagesPerRequest: 50,
			MaxDialogsPerRequest:  100,
		},
		Media: config.MediaConfig{},
	}
}

func newTestChecker(match string, perms ...config.Permission) *acl.Checker {
	checker, _ := acl.NewChecker(config.ACLConfig{
		Chats: []config.ChatRule{
			{Match: match, Permissions: perms},
		},
	})
	return checker
}

func newAllowedResolver() *mockResolver {
	return &mockResolver{
		peer:     &mockPeer{inputPeer: &tg.InputPeerUser{UserID: 100}},
		identity: acl.PeerIdentity{Kind: acl.KindUser, ID: 100, Username: "testchat"},
	}
}

func newErrorResolver(msg string) *mockResolver {
	return &mockResolver{err: fmt.Errorf("%s", msg)}
}

func newChannelResolver() *mockResolver {
	return &mockResolver{
		peer: &mockChannelPeer{
			inputPeer:    &tg.InputPeerChannel{ChannelID: 200},
			inputChannel: &tg.InputChannel{ChannelID: 200},
		},
		identity: acl.PeerIdentity{Kind: acl.KindChannel, ID: 200, Username: "testchat"},
	}
}

// resultText extracts text from a CallToolResult.
func resultText(r *mcp.CallToolResult) string {
	if r == nil || len(r.Content) == 0 {
		return ""
	}
	if tc, ok := r.Content[0].(*mcp.TextContent); ok {
		return tc.Text
	}
	return ""
}
