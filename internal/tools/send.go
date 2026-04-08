package tools

import (
	"context"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Prgebish/mcp-telegram/internal/config"
	"github.com/gotd/td/telegram/uploader"
	"github.com/gotd/td/tg"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type sendInput struct {
	Chat    string `json:"chat" jsonschema:"required,Chat reference: @username, user:ID, chat:ID, or channel:ID"`
	Text    string `json:"text,omitempty" jsonschema:"Message text to send"`
	File    string `json:"file,omitempty" jsonschema:"Absolute path to a file to send"`
	ReplyTo string `json:"reply_to,omitempty" jsonschema:"Message ID to reply to"`
}

func registerSend(server *mcp.Server, deps *Deps) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "tg_send",
		Description: "Send a message or file to a whitelisted chat. Supports reply to specific messages.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: ptrBool(true),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input sendInput) (*mcp.CallToolResult, any, error) {
		return handleSend(ctx, deps, input), nil, nil
	})
}

func handleSend(ctx context.Context, deps *Deps, input sendInput) *mcp.CallToolResult {
	if input.Text == "" && input.File == "" {
		return toolError("either text or file (or both) must be provided")
	}

	peer, identity, err := deps.Resolver.ResolvePeerForTool(ctx, input.Chat)
	if err != nil {
		return toolError(fmt.Sprintf("cannot resolve chat: %v", err))
	}

	if !deps.ACL.Allowed(identity, config.PermSend) {
		return toolError(fmt.Sprintf("access denied: %s does not have 'send' permission", input.Chat))
	}

	var replyTo tg.InputReplyToClass
	var replyToID int
	if input.ReplyTo != "" {
		replyToID, err = strconv.Atoi(input.ReplyTo)
		if err != nil {
			return toolError(fmt.Sprintf("invalid reply_to: %v", err))
		}
		replyTo = &tg.InputReplyToMessage{ReplyToMsgID: replyToID}
	}

	// Send file if provided.
	if input.File != "" {
		result, _, _ := sendFile(ctx, deps, peer, input, replyTo)
		return result
	}

	// Send text message.
	_, err = deps.API.MessagesSendMessage(ctx, &tg.MessagesSendMessageRequest{
		Peer:     peer.InputPeer(),
		Message:  input.Text,
		RandomID: rand.Int64(),
		ReplyTo:  replyTo,
	})
	if err != nil {
		return toolError(fmt.Sprintf("failed to send message: %v", err))
	}

	result := fmt.Sprintf("Message sent to %s", input.Chat)
	if replyToID > 0 {
		result += fmt.Sprintf(" (reply to %d)", replyToID)
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: result},
		},
	}
}

func sendFile(ctx context.Context, deps *Deps, peer Peer, input sendInput, replyTo tg.InputReplyToClass) (*mcp.CallToolResult, any, error) {
	// Verify file exists.
	info, err := os.Stat(input.File)
	if err != nil {
		return toolError(fmt.Sprintf("file not found: %v", err)), nil, nil
	}
	if info.IsDir() {
		return toolError("cannot send a directory"), nil, nil
	}

	// Upload file.
	u := uploader.NewUploader(deps.API)
	uploaded, err := u.FromPath(ctx, input.File)
	if err != nil {
		return toolError(fmt.Sprintf("failed to upload file: %v", err)), nil, nil
	}

	// Determine media type by extension.
	ext := strings.ToLower(filepath.Ext(input.File))
	var media tg.InputMediaClass

	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp":
		media = &tg.InputMediaUploadedPhoto{
			File: uploaded,
		}
	default:
		media = &tg.InputMediaUploadedDocument{
			File: uploaded,
			Attributes: []tg.DocumentAttributeClass{
				&tg.DocumentAttributeFilename{FileName: filepath.Base(input.File)},
			},
		}
	}

	caption := input.Text
	_, err = deps.API.MessagesSendMedia(ctx, &tg.MessagesSendMediaRequest{
		Peer:     peer.InputPeer(),
		Media:    media,
		Message:  caption,
		RandomID: rand.Int64(),
		ReplyTo:  replyTo,
	})
	if err != nil {
		return toolError(fmt.Sprintf("failed to send file: %v", err)), nil, nil
	}

	result := fmt.Sprintf("File %s sent to %s", filepath.Base(input.File), input.Chat)
	if caption != "" {
		result += " with caption"
	}
	if input.ReplyTo != "" {
		result += fmt.Sprintf(" (reply to %s)", input.ReplyTo)
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: result},
		},
	}, nil, nil
}
