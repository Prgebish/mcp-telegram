package tools

import (
	"context"
	"fmt"

	"github.com/chestnykh/mcp-telegram/internal/config"
	"github.com/gotd/td/telegram/peers"
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
		peer, identity, err := deps.Client.ResolvePeerForTool(ctx, input.Chat)
		if err != nil {
			return toolError(fmt.Sprintf("cannot resolve chat: %v", err)), nil, nil
		}

		if !deps.ACL.Allowed(identity, config.PermMarkRead) {
			return toolError(fmt.Sprintf("access denied: %s does not have 'mark_read' permission", input.Chat)), nil, nil
		}

		api := deps.Client.API()

		switch p := peer.(type) {
		case peers.Channel:
			_, err = api.ChannelsReadHistory(ctx, &tg.ChannelsReadHistoryRequest{
				Channel: p.InputChannel(),
			})
		default:
			_, err = api.MessagesReadHistory(ctx, &tg.MessagesReadHistoryRequest{
				Peer: peer.InputPeer(),
			})
		}
		if err != nil {
			return toolError(fmt.Sprintf("failed to mark as read: %v", err)), nil, nil
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Marked as read: %s", input.Chat)},
			},
		}, nil, nil
	})
}
