package tools

import (
	"testing"

	"github.com/chestnykh/mcp-telegram/internal/acl"
	"github.com/chestnykh/mcp-telegram/internal/config"
)

func TestSendACLDeny(t *testing.T) {
	checker, err := acl.NewChecker(config.ACLConfig{
		Chats: []config.ChatRule{
			{Match: "@alice", Permissions: []config.Permission{config.PermRead}},
		},
	})
	if err != nil {
		t.Fatalf("NewChecker: %v", err)
	}

	peer := acl.PeerIdentity{Kind: acl.KindUser, ID: 1, Username: "alice"}

	if checker.Allowed(peer, config.PermSend) {
		t.Error("@alice should not have send permission (only read)")
	}
	if !checker.Allowed(peer, config.PermRead) {
		t.Error("@alice should have read permission")
	}
}

func TestSendACLAllow(t *testing.T) {
	checker, err := acl.NewChecker(config.ACLConfig{
		Chats: []config.ChatRule{
			{Match: "@bob", Permissions: []config.Permission{config.PermRead, config.PermSend}},
		},
	})
	if err != nil {
		t.Fatalf("NewChecker: %v", err)
	}

	peer := acl.PeerIdentity{Kind: acl.KindUser, ID: 2, Username: "bob"}

	if !checker.Allowed(peer, config.PermSend) {
		t.Error("@bob should have send permission")
	}
}
