# mcp-telegram

MCP-сервер на Go для интеграции Claude Code с Telegram через User API (MTProto). Использует gotd/td и официальный MCP Go SDK.

## Architecture

- `cmd/mcp-telegram/main.go` — точка входа
- `internal/config` — YAML-конфиг
- `internal/acl` — ACL-движок (белый список чатов, гранулярные права)
- `internal/telegram` — долгоживущий Telegram-клиент, peer resolution через peers.Manager
- `internal/tools` — 5 MCP-инструментов (tg_me, tg_dialogs, tg_history, tg_draft, tg_mark_read)
- `internal/ratelimit` — глобальный rate limiter

## Verification Commands

After making code changes, run:

```bash
# Build
go build ./...

# Test
go test ./...

# Vet
go vet ./...
```

## Planning Files

- `task_plan.md` — phases and current status
- `architecture.md` — detailed module design
- `features.md` — **ALWAYS CHECK** for implemented behavior and examples
