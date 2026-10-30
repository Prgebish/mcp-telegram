package acl

import (
	"testing"

	"github.com/Prgebish/mcp-telegram/internal/config"
)

func TestChecker_UsernameMatch(t *testing.T) {
	checker := mustNewChecker(t, []config.ChatRule{
		{Match: "@alice", Permissions: []config.Permission{config.PermRead, config.PermDraft}},
	})

	peer := PeerIdentity{Kind: KindUser, ID: 1, Username: "alice"}

	if !checker.Allowed(peer, config.PermRead) {
		t.Error("expected read to be allowed for @alice")
	}
	if !checker.Allowed(peer, config.PermDraft) {
		t.Error("expected draft to be allowed for @alice")
	}
	if checker.Allowed(peer, config.PermMarkRead) {
		t.Error("expected mark_read to be denied for @alice")
	}
}

func TestChecker_UsernameCaseInsensitive(t *testing.T) {
	checker := mustNewChecker(t, []config.ChatRule{
		{Match: "@Alice", Permissions: []config.Permission{config.PermRead}},
	})

	peer := PeerIdentity{Kind: KindUser, ID: 1, Username: "aLiCe"}
	if !checker.Allowed(peer, config.PermRead) {
		t.Error("username matching should be case-insensitive")
	}
}

func TestChecker_PhoneMatch(t *testing.T) {
	checker := mustNewChecker(t, []config.ChatRule{
		{Match: "+79001234567", Permissions: []config.Permission{config.PermRead}},
	})

	peer := PeerIdentity{Kind: KindUser, ID: 1, Phone: "+79001234567"}
	if !checker.Allowed(peer, config.PermRead) {
		t.Error("expected read to be allowed for phone match")
	}

	other := PeerIdentity{Kind: KindUser, ID: 2, Phone: "+79009999999"}
	if checker.Allowed(other, config.PermRead) {
		t.Error("expected read to be denied for different phone")
	}
}

func TestChecker_TypedIDMatch(t *testing.T) {
	checker := mustNewChecker(t, []config.ChatRule{
		{Match: "user:100", Permissions: []config.Permission{config.PermRead}},
		{Match: "chat:100", Permissions: []config.Permission{config.PermDraft}},
		{Match: "channel:100", Permissions: []config.Permission{config.PermMarkRead}},
	})

	user := PeerIdentity{Kind: KindUser, ID: 100}
	chat := PeerIdentity{Kind: KindChat, ID: 100}
	channel := PeerIdentity{Kind: KindChannel, ID: 100}

	// Same numeric ID, different types — no collision
	if !checker.Allowed(user, config.PermRead) {
		t.Error("user:100 should have read")
	}
	if checker.Allowed(user, config.PermDraft) {
		t.Error("user:100 should not have draft (that's chat:100)")
	}

	if !checker.Allowed(chat, config.PermDraft) {
		t.Error("chat:100 should have draft")
	}
	if checker.Allowed(chat, config.PermRead) {
		t.Error("chat:100 should not have read (that's user:100)")
	}

	if !checker.Allowed(channel, config.PermMarkRead) {
		t.Error("channel:100 should have mark_read")
	}
	if checker.Allowed(channel, config.PermRead) {
		t.Error("channel:100 should not have read (that's user:100)")
	}
}

func TestChecker_DefaultDeny(t *testing.T) {
	checker := mustNewChecker(t, []config.ChatRule{
		{Match: "@alice", Permissions: []config.Permission{config.PermRead}},
	})

	unknown := PeerIdentity{Kind: KindUser, ID: 999, Username: "bob"}
	if checker.Allowed(unknown, config.PermRead) {
		t.Error("unmatched peer should be denied (default-deny)")
	}
	if checker.MatchesAny(unknown) {
		t.Error("unmatched peer should not match any rule")
	}
}

func TestChecker_MatchesAny(t *testing.T) {
	checker := mustNewChecker(t, []config.ChatRule{
		{Match: "channel:42", Permissions: []config.Permission{config.PermMarkRead}},
	})

	peer := PeerIdentity{Kind: KindChannel, ID: 42}
	if !checker.MatchesAny(peer) {
		t.Error("channel:42 should match")
	}

	other := PeerIdentity{Kind: KindChannel, ID: 99}
	if checker.MatchesAny(other) {
		t.Error("channel:99 should not match")
	}
}

func TestChecker_PermissionMerging(t *testing.T) {
	// Same peer referenced by @username and user:ID — permissions should merge
	checker := mustNewChecker(t, []config.ChatRule{
		{Match: "@alice", Permissions: []config.Permission{config.PermRead}},
		{Match: "user:100", Permissions: []config.Permission{config.PermDraft}},
	})

	peer := PeerIdentity{Kind: KindUser, ID: 100, Username: "alice"}

	if !checker.Allowed(peer, config.PermRead) {
		t.Error("should have read from @alice rule")
	}
	if !checker.Allowed(peer, config.PermDraft) {
		t.Error("should have draft from user:100 rule (permissions must merge, not shadow)")
	}
	if checker.Allowed(peer, config.PermMarkRead) {
		t.Error("should not have mark_read (not granted by either rule)")
	}
}

func TestChecker_PhoneAndUsernameForSamePeer(t *testing.T) {
	checker := mustNewChecker(t, []config.ChatRule{
		{Match: "+79001234567", Permissions: []config.Permission{config.PermRead}},
		{Match: "@alice", Permissions: []config.Permission{config.PermDraft}},
	})

	// Peer with both phone and username — should match both rules
	peer := PeerIdentity{Kind: KindUser, ID: 1, Username: "alice", Phone: "+79001234567"}
	if !checker.Allowed(peer, config.PermRead) {
		t.Error("should have read from phone rule")
	}
	if !checker.Allowed(peer, config.PermDraft) {
		t.Error("should have draft from username rule")
	}
}

func TestChecker_PhoneNormalization(t *testing.T) {
	checker := mustNewChecker(t, []config.ChatRule{
		{Match: "+1 (311) 555-6162", Permissions: []config.Permission{config.PermRead}},
	})

	// Phone from Telegram Raw() is typically digits only with +
	peer := PeerIdentity{Kind: KindUser, ID: 1, Phone: "+13115556162"}
	if !checker.Allowed(peer, config.PermRead) {
		t.Error("phone matching should normalize: +1 (311) 555-6162 == +13115556162")
	}

	// Reverse: config has clean phone, identity has formatted
	checker2 := mustNewChecker(t, []config.ChatRule{
		{Match: "+13115556162", Permissions: []config.Permission{config.PermRead}},
	})
	peer2 := PeerIdentity{Kind: KindUser, ID: 1, Phone: "+1 311 555-6162"}
	if !checker2.Allowed(peer2, config.PermRead) {
		t.Error("phone matching should normalize in both directions")
	}
}

func TestChecker_PhoneMatchInDialogs(t *testing.T) {
	// Peer added by phone should be found in dialogs even when identity
	// comes from PeerToIdentity (which fills Phone from Raw())
	checker := mustNewChecker(t, []config.ChatRule{
		{Match: "+79001234567", Permissions: []config.Permission{config.PermRead}},
	})

	// Simulate a peer identity as it would come from PeerToIdentity
	peer := PeerIdentity{Kind: KindUser, ID: 42, Phone: "+79001234567"}
	if !checker.MatchesAny(peer) {
		t.Error("peer with matching phone should appear in dialogs")
	}
}

func TestNewChecker_InvalidMatch(t *testing.T) {
	_, err := NewChecker(config.ACLConfig{
		Chats: []config.ChatRule{
			{Match: "invalid", Permissions: []config.Permission{config.PermRead}},
		},
	})
	if err == nil {
		t.Error("expected error for invalid match format")
	}
}

func TestNewChecker_InvalidID(t *testing.T) {
	_, err := NewChecker(config.ACLConfig{
		Chats: []config.ChatRule{
			{Match: "user:notanumber", Permissions: []config.Permission{config.PermRead}},
		},
	})
	if err == nil {
		t.Error("expected error for non-numeric user ID")
	}
}

func mustNewChecker(t *testing.T, chats []config.ChatRule) *Checker {
	t.Helper()
	c, err := NewChecker(config.ACLConfig{Chats: chats})
	if err != nil {
		t.Fatalf("NewChecker: %v", err)
	}
	return c
}

func TestMayAllow(t *testing.T) {
	read := []config.Permission{config.PermRead}

	idOnly := mustNewChecker(t, []config.ChatRule{
		{Match: "user:100", Permissions: []config.Permission{config.PermRead, config.PermSend}},
		{Match: "channel:300", Permissions: read},
	})
	withUsername := mustNewChecker(t, []config.ChatRule{
		{Match: "user:100", Permissions: read},
		{Match: "@alice", Permissions: read},
	})
	withPhone := mustNewChecker(t, []config.ChatRule{
		{Match: "+7 900 123-45-67", Permissions: read},
	})

	tests := []struct {
		name    string
		checker *Checker
		ref     string
		perm    config.Permission
		want    bool
	}{
		{"typed ref in ACL", idOnly, "user:100", config.PermRead, true},
		{"typed ref without perm", idOnly, "channel:300", config.PermSend, false},
		{"typed ref outside ACL", idOnly, "user:999", config.PermRead, false},
		{"same ID, other kind", idOnly, "chat:100", config.PermRead, false},
		{"channel in ACL", idOnly, "channel:300", config.PermRead, true},
		{"username needs resolution against ID rules", idOnly, "@bob", config.PermRead, true},
		{"username needs resolution, perm nowhere", idOnly, "@bob", config.PermDraft, false},
		{"typed ref may match username rule", withUsername, "user:999", config.PermRead, true},
		{"username rule match", withUsername, "@ALICE", config.PermRead, true},
		{"phone rule match", withPhone, "+79001234567", config.PermRead, true},
		{"phone rule mismatch", withPhone, "+79990000000", config.PermRead, false},
		{"typed ref vs phone-only ACL", withPhone, "user:1", config.PermRead, true},
		{"unparseable ref left to resolver", idOnly, "user:abc", config.PermRead, true},
		{"username with spaces", withUsername, " @alice ", config.PermRead, true},
		{"secondary username decided after resolution", withUsername, "@alt", config.PermRead, true},
		{"phone with extra plus", withPhone, "+7+900 123-45-67", config.PermRead, true},
		{"deeplink after @ left to resolver", withUsername, "@https://t.me/alice", config.PermRead, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.checker.MayAllow(tt.ref, tt.perm); got != tt.want {
				t.Errorf("MayAllow(%q, %s) = %v, want %v", tt.ref, tt.perm, got, tt.want)
			}
		})
	}
}

func TestParseRef(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"@Alice", "@alice", false},
		{"  @alice\t", "@alice", false},
		{"@alice_bot2", "@alice_bot2", false},
		{"+7 (900) 123-45-67", "+79001234567", false},
		{"+7+900 123", "+7900123", false},
		{"user:100", "user:100", false},
		{"chat:200", "chat:200", false},
		{"channel:300", "channel:300", false},
		{"@", "", true},
		{"@ alice", "", true},
		{"@https://t.me/alice", "", true},
		{"@+79001234567", "", true},
		{"+", "", true},
		{"+abc", "", true},
		{"user:abc", "", true},
		{"alice", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParseRef(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseRef(%q) = %v, want error", tt.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseRef(%q): %v", tt.in, err)
			}
			if got.String() != tt.want {
				t.Errorf("ParseRef(%q) = %q, want %q", tt.in, got.String(), tt.want)
			}
		})
	}
}

func TestPeerIdentityRef(t *testing.T) {
	tests := []struct {
		identity PeerIdentity
		typed    string
		ref      string
	}{
		{PeerIdentity{Kind: KindUser, ID: 1, Username: "alice"}, "user:1", "@alice"},
		{PeerIdentity{Kind: KindUser, ID: 1}, "user:1", "user:1"},
		{PeerIdentity{Kind: KindChat, ID: 2}, "chat:2", "chat:2"},
		{PeerIdentity{Kind: KindChannel, ID: 3, Username: "news"}, "channel:3", "@news"},
		{PeerIdentity{}, "", ""},
	}
	for _, tt := range tests {
		if got := tt.identity.TypedRef(); got != tt.typed {
			t.Errorf("%+v.TypedRef() = %q, want %q", tt.identity, got, tt.typed)
		}
		if got := tt.identity.Ref(); got != tt.ref {
			t.Errorf("%+v.Ref() = %q, want %q", tt.identity, got, tt.ref)
		}
	}
}
