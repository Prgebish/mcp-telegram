package tools

import (
	"testing"

	"github.com/Prgebish/mcp-telegram/internal/acl"
	"github.com/gotd/td/telegram/message/peer"
	"github.com/gotd/td/tg"
)

func TestIdentityFromElem_User(t *testing.T) {
	users := map[int64]*tg.User{
		100: {ID: 100, FirstName: "Alice", LastName: "Smith", Username: "alice", Phone: "79001234567"},
	}
	entities := peer.NewEntities(users, map[int64]*tg.Chat{}, map[int64]*tg.Channel{})
	inputPeer := &tg.InputPeerUser{UserID: 100, AccessHash: 999}

	identity, ref, name, peerType := identityFromElem(inputPeer, entities)

	if identity.Kind != acl.KindUser {
		t.Errorf("Kind = %d, want KindUser", identity.Kind)
	}
	if identity.ID != 100 {
		t.Errorf("ID = %d, want 100", identity.ID)
	}
	if identity.Username != "alice" {
		t.Errorf("Username = %q, want %q", identity.Username, "alice")
	}
	if identity.Phone != "+79001234567" {
		t.Errorf("Phone = %q, want %q", identity.Phone, "+79001234567")
	}
	if ref != "@alice" {
		t.Errorf("ref = %q, want %q", ref, "@alice")
	}
	if name != "Alice Smith" {
		t.Errorf("name = %q, want %q", name, "Alice Smith")
	}
	if peerType != "user" {
		t.Errorf("peerType = %q, want %q", peerType, "user")
	}
}

func TestIdentityFromElem_UserNoUsername(t *testing.T) {
	users := map[int64]*tg.User{
		200: {ID: 200, FirstName: "Bob"},
	}
	entities := peer.NewEntities(users, map[int64]*tg.Chat{}, map[int64]*tg.Channel{})
	inputPeer := &tg.InputPeerUser{UserID: 200}

	identity, ref, name, _ := identityFromElem(inputPeer, entities)

	if identity.Username != "" {
		t.Errorf("Username = %q, want empty", identity.Username)
	}
	if ref != "user:200" {
		t.Errorf("ref = %q, want %q", ref, "user:200")
	}
	if name != "Bob" {
		t.Errorf("name = %q, want %q", name, "Bob")
	}
}

func TestIdentityFromElem_Bot(t *testing.T) {
	users := map[int64]*tg.User{
		300: {ID: 300, FirstName: "BotHelper", Bot: true, Username: "helper_bot"},
	}
	entities := peer.NewEntities(users, map[int64]*tg.Chat{}, map[int64]*tg.Channel{})
	inputPeer := &tg.InputPeerUser{UserID: 300}

	_, _, _, peerType := identityFromElem(inputPeer, entities)
	if peerType != "bot" {
		t.Errorf("peerType = %q, want %q", peerType, "bot")
	}
}

func TestIdentityFromElem_Chat(t *testing.T) {
	chats := map[int64]*tg.Chat{
		400: {ID: 400, Title: "Dev Team"},
	}
	entities := peer.NewEntities(map[int64]*tg.User{}, chats, map[int64]*tg.Channel{})
	inputPeer := &tg.InputPeerChat{ChatID: 400}

	identity, ref, name, peerType := identityFromElem(inputPeer, entities)

	if identity.Kind != acl.KindChat {
		t.Errorf("Kind = %d, want KindChat", identity.Kind)
	}
	if identity.ID != 400 {
		t.Errorf("ID = %d, want 400", identity.ID)
	}
	if ref != "chat:400" {
		t.Errorf("ref = %q, want %q", ref, "chat:400")
	}
	if name != "Dev Team" {
		t.Errorf("name = %q, want %q", name, "Dev Team")
	}
	if peerType != "chat" {
		t.Errorf("peerType = %q, want %q", peerType, "chat")
	}
}

func TestIdentityFromElem_Channel(t *testing.T) {
	channels := map[int64]*tg.Channel{
		500: {ID: 500, Title: "News", Username: "news_channel", Broadcast: true},
	}
	entities := peer.NewEntities(map[int64]*tg.User{}, map[int64]*tg.Chat{}, channels)
	inputPeer := &tg.InputPeerChannel{ChannelID: 500}

	identity, ref, name, peerType := identityFromElem(inputPeer, entities)

	if identity.Kind != acl.KindChannel {
		t.Errorf("Kind = %d, want KindChannel", identity.Kind)
	}
	if identity.Username != "news_channel" {
		t.Errorf("Username = %q, want %q", identity.Username, "news_channel")
	}
	if ref != "@news_channel" {
		t.Errorf("ref = %q, want %q", ref, "@news_channel")
	}
	if name != "News" {
		t.Errorf("name = %q, want %q", name, "News")
	}
	if peerType != "channel" {
		t.Errorf("peerType = %q, want %q", peerType, "channel")
	}
}

func TestIdentityFromElem_Supergroup(t *testing.T) {
	channels := map[int64]*tg.Channel{
		600: {ID: 600, Title: "Devs", Broadcast: false},
	}
	entities := peer.NewEntities(map[int64]*tg.User{}, map[int64]*tg.Chat{}, channels)
	inputPeer := &tg.InputPeerChannel{ChannelID: 600}

	_, ref, _, peerType := identityFromElem(inputPeer, entities)

	if ref != "channel:600" {
		t.Errorf("ref = %q, want %q", ref, "channel:600")
	}
	if peerType != "supergroup" {
		t.Errorf("peerType = %q, want %q", peerType, "supergroup")
	}
}

func TestIdentityFromElem_UserNotInEntities(t *testing.T) {
	entities := peer.NewEntities(map[int64]*tg.User{}, map[int64]*tg.Chat{}, map[int64]*tg.Channel{})
	inputPeer := &tg.InputPeerUser{UserID: 999}

	identity, ref, name, peerType := identityFromElem(inputPeer, entities)

	if identity.Kind != acl.KindUser {
		t.Errorf("Kind = %d, want KindUser", identity.Kind)
	}
	if identity.ID != 999 {
		t.Errorf("ID = %d, want 999", identity.ID)
	}
	if ref != "user:999" {
		t.Errorf("ref = %q, want %q (fallback)", ref, "user:999")
	}
	if name != "User 999" {
		t.Errorf("name = %q, want %q (fallback)", name, "User 999")
	}
	if peerType != "user" {
		t.Errorf("peerType = %q, want %q", peerType, "user")
	}
}

func TestIdentityFromElem_Unknown(t *testing.T) {
	entities := peer.NewEntities(map[int64]*tg.User{}, map[int64]*tg.Chat{}, map[int64]*tg.Channel{})

	identity, ref, name, peerType := identityFromElem(&tg.InputPeerEmpty{}, entities)

	if identity.ID != 0 {
		t.Errorf("ID = %d, want 0", identity.ID)
	}
	if ref != "unknown" {
		t.Errorf("ref = %q, want %q", ref, "unknown")
	}
	if name != "Unknown" {
		t.Errorf("name = %q, want %q", name, "Unknown")
	}
	if peerType != "unknown" {
		t.Errorf("peerType = %q, want %q", peerType, "unknown")
	}
}
