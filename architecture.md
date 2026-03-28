# Architecture: mcp-telegram

## Overview
MCP-сервер на Go, предоставляющий Claude Code контролируемый доступ к Telegram через User API (MTProto). Работает локально, общается с Claude Code через stdio, подключается напрямую к Telegram.

```
Claude Code  ←stdio→  mcp-telegram  ←MTProto→  Telegram API
```

## File Structure
```
mcp-telegram/
├── cmd/mcp-telegram/
│   └── main.go              # точка входа, wiring, signal handling
├── internal/
│   ├── config/
│   │   ├── config.go         # YAML парсинг, валидация, env expansion
│   │   └── config_test.go
│   ├── acl/
│   │   ├── acl.go            # ACL-движок, компиляция правил
│   │   └── acl_test.go
│   ├── telegram/
│   │   ├── client.go         # долгоживущий Telegram-клиент
│   │   └── peers.go          # peer resolution, форматирование
│   ├── tools/
│   │   ├── tools.go          # регистрация, общие типы (Deps)
│   │   ├── me.go             # tg_me
│   │   ├── dialogs.go        # tg_dialogs
│   │   ├── history.go        # tg_history
│   │   ├── draft.go          # tg_draft
│   │   └── markread.go       # tg_mark_read
│   └── ratelimit/
│       └── ratelimit.go      # обёртка над rate.Limiter
├── config.example.yaml
├── go.mod
└── go.sum
```

## Modules

### config
- Purpose: загрузка и валидация YAML-конфига
- Dependencies: gopkg.in/yaml.v3
- Interface:
  - `Load(path string) (*Config, error)` — читает, расширяет env vars, валидирует
- Key types: Config, TelegramConfig, ACLConfig, ChatRule, Permission, LimitsConfig, RateConfig, LoggingConfig

### acl
- Purpose: проверка прав доступа к чатам
- Dependencies: config (типы Permission, ChatRule)
- Interface:
  - `NewChecker(cfg ACLConfig) (*Checker, error)` — компилирует правила
  - `Checker.Allowed(peer PeerIdentity, perm Permission) bool` — проверка конкретного права
  - `Checker.MatchesAny(peer PeerIdentity) bool` — есть ли хоть одно правило для peer
- Key types: PeerKind (User/Chat/Channel), PeerIdentity (Kind + ID + Username + Phone)
- Матчинг: `@username` (case-insensitive), `+phone`, `user:ID`, `chat:ID`, `channel:ID`
- Default-deny: всё, что не в белом списке — запрещено

### telegram
- Purpose: долгоживущий Telegram-клиент и peer resolution
- Dependencies: gotd/td (telegram, peers, tg, session), acl
- Interface:
  - `New(cfg TelegramConfig) *Client`
  - `Client.Start(ctx) error` — запускает Run() в горутине, ждёт ready
  - `Client.Stop()` — graceful shutdown
  - `Client.API() *tg.Client` — raw Telegram API
  - `Client.Peers() *peers.Manager` — peer manager
  - `Client.ResolvePeerForTool(ctx, ref) (peers.Peer, PeerIdentity, error)` — резолвинг по строке
  - `FormatPeerRef(peer) string` — безопасная ссылка (без access hash)
  - `FormatPeerName(peer) string` — отображаемое имя
  - `PeerTypeName(peer) string` — тип (user/bot/chat/channel/supergroup)

### tools
- Purpose: MCP-инструменты для Telegram
- Dependencies: telegram, acl, ratelimit, config, MCP SDK
- Interface:
  - `Register(server *mcp.Server, deps *Deps)` — регистрирует все 5 инструментов
- Key type: `Deps` — общие зависимости (Client, ACL, Limiter, Limits)
- Каждый handler: limiter.Wait → resolve peer → ACL check → API call → format response
- Ошибки ACL возвращаются как `CallToolResult{IsError: true}`, не Go error

### ratelimit
- Purpose: ограничение частоты запросов к Telegram API
- Dependencies: golang.org/x/time/rate
- Interface:
  - `New(cfg RateConfig) *Limiter`
  - `Limiter.Wait(ctx) error`

## Data Flow

```
MCP request (stdin)
  → mcp.Server dispatches to tool handler
    → limiter.Wait(ctx)
    → client.ResolvePeerForTool(ref)
    → acl.Allowed(identity, permission)
    → client.API().SomeTelegramMethod(...)
    → format response (без access hash, без секретов)
  → mcp.CallToolResult (stdout)
```

## Client Lifecycle

```
main() → New(cfg) → Start(ctx):
  goroutine: telegram.Client.Run(ctx, callback)
    callback: init peers.Manager → close(ready) → block on ctx.Done()
  main goroutine: wait on <-ready or error
  → client ready, MCP server starts

shutdown:
  signal/MCP disconnect → cancel ctx → Run returns → Stop() returns
```

## Security Model
- Session file: directory 0700, file 0600
- Log file: 0600
- Never log: api_hash, phone numbers, message content
- Never expose: access hashes in any output
- ACL: программное ограничение (сессия имеет полные права, ограничение в коде)

## Key Design Decisions
- peers.Manager вместо ручного управления access hash: устраняет баги оригинала
- Типизированные peer-ссылки (user:ID, chat:ID, channel:ID): нет коллизий ID
- Один глобальный token bucket: Telegram считает flood на уровне аккаунта
- text output (не JSON): проще для LLM-потребления
- CallToolResult{IsError: true} для ACL-отказов: AI может понять и адаптироваться
