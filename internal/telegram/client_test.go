package telegram

import (
	"testing"

	"github.com/gotd/td/tg"
)

func TestExtractEntities_Updates(t *testing.T) {
	users := []tg.UserClass{
		&tg.User{ID: 1, FirstName: "Alice"},
	}
	chats := []tg.ChatClass{
		&tg.Chat{ID: 2, Title: "Group"},
	}

	u := &tg.Updates{Users: users, Chats: chats}
	gotUsers, gotChats := extractEntities(u)

	if len(gotUsers) != 1 {
		t.Errorf("users len = %d, want 1", len(gotUsers))
	}
	if len(gotChats) != 1 {
		t.Errorf("chats len = %d, want 1", len(gotChats))
	}
}

func TestExtractEntities_UpdatesCombined(t *testing.T) {
	users := []tg.UserClass{
		&tg.User{ID: 1},
		&tg.User{ID: 2},
	}
	u := &tg.UpdatesCombined{Users: users}
	gotUsers, _ := extractEntities(u)

	if len(gotUsers) != 2 {
		t.Errorf("users len = %d, want 2", len(gotUsers))
	}
}

func TestExtractEntities_Other(t *testing.T) {
	u := &tg.UpdateShort{}
	users, chats := extractEntities(u)
	if users != nil || chats != nil {
		t.Error("expected nil for non-container update type")
	}
}

func TestPeerToInputPeer_User(t *testing.T) {
	users := []tg.UserClass{
		&tg.User{ID: 100, AccessHash: 999},
	}
	peer := &tg.PeerUser{UserID: 100}

	result := peerToInputPeer(peer, users, nil)
	inp, ok := result.(*tg.InputPeerUser)
	if !ok {
		t.Fatalf("expected *tg.InputPeerUser, got %T", result)
	}
	if inp.UserID != 100 {
		t.Errorf("UserID = %d, want 100", inp.UserID)
	}
	if inp.AccessHash != 999 {
		t.Errorf("AccessHash = %d, want 999", inp.AccessHash)
	}
}

func TestPeerToInputPeer_UserNotFound(t *testing.T) {
	peer := &tg.PeerUser{UserID: 100}
	result := peerToInputPeer(peer, nil, nil)
	inp, ok := result.(*tg.InputPeerUser)
	if !ok {
		t.Fatalf("expected *tg.InputPeerUser, got %T", result)
	}
	if inp.UserID != 100 {
		t.Errorf("UserID = %d, want 100", inp.UserID)
	}
	if inp.AccessHash != 0 {
		t.Errorf("AccessHash = %d, want 0 (not found)", inp.AccessHash)
	}
}

func TestPeerToInputPeer_Chat(t *testing.T) {
	peer := &tg.PeerChat{ChatID: 200}
	result := peerToInputPeer(peer, nil, nil)
	inp, ok := result.(*tg.InputPeerChat)
	if !ok {
		t.Fatalf("expected *tg.InputPeerChat, got %T", result)
	}
	if inp.ChatID != 200 {
		t.Errorf("ChatID = %d, want 200", inp.ChatID)
	}
}

func TestPeerToInputPeer_Channel(t *testing.T) {
	chats := []tg.ChatClass{
		&tg.Channel{ID: 300, AccessHash: 888},
	}
	peer := &tg.PeerChannel{ChannelID: 300}
	result := peerToInputPeer(peer, nil, chats)
	inp, ok := result.(*tg.InputPeerChannel)
	if !ok {
		t.Fatalf("expected *tg.InputPeerChannel, got %T", result)
	}
	if inp.ChannelID != 300 {
		t.Errorf("ChannelID = %d, want 300", inp.ChannelID)
	}
	if inp.AccessHash != 888 {
		t.Errorf("AccessHash = %d, want 888", inp.AccessHash)
	}
}

func TestPeerToInputPeer_Empty(t *testing.T) {
	result := peerToInputPeer(nil, nil, nil)
	if _, ok := result.(*tg.InputPeerEmpty); !ok {
		t.Errorf("expected *tg.InputPeerEmpty for nil peer, got %T", result)
	}
}
