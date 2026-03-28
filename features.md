# Features: mcp-telegram

## Конфигурация

### YAML-конфиг с env expansion
**Description:** Конфигурация загружается из YAML-файла. Переменные окружения раскрываются через `${VAR}`.

**Example:**
```yaml
telegram:
  app_id: ${TG_APP_ID}       # → значение из env
  api_hash: ${TG_API_HASH}
```

### Валидация конфига
**Description:** При загрузке проверяются обязательные поля, допустимые значения и форматы.

**Example — невалидный match:**
```yaml
acl:
  chats:
    - match: "justtext"    # ошибка: должен начинаться с @, +, user:, chat:, channel:
```
```
Result: error "acl.chats[0].match "justtext": must start with @, +, or be user:/chat:/channel: prefixed"
```

**Example — неизвестное право:**
```yaml
- match: "@test"
  permissions: [read, delete]    # ошибка: delete не существует
```
```
Result: error "acl.chats[0].permissions: unknown permission "delete""
```

### Дефолты
**Description:** Необязательные поля имеют разумные значения по умолчанию.

| Параметр | Дефолт |
|----------|--------|
| session_path | ~/.config/mcp-telegram/session.json |
| max_messages_per_request | 50 |
| max_dialogs_per_request | 100 |
| requests_per_second | 2.0 |
| burst | 3 |
| logging.level | info |

## ACL (Access Control List)

### Белый список чатов
**Description:** Только явно разрешённые чаты доступны AI. Всё остальное — запрещено (default-deny).

### Типизированные матчеры
**Description:** Чаты адресуются с учётом типа, что исключает коллизии ID.

**Форматы:**
```
@username       — по username (case-insensitive)
+79001234567    — по номеру телефона
user:123456789  — пользователь по ID
chat:123456789  — групповой чат по ID
channel:123456789 — канал по ID
```

**Example — нет коллизии при одинаковых числовых ID:**
```yaml
- match: "user:100"
  permissions: [read]
- match: "chat:100"
  permissions: [draft]
```
user:100 имеет `read`, chat:100 имеет `draft` — не путаются.

### Гранулярные права с объединением
**Description:** Каждому чату можно назначить комбинацию из трёх прав. Если один peer описан несколькими правилами (по @username, +phone, user:ID), права объединяются — не затеняют друг друга.

| Право | Что разрешает |
|-------|---------------|
| read | Чтение истории сообщений (tg_history) |
| draft | Сохранение черновика (tg_draft) |
| mark_read | Пометка прочитанным (tg_mark_read) |

**Example — объединение прав:**
```yaml
- match: "@alice"
  permissions: [read]
- match: "user:100"
  permissions: [draft]
```
Peer alice (user:100) получает и `read`, и `draft` — правила объединяются.

## MCP-инструменты

### tg_me
**Description:** Информация о текущем Telegram-аккаунте. Не требует ACL.

**Example:**
```
Input: (нет параметров)
Output:
  ID: 123456789
  First name: Alice
  Last name: Smith
  Username: @alice
```

### tg_dialogs
**Description:** Список диалогов. Показывает только чаты из белого списка.

**Example:**
```
Input: {"only_unread": true}
Output:
  - Alice Smith (@alice) [user] unread:3
  - Dev Team (chat:4626931529) [chat] unread:12
```

**Edge case — нет совпадений:**
```
Input: {}
Output: "No matching dialogs found."
```

### tg_history
**Description:** История сообщений чата. Требует право `read`.

**Example:**
```
Input: {"chat": "@alice", "limit": 3}
Output:
  [1234] Alice Smith (2026-03-28 10:30): Привет!
  [1233] Bob (2026-03-28 10:29): [photo]
  [1232] Alice Smith (2026-03-28 10:28): Как дела?

  [Use offset_id=1232 to load older messages]
```

**Edge case — ACL-отказ:**
```
Input: {"chat": "@secret_chat"}
Output: error "access denied: @secret_chat does not have 'read' permission"
```

**Edge case — пагинация:**
```
Input: {"chat": "@alice", "offset_id": 1232, "limit": 50}
Output: (следующие 50 сообщений)
```

### tg_draft
**Description:** Сохраняет черновик сообщения. НЕ отправляет. Требует право `draft`.

**Example:**
```
Input: {"chat": "@alice", "text": "Напоминание: встреча в 15:00"}
Output: "Draft saved in @alice"
```

### tg_mark_read
**Description:** Помечает все сообщения в чате как прочитанные. Требует право `mark_read`.

**Example:**
```
Input: {"chat": "channel:2225853048"}
Output: "Marked as read: channel:2225853048"
```

## Безопасность

### Секреты не логируются
api_hash, телефоны, содержимое сообщений никогда не попадают в логи.

### Access hash не утекает
Ссылки на peer всегда в формате `@username` или `type:ID`, без access hash.

### Безопасные права файлов
- Директория сессии: 0700
- Файл сессии: 0600
- Лог-файл: 0600

## Rate Limiting

Один глобальный token bucket, реализованный как `telegram.Middleware`. Каждый RPC-вызов к Telegram API (включая внутренние вызовы итераторов, `ResolvePeer`, batch-запросы) проходит через rate limiter. Лимитирование происходит на уровне транспорта, а не на уровне handler.

## Not Yet Implemented
- CLI-команда для первичной аутентификации (auth flow)
- Тесты для tool handlers (с моками)
- Тесты для ratelimit
- goreleaser / Homebrew / npm дистрибуция
