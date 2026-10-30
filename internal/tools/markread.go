package tools

import (
	"context"
	"fmt"

	"github.com/Prgebish/mcp-telegram/internal/config"
	"github.com/gotd/td/tg"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type markReadInput struct {
	Chat string `json:"chat" jsonschema:"required,Chat reference: @username, user:ID, chat:ID, or channel:ID"`
}

func registerMarkRead(server *mcp.Server, deps *Deps) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "tg_mark_read",
		Description: "Mark all messages as read in a whitelisted chat.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: ptrBool(false),
			IdempotentHint:  true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input markReadInput) (*mcp.CallToolResult, any, error) {
		return handleMarkRead(ctx, deps, input), nil, nil
	})
}

func handleMarkRead(ctx context.Context, deps *Deps, input markReadInput) *mcp.CallToolResult {
	peer, _, denied := resolveAllowed(ctx, deps, input.Chat, config.PermMarkRead)
	if denied != nil {
		return denied
	}

	var err error
	switch p := peer.(type) {
	case ChannelPeer:
		_, err = deps.API.ChannelsReadHistory(ctx, &tg.ChannelsReadHistoryRequest{
			Channel: p.InputChannel(),
		})
	default:
		_, err = deps.API.MessagesReadHistory(ctx, &tg.MessagesReadHistoryRequest{
			Peer: peer.InputPeer(),
		})
	}
	if err != nil {
		return toolError(fmt.Sprintf("failed to mark as read: %v", err))
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Marked as read: %s", input.Chat)},
		},
	}
}
