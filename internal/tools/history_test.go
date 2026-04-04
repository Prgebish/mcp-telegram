package tools

import (
	"context"
	"testing"

	"github.com/chestnykh/mcp-telegram/internal/config"
	"github.com/gotd/td/telegram/downloader"
	"github.com/gotd/td/tg"
)

func TestFormatMedia(t *testing.T) {
	deps := &Deps{
		Media: config.MediaConfig{},
	}
	dl := downloader.NewDownloader()
	ctx := context.Background()
	peer := &tg.InputPeerUser{UserID: 100}

	// InputPeerUser — no deep links for private chats.
	tests := []struct {
		name string
		msg  *tg.Message
		want string
	}{
		{"nil media", &tg.Message{}, "[empty]"},
		{"photo", &tg.Message{ID: 1, Media: &tg.MessageMediaPhoto{}}, "[photo]"},
		{"document", &tg.Message{ID: 2, Media: &tg.MessageMediaDocument{}}, "[document]"},
		{"geo", &tg.Message{ID: 3, Media: &tg.MessageMediaGeo{}}, "[location]"},
		{"contact", &tg.Message{ID: 4, Media: &tg.MessageMediaContact{}}, "[contact]"},
		{"webpage", &tg.Message{ID: 5, Media: &tg.MessageMediaWebPage{}}, "[webpage]"},
		{"poll", &tg.Message{ID: 6, Media: &tg.MessageMediaPoll{}}, "[poll]"},
		{"venue", &tg.Message{ID: 7, Media: &tg.MessageMediaVenue{}}, "[venue]"},
		{"live location", &tg.Message{ID: 8, Media: &tg.MessageMediaGeoLive{}}, "[live location]"},
		{"dice", &tg.Message{ID: 9, Media: &tg.MessageMediaDice{}}, "[dice]"},
		{"unknown media", &tg.Message{ID: 10, Media: &tg.MessageMediaGame{}}, "[media]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatMedia(ctx, deps, dl, tt.msg, peer)
			if got != tt.want {
				t.Errorf("formatMedia() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTelegramLink(t *testing.T) {
	tests := []struct {
		name string
		peer tg.InputPeerClass
		want string
	}{
		{"user", &tg.InputPeerUser{UserID: 123}, ""},
		{"chat", &tg.InputPeerChat{ChatID: 456}, "tg://privatepost?channel=456&post=1"},
		{"channel", &tg.InputPeerChannel{ChannelID: 789}, "tg://privatepost?channel=789&post=1"},
		{"empty", &tg.InputPeerEmpty{}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := telegramLink(tt.peer, 1)
			if got != tt.want {
				t.Errorf("telegramLink() = %q, want %q", got, tt.want)
			}
		})
	}
}
