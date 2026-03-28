# Task Plan: MCP-сервер для Telegram

## Goal
Надёжный MCP-сервер на Go, дающий Claude Code контролируемый доступ к Telegram через User API с ACL, rate limiting и безопасным логированием.

## Phases

- [x] Phase 1: Проектирование
  - [x] 1.1 Анализ существующего chaindead/telegram-mcp
  - [x] 1.2 Выбор языка и библиотек (Go, gotd/td, official MCP SDK)
  - [x] 1.3 Проектирование архитектуры (ACL, client lifecycle, tools)

- [x] Phase 2: Базовая реализация
  - [x] 2.1 Структура проекта, go.mod, зависимости
  - [x] 2.2 Config — парсинг YAML, валидация, env expansion
  - [x] 2.3 ACL — движок с типизированными матчерами
  - [x] 2.4 Rate limiter — обёртка над golang.org/x/time/rate
  - [x] 2.5 Telegram client — долгоживущий клиент с peers.Manager
  - [x] 2.6 Peer resolution — ResolvePeerForTool, FormatPeerRef, FormatPeerName
  - [x] 2.7 MCP tools — tg_me, tg_dialogs, tg_history, tg_draft, tg_mark_read
  - [x] 2.8 main.go — wiring, signal handling, graceful shutdown

- [x] Phase 3: Тесты (базовые)
  - [x] 3.1 ACL тесты — матчеры, default-deny, гранулярность прав, коллизии ID
  - [x] 3.2 Config тесты — парсинг, валидация, дефолты, env expansion

- [ ] Phase 4: Ревью и доработка
  - [ ] 4.1 Ревью кода другой нейросетью
  - [ ] 4.2 Доработка по результатам ревью
  - [ ] 4.3 Дополнительные тесты (ratelimit, tool handlers с моками)

- [ ] Phase 5: Интеграция и ручное тестирование
  - [ ] 5.1 Аутентификация с реальным Telegram-аккаунтом
  - [ ] 5.2 Проверка tg_me, tg_dialogs, tg_history
  - [ ] 5.3 Проверка ACL-отказов
  - [ ] 5.4 Проверка tg_draft, tg_mark_read
  - [ ] 5.5 Подключение к Claude Code как MCP-сервер

## Blocked / Open Questions
- [ ] Нужна ли команда auth (отдельный CLI-флоу для первичной аутентификации)?
- [ ] Формат вывода tg_history — plain text или structured JSON?

## Decisions Made
- Go (gotd/td + official MCP SDK): лучший баланс для локального stateful-сервера
- User API вместо Bot API: нужна полная история чатов
- ACL белый список: default-deny, типизированные матчеры (@, +, user:, chat:, channel:)
- Один глобальный rate limiter: Telegram считает flood на уровне аккаунта
- Один долгоживущий клиент: не создавать соединение на каждый вызов
- log/slog из stdlib: минимум зависимостей
- ACL-отказ как CallToolResult{IsError: true}: AI может понять и адаптироваться

## Status
**Phase 4.1** — Ожидание ревью другой нейросетью

## Files
- `task_plan.md` — this file
- `architecture.md` — solution structure
- `features.md` — implemented behavior with examples
