package tools

import (
	"fmt"

	"github.com/Prgebish/mcp-telegram/internal/markdown"
	"github.com/gotd/td/tg"
)

// parseModeMarkdown is the only non-empty parse_mode accepted today.
const parseModeMarkdown = "markdown"

// parseModeDescription is shared by every tool that accepts the field, so the
// schema stays identical across tools.
const parseModeDescription = `Formatting of 'text'. "" (default) sends it verbatim; ` +
	`"markdown" interprets **bold**, __italic__, ~~strike~~, ||spoiler||, ` +
	"`code`, ```pre``` and [text](url)."

// renderText turns the caller's text into the plain string and entities
// Telegram expects.
//
// An empty parse mode returns the text untouched and no entities, which keeps
// existing callers byte-for-byte compatible.
func renderText(text, parseMode string) (string, []tg.MessageEntityClass, error) {
	switch parseMode {
	case "":
		return text, nil, nil
	case parseModeMarkdown:
		return markdown.Parse(text)
	default:
		return "", nil, fmt.Errorf("unsupported parse_mode %q (supported: %q)", parseMode, parseModeMarkdown)
	}
}
