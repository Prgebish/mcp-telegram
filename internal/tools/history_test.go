package tools

import (
	"testing"

	"github.com/gotd/td/tg"
)

func TestFormatMediaType(t *testing.T) {
	tests := []struct {
		name string
		msg  *tg.Message
		want string
	}{
		{"nil media", &tg.Message{}, "[empty]"},
		{"photo", &tg.Message{Media: &tg.MessageMediaPhoto{}}, "[photo]"},
		{"document", &tg.Message{Media: &tg.MessageMediaDocument{}}, "[document]"},
		{"geo", &tg.Message{Media: &tg.MessageMediaGeo{}}, "[location]"},
		{"contact", &tg.Message{Media: &tg.MessageMediaContact{}}, "[contact]"},
		{"webpage", &tg.Message{Media: &tg.MessageMediaWebPage{}}, "[webpage]"},
		{"poll", &tg.Message{Media: &tg.MessageMediaPoll{}}, "[poll]"},
		{"venue", &tg.Message{Media: &tg.MessageMediaVenue{}}, "[venue]"},
		{"live location", &tg.Message{Media: &tg.MessageMediaGeoLive{}}, "[live location]"},
		{"dice", &tg.Message{Media: &tg.MessageMediaDice{}}, "[dice]"},
		{"unknown media", &tg.Message{Media: &tg.MessageMediaGame{}}, "[media]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatMediaType(tt.msg)
			if got != tt.want {
				t.Errorf("formatMediaType() = %q, want %q", got, tt.want)
			}
		})
	}
}
