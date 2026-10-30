package telegram

import (
	"context"
	"fmt"

	"github.com/Prgebish/mcp-telegram/internal/acl"
	"github.com/gotd/td/telegram/peers"
)

// ResolvePeerForTool resolves a chat reference parsed by acl.ParseRef, the
// same grammar the ACL uses, so a reference is read the same way here and in
// the ACL pre-check.
func (c *Client) ResolvePeerForTool(ctx context.Context, ref string) (peers.Peer, acl.PeerIdentity, error) {
	r, err := acl.ParseRef(ref)
	if err != nil {
		return nil, acl.PeerIdentity{}, err
	}
	pm := c.Peers()

	var peer peers.Peer
	switch {
	case r.Form == acl.RefUsername:
		peer, err = pm.ResolveDomain(ctx, r.Username)
	case r.Form == acl.RefPhone:
		var u peers.User
		u, err = pm.ResolvePhone(ctx, r.Phone)
		peer = u
	case r.Kind == acl.KindUser:
		var u peers.User
		u, err = pm.ResolveUserID(ctx, r.ID)
		peer = u
	case r.Kind == acl.KindChat:
		var ch peers.Chat
		ch, err = pm.ResolveChatID(ctx, r.ID)
		peer = ch
	default:
		var ch peers.Channel
		ch, err = pm.ResolveChannelID(ctx, r.ID)
		peer = ch
	}
	if err != nil {
		return nil, acl.PeerIdentity{}, fmt.Errorf("resolve %s: %w", r, err)
	}

	identity := PeerToIdentity(peer)
	// Keep the phone Telegram returned; if Raw() has none (hidden by privacy
	// settings), the user is still the one the phone resolved to.
	if r.Form == acl.RefPhone && identity.Phone == "" {
		identity.Phone = r.Phone
	}
	return peer, identity, nil
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
		if phone := p.Raw().Phone; phone != "" {
			identity.Phone = "+" + phone
		}
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
	switch peer.(type) {
	case peers.User, peers.Chat, peers.Channel:
		return PeerToIdentity(peer).Ref()
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
