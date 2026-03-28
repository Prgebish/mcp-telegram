package acl

import (
	"testing"

	"github.com/chestnykh/mcp-telegram/internal/config"
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
