# RFC 03: Поиск и расширенные фильтры списка чатов (Трек 2C)

- **Status:** Implemented
- **Date:** 2026-11-28
- **Owner:** Группа 2 (Трек 2C)
- **Источник требований:** `docs/group2_ai_and_product.md` (Промпт 2C, ТЗ FR-19..FR-22)

## 1. Фича

Руководитель может искать диалоги по имени/username собеседника и тексту сообщений, фильтровать список по аккаунту, приоритету, статусу, сотруднику и признаку «ожидает ответа», сортировать по свежести или длительности ожидания и листать результаты постранично.

## 2. Бизнес-требования

- **Пользовательские сценарии:**
  - Руководитель открывает список чатов (`GET /chats`) и применяет любую комбинацию фильтров (`status`, `employee_id`, `account_id`, `priority`, `unanswered`, `q`), сортировку (`sort`) и пагинацию (`limit`, `offset`).
  - Без новых параметров список возвращается в порядке последнего сообщения (`last_message_at DESC, id DESC`), по умолчанию до 50 записей.
- **Роли и права:** компания определяется через `parseCompanyID` (`company_id` query, default `1` — известное ограничение проекта). Все выборки и подзапросы по контактам/сообщениям строго ограничены `accounts.company_id = companyID`.
- **Разрешённые и запрещённые действия:**
  - Все фильтры комбинируются через логическое `AND`.
  - Пустые значения query-параметров (`status=`, `employee_id=`, `account_id=`, `priority=`, `unanswered=`, `q=`, `sort=`, `limit=`, `offset=`) игнорируются как не заданные (сохраняя совместимость с `chat_adversarial_test.go`).
  - Невалидные значения любого параметра возвращают `400 Bad Request` (`domain.ErrValidation`).
  - Поиск по `q` выполняется без учёта регистра с экранированием спецсимволов `LIKE` (`\`, `%`, `_`).
- **Граничные случаи:**
  - Нет совпадений → `200 OK` и строго `[]` (никогда `null`).
  - `account_id` чужой компании → `200 OK` и `[]`.
  - `limit` вне диапазона `1..100` (кроме `0` в Go-структуре, который нормализуется в default `50`; явный query `limit=0` или `limit=-1` или `limit=101` → `400`).
  - `offset < 0` → `400`.
  - `unanswered` допускает только `"true"` или `"false"` (иное непустое значение → `400`).
  - `sort` допускает только `"last_message"` (default) или `"awaiting_reply"` (иное непустое значение → `400`).
- **Non-goals:** AI-саммаризация (2A), автостатусы и алерты (2B), Telegram-синхронизация (Группа 1), новые миграции БД, изменения `cmd/`, `internal/config/`, `go.mod`, `Dockerfile`.
- **Наблюдаемые критерии приёмки:**
  1. `GET /chats` без параметров возвращает JSON-массив (`[]` если пусто), отсортированный по `last_message_at DESC, id DESC`, не более 50 элементов.
  2. `?q=<text>` ищет без учёта регистра по `contacts.name`, `contacts.username` и тексту сообщений чата (`messages.text`), при этом `%` и `_` трактуются буквально.
  3. `?unanswered=true` возвращает только чаты с `awaiting_reply_since IS NOT NULL`.
  4. `?priority=low|normal|high` фильтрует по приоритету; иное значение → `400`.
  5. `?account_id=<pos int>` фильтрует по аккаунту; `<= 0` или не число → `400`.
  6. `?sort=last_message` (default) сортирует по `last_message_at DESC, id DESC`; `?sort=awaiting_reply` сортирует по `awaiting_reply_since ASC NULLS LAST, id DESC`; иное значение → `400`.
  7. `?limit=1..100` и `?offset>=0` пагинируют стабильно; нарушение границ → `400`.
  8. Существующие тесты `internal/handler/handler_test.go`, `internal/handler/chat_adversarial_test.go`, `internal/storage/postgres/chat_contact_test.go` и `internal/usecase/chat_test.go` остаются зелёными.

## 3. Технические требования

- **Идентификаторы и источники:** `companyID` из `parseCompanyID(c)`; фильтры из query-параметров `GET /chats`.
- **Company scope:** `ChatUsecase.ListByFilter` проверяет `filter.CompanyID > 0`; `ChatStorage.ListByFilter` всегда фильтрует `WHERE a.company_id = $1` и джойнит `contacts ct ON c.contact_id = ct.id AND ct.company_id = a.company_id`.
- **Затрагиваемые слои (Zone Impact Map):**
  - **In-Scope Zones (порядок выполнения):**
    1. `zone-domain-migrations` — `internal/domain/chat.go` (расширение `ChatFilter`, константы сортировки `ChatSortLastMessage`, `ChatSortAwaitingReply`). Миграции не требуются.
    2. `zone-storage-postgres` — `internal/storage/postgres/chat.go` (`ListByFilter` с параметризованным SQL и экранированием `ILIKE`), `internal/storage/postgres/chat_contact_test.go` (unit-тесты `ListByFilter` на `mock_pg`). `ListByCompany` сохраняется без изменений.
    3. `zone-usecase` — `internal/usecase/interfaces.go` (добавление `ListByFilter` в `ChatRepository` при сохранении `ListByCompany`), `internal/usecase/chat.go` (`ListByFilter` с валидацией всех полей и дефолтами), `internal/usecase/chat_test.go` (обновление `mockChatRepo` и тесты `ListByFilter`).
    4. `zone-handler-api` — `internal/handler/handler.go` (обновление `ListChats` для всех query-параметров в mock- и usecase-ветках), `internal/handler/handler_test.go` (добавление `ListByFilter` в `handlerMockChatRepo`), `internal/handler/chat_filter_test.go` (новые тесты всех параметров и граничных случаев).
  - **Out of Scope (Do Not Touch):** `cmd/`, `internal/config/`, `internal/telegram/`, `internal/ai/`, `migrations/`, `go.mod`, `go.sum`, `Dockerfile`, `docker-compose.yml`. Integration blockers отсутствуют.
- **Зависимости и интерфейсы:** `handler -> usecase -> domain`, `storage/postgres -> domain` (реализует `usecase.ChatRepository`).
- **Ошибки:** невалидные параметры → `domain.ErrValidation` → HTTP `400 Bad Request`; ошибки БД → обёрнутая ошибка / `mapPgError` → HTTP `500`.
- **Транзакционность:** не требуется (read-only `SELECT`).
- **Идемпотентность:** `GET /chats` идемпотентен.
- **Фоновые задачи:** не требуются.
- **Таймауты и внешние отказы:** `ctx` пробрасывается из HTTP-запроса в `usecase` и `sqlx.SelectContext`.
- **Открытые вопросы:** нет.
- **Ожидаемые проверки:** `gofmt -l`, `go vet ./...`, `go test -race ./...` (или `go test -count=1 ./...` при отсутствии cgo на Windows с явной фиксацией), `go build ./...`.

## 4. Контракты

- **Метод и путь:** `GET /chats`
- **Query-параметры:**
  - `company_id`: positive int, optional, default `1` (невалидное → `400`)
  - `status`: `new_lead|negotiation|waiting_reply|interested|unhappy|lost|action_required`, optional (невалидное → `400`)
  - `employee_id`: positive int, optional (невалидное → `400`)
  - `account_id`: positive int, optional (невалидное → `400`)
  - `priority`: `low|normal|high`, optional (невалидное → `400`)
  - `unanswered`: `true|false`, optional, default `false` (невалидное → `400`)
  - `q`: string, optional (trim пробелов; пустое после trim игнорируется)
  - `sort`: `last_message|awaiting_reply`, optional, default `last_message` (невалидное → `400`)
  - `limit`: int `1..100`, optional, default `50` (невалидное → `400`)
  - `offset`: int `>= 0`, optional, default `0` (невалидное → `400`)
- **Response body:** JSON-массив `[]domain.Chat` (никогда `null`, пустой результат — `[]`).
- **Success status:** `200 OK`.
- **Ошибки:** `400 Bad Request` (`{"error": "..."}`) при нарушении валидации; `500 Internal Server Error` при сбое хранилища.
- **Совместимость:** сохранена 100% обратная совместимость с существующими клиентами и тестами (`handler_test.go`, `chat_adversarial_test.go`).

## 5. Схема БД

- **Используемые существующие таблицы:**
  - `chats` (`id`, `account_id`, `contact_id`, `status`, `status_source`, `priority`, `last_message_at`, `awaiting_reply_since`, `unread_count`)
  - `accounts` (`id`, `company_id`, `employee_id`)
  - `contacts` (`id`, `company_id`, `name`, `username`)
  - `messages` (`id`, `chat_id`, `text`)
- **Новые таблицы и миграции:** `migrations/000010_chat_search_trgm_indexes.{up,down}.sql` (расширение `pg_trgm`, GIN-индексы по `contacts.name`, `contacts.username`, `messages.text` и частичный индекс по `chats.awaiting_reply_since WHERE awaiting_reply_since IS NOT NULL`; номер `000010` выбран без конфликта с `000009` трека 2B).
- **Company scope и связи:** `chats.account_id -> accounts.id` (`a.company_id = $1`), `chats.contact_id -> contacts.id` (`ct.company_id = a.company_id`), `messages.chat_id -> chats.id`.
- **Существующие данные:** не затрагиваются.
