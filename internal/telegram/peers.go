package telegram

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/chestnykh/mcp-telegram/internal/acl"
	"github.com/gotd/td/telegram/peers"
)

func (c *Client) ResolvePeerForTool(ctx context.Context, ref string) (peers.Peer, acl.PeerIdentity, error) {
	pm := c.Peers()

	switch {
	case strings.HasPrefix(ref, "@"):
		domain := ref[1:]
		peer, err := pm.Resolve(ctx, domain)
		if err != nil {
			return nil, acl.PeerIdentity{}, fmt.Errorf("resolve @%s: %w", domain, err)
		}
		identity := PeerToIdentity(peer)
		return peer, identity, nil

	case strings.HasPrefix(ref, "+"):
		peer, err := pm.ResolvePhone(ctx, ref)
		if err != nil {
			return nil, acl.PeerIdentity{}, fmt.Errorf("resolve phone %s: %w", ref, err)
		}
		identity := PeerToIdentity(peer)
		identity.Phone = ref
		return peer, identity, nil

	case strings.HasPrefix(ref, "user:"):
		id, err := strconv.ParseInt(ref[5:], 10, 64)
		if err != nil {
			return nil, acl.PeerIdentity{}, fmt.Errorf("invalid user ID in %q: %w", ref, err)
		}
		peer, err := pm.ResolveUserID(ctx, id)
		if err != nil {
			return nil, acl.PeerIdentity{}, fmt.Errorf("resolve user:%d: %w", id, err)
		}
		identity := PeerToIdentity(peer)
		return peer, identity, nil

	case strings.HasPrefix(ref, "chat:"):
		id, err := strconv.ParseInt(ref[5:], 10, 64)
		if err != nil {
			return nil, acl.PeerIdentity{}, fmt.Errorf("invalid chat ID in %q: %w", ref, err)
		}
		peer, err := pm.ResolveChatID(ctx, id)
		if err != nil {
			return nil, acl.PeerIdentity{}, fmt.Errorf("resolve chat:%d: %w", id, err)
		}
		identity := PeerToIdentity(peer)
		return peer, identity, nil

	case strings.HasPrefix(ref, "channel:"):
		id, err := strconv.ParseInt(ref[8:], 10, 64)
		if err != nil {
			return nil, acl.PeerIdentity{}, fmt.Errorf("invalid channel ID in %q: %w", ref, err)
		}
		peer, err := pm.ResolveChannelID(ctx, id)
		if err != nil {
			return nil, acl.PeerIdentity{}, fmt.Errorf("resolve channel:%d: %w", id, err)
		}
		identity := PeerToIdentity(peer)
		return peer, identity, nil

	default:
		return nil, acl.PeerIdentity{}, fmt.Errorf("unknown peer reference format %q: use @username, +phone, user:ID, chat:ID, or channel:ID", ref)
	}
}

func PeerToIdentity(peer peers.Peer) acl.PeerIdentity {
	identity := acl.PeerIdentity{
		ID: peer.ID(),
	}

	switch p := peer.(type) {
	case peers.User:
		identity.Kind = acl.KindUser
		username, _ := p.Username()
		identity.Username = username
	case peers.Chat:
		identity.Kind = acl.KindChat
	case peers.Channel:
		identity.Kind = acl.KindChannel
		username, _ := p.Username()
		identity.Username = username
	}

	return identity
}

func FormatPeerRef(peer peers.Peer) string {
	switch p := peer.(type) {
	case peers.User:
		if username, ok := p.Username(); ok {
			return "@" + username
		}
		return fmt.Sprintf("user:%d", p.ID())
	case peers.Chat:
		return fmt.Sprintf("chat:%d", p.ID())
	case peers.Channel:
		if username, ok := p.Username(); ok {
			return "@" + username
		}
		return fmt.Sprintf("channel:%d", p.ID())
	default:
		return fmt.Sprintf("unknown:%d", peer.ID())
	}
}

func FormatPeerName(peer peers.Peer) string {
	switch p := peer.(type) {
	case peers.User:
		raw := p.Raw()
		name := raw.FirstName
		if raw.LastName != "" {
			name += " " + raw.LastName
		}
		return name
	case peers.Chat:
		return p.Raw().Title
	case peers.Channel:
		return p.Raw().Title
	default:
		return "Unknown"
	}
}

func PeerTypeName(peer peers.Peer) string {
	switch p := peer.(type) {
	case peers.User:
		if p.Raw().Bot {
			return "bot"
		}
		return "user"
	case peers.Chat:
		return "chat"
	case peers.Channel:
		if p.Raw().Broadcast {
			return "channel"
		}
		return "supergroup"
	default:
		return "unknown"
	}
}
