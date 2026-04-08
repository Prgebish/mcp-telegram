package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Prgebish/mcp-telegram/internal/config"
	"github.com/gotd/td/telegram/downloader"
	"github.com/gotd/td/telegram/message/peer"
	"github.com/gotd/td/telegram/query/messages"
	"github.com/gotd/td/tg"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type historyInput struct {
	Chat       string `json:"chat" jsonschema:"required,Chat reference: @username, user:ID, chat:ID, or channel:ID"`
	Limit      int    `json:"limit,omitempty" jsonschema:"Max messages to return (default from config)"`
	OffsetID   int    `json:"offset_id,omitempty" jsonschema:"Message ID to start from for pagination"`
	Since      string `json:"since,omitempty" jsonschema:"Read messages from this date (format: 2006-01-02 or 2006-01-02 15:04)"`
	Until      string `json:"until,omitempty" jsonschema:"Read messages until this date (format: 2006-01-02 or 2006-01-02 15:04)"`
	DownloadTo string `json:"download_to,omitempty" jsonschema:"Directory to download media to (overrides config)"`
}

var dateFormats = []string{
	"2006-01-02 15:04:05",
	"2006-01-02 15:04",
	"2006-01-02",
}

func parseDate(s string) (time.Time, error) {
	for _, f := range dateFormats {
		if t, err := time.ParseInLocation(f, s, time.Local); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid date %q, expected format: 2006-01-02, 2006-01-02 15:04, or 2006-01-02 15:04:05", s)
}

func registerHistory(server *mcp.Server, deps *Deps) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "tg_history",
		Description: "Get message history from a whitelisted chat. Supports pagination (offset_id) and date range filtering (since/until).",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    true,
			DestructiveHint: ptrBool(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input historyInput) (*mcp.CallToolResult, any, error) {
		peer, identity, err := deps.Resolver.ResolvePeerForTool(ctx, input.Chat)
		if err != nil {
			return toolError(fmt.Sprintf("cannot resolve chat: %v", err)), nil, nil
		}

		if !deps.ACL.Allowed(identity, config.PermRead) {
			return toolError(fmt.Sprintf("access denied: %s does not have 'read' permission", input.Chat)), nil, nil
		}

		// Parse date filters.
		var sinceTime, untilTime time.Time
		if input.Since != "" {
			sinceTime, err = parseDate(input.Since)
			if err != nil {
				return toolError(err.Error()), nil, nil
			}
		}
		if input.Until != "" {
			untilTime, err = parseDate(input.Until)
			if err != nil {
				return toolError(err.Error()), nil, nil
			}
			// If only date (no time), set to end of day.
			if len(input.Until) == 10 {
				untilTime = untilTime.Add(24*time.Hour - time.Second)
			}
		}

		limit := input.Limit
		if limit <= 0 || limit > deps.Limits.MaxMessagesPerRequest {
			limit = deps.Limits.MaxMessagesPerRequest
		}

		qb := messages.NewQueryBuilder(deps.API).
			GetHistory(peer.InputPeer()).
			BatchSize(limit)

		if input.OffsetID > 0 {
			qb = qb.OffsetID(input.OffsetID)
		}

		// If until is set, use OffsetDate to start from that point.
		// Telegram returns messages BEFORE OffsetDate.
		if !untilTime.IsZero() && input.OffsetID == 0 {
			qb = qb.OffsetDate(int(untilTime.Unix()) + 1)
		}

		// Build effective media config: download_to overrides config.
		mediaCfg := deps.Media
		if input.DownloadTo != "" {
			if deps.Media.Directory == "" {
				return toolError("download_to requires media.directory to be configured"), nil, nil
			}
			dir := expandTilde(input.DownloadTo)
			if !isPathUnder(dir, []string{deps.Media.Directory}) {
				return toolError(fmt.Sprintf("download_to must be under media.directory (%s)", deps.Media.Directory)), nil, nil
			}
			mediaCfg = config.MediaConfig{
				Download:  []string{"photo", "document", "video", "voice", "audio"},
				Directory: dir,
			}
		}
		if len(mediaCfg.Download) > 0 && mediaCfg.Directory != "" {
			if err := os.MkdirAll(mediaCfg.Directory, 0700); err != nil {
				return toolError(fmt.Sprintf("cannot create media directory: %v", err)), nil, nil
			}
		}

		// Use effective media config for this request.
		effectiveDeps := *deps
		effectiveDeps.Media = mediaCfg

		dl := downloader.NewDownloader()
		iter := qb.Iter()
		var lines []string
		count := 0
		lastID := 0

		for iter.Next(ctx) {
			if count >= limit {
				break
			}

			elem := iter.Value()
			msg, ok := elem.Msg.(*tg.Message)
			if !ok {
				continue
			}

			msgTime := time.Unix(int64(msg.Date), 0).In(time.Local)

			// Stop if message is older than since.
			if !sinceTime.IsZero() && msgTime.Before(sinceTime) {
				break
			}

			// Skip if message is newer than until.
			if !untilTime.IsZero() && msgTime.After(untilTime) {
				continue
			}

			from := resolveFromName(msg, elem.Peer, elem.Entities)
			ts := msgTime.Format("2006-01-02 15:04")
			text := msg.Message
			if text == "" {
				text = formatMedia(ctx, &effectiveDeps, dl, msg, elem.Peer)
			} else if msg.Media != nil {
				// Message has both text and media.
				mediaText := formatMedia(ctx, &effectiveDeps, dl, msg, elem.Peer)
				text += " " + mediaText
			}

			lines = append(lines, fmt.Sprintf("[%d] %s (%s): %s", msg.ID, from, ts, text))
			lastID = msg.ID
			count++
		}
		if err := iter.Err(); err != nil {
			return toolError(fmt.Sprintf("failed to fetch history: %v", err)), nil, nil
		}

		if len(lines) == 0 {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: "No messages found."},
				},
			}, nil, nil
		}

		result := strings.Join(lines, "\n")
		if count == limit && lastID > 0 {
			result += fmt.Sprintf("\n\n[Use offset_id=%d to load older messages]", lastID)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: result},
			},
		}, nil, nil
	})
}

// formatMedia returns a text representation of the message's media.
// If media downloading is configured, it downloads the file and returns the path.
// For media messages, appends a deep link to open in Telegram.
func formatMedia(ctx context.Context, deps *Deps, dl *downloader.Downloader, msg *tg.Message, dialogPeer tg.InputPeerClass) string {
	if msg.Media == nil {
		return "[empty]"
	}

	link := telegramLink(dialogPeer, msg.ID)
	var label string

	switch media := msg.Media.(type) {
	case *tg.MessageMediaPhoto:
		if deps.Media.ShouldDownload("photo") {
			if path, err := downloadPhoto(ctx, deps, dl, media, peerIDPrefix(dialogPeer), msg.ID); err == nil {
				label = fmt.Sprintf("[photo: %s]", path)
				break
			}
		}
		label = "[photo]"
	case *tg.MessageMediaDocument:
		if media.Document == nil {
			label = "[document]"
			break
		}
		doc, ok := media.Document.AsNotEmpty()
		if !ok {
			label = "[document]"
			break
		}
		mediaType := classifyDocument(doc)
		if deps.Media.ShouldDownload(mediaType) {
			if path, err := downloadDocument(ctx, deps, dl, doc, peerIDPrefix(dialogPeer), msg.ID); err == nil {
				label = fmt.Sprintf("[%s: %s]", mediaType, path)
				break
			}
		}
		label = fmt.Sprintf("[%s]", mediaType)
	case *tg.MessageMediaGeo:
		label = "[location]"
	case *tg.MessageMediaContact:
		label = "[contact]"
	case *tg.MessageMediaWebPage:
		return "[webpage]" // No deep link needed for webpages.
	case *tg.MessageMediaPoll:
		label = "[poll]"
	case *tg.MessageMediaVenue:
		label = "[venue]"
	case *tg.MessageMediaGeoLive:
		label = "[live location]"
	case *tg.MessageMediaDice:
		label = "[dice]"
	default:
		label = "[media]"
	}

	if link != "" {
		return label + " " + link
	}
	return label
}

// telegramLink builds a deep link to open a specific message in Telegram.
// Only works for channels/supergroups — private chats and basic groups
// don't support message deep links.
func telegramLink(dialogPeer tg.InputPeerClass, msgID int) string {
	switch p := dialogPeer.(type) {
	case *tg.InputPeerChannel:
		return fmt.Sprintf("tg://privatepost?channel=%d&post=%d", p.ChannelID, msgID)
	default:
		return ""
	}
}

// peerIDPrefix returns a string prefix for file naming to avoid collisions
// between messages with the same ID in different chats.
func peerIDPrefix(p tg.InputPeerClass) string {
	switch peer := p.(type) {
	case *tg.InputPeerUser:
		return fmt.Sprintf("u%d", peer.UserID)
	case *tg.InputPeerChat:
		return fmt.Sprintf("c%d", peer.ChatID)
	case *tg.InputPeerChannel:
		return fmt.Sprintf("ch%d", peer.ChannelID)
	default:
		return "unknown"
	}
}

func downloadPhoto(ctx context.Context, deps *Deps, dl *downloader.Downloader, media *tg.MessageMediaPhoto, chatPrefix string, msgID int) (string, error) {
	photo, ok := media.Photo.AsNotEmpty()
	if !ok {
		return "", fmt.Errorf("empty photo")
	}

	// Pick the largest size.
	var bestSize tg.PhotoSizeClass
	var bestPixels int
	for _, size := range photo.Sizes {
		switch s := size.(type) {
		case *tg.PhotoSize:
			px := s.W * s.H
			if px > bestPixels {
				bestPixels = px
				bestSize = size
			}
		case *tg.PhotoSizeProgressive:
			px := s.W * s.H
			if px > bestPixels {
				bestPixels = px
				bestSize = size
			}
		}
	}
	if bestSize == nil {
		return "", fmt.Errorf("no suitable photo size")
	}

	// Determine size type string.
	var sizeType string
	switch s := bestSize.(type) {
	case *tg.PhotoSize:
		sizeType = s.Type
	case *tg.PhotoSizeProgressive:
		sizeType = s.Type
	}

	filename := fmt.Sprintf("%s_%d.jpg", chatPrefix, msgID)
	path := filepath.Join(deps.Media.Directory, filename)

	loc := &tg.InputPhotoFileLocation{
		ID:            photo.ID,
		AccessHash:    photo.AccessHash,
		FileReference: photo.FileReference,
		ThumbSize:     sizeType,
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return "", err
	}
	defer f.Close()

	_, err = dl.Download(deps.API, loc).Stream(ctx, f)
	if err != nil {
		os.Remove(path)
		return "", err
	}

	return path, nil
}

func downloadDocument(ctx context.Context, deps *Deps, dl *downloader.Downloader, doc *tg.Document, chatPrefix string, msgID int) (string, error) {
	ext := ".bin"
	for _, attr := range doc.Attributes {
		if fn, ok := attr.(*tg.DocumentAttributeFilename); ok {
			ext = filepath.Ext(fn.FileName)
			break
		}
	}

	filename := fmt.Sprintf("%s_%d%s", chatPrefix, msgID, ext)
	path := filepath.Join(deps.Media.Directory, filename)

	loc := &tg.InputDocumentFileLocation{
		ID:            doc.ID,
		AccessHash:    doc.AccessHash,
		FileReference: doc.FileReference,
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return "", err
	}
	defer f.Close()

	_, err = dl.Download(deps.API, loc).Stream(ctx, f)
	if err != nil {
		os.Remove(path)
		return "", err
	}

	return path, nil
}

// classifyDocument determines the media type of a document based on its attributes.
func classifyDocument(doc *tg.Document) string {
	for _, attr := range doc.Attributes {
		switch attr.(type) {
		case *tg.DocumentAttributeVideo:
			return "video"
		case *tg.DocumentAttributeAudio:
			a := attr.(*tg.DocumentAttributeAudio)
			if a.Voice {
				return "voice"
			}
			return "audio"
		}
	}
	return "document"
}

func expandTilde(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return home + path[1:]
	}
	return path
}

// resolveFromName extracts sender name from message using entities from
// the same API response. If FromID is nil (common in private chats for
// the other party's messages), falls back to the dialog peer.
func resolveFromName(msg *tg.Message, dialogPeer tg.InputPeerClass, entities peer.Entities) string {
	fromID := msg.FromID
	if fromID == nil {
		// In private chats, the other party's messages have no FromID.
		// In channels, posts from the channel itself have no FromID.
		// Fall back to the dialog peer.
		switch p := dialogPeer.(type) {
		case *tg.InputPeerUser:
			fromID = &tg.PeerUser{UserID: p.UserID}
		case *tg.InputPeerChannel:
			fromID = &tg.PeerChannel{ChannelID: p.ChannelID}
		case *tg.InputPeerChat:
			fromID = &tg.PeerChat{ChatID: p.ChatID}
		}
	}
	if fromID == nil {
		return "unknown"
	}

	switch from := fromID.(type) {
	case *tg.PeerUser:
		if u, ok := entities.User(from.UserID); ok {
			name := u.FirstName
			if u.LastName != "" {
				name += " " + u.LastName
			}
			return name
		}
		return fmt.Sprintf("user:%d", from.UserID)
	case *tg.PeerChat:
		if c, ok := entities.Chat(from.ChatID); ok {
			return c.Title
		}
		return fmt.Sprintf("chat:%d", from.ChatID)
	case *tg.PeerChannel:
		if ch, ok := entities.Channel(from.ChannelID); ok {
			return ch.Title
		}
		return fmt.Sprintf("channel:%d", from.ChannelID)
	default:
		return "unknown"
	}
}
