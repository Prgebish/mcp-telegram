package tools

import (
	"context"
	"fmt"

	"github.com/Prgebish/mcp-telegram/internal/config"
	"github.com/gotd/td/tg"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type draftInput struct {
	Chat string `json:"chat" jsonschema:"required,Chat reference: @username, user:ID, chat:ID, or channel:ID"`
	Text string `json:"text" jsonschema:"required,Draft message text"`
}

func registerDraft(server *mcp.Server, deps *Deps) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "tg_draft",
		Description: "Save a draft message in a whitelisted chat. Does NOT send the message.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: ptrBool(false),
			IdempotentHint:  true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input draftInput) (*mcp.CallToolResult, any, error) {
		return handleDraft(ctx, deps, input), nil, nil
	})
}

func handleDraft(ctx context.Context, deps *Deps, input draftInput) *mcp.CallToolResult {
	peer, _, denied := resolveAllowed(ctx, deps, input.Chat, config.PermDraft)
	if denied != nil {
		return denied
	}

	_, err := deps.API.MessagesSaveDraft(ctx, &tg.MessagesSaveDraftRequest{
		Peer:    peer.InputPeer(),
		Message: input.Text,
	})
	if err != nil {
		return toolError(fmt.Sprintf("failed to save draft: %v", err))
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Draft saved in %s", input.Chat)},
		},
	}
}
