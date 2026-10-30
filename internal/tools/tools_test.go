package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Prgebish/mcp-telegram/internal/acl"
	"github.com/Prgebish/mcp-telegram/internal/config"
	"github.com/gotd/td/tg"
)

func TestToolError(t *testing.T) {
	result := toolError("something went wrong")
	if !result.IsError {
		t.Error("expected IsError to be true")
	}
	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(result.Content))
	}
}

func TestIsPathUnder(t *testing.T) {
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "sub")
	os.MkdirAll(subDir, 0755)

	tests := []struct {
		name    string
		path    string
		allowed []string
		want    bool
	}{
		{"exact match", tmpDir, []string{tmpDir}, true},
		{"subdirectory", filepath.Join(tmpDir, "sub", "file.txt"), []string{tmpDir}, true},
		{"outside", "/etc/passwd", []string{tmpDir}, false},
		{"traversal attempt", filepath.Join(tmpDir, "..", "etc", "passwd"), []string{tmpDir}, false},
		{"empty allowed", tmpDir, nil, false},
		{"multiple allowed", filepath.Join(subDir, "file.txt"), []string{"/nonexistent", subDir}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isPathUnder(tt.path, tt.allowed)
			if got != tt.want {
				t.Errorf("isPathUnder(%q, %v) = %v, want %v", tt.path, tt.allowed, got, tt.want)
			}
		})
	}
}

func TestIsPathUnder_SymlinkBypass(t *testing.T) {
	tmpDir := t.TempDir()
	allowed := filepath.Join(tmpDir, "allowed")
	os.MkdirAll(allowed, 0755)

	// Create a target outside the allowed directory.
	outside := filepath.Join(tmpDir, "outside")
	os.MkdirAll(outside, 0755)
	os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0600)

	// Create a symlink inside allowed that points outside.
	link := filepath.Join(allowed, "link")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}

	// /allowed/link/secret.txt should NOT be considered under /allowed
	// because the symlink resolves to /outside/secret.txt.
	if isPathUnder(filepath.Join(link, "secret.txt"), []string{allowed}) {
		t.Error("symlink bypass: path through symlink directory should not be considered under allowed dir")
	}

	// Leaf symlink: /allowed/secret_link -> /outside/secret.txt
	leafLink := filepath.Join(allowed, "secret_link")
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), leafLink); err != nil {
		t.Skipf("cannot create leaf symlink: %v", err)
	}
	if isPathUnder(leafLink, []string{allowed}) {
		t.Error("symlink bypass: leaf symlink pointing outside should not be considered under allowed dir")
	}
}

func TestPtrBool(t *testing.T) {
	v := ptrBool(true)
	if v == nil || *v != true {
		t.Error("ptrBool(true) should return pointer to true")
	}
	v = ptrBool(false)
	if v == nil || *v != false {
		t.Error("ptrBool(false) should return pointer to false")
	}
}

func TestResolveAllowed_DeniesBeforeResolve(t *testing.T) {
	resolver := &dynamicMockResolver{
		fn: func(_ context.Context, ref string) (Peer, acl.PeerIdentity, error) {
			t.Fatalf("resolver called for %q: chats outside the ACL must be denied before any lookup", ref)
			return nil, acl.PeerIdentity{}, nil
		},
	}
	deps := newTestDeps(resolver, nil)
	deps.ACL = newTestChecker("user:100", config.PermRead)

	for _, ref := range []string{"user:999", "chat:100", "channel:100"} {
		_, _, denied := resolveAllowed(context.Background(), deps, ref, config.PermRead)
		if denied == nil || !strings.Contains(resultText(denied), "access denied") {
			t.Errorf("ref %q: expected access denied, got %v", ref, denied)
		}
	}
	_, _, denied := resolveAllowed(context.Background(), deps, "user:100", config.PermSend)
	if denied == nil || !strings.Contains(resultText(denied), "'send' permission") {
		t.Errorf("expected send denied, got %v", denied)
	}
}

func TestResolveAllowed_ChecksResolvedIdentity(t *testing.T) {
	// @username refs cannot be decided by the string alone, so they are
	// resolved and then checked against the resolved identity.
	resolver := &mockResolver{
		peer:     &mockPeer{inputPeer: &tg.InputPeerUser{UserID: 555}},
		identity: acl.PeerIdentity{Kind: acl.KindUser, ID: 555, Username: "stranger"},
	}
	deps := newTestDeps(resolver, nil)
	deps.ACL = newTestChecker("user:100", config.PermRead)

	_, _, denied := resolveAllowed(context.Background(), deps, "@stranger", config.PermRead)
	if denied == nil || !strings.Contains(resultText(denied), "access denied") {
		t.Errorf("expected access denied after resolution, got %v", denied)
	}

	resolver.identity.ID = 100
	if _, _, denied := resolveAllowed(context.Background(), deps, "@stranger", config.PermRead); denied != nil {
		t.Errorf("expected allowed, got %s", resultText(denied))
	}
}
