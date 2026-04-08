package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Prgebish/mcp-telegram/internal/config"
	"github.com/gotd/td/telegram/downloader"
	"github.com/gotd/td/telegram/message/peer"
	"github.com/gotd/td/tg"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type searchInput struct {
	Chat     string `json:"chat" jsonschema:"required,Chat reference: @username, user:ID, chat:ID, or channel:ID"`
	Query    string `json:"query" jsonschema:"required,Text to search for"`
	Limit    int    `json:"limit,omitempty" jsonschema:"Max results to return (default from config)"`
	OffsetID int    `json:"offset_id,omitempty" jsonschema:"Message ID to start from for pagination"`
	From     string `json:"from,omitempty" jsonschema:"Filter by sender: @username or user:ID"`
}

func registerSearch(server *mcp.Server, deps *Deps) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "tg_search",
		Description: "Search for messages in a whitelisted chat by text query. Supports pagination (offset_id) and sender filter (from).",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    true,
			DestructiveHint: ptrBool(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input searchInput) (*mcp.CallToolResult, any, error) {
		return handleSearch(ctx, deps, input), nil, nil
	})
}

func handleSearch(ctx context.Context, deps *Deps, input searchInput) *mcp.CallToolResult {
	if input.Query == "" {
		return toolError("query is required")
	}

	chatPeer, identity, err := deps.Resolver.ResolvePeerForTool(ctx, input.Chat)
	if err != nil {
		return toolError(fmt.Sprintf("cannot resolve chat: %v", err))
	}

	if !deps.ACL.Allowed(identity, config.PermRead) {
		return toolError(fmt.Sprintf("access denied: %s does not have 'read' permission", input.Chat))
	}

	limit := input.Limit
	if limit <= 0 || limit > deps.Limits.MaxMessagesPerRequest {
		limit = deps.Limits.MaxMessagesPerRequest
	}

	req := &tg.MessagesSearchRequest{
		Peer:     chatPeer.InputPeer(),
		Q:        input.Query,
		Filter:   &tg.InputMessagesFilterEmpty{},
		Limit:    limit,
		OffsetID: input.OffsetID,
	}

	// Optional sender filter.
	if input.From != "" {
		fromPeer, _, err := deps.Resolver.ResolvePeerForTool(ctx, input.From)
		if err != nil {
			return toolError(fmt.Sprintf("cannot resolve from: %v", err))
		}
		req.SetFromID(fromPeer.InputPeer())
	}

	result, err := deps.API.MessagesSearch(ctx, req)
	if err != nil {
		return toolError(fmt.Sprintf("search failed: %v", err))
	}

	modified, ok := result.AsModified()
	if !ok {
		return toolError("unexpected search response type")
	}

	messages := modified.GetMessages()
	users := modified.GetUsers()
	chats := modified.GetChats()

	usersMap := make(map[int64]*tg.User, len(users))
	for _, u := range users {
		if user, ok := u.AsNotEmpty(); ok {
			usersMap[user.ID] = user
		}
	}
	chatsMap := make(map[int64]*tg.Chat)
	channelsMap := make(map[int64]*tg.Channel)
	for _, c := range chats {
		switch v := c.(type) {
		case *tg.Chat:
			chatsMap[v.ID] = v
		case *tg.Channel:
			channelsMap[v.ID] = v
		}
	}

	entities := peer.NewEntities(usersMap, chatsMap, channelsMap)
	dl := downloader.NewDownloader()

	var lines []string
	var lastID int
	for _, msgClass := range messages {
		msg, ok := msgClass.(*tg.Message)
		if !ok {
			continue
		}

		from := resolveFromName(msg, chatPeer.InputPeer(), entities)
		ts := time.Unix(int64(msg.Date), 0).In(time.Local).Format("2006-01-02 15:04")
		text := msg.Message
		if text == "" {
			text = formatMedia(ctx, deps, dl, msg, chatPeer.InputPeer())
		} else if msg.Media != nil {
			mediaText := formatMedia(ctx, deps, dl, msg, chatPeer.InputPeer())
			text += " " + mediaText
		}

		lines = append(lines, fmt.Sprintf("[%d] %s (%s): %s", msg.ID, from, ts, text))
		lastID = msg.ID
	}

	if len(lines) == 0 {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("No messages found matching %q.", input.Query)},
			},
		}
	}

	out := strings.Join(lines, "\n")
	if len(lines) == limit && lastID > 0 {
		out += fmt.Sprintf("\n\n[Use offset_id=%d to load more results]", lastID)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: out},
		},
	}
}
