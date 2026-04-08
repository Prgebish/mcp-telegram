package tools

import (
	"context"
	"fmt"
	"testing"

	"github.com/Prgebish/mcp-telegram/internal/acl"
	"github.com/Prgebish/mcp-telegram/internal/config"
	"github.com/gotd/td/telegram/downloader"
	"github.com/gotd/td/telegram/message/peer"
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

func TestPeerIDPrefix(t *testing.T) {
	tests := []struct {
		name string
		peer tg.InputPeerClass
		want string
	}{
		{"user", &tg.InputPeerUser{UserID: 123}, "u123"},
		{"chat", &tg.InputPeerChat{ChatID: 456}, "c456"},
		{"channel", &tg.InputPeerChannel{ChannelID: 789}, "ch789"},
		{"empty", &tg.InputPeerEmpty{}, "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := peerIDPrefix(tt.peer)
			if got != tt.want {
				t.Errorf("peerIDPrefix() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMediaFilenameUniqueness(t *testing.T) {
	// Same message ID in different chats must produce different filenames.
	msgID := 100
	user := peerIDPrefix(&tg.InputPeerUser{UserID: 1})
	channel := peerIDPrefix(&tg.InputPeerChannel{ChannelID: 2})

	userFile := fmt.Sprintf("%s_%d.jpg", user, msgID)
	channelFile := fmt.Sprintf("%s_%d.jpg", channel, msgID)

	if userFile == channelFile {
		t.Errorf("filenames should differ: user=%q, channel=%q", userFile, channelFile)
	}
	if userFile != "u1_100.jpg" {
		t.Errorf("user filename = %q, want %q", userFile, "u1_100.jpg")
	}
	if channelFile != "ch2_100.jpg" {
		t.Errorf("channel filename = %q, want %q", channelFile, "ch2_100.jpg")
	}
}

func TestTelegramLink(t *testing.T) {
	tests := []struct {
		name string
		peer tg.InputPeerClass
		want string
	}{
		{"user", &tg.InputPeerUser{UserID: 123}, ""},
		{"chat", &tg.InputPeerChat{ChatID: 456}, ""},
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

func TestFormatMedia_ChannelDeepLinks(t *testing.T) {
	deps := &Deps{Media: config.MediaConfig{}}
	dl := downloader.NewDownloader()
	ctx := context.Background()
	channelPeer := &tg.InputPeerChannel{ChannelID: 42}

	// Channel peer should include deep links in media labels.
	msg := &tg.Message{ID: 5, Media: &tg.MessageMediaPhoto{}}
	got := formatMedia(ctx, deps, dl, msg, channelPeer)
	want := "[photo] tg://privatepost?channel=42&post=5"
	if got != want {
		t.Errorf("formatMedia() = %q, want %q", got, want)
	}
}

func TestClassifyDocument(t *testing.T) {
	tests := []struct {
		name  string
		attrs []tg.DocumentAttributeClass
		want  string
	}{
		{"video", []tg.DocumentAttributeClass{&tg.DocumentAttributeVideo{}}, "video"},
		{"audio", []tg.DocumentAttributeClass{&tg.DocumentAttributeAudio{Voice: false}}, "audio"},
		{"voice", []tg.DocumentAttributeClass{&tg.DocumentAttributeAudio{Voice: true}}, "voice"},
		{"document with filename", []tg.DocumentAttributeClass{&tg.DocumentAttributeFilename{FileName: "test.pdf"}}, "document"},
		{"empty attrs", nil, "document"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := &tg.Document{Attributes: tt.attrs}
			got := classifyDocument(doc)
			if got != tt.want {
				t.Errorf("classifyDocument() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseDate(t *testing.T) {
	tests := []struct {
		input   string
		wantErr bool
		wantStr string // expected Format("2006-01-02 15:04:05")
	}{
		{"2024-01-15", false, "2024-01-15 00:00:00"},
		{"2024-01-15 14:30", false, "2024-01-15 14:30:00"},
		{"2024-01-15 14:30:45", false, "2024-01-15 14:30:45"},
		{"not-a-date", true, ""},
		{"2024/01/15", true, ""},
		{"", true, ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseDate(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if s := got.Format("2006-01-02 15:04:05"); s != tt.wantStr {
				t.Errorf("parseDate(%q) = %q, want %q", tt.input, s, tt.wantStr)
			}
		})
	}
}

func TestExpandTilde(t *testing.T) {
	tests := []struct {
		input string
		home  bool // whether result should start with home dir
	}{
		{"~/Documents", true},
		{"/absolute/path", false},
		{"relative/path", false},
		{"~notuser/path", false}, // only ~/ is expanded
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := expandTilde(tt.input)
			if tt.home {
				if got == tt.input {
					t.Error("expected tilde to be expanded")
				}
				if got[0] != '/' {
					t.Errorf("expanded path should be absolute, got %q", got)
				}
			} else {
				if got != tt.input {
					t.Errorf("expandTilde(%q) = %q, want unchanged", tt.input, got)
				}
			}
		})
	}
}

func TestResolveFromName(t *testing.T) {
	users := map[int64]*tg.User{
		100: {ID: 100, FirstName: "Alice", LastName: "Smith"},
	}
	channels := map[int64]*tg.Channel{
		200: {ID: 200, Title: "News Channel"},
	}
	chats := map[int64]*tg.Chat{
		300: {ID: 300, Title: "Dev Group"},
	}
	entities := peer.NewEntities(users, chats, channels)

	tests := []struct {
		name       string
		msg        *tg.Message
		dialogPeer tg.InputPeerClass
		want       string
	}{
		{
			"user from entities",
			&tg.Message{FromID: &tg.PeerUser{UserID: 100}},
			&tg.InputPeerUser{UserID: 100},
			"Alice Smith",
		},
		{
			"channel from entities",
			&tg.Message{FromID: &tg.PeerChannel{ChannelID: 200}},
			&tg.InputPeerChannel{ChannelID: 200},
			"News Channel",
		},
		{
			"chat from entities",
			&tg.Message{FromID: &tg.PeerChat{ChatID: 300}},
			&tg.InputPeerChat{ChatID: 300},
			"Dev Group",
		},
		{
			"nil FromID falls back to user dialog peer",
			&tg.Message{},
			&tg.InputPeerUser{UserID: 100},
			"Alice Smith",
		},
		{
			"nil FromID falls back to channel dialog peer",
			&tg.Message{},
			&tg.InputPeerChannel{ChannelID: 200},
			"News Channel",
		},
		{
			"unknown user falls back to user:ID",
			&tg.Message{FromID: &tg.PeerUser{UserID: 999}},
			&tg.InputPeerUser{UserID: 999},
			"user:999",
		},
		{
			"unknown channel falls back to channel:ID",
			&tg.Message{FromID: &tg.PeerChannel{ChannelID: 999}},
			&tg.InputPeerChannel{ChannelID: 999},
			"channel:999",
		},
		{
			"user first name only",
			&tg.Message{FromID: &tg.PeerUser{UserID: 100}},
			&tg.InputPeerUser{UserID: 100},
			"Alice Smith",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveFromName(tt.msg, tt.dialogPeer, entities)
			if got != tt.want {
				t.Errorf("resolveFromName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveFromName_FirstNameOnly(t *testing.T) {
	users := map[int64]*tg.User{
		1: {ID: 1, FirstName: "Bob"},
	}
	entities := peer.NewEntities(users, map[int64]*tg.Chat{}, map[int64]*tg.Channel{})

	got := resolveFromName(
		&tg.Message{FromID: &tg.PeerUser{UserID: 1}},
		&tg.InputPeerUser{UserID: 1},
		entities,
	)
	if got != "Bob" {
		t.Errorf("resolveFromName() = %q, want %q", got, "Bob")
	}
}

func TestFormatMedia_DocumentTypes(t *testing.T) {
	deps := &Deps{Media: config.MediaConfig{}}
	dl := downloader.NewDownloader()
	ctx := context.Background()
	userPeer := &tg.InputPeerUser{UserID: 1}

	tests := []struct {
		name  string
		media tg.MessageMediaClass
		want  string
	}{
		{
			"video document",
			&tg.MessageMediaDocument{
				Document: &tg.Document{
					ID:         1,
					AccessHash: 1,
					Attributes: []tg.DocumentAttributeClass{&tg.DocumentAttributeVideo{}},
				},
			},
			"[video]",
		},
		{
			"voice document",
			&tg.MessageMediaDocument{
				Document: &tg.Document{
					ID:         2,
					AccessHash: 2,
					Attributes: []tg.DocumentAttributeClass{&tg.DocumentAttributeAudio{Voice: true}},
				},
			},
			"[voice]",
		},
		{
			"audio document",
			&tg.MessageMediaDocument{
				Document: &tg.Document{
					ID:         3,
					AccessHash: 3,
					Attributes: []tg.DocumentAttributeClass{&tg.DocumentAttributeAudio{Voice: false}},
				},
			},
			"[audio]",
		},
		{
			"nil document in media",
			&tg.MessageMediaDocument{Document: nil},
			"[document]",
		},
		{
			"empty document",
			&tg.MessageMediaDocument{Document: &tg.DocumentEmpty{}},
			"[document]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := &tg.Message{ID: 1, Media: tt.media}
			got := formatMedia(ctx, deps, dl, msg, userPeer)
			if got != tt.want {
				t.Errorf("formatMedia() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatMedia_ShouldDownload(t *testing.T) {
	// With download config but no directory, ShouldDownload returns true
	// but download will fail, so it falls back to label.
	deps := &Deps{
		Media: config.MediaConfig{
			Download:  []string{"photo"},
			Directory: "/nonexistent/path",
		},
	}
	dl := downloader.NewDownloader()
	ctx := context.Background()
	userPeer := &tg.InputPeerUser{UserID: 1}

	// Photo with empty photo data — download attempt fails, falls back to [photo].
	msg := &tg.Message{ID: 1, Media: &tg.MessageMediaPhoto{Photo: &tg.PhotoEmpty{}}}
	got := formatMedia(ctx, deps, dl, msg, userPeer)
	if got != "[photo]" {
		t.Errorf("formatMedia() = %q, want %q (fallback after failed download)", got, "[photo]")
	}
}

func TestMediaConfig_ShouldDownload(t *testing.T) {
	cfg := config.MediaConfig{Download: []string{"photo", "video"}}
	if !cfg.ShouldDownload("photo") {
		t.Error("expected photo to be downloadable")
	}
	if !cfg.ShouldDownload("video") {
		t.Error("expected video to be downloadable")
	}
	if cfg.ShouldDownload("document") {
		t.Error("expected document NOT to be downloadable")
	}
	if cfg.ShouldDownload("") {
		t.Error("expected empty string NOT to be downloadable")
	}
}

func TestIdentityFromElem_ChatNotInEntities(t *testing.T) {
	entities := peer.NewEntities(map[int64]*tg.User{}, map[int64]*tg.Chat{}, map[int64]*tg.Channel{})
	inputPeer := &tg.InputPeerChat{ChatID: 999}

	identity, ref, name, peerType := identityFromElem(inputPeer, entities)

	if identity.Kind != acl.KindChat {
		t.Errorf("Kind = %d, want KindChat", identity.Kind)
	}
	if ref != "chat:999" {
		t.Errorf("ref = %q, want %q", ref, "chat:999")
	}
	if name != "Chat 999" {
		t.Errorf("name = %q, want %q", name, "Chat 999")
	}
	if peerType != "chat" {
		t.Errorf("peerType = %q, want %q", peerType, "chat")
	}
}

func TestIdentityFromElem_ChannelNotInEntities(t *testing.T) {
	entities := peer.NewEntities(map[int64]*tg.User{}, map[int64]*tg.Chat{}, map[int64]*tg.Channel{})
	inputPeer := &tg.InputPeerChannel{ChannelID: 999}

	identity, ref, name, peerType := identityFromElem(inputPeer, entities)

	if identity.Kind != acl.KindChannel {
		t.Errorf("Kind = %d, want KindChannel", identity.Kind)
	}
	if ref != "channel:999" {
		t.Errorf("ref = %q, want %q", ref, "channel:999")
	}
	if name != "Channel 999" {
		t.Errorf("name = %q, want %q", name, "Channel 999")
	}
	if peerType != "supergroup" {
		t.Errorf("peerType = %q, want %q", peerType, "supergroup")
	}
}
