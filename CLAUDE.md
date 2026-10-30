# mcp-telegram

MCP-сервер на Go для интеграции Claude Code с Telegram через User API (MTProto). Использует gotd/td и официальный MCP Go SDK.

## Architecture

- `cmd/mcp-telegram/main.go` — точка входа; команды `auth` (код или QR), `dialogs` (все диалоги без ACL, для выбора правил), `serve`
- `internal/config` — YAML-конфиг
- `internal/acl` — ACL-движок (белый список чатов, гранулярные права); `ParseRef` — единая грамматика ссылок на чаты для правил, pre-check и резолвера
- `internal/telegram` — долгоживущий Telegram-клиент, peer resolution через peers.Manager, обход диалогов без ACL (`ListAllDialogs`)
- `internal/tools` — 9 MCP-инструментов (tg_me, tg_dialogs, tg_history, tg_search, tg_send, tg_forward, tg_draft, tg_mark_read, tg_transcribe — последний только при `transcribe.enabled`); всё, что принимает чат, идёт через ACL (`resolveAllowed`)
- `internal/ratelimit` — глобальный rate limiter
- `deploy/` — установка под отдельным пользователем `_mcptg` на macOS (`install-macos.sh`, `run`, шаблон `config.example.yaml`)

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
