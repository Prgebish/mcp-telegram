package tools

import (
	"context"
	"fmt"
	"strings"

	tgclient "github.com/chestnykh/mcp-telegram/internal/telegram"
	"github.com/gotd/td/telegram/query/dialogs"
	"github.com/gotd/td/tg"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type dialogsInput struct {
	OnlyUnread bool `json:"only_unread,omitempty" jsonschema:"Filter to only dialogs with unread messages"`
}

func registerDialogs(server *mcp.Server, deps *Deps) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "tg_dialogs",
		Description: "List Telegram dialogs (only whitelisted chats). Returns ref, type, title, and unread count.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    true,
			DestructiveHint: ptrBool(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input dialogsInput) (*mcp.CallToolResult, any, error) {
		if err := deps.Limiter.Wait(ctx); err != nil {
			return nil, nil, err
		}

		api := deps.Client.API()
		pm := deps.Client.Peers()
		iter := dialogs.NewQueryBuilder(api).GetDialogs().BatchSize(100).Iter()

		var lines []string
		count := 0

		for iter.Next(ctx) {
			if count >= deps.Limits.MaxDialogsPerRequest {
				break
			}

			elem := iter.Value()
			dlg, ok := elem.Dialog.(*tg.Dialog)
			if !ok {
				continue
			}

			if input.OnlyUnread && dlg.UnreadCount == 0 {
				continue
			}

			peer, err := pm.ResolvePeer(ctx, dlg.Peer)
			if err != nil {
				continue
			}

			identity := tgclient.PeerToIdentity(peer)
			if !deps.ACL.MatchesAny(identity) {
				continue
			}

			ref := tgclient.FormatPeerRef(peer)
			name := tgclient.FormatPeerName(peer)
			peerType := tgclient.PeerTypeName(peer)

			line := fmt.Sprintf("- %s (%s) [%s]", name, ref, peerType)
			if dlg.UnreadCount > 0 {
				line += fmt.Sprintf(" unread:%d", dlg.UnreadCount)
			}
			lines = append(lines, line)
			count++
		}
		if err := iter.Err(); err != nil {
			return toolError(fmt.Sprintf("failed to fetch dialogs: %v", err)), nil, nil
		}

		if len(lines) == 0 {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: "No matching dialogs found."},
				},
			}, nil, nil
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: strings.Join(lines, "\n")},
			},
		}, nil, nil
	})
}
