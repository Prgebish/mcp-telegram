package tools

import (
	"github.com/chestnykh/mcp-telegram/internal/acl"
	"github.com/chestnykh/mcp-telegram/internal/config"
	tgclient "github.com/chestnykh/mcp-telegram/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Deps struct {
	Client *tgclient.Client
	ACL    *acl.Checker
	Limits config.LimitsConfig
	Media  config.MediaConfig
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

func toolError(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: msg},
		},
		IsError: true,
	}
}
