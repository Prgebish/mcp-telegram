package telegram

import (
	"context"
	"fmt"

	"github.com/Prgebish/mcp-telegram/internal/acl"
	"github.com/gotd/td/telegram/message/peer"
	"github.com/gotd/td/telegram/query/dialogs"
	"github.com/gotd/td/tg"
)

// DialogPeer builds the PeerIdentity, display name and type of a dialog
// directly from the dialog iterator's data — no peers.Manager lookup needed,
// which may fail on a cold cache. ok is false for a peer of unknown type.
func DialogPeer(inputPeer tg.InputPeerClass, entities peer.Entities) (identity acl.PeerIdentity, name, peerType string, ok bool) {
	switch p := inputPeer.(type) {
	case *tg.InputPeerUser:
		identity := acl.PeerIdentity{Kind: acl.KindUser, ID: p.UserID}
		name := fmt.Sprintf("User %d", p.UserID)
		peerType := "user"
		if u, ok := entities.User(p.UserID); ok {
			identity.Username = u.Username
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
		return identity, name, peerType, true

	case *tg.InputPeerChat:
		identity := acl.PeerIdentity{Kind: acl.KindChat, ID: p.ChatID}
		name := fmt.Sprintf("Chat %d", p.ChatID)
		if c, ok := entities.Chat(p.ChatID); ok {
			name = c.Title
		}
		return identity, name, "chat", true

	case *tg.InputPeerChannel:
		identity := acl.PeerIdentity{Kind: acl.KindChannel, ID: p.ChannelID}
		name := fmt.Sprintf("Channel %d", p.ChannelID)
		peerType := "supergroup"
		if ch, ok := entities.Channel(p.ChannelID); ok {
			identity.Username = ch.Username
			name = ch.Title
			if ch.Broadcast {
				peerType = "channel"
			}
		}
		return identity, name, peerType, true

	default:
		return acl.PeerIdentity{}, "", "", false
	}
}

// ListAllDialogs walks every dialog of the account, ignoring the ACL, and
// calls fn with a typed reference (user:ID, chat:ID or channel:ID), the peer
// type and the title. It backs the `dialogs` CLI command used to pick chats
// for the ACL; it lives here rather than in internal/tools, where everything
// goes through the ACL, and is not exposed as an MCP tool.
func ListAllDialogs(ctx context.Context, api *tg.Client, fn func(ref, peerType, name string)) error {
	iter := dialogs.NewQueryBuilder(api).GetDialogs().BatchSize(100).Iter()
	for iter.Next(ctx) {
		elem := iter.Value()
		identity, name, peerType, ok := DialogPeer(elem.Peer, elem.Entities)
		if !ok || identity.TypedRef() == "" {
			continue
		}
		fn(identity.TypedRef(), peerType, name)
	}
	return iter.Err()
}
