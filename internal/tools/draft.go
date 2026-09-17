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
	// ParseMode is optional; an empty value preserves the previous behaviour
	// of sending the text verbatim.
	ParseMode string `json:"parse_mode,omitempty" jsonschema:"Formatting of 'text'. Empty sends it verbatim; \"markdown\" interprets **bold**, __italic__, ~~strike~~, ||spoiler||, code, pre and [text](url)."`
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
	peer, identity, err := deps.Resolver.ResolvePeerForTool(ctx, input.Chat)
	if err != nil {
		return toolError(fmt.Sprintf("cannot resolve chat: %v", err))
	}

	if !deps.ACL.Allowed(identity, config.PermDraft) {
		return toolError(fmt.Sprintf("access denied: %s does not have 'draft' permission", input.Chat))
	}

	message, entities, err := renderText(input.Text, input.ParseMode)
	if err != nil {
		return toolError(err.Error())
	}

	req := &tg.MessagesSaveDraftRequest{
		Peer:    peer.InputPeer(),
		Message: message,
	}
	if len(entities) > 0 {
		req.SetEntities(entities)
	}

	_, err = deps.API.MessagesSaveDraft(ctx, req)
	if err != nil {
		return toolError(fmt.Sprintf("failed to save draft: %v", err))
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Draft saved in %s", input.Chat)},
		},
	}
}
