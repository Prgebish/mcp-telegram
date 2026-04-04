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

