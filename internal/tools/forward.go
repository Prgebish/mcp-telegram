package tools

import (
	"context"
	"fmt"
	"math/rand/v2"
	"strconv"
	"strings"

	"github.com/Prgebish/mcp-telegram/internal/config"
	"github.com/gotd/td/tg"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type forwardInput struct {
	FromChat   string `json:"from_chat" jsonschema:"required,Source chat: @username, user:ID, chat:ID, or channel:ID"`
	ToChat     string `json:"to_chat" jsonschema:"required,Destination chat: @username, user:ID, chat:ID, or channel:ID"`
	MessageIDs string `json:"message_ids" jsonschema:"required,Comma-separated message IDs to forward (e.g. 123,456,789)"`
}

func registerForward(server *mcp.Server, deps *Deps) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "tg_forward",
		Description: "Forward messages from one whitelisted chat to another. Requires 'read' on source and 'send' on destination.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: ptrBool(true),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input forwardInput) (*mcp.CallToolResult, any, error) {
		return handleForward(ctx, deps, input), nil, nil
	})
}

func handleForward(ctx context.Context, deps *Deps, input forwardInput) *mcp.CallToolResult {
	// Parse message IDs.
	ids, err := parseMessageIDs(input.MessageIDs)
	if err != nil {
		return toolError(fmt.Sprintf("invalid message_ids: %v", err))
	}

	// Resolve and check source chat.
	fromPeer, fromIdentity, err := deps.Resolver.ResolvePeerForTool(ctx, input.FromChat)
	if err != nil {
		return toolError(fmt.Sprintf("cannot resolve from_chat: %v", err))
	}
	if !deps.ACL.Allowed(fromIdentity, config.PermRead) {
		return toolError(fmt.Sprintf("access denied: %s does not have 'read' permission", input.FromChat))
	}

	// Resolve and check destination chat.
	toPeer, toIdentity, err := deps.Resolver.ResolvePeerForTool(ctx, input.ToChat)
	if err != nil {
		return toolError(fmt.Sprintf("cannot resolve to_chat: %v", err))
	}
	if !deps.ACL.Allowed(toIdentity, config.PermSend) {
		return toolError(fmt.Sprintf("access denied: %s does not have 'send' permission", input.ToChat))
	}

	// Generate random IDs for each message.
	randomIDs := make([]int64, len(ids))
	for i := range randomIDs {
		randomIDs[i] = rand.Int64()
	}

	_, err = deps.API.MessagesForwardMessages(ctx, &tg.MessagesForwardMessagesRequest{
		FromPeer: fromPeer.InputPeer(),
		ToPeer:   toPeer.InputPeer(),
		ID:       ids,
		RandomID: randomIDs,
	})
	if err != nil {
		return toolError(fmt.Sprintf("failed to forward messages: %v", err))
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Forwarded %d message(s) from %s to %s", len(ids), input.FromChat, input.ToChat)},
		},
	}
}

func parseMessageIDs(s string) ([]int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("message_ids is empty")
	}
	parts := strings.Split(s, ",")
	ids := make([]int, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		id, err := strconv.Atoi(p)
		if err != nil {
			return nil, fmt.Errorf("invalid ID %q: %v", p, err)
		}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("no valid message IDs")
	}
	return ids, nil
}
