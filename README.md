# mcp-telegram

[![MCP Server](https://img.shields.io/badge/MCP-Server-blue)](https://modelcontextprotocol.io)
[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

An [MCP](https://modelcontextprotocol.io) server that connects AI assistants like Claude to your **real Telegram account** via the User API (MTProto). Not a bot — Claude reads and sends messages as you.

Built with [gotd/td](https://github.com/gotd/td) and the official [MCP Go SDK](https://github.com/modelcontextprotocol/go-sdk).

> **Telegram API Terms of Service**: This project uses the Telegram User API. You must obtain your own `api_id` and `api_hash` from [my.telegram.org](https://my.telegram.org) and comply with the [Telegram API Terms of Service](https://core.telegram.org/api/terms). Misuse of the User API (spam, bulk messaging, scraping) may result in your account being banned. You are solely responsible for how you use this tool.

## Contents

- [Features](#features)
- [What you can do with it](#what-you-can-do-with-it)
- [How it compares to chaindead/telegram-mcp](#how-it-compares-to-chaindeadtelegram-mcp)
- [Quick start](#quick-start)
- [Client configuration](#client-configuration)
- [Configuration reference](#configuration)
- [Security](#security)

---

## Features

| Tool | What it does | Required permission |
|------|-------------|---------------------|
| `tg_me` | Returns current account info | — |
| `tg_dialogs` | Lists dialogs visible to the ACL whitelist | — |
| `tg_history` | Fetches message history with pagination, date filtering, and media download | `read` |
| `tg_send` | Sends a text message or file, with optional reply-to | `send` |
| `tg_draft` | Saves a draft message (does not send) | `draft` |
| `tg_mark_read` | Marks a chat as read | `mark_read` |

**Additional capabilities:**

- File and photo sending
- Reply to specific messages
- Download photos and documents from message history
- Filter history by date range (`since` / `until`)
- Typed peer references (`user:ID`, `chat:ID`, `channel:ID`) to prevent ID collisions
- Lazy peer resolution — avoids `FLOOD_WAIT` errors at startup
- Global rate limiting at the RPC level

## What you can do with it

Once connected, you can ask your AI assistant things like:

**Catch up on messages**
- "Check my unread Telegram messages and give me a summary"
- "What did @alice write in the last 24 hours?"
- "Show me messages from the Dev Team chat since Monday"

**Reply and communicate**
- "Draft a response to the last message from @bob — don't send it yet"
- "Send 'sounds good, let's meet at 3pm' to @alice"
- "Reply to message 1234 in the project chat with my feedback"

**Manage your inbox**
- "Mark all read in the news channel"
- "Which of my whitelisted chats have unread messages?"
- "Download the photos from today's messages in the design chat"

**Research and analyze**
- "Find all messages mentioning the deployment in the last week"
- "Summarize the discussion in the team chat from yesterday"
- "What files were shared in the project channel this month?"

---

## How it compares to chaindead/telegram-mcp

| | mcp-telegram | chaindead/telegram-mcp |
|---|---|---|
| Access control | Default-deny ACL whitelist with granular per-chat permissions | Full access to all chats |
| Peer addressing | Typed references (`user:ID`, `chat:ID`, `channel:ID`) | Numeric IDs only (collision-prone) |
| Configuration | YAML config with environment variable expansion | CLI flags |
| Startup safety | Lazy peer resolution (no bulk API calls) | Eager resolution (FLOOD_WAIT risk) |
| Rate limiting | Built-in token bucket middleware | None |
| File support | Send files, photos; download media from history | Text only |
| Reply support | Yes | No |
| Date filtering | Yes | No |

---

## Quick start

### Prerequisites

- Go 1.26+
- A Telegram account
- API credentials from [my.telegram.org](https://my.telegram.org) (`api_id` and `api_hash`)

### Install

**Homebrew (macOS / Linux):**

```bash
brew install Prgebish/tap/mcp-telegram
```

**Pre-built binaries (macOS / Linux / Windows):**

Download from [GitHub Releases](https://github.com/Prgebish/mcp-telegram/releases).

**Go install:**

```bash
go install github.com/Prgebish/mcp-telegram/cmd/mcp-telegram@latest
```

**From source:**

```bash
git clone https://github.com/Prgebish/mcp-telegram.git
cd mcp-telegram
go build ./cmd/mcp-telegram
```

This produces `mcp-telegram` (or `mcp-telegram.exe` on Windows) in the current directory.

### Authenticate

Run the auth command once to create a session file. You will be prompted for your phone number, the login code, and (if enabled) your 2FA password.

**macOS / Linux:**

```bash
export TG_APP_ID=12345
export TG_API_HASH="your_api_hash"

mcp-telegram auth --config config.yaml
```

**Windows (PowerShell):**

```powershell
$env:TG_APP_ID = "12345"
$env:TG_API_HASH = "your_api_hash"

mcp-telegram.exe auth --config config.yaml
```

**Windows (cmd):**

```cmd
set TG_APP_ID=12345
set TG_API_HASH=your_api_hash

mcp-telegram.exe auth --config config.yaml
```

### Configure

Create a `config.yaml`:

```yaml
telegram:
  app_id: ${TG_APP_ID}
  api_hash: ${TG_API_HASH}
  session_path: ~/.config/mcp-telegram/session.json

acl:
  chats:
    - match: "@username"
      permissions: [read, draft, mark_read]
    - match: "user:123456789"
      permissions: [read, send]
    - match: "channel:2225853048"
      permissions: [read, mark_read]

limits:
  max_messages_per_request: 50
  max_dialogs_per_request: 100
  rate:
    requests_per_second: 2.0
    burst: 3

logging:
  level: info
```

Environment variables in `${...}` syntax are expanded at load time.

### Client configuration

The server communicates over stdio — your MCP client starts and manages the process.

**Claude Code** (CLI — add via command):

```bash
claude mcp add telegram -- /path/to/mcp-telegram serve --config /path/to/config.yaml
```

**Claude Desktop / Claude Code** (`~/.claude.json` or `claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "telegram": {
      "command": "/path/to/mcp-telegram",
      "args": ["serve", "--config", "/path/to/config.yaml"],
      "env": {
        "TG_APP_ID": "12345",
        "TG_API_HASH": "your_api_hash"
      }
    }
  }
}
```

**Cursor** (Settings > MCP Servers > Add):

```json
{
  "telegram": {
    "command": "/path/to/mcp-telegram",
    "args": ["serve", "--config", "/path/to/config.yaml"],
    "env": {
      "TG_APP_ID": "12345",
      "TG_API_HASH": "your_api_hash"
    }
  }
}
```

---

## Configuration

### ACL

The ACL is **default-deny**. Only chats explicitly listed in `acl.chats` are accessible, and only with the permissions you specify.

Supported match patterns:

| Pattern | Example | Description |
|---------|---------|-------------|
| `@username` | `@johndoe` | Match by Telegram username (case-insensitive) |
| `+phone` | `+79001234567` | Match by phone number |
| `user:ID` | `user:123456789` | Match a user by numeric ID |
| `chat:ID` | `chat:987654321` | Match a group chat by numeric ID |
| `channel:ID` | `channel:2225853048` | Match a channel or supergroup by numeric ID |

Permission types: `read`, `send`, `draft`, `mark_read`.

If the same peer matches multiple rules (e.g. via `@username` and `user:ID`), permissions are merged — they never shadow each other.

### Rate limiting

The `limits.rate` section configures a global token bucket that wraps all Telegram RPC calls:

- `requests_per_second` — sustained rate (default: 2.0)
- `burst` — maximum burst size (default: 3)

### Media download

```yaml
media:
  download: [photo, document, video, voice, audio]
  directory: ~/telegram-media
  allowed_upload_dirs:
    - ~/Documents
    - ~/Downloads
```

When configured, `tg_history` will automatically download media files to the specified directory. The `download_to` parameter can override the path, but only to `media.directory` or its subdirectories.

`allowed_upload_dirs` restricts which directories `tg_send` can read files from. File sending is disabled unless this is configured.

---

## Security

- **Default-deny ACL** — no chat is accessible unless explicitly whitelisted
- **Filesystem boundary** — `tg_send` can only read files from `allowed_upload_dirs`; `download_to` is restricted to subdirectories of `media.directory`
- **Session file permissions** — enforced to `0600` (owner-only read/write)
- **No secret logging** — API hashes, session tokens, and auth keys are never written to logs
- **No access hash exposure** — internal Telegram access hashes are stripped from all tool output
- **Rate limiting** — prevents accidental API abuse
- **Local timezone** — date filters use your system timezone, not UTC

---

## License

[MIT](LICENSE)

---

If you find this project useful, please give it a star — it helps others discover it.
