package tools

import (
	"github.com/chestnykh/mcp-telegram/internal/acl"
	"github.com/chestnykh/mcp-telegram/internal/config"
	"github.com/chestnykh/mcp-telegram/internal/ratelimit"
	tgclient "github.com/chestnykh/mcp-telegram/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Deps struct {
	Client  *tgclient.Client
	ACL     *acl.Checker
	Limiter *ratelimit.Limiter
	Limits  config.LimitsConfig
}

func Register(server *mcp.Server, deps *Deps) {
	registerMe(server, deps)
	registerDialogs(server, deps)
	registerHistory(server, deps)
	registerDraft(server, deps)
	registerMarkRead(server, deps)
}

func ptrBool(v bool) *bool {
	return &v
}

func toolError(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: msg},
		},
		IsError: true,
	}
}
