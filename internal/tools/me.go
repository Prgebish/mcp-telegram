package tools

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type meInput struct{}

func registerMe(server *mcp.Server, deps *Deps) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "tg_me",
		Description: "Get current Telegram account info",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    true,
			DestructiveHint: ptrBool(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, _ meInput) (*mcp.CallToolResult, any, error) {
		if err := deps.Limiter.Wait(ctx); err != nil {
			return nil, nil, err
		}

		self, err := deps.Client.API().UsersGetFullUser(ctx, nil)
		if err != nil {
			return toolError(fmt.Sprintf("failed to get account info: %v", err)), nil, nil
		}

		if len(self.Users) == 0 {
			return toolError("no user info returned"), nil, nil
		}

		user, ok := self.Users[0].AsNotEmpty()
		if !ok {
			return toolError("empty user info"), nil, nil
		}

		text := fmt.Sprintf("ID: %d\nFirst name: %s", user.ID, user.FirstName)
		if user.LastName != "" {
			text += fmt.Sprintf("\nLast name: %s", user.LastName)
		}
		if user.Username != "" {
			text += fmt.Sprintf("\nUsername: @%s", user.Username)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: text},
			},
		}, nil, nil
	})
}
