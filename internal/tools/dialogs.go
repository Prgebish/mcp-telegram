package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/Prgebish/mcp-telegram/internal/acl"
	"github.com/gotd/td/telegram/message/peer"
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
		iter := dialogs.NewQueryBuilder(deps.API).GetDialogs().BatchSize(100).Iter()

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

			// Extract identity directly from iterator's Peer and Entities
			// instead of re-resolving through peers.Manager (which may fail
			// on cold cache).
			identity, ref, name, peerType := identityFromElem(elem.Peer, elem.Entities)

			if !deps.ACL.MatchesAny(identity) {
				continue
			}

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

// identityFromElem builds PeerIdentity, ref string, display name, and type
// directly from the dialog iterator's data — no peers.Manager lookup needed.
func identityFromElem(inputPeer tg.InputPeerClass, entities peer.Entities) (acl.PeerIdentity, string, string, string) {
	switch p := inputPeer.(type) {
	case *tg.InputPeerUser:
		identity := acl.PeerIdentity{Kind: acl.KindUser, ID: p.UserID}
		ref := fmt.Sprintf("user:%d", p.UserID)
		name := fmt.Sprintf("User %d", p.UserID)
		peerType := "user"
		if u, ok := entities.User(p.UserID); ok {
			if u.Username != "" {
				identity.Username = u.Username
				ref = "@" + u.Username
			}
			if u.Phone != "" {
				identity.Phone = "+" + u.Phone
			}
			name = u.FirstName
			if u.LastName != "" {
				name += " " + u.LastName
			}
			if u.Bot {
				peerType = "bot"
			}
		}
		return identity, ref, name, peerType

	case *tg.InputPeerChat:
		identity := acl.PeerIdentity{Kind: acl.KindChat, ID: p.ChatID}
		ref := fmt.Sprintf("chat:%d", p.ChatID)
		name := fmt.Sprintf("Chat %d", p.ChatID)
		if c, ok := entities.Chat(p.ChatID); ok {
			name = c.Title
		}
		return identity, ref, name, "chat"

	case *tg.InputPeerChannel:
		identity := acl.PeerIdentity{Kind: acl.KindChannel, ID: p.ChannelID}
		ref := fmt.Sprintf("channel:%d", p.ChannelID)
		name := fmt.Sprintf("Channel %d", p.ChannelID)
		peerType := "supergroup"
		if ch, ok := entities.Channel(p.ChannelID); ok {
			if ch.Username != "" {
				identity.Username = ch.Username
				ref = "@" + ch.Username
			}
			name = ch.Title
			if ch.Broadcast {
				peerType = "channel"
			}
		}
		return identity, ref, name, peerType

	default:
		return acl.PeerIdentity{}, "unknown", "Unknown", "unknown"
	}
}
