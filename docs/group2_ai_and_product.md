# Группа 2 (3 человека): AI-саммаризация, умные статусы/алерты и поиск по чатам

## Цель группы
Реализовать интеллектуальный слой продукта и ключевые пользовательские фичи MVP из ТЗ (`docs/telecorp-spec.docx`):
1. Генерацию структурированных AI-саммари по каждому чату через внешний OpenAI-совместимый API.
2. Автоматическую расстановку статусов диалогов (без затирания ручных статусов) и генерацию алертов (`complaint`, `refusal`, `negative`, `urgent`, а также просроченных `no_reply` по `companies.no_reply_threshold`).
3. Полноценный поиск и фильтрацию чатов (`?q=`, `?unanswered=true`, `?priority=`, `?account_id=`, сортировка и пагинация), чтобы руководитель мог находить проблемные диалоги за секунды.

---

## Разделение работы внутри группы (3 разработчика)

Работа разделена на три независимых трека, чтобы свести конфликты при мерже к минимуму:

| Роль | Зона ответственности | Основные файлы |
| :--- | :--- | :--- |
| **Участник 2A — AI-клиент и генерация саммари** | Секция `AI` в конфиге, абстрактный интерфейс `ChatSummarizer`, HTTP-адаптер к OpenAI-совместимому API (`internal/ai/`), строгий JSON-парсинг и валидация `Risk`, кэширование по `LastMessageID`, неблокирующая очередь пересчёта, ручка `POST /chats/:id/summary/generate`, тесты. | `internal/config/config.go`, `internal/ai/*`, `internal/usecase/interfaces.go` (`ChatSummarizer`), `internal/usecase/chat_summary.go`, `internal/usecase/chat_summary_test.go`, `internal/handler/handler.go` |
| **Участник 2B — Автостатусы и умные алерты** | Обновление автостатуса чата по результату AI (только если `status_source != manual`), создание алертов по флагам AI без дублей активных сигналов, фоновая проверка просроченных диалогов (`CheckNoReplyAlerts` по `companies.no_reply_threshold`) с graceful shutdown, тесты. | `internal/storage/postgres/chat.go` (`UpdateAutoStatus`), `internal/storage/postgres/alert.go` (дедупликация активных алертов), `internal/usecase/alert.go`, `internal/usecase/alert_test.go`, `cmd/app/main.go` |
| **Участник 2C — Поиск и расширенные фильтры чатов** | Расширение `domain.ChatFilter`, обновление `ChatStorage.ListByCompany` / `ListByFilter` (поиск `?q=` по контакту и тексту сообщений, фильтры `?unanswered=true`, `?priority=`, `?account_id=`, сортировка по времени ожидания, пагинация `limit`/`offset`), разбор фильтров в `GET /chats`, тесты. | `internal/domain/chat.go`, `internal/storage/postgres/chat.go` (`ListByFilter`), `internal/usecase/chat.go`, `internal/usecase/chat_test.go`, `internal/handler/handler.go` (`ListChats`), `internal/handler/handler_test.go` |

### Точки синхронизации с другими группами
- **Не трогать:** `internal/telegram/` и `internal/usecase/sync.go` — это зона Группы 1.
- **Стыковка с Группой 1:** Участник 2A предоставляет неблокирующий метод `Enqueue(companyID, chatID)` в `ChatSummaryUsecase`, который в `cmd/app/main.go` передаётся как коллбек `OnChatUpdated` в `SyncUsecase` Группы 1.
- **Стыковка внутри Группы 2:** Участники 2A и 2B согласуют структуру `AIAnalysisResult` в `internal/usecase/interfaces.go`, чтобы после генерации саммари вызывались `UpdateAutoStatus` и создание алертов. Участники 2B и 2C правят разные методы `internal/storage/postgres/chat.go` (`UpdateAutoStatus` и `ListByFilter`), поэтому лучше мержить их последовательно.
- **Номер миграции:** если потребуется миграция (индекс под поиск по тексту или частичный уникальный индекс активных `no_reply`-алертов), согласуйте порядковый номер (`000009` / `000010`) с Группой 1.

---

## План реализации по шагам (сначала короткий RFC)

Перед написанием кода зафиксируйте в `docs/rfc/02-ai-summary-and-search.md` 5 блоков по методике курса:
1. **Фича:** автоматическая выжимка диалогов с оценкой риска, автостатусами и сигналами о проблемах, плюс быстрый поиск и фильтры по списку чатов.
2. **Бизнес-требования:** ручной статус (`status_source = manual`) не перезаписывается автоматикой; если новых сообщений после `LastMessageID` нет, токены не тратятся; без API-ключа сервер работает штатно, отключая только AI-генерацию.
3. **Технические требования:** OpenAI-совместимый клиент на стандартном `net/http`, жёсткий таймаут, обрезка контекста по лимиту сообщений/символов, неблокирующая очередь задач, единый `company_id` scope.
4. **Контракты:**
   - `POST /chats/:id/summary/generate` (опционально `?force=true`) — запуск AI-анализа чата и возврат свежего `ChatSummary`.
   - `GET /chats?status=...&employee_id=...&account_id=...&priority=...&unanswered=true&q=...&sort=...&limit=50&offset=0` — расширенный список чатов.
5. **Схема БД:** переиспользуются `chat_summaries`, `chats`, `messages`, `alerts`, `companies`; при необходимости добавляются индексы под поиск и уникальность активных алертов.

---

## Готовые промпты для кодинг-агентов (Группа 2)

### Промпт 2A — AI-клиент, генерация саммари и очередь

```text
Изучи доменные модели ChatSummary (internal/domain/chat_summary.go), Message (internal/domain/message.go), Chat (internal/domain/chat.go) и Alert (internal/domain/alert.go), а также internal/usecase/chat_summary.go, internal/usecase/interfaces.go, internal/config/config.go, internal/handler/handler.go и cmd/app/main.go.

Нужно спроектировать и внедрить модуль AI-саммаризации переписок по каждому чату через внешний OpenAI-совместимый HTTP API, сохраняя чистую архитектуру проекта:

1. Конфигурация (internal/config/config.go):
   - Добавь секцию AIConfig: AI_API_BASE_URL (по умолчанию https://api.openai.com/v1), AI_API_KEY, AI_MODEL, AI_TIMEOUT (по умолчанию 30s), AI_MAX_MESSAGES (по умолчанию 50) и AI_MAX_CHARS (по умолчанию 12000).
   - Если AI_API_KEY не задан, сервер НЕ должен падать при старте — модуль саммаризации отключается с понятным логом, а ручной вызов генерации возвращает 503 Service Unavailable.

2. Интерфейс в usecase (internal/usecase/interfaces.go):
   - Определи абстрактный интерфейс ChatSummarizer с методом AnalyzeChat(ctx, input) (*AIAnalysisResult, error).
   - Вход: история сообщений (хронологически) и текущее саммари (если оно уже существует).
   - Результат (AIAnalysisResult): поля для domain.ChatSummary (Topic, Agreements, NextStep, OpenQuestions, Risk: low|medium|high), рекомендуемый авто-статус диалога (SuggestedStatus *domain.ChatStatus) и список флагов для алертов (AlertTypes []domain.AlertType из complaint, refusal, negative, urgent).

3. Адаптер внешнего API (internal/ai/):
   - Реализуй OpenAI-совместимый клиент на стандартном net/http с таймаутом и поддержкой context.Context.
   - Перед отправкой форматируй сообщения в хронологическом порядке с явной разметкой роли («Клиент» для in, «Менеджер» для out) и временем отправки; обрезай старую часть истории по лимитам AI_MAX_MESSAGES и AI_MAX_CHARS. Не передавай номера телефонов.
   - Системный промпт должен требовать строгий JSON по заданной схеме без пояснений и markdown-обёрток (с безопасной очисткой ```json ... ``` на всякий случай).
   - Валидируй Risk (low/medium/high), SuggestedStatus и AlertTypes; при 429, сетевых ошибках, таймауте или битом JSON возвращай ошибку без паники и без сохранения мусора в БД.

4. Бизнес-логика и асинхронная очередь (internal/usecase/chat_summary.go):
   - Добавь метод Generate(ctx, companyID, chatID int64, force bool) (*domain.ChatSummary, error):
     a) Проверяет существование чата и принадлежность companyID.
     b) Загружает последние сообщения чата; если сообщений нет — возвращает domain.ErrValidation.
     c) Сравнивает ID последнего сообщения с текущим ChatSummary.LastMessageID: если новых сообщений нет и force == false — возвращает существующее саммари без вызова LLM API.
     d) Вызывает ChatSummarizer.AnalyzeChat, сохраняет результат через ChatSummaryRepository.Upsert (с фиксацией LastMessageID и GeneratedAt = UTC) и вызывает опциональный коллбек/обработчик для автостатусов и алертов (зона Участника 2B).
   - Реализуй неблокирующую очередь/воркер (buffered channel + горутина с graceful shutdown в cmd/app/main.go) с методом Enqueue(companyID, chatID), чтобы пересчёт саммари после синхронизации не тормозил чтение Telegram.

5. HTTP API и тесты:
   - Сохрани существующие ручки GET /chats/:id/summary и ручной POST /chats/:id/summary, а генерацию повесь на POST /chats/:id/summary/generate (с поддержкой ?force=true).
   - Напиши unit-тесты на internal/ai (httptest.Server: валидный JSON, битый JSON, 429, таймаут) и на ChatSummaryUsecase.Generate (кэширование по LastMessageID, force=true, изоляция по companyID). Не меняй версии в go.mod и Dockerfile.
```

### Промпт 2B — Автостатусы чатов и умные алерты

```text
Изучи internal/domain/chat.go, internal/domain/alert.go, internal/domain/company.go, internal/storage/postgres/chat.go, internal/storage/postgres/alert.go, internal/usecase/alert.go, internal/usecase/chat_summary.go и cmd/app/main.go.

Нужно реализовать автоматическую простановку статусов диалогов и систему генерации алертов (по ТЗ FR-30, FR-31, FR-32, FR-33), не трогая фильтрацию/поиск в ListByCompany (зона Участника 2C):

1. Автостатусы с защитой ручного выбора:
   - Добавь в ChatRepository и ChatStorage метод UpdateAutoStatus(ctx, chatID int64, status domain.ChatStatus) error, который обновляет status = $1, status_source = 'auto' строго при условии WHERE id = $2 AND status_source != 'manual'. Если статус был выставлен вручную — обновление пропускается без ошибки.
   - Подключи применение SuggestedStatus из AIAnalysisResult после генерации саммари.

2. Дедупликация алертов и AI-сигналы:
   - Добавь защиту от создания дублирующегося активного алерта того же типа для того же чата (WHERE chat_id = $1 AND type = $2 AND resolved_at IS NULL — через проверку в AlertStorage/AlertUsecase и, при необходимости, частичный уникальный индекс в новой миграции).
   - Подключи создание алертов типов complaint, refusal, negative, urgent на основе флагов AlertTypes из AIAnalysisResult.

3. Фоновая проверка просроченных диалогов (no_reply):
   - Реализуй в AlertUsecase (или отдельном сервисе) метод CheckNoReplyAlerts(ctx context.Context) (int, error), который находит чаты, где awaiting_reply_since IS NOT NULL и NOW() - awaiting_reply_since > (companies.no_reply_threshold / 1000000000.0) * INTERVAL '1 second', и для которых ещё нет активного no_reply-алерта, и создаёт для них алерт AlertTypeNoReply.
   - Подключи периодический фоновый запуск CheckNoReplyAlerts в cmd/app/main.go с настраиваемым интервалом и остановкой через контекст graceful shutdown.

4. Тесты и проверки:
   - Покрой unit-тестами защиту manual-статуса от перезаписи, дедупликацию активных алертов и логику CheckNoReplyAlerts.
   - Проверь gofmt -w ., go vet ./..., go test -race ./... и go build ./....
```

### Промпт 2C — Поиск и расширенные фильтры списка чатов

```text
Изучи internal/domain/chat.go (включая существующую структуру ChatFilter), internal/storage/postgres/chat.go, internal/usecase/chat.go, internal/handler/handler.go (ListChats) и тесты в internal/handler/handler_test.go и internal/handler/chat_adversarial_test.go.

Нужно реализовать расширенный поиск, фильтрацию, сортировку и пагинацию для списка чатов GET /chats (по ТЗ FR-19, FR-20, FR-21, FR-22), сохранив 100% обратную совместимость со всеми существующими тестами:

1. Доменная модель и интерфейс (internal/domain/chat.go, internal/usecase/interfaces.go):
   - Расширь domain.ChatFilter полями: CompanyID, EmployeeID (*int64), AccountID (*int64), Status (*domain.ChatStatus), Priority (*domain.ChatPriority), UnansweredOnly (bool), Query (string), SortBy (string: "last_message" по умолчанию или "awaiting_reply"), Limit (int), Offset (int).
   - Добавь метод ListByFilter(ctx context.Context, filter domain.ChatFilter) ([]domain.Chat, error) в ChatRepository (сохранив существующий ListByCompany или делегировав его в ListByFilter, чтобы не сломать существующие тесты и моки).

2. SQL-реализация (internal/storage/postgres/chat.go):
   - Реализуй ListByFilter с безопасными параметризованными условиями:
     - обязательная изоляция по a.company_id;
     - опциональные фильтры по c.status, a.employee_id, c.account_id, c.priority;
     - при UnansweredOnly == true: c.awaiting_reply_since IS NOT NULL;
     - при непустом Query: поиск без учёта регистра (ILIKE) по contacts.name, contacts.username ИЛИ по тексту сообщений диалога через EXISTS (SELECT 1 FROM messages m WHERE m.chat_id = c.id AND m.text ILIKE ...);
     - сортировка: по умолчанию c.last_message_at DESC, c.id DESC; при SortBy == "awaiting_reply" — c.awaiting_reply_since ASC NULLS LAST, c.id DESC;
     - пагинация LIMIT / OFFSET (limit от 1 до 100, offset >= 0).

3. Usecase и HTTP-хендлер (internal/usecase/chat.go, internal/handler/handler.go):
   - В ChatUsecase добавь валидацию всех полей ChatFilter (status, priority, account_id > 0, employee_id > 0, sort в допустимых значениях, limit 1..100, offset >= 0).
   - Обнови Handler.ListChats для разбора новых query-параметров (?account_id=, ?priority=, ?unanswered=true, ?q=, ?sort=, ?limit=, ?offset=) с возвратом 400 Bad Request на невалидные значения и поддержкой мок-ветки (чтобы старые тесты handler_test.go и chat_adversarial_test.go остались зелёными).

4. Тесты и проверки:
   - Покрой unit-тестами валидацию ChatFilter, SQL-построение/параметры и все новые query-параметры в ListChats (включая граничные и невалидные значения).
   - Проверь gofmt -w ., go vet ./..., go test -race ./... и go build ./....
```

---

## Критерии приёмки (Definition of Done)
- Сервер запускается без `AI_API_KEY` без падений; при вызове `POST /chats/:id/summary/generate` без ключа отдаёт `503`.
- Повторный вызов генерации без новых сообщений (и без `force=true`) не делает HTTP-запрос к LLM и возвращает кэшированное саммари из БД.
- Если руководитель вручную выставил статус через `POST /chats/:id/status`, последующий AI-анализ этот статус не перезаписывает.
- Повторный запуск `CheckNoReplyAlerts` и AI-анализа не создаёт дублирующих активных алертов для одного и того же чата.
- `GET /chats` поддерживает поиск `?q=`, фильтры `?unanswered=true`, `?priority=`, `?account_id=`, сортировку `?sort=awaiting_reply` и пагинацию, при этом все старые тесты в `internal/handler/handler_test.go` и `chat_adversarial_test.go` остаются зелёными.
