# mcp-telegram

[![MCP Server](https://img.shields.io/badge/MCP-Server-blue)](https://modelcontextprotocol.io)
[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

An MCP (Model Context Protocol) server that connects AI assistants like Claude to your **real Telegram account** via the User API (MTProto). Not a bot — Claude reads and sends messages as you.

Built with [gotd/td](https://github.com/gotd/td) and the official [MCP Go SDK](https://github.com/modelcontextprotocol/go-sdk).

> **Telegram API Terms of Service**: This project uses the Telegram User API. You must obtain your own `api_id` and `api_hash` from [my.telegram.org](https://my.telegram.org) and comply with the [Telegram API Terms of Service](https://core.telegram.org/api/terms). Misuse of the User API (spam, bulk messaging, scraping) may result in your account being banned. You are solely responsible for how you use this tool.

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

### Build

```bash
go install github.com/Prgebish/mcp-telegram/cmd/mcp-telegram@latest
```

Or build from source:

```bash
git clone https://github.com/Prgebish/mcp-telegram.git
cd mcp-telegram
go build -o mcp-telegram ./cmd/mcp-telegram
```

On Windows, the binary will be named `mcp-telegram.exe`.

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

### Connect to your MCP client

Add the server to your Claude Code config (`~/.claude.json`) or Claude Desktop config:

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

The server starts in stdio mode — the MCP client manages its lifecycle.

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
```

When configured, `tg_history` will automatically download media files to the specified directory. The `download_to` parameter in `tg_history` can override this per-request.

---

## Security

- **Default-deny ACL** — no chat is accessible unless explicitly whitelisted
- **Session file permissions** — created with mode `0600` (owner-only read/write)
- **No secret logging** — API hashes, session tokens, and auth keys are never written to logs
- **No access hash exposure** — internal Telegram access hashes are stripped from all tool output
- **Rate limiting** — prevents accidental API abuse

---

## License

[MIT](LICENSE)

---

If you find this project useful, please give it a star — it helps others discover it.
