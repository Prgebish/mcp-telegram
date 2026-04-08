package tools

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/Prgebish/mcp-telegram/internal/acl"
	"github.com/Prgebish/mcp-telegram/internal/config"
	"github.com/gotd/td/tg"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Peer represents a resolved Telegram peer used by tool handlers.
type Peer interface {
	InputPeer() tg.InputPeerClass
}

// ChannelPeer is a peer that supports channel-specific operations.
type ChannelPeer interface {
	Peer
	InputChannel() tg.InputChannelClass
}

// PeerResolver resolves chat reference strings to Telegram peers.
type PeerResolver interface {
	ResolvePeerForTool(ctx context.Context, ref string) (Peer, acl.PeerIdentity, error)
}

// Deps holds all dependencies for tool handlers.
type Deps struct {
	Resolver PeerResolver
	API      *tg.Client
	ACL      *acl.Checker
	Limits   config.LimitsConfig
	Media    config.MediaConfig
}

func Register(server *mcp.Server, deps *Deps) {
	registerMe(server, deps)
	registerDialogs(server, deps)
	registerHistory(server, deps)
	registerSend(server, deps)
	registerDraft(server, deps)
	registerMarkRead(server, deps)
}

func ptrBool(v bool) *bool {
	return &v
}

// isPathUnder checks whether path resolves to a location under one of the allowed directories.
// All symlinks (including the leaf component) are resolved to prevent bypass.
func isPathUnder(path string, allowedDirs []string) bool {
	absPath, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return false
	}
	// Try to resolve the full path (covers existing files and leaf symlinks).
	// If it doesn't exist yet (new file), resolve the parent directory.
	resolvedPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		parentDir, err := filepath.EvalSymlinks(filepath.Dir(absPath))
		if err != nil {
			return false
		}
		resolvedPath = filepath.Join(parentDir, filepath.Base(absPath))
	}

	for _, allowed := range allowedDirs {
		absAllowed, err := filepath.Abs(filepath.Clean(allowed))
		if err != nil {
			continue
		}
		resolvedAllowed, err := filepath.EvalSymlinks(absAllowed)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(resolvedAllowed, resolvedPath)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func toolError(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: msg},
		},
		IsError: true,
	}
}
