package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/chestnykh/mcp-telegram/internal/config"
	tgclient "github.com/chestnykh/mcp-telegram/internal/telegram"
	"github.com/gotd/td/telegram/query/messages"
	"github.com/gotd/td/tg"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type historyInput struct {
	Chat     string `json:"chat" jsonschema:"required,Chat reference: @username, user:ID, chat:ID, or channel:ID"`
	Limit    int    `json:"limit,omitempty" jsonschema:"Max messages to return (default from config)"`
	OffsetID int    `json:"offset_id,omitempty" jsonschema:"Message ID to start from for pagination"`
}

func registerHistory(server *mcp.Server, deps *Deps) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "tg_history",
		Description: "Get message history from a whitelisted chat. Use offset_id for pagination.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    true,
			DestructiveHint: ptrBool(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input historyInput) (*mcp.CallToolResult, any, error) {
		if err := deps.Limiter.Wait(ctx); err != nil {
			return nil, nil, err
		}

		peer, identity, err := deps.Client.ResolvePeerForTool(ctx, input.Chat)
		if err != nil {
			return toolError(fmt.Sprintf("cannot resolve chat: %v", err)), nil, nil
		}

		if !deps.ACL.Allowed(identity, config.PermRead) {
			return toolError(fmt.Sprintf("access denied: %s does not have 'read' permission", input.Chat)), nil, nil
		}

		limit := input.Limit
		if limit <= 0 || limit > deps.Limits.MaxMessagesPerRequest {
			limit = deps.Limits.MaxMessagesPerRequest
		}

		qb := messages.NewQueryBuilder(deps.Client.API()).
			GetHistory(peer.InputPeer()).
			BatchSize(limit)

		if input.OffsetID > 0 {
			qb = qb.OffsetID(input.OffsetID)
		}

		iter := qb.Iter()
		var lines []string
		count := 0
		lastID := 0

		for iter.Next(ctx) {
			if count >= limit {
				break
			}

			elem := iter.Value()
			msg, ok := elem.Msg.(*tg.Message)
			if !ok {
				continue
			}

			from := resolveFromName(ctx, deps, msg)
			ts := time.Unix(int64(msg.Date), 0).UTC().Format("2006-01-02 15:04")
			text := msg.Message
			if text == "" {
				text = formatMediaType(msg)
			}

			lines = append(lines, fmt.Sprintf("[%d] %s (%s): %s", msg.ID, from, ts, text))
			lastID = msg.ID
			count++
		}
		if err := iter.Err(); err != nil {
			return toolError(fmt.Sprintf("failed to fetch history: %v", err)), nil, nil
		}

		if len(lines) == 0 {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: "No messages found."},
				},
			}, nil, nil
		}

		result := strings.Join(lines, "\n")
		if count == limit && lastID > 0 {
			result += fmt.Sprintf("\n\n[Use offset_id=%d to load older messages]", lastID)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: result},
			},
		}, nil, nil
	})
}

func resolveFromName(ctx context.Context, deps *Deps, msg *tg.Message) string {
	if msg.FromID == nil {
		return "unknown"
	}

	pm := deps.Client.Peers()
	peer, err := pm.ResolvePeer(ctx, msg.FromID)
	if err != nil {
		return "unknown"
	}
	return tgclient.FormatPeerName(peer)
}

func formatMediaType(msg *tg.Message) string {
	if msg.Media == nil {
		return "[empty]"
	}
	switch msg.Media.(type) {
	case *tg.MessageMediaPhoto:
		return "[photo]"
	case *tg.MessageMediaDocument:
		return "[document]"
	case *tg.MessageMediaGeo:
		return "[location]"
	case *tg.MessageMediaContact:
		return "[contact]"
	case *tg.MessageMediaWebPage:
		return "[webpage]"
	case *tg.MessageMediaPoll:
		return "[poll]"
	case *tg.MessageMediaVenue:
		return "[venue]"
	case *tg.MessageMediaGeoLive:
		return "[live location]"
	case *tg.MessageMediaDice:
		return "[dice]"
	default:
		return "[media]"
	}
}
