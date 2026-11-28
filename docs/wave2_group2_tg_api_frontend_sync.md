# Волна 2 — Команда 2 (3 человека): Подключение Telegram из UI и стыковка API с фронтендом

## Цель команды
Довести бизнес-API до 100% готовности MVP (`docs/telecorp-spec.docx` + `PLAN.md` готового фронтенда):
1. Реализовать HTTP-флоу подключения и отключения Telegram-аккаунтов из веб-кабинета (QR с TTL / телефон + код + 2FA, отключение без потери истории).
2. Обогатить карточку аккаунта `GET /accounts/:id` метриками активности и закрыть мелкие баги аналитики/алертов.
3. Полностью выровнять API чатов, алертов и компании под ожидания готового фронтенда (`PLAN.md`) и описать итоговый контракт в `api/openapi.yaml`.

**Важные ограничения для стыковки с фронтом (`PLAN.md`):**
- **Не менять** формат `Company.no_reply_threshold`, `DashboardStats.avg_response_time` и `AccountRating.avg_response_time` — они остаются в **наносекундах** (`int64`), фронт уже использует адаптер `нс ↔ сек/мин`.
- **Не менять** сортировку `GET /chats/:id/messages` (`sent_at ASC, id ASC`) и наличие `total_messages` в `GET /chats/:id/analytics` — на них опирается пагинация ленты сообщений на фронте.
- **Пути в корне** (без `/api/v1`), плоский формат ошибок `{"error": "..."}`.

---

## Разделение работы внутри команды (3 разработчика)

| Роль | Зона ответственности | Основные файлы |
| :--- | :--- | :--- |
| **Участник 2A — HTTP-подключение и отключение Telegram (FR-9, FR-13)** | Серверный менеджер интерактивных сессий подключения (`internal/telegram/connect.go`): старт QR-логина (выдача `qr_url` + `expires_at` / TTL) или вход по телефону, `POST /accounts/:id/confirm` (код подтверждения и/или 2FA-пароль), статус подключения (`waiting_qr` / `code_required` / `2fa_required` / `active` / `error`), отмена, reconnect, `POST /accounts/:id/disconnect` (очистка `accounts.session` и перевод в `disconnected` без удаления чатов/сообщений). | `internal/telegram/connect.go`, `internal/telegram/connect_test.go`, `internal/usecase/account_connect.go`, `internal/storage/postgres/account.go` (`ClearSession`), `internal/handler/account_connect.go`, `cmd/app/main.go` |
| **Участник 2B — Метрики в карточке аккаунта и докрутка аналитики (FR-12)** | Обогащение `GET /accounts/:id` сводными метриками аккаунта за период (`?from=&to=`, по умолчанию 30 дней: `active_chats`, `messages_in`, `messages_out`, `avg_response_time`, `new_contacts`), фикс независимых дефолтов `from`/`to` в `account_daily_stats.go`, защита от `0001-01-01` при нулевом `CreatedAt` в `AlertStorage.Create`, маппинг `DeadlineExceeded` → `504` в `POST /accounts/:id/sync`. | `internal/usecase/account.go`, `internal/usecase/account_daily_stats.go`, `internal/storage/postgres/alert.go`, `internal/handler/handler.go` (`GetByID`), `internal/handler/account_sync.go`, тесты |
| **Участник 2C — Стыковка API с `PLAN.md` и `api/openapi.yaml` (FR-19, FR-21, NFR-28)** | Привести `GET /chats?unanswered=true` к формуле с порогом компании (`companies.no_reply_threshold`, чтобы сходилось с дашбордом), добавить `last_message_text` (preview) в `GET /chats`, фильтры по периоду (`?from=&to=`) и «проблемным» чатам (`?problematic=true`), добавить `PATCH /companies/:id` и фильтр архива `GET /alerts?resolved=true\|false\|all`, создать `api/openapi.yaml`. | `internal/domain/chat.go`, `internal/storage/postgres/chat.go`, `internal/storage/postgres/company.go`, `internal/storage/postgres/alert.go`, `internal/usecase/chat.go`, `internal/usecase/company.go`, `internal/usecase/alert.go`, `internal/handler/handler.go`, `internal/handler/alert.go`, `api/openapi.yaml` |

### Точки синхронизации с другими командами
- **Не трогать:** auth/JWT-middleware (Команда 1), `Dockerfile`/`docker-compose.yml`/CI/метрики (Команда 3).
- **Стыковка внутри Команды 2:** Участник 2A работает в новых файлах `account_connect.go` / `connect.go` и не пересекается с 2B/2C; Участники 2B и 2C правят разные методы `handler.go` и `alert.go`, мержятся последовательно.
- **Стыковка с Командой 1:** новые ручки (`POST /accounts/:id/connect`, `POST /accounts/:id/confirm`, `POST /accounts/:id/disconnect`, `PATCH /companies/:id`) используют `parseCompanyID(c)` (а после мержа Команды 1 — `requestAuth(c)` с правами `owner`).

---

## Короткий RFC (5 блоков — сохранить в `docs/rfc/05-tg-connect-and-frontend-api.md`)

1. **Фича:** подключение/отключение Telegram-аккаунтов прямо из UI, метрики в карточке аккаунта и доработка контрактов чатов/алертов/компании под готовый фронтенд + OpenAPI.
2. **Бизнес-требования:** руководитель подключает аккаунт по QR или телефону (+2FA) и может отключить его без потери истории; цифра «без ответа» на дашборде и в фильтре чатов совпадает; в списке чатов видно превью последнего сообщения; доступны архив закрытых алертов и редактирование настроек компании.
3. **Технические требования:** шифрование полученной сессии через существующий `AccountSessionStorage` (AES-256-GCM + AAD), таймауты и отмена зависших QR-сессий в памяти (без утечек горутин), сохранение обратной совместимости форматов времени (`int64` нс) и пагинации сообщений.
4. **Контракты:**
   - `POST /accounts/:id/connect` (`{mode: "qr"|"phone"}`) → `200 {state, qr_url?, expires_at?}`
   - `GET /accounts/:id/connect/status` → `200 {state, qr_url?, expires_at?, error?}`
   - `POST /accounts/:id/confirm` (`{code?, password?}`) → `200 {state}`
   - `POST /accounts/:id/connect/cancel` → `200 {status: "canceled"}`
   - `POST /accounts/:id/disconnect` → `200 {status: "disconnected"}`
   - `GET /accounts/:id?from=&to=` → `Account` + `metrics`
   - `GET /chats?...&from=&to=&problematic=true` → `Chat` + `last_message_text` (`unanswered=true` с учётом `no_reply_threshold`)
   - `PATCH /companies/:id` (`{name?, timezone?, no_reply_threshold?}`) → `200 Company`
   - `GET /alerts?resolved=true|false|all` → `200 []Alert`
5. **Схема БД:** новых таблиц не требуется (используются существующие `accounts`, `chats`, `messages`, `alerts`, `companies`, `account_daily_stats`).

---

## Готовые промпты для кодинг-агентов (Команда 2)

### Промпт 2A — HTTP-подключение (QR / телефон + 2FA) и отключение Telegram

```text
Изучи internal/telegram/client.go (существующий QR-логин через qrlogin.OnLoginToken, прокси createProxyDialer и 2FA), internal/telegram/account_session.go (AES-256-GCM шифрование с AAD), internal/telegram/enrollment.go (PrepareEnrollment и проверка совпадения phone/userID), internal/storage/postgres/account.go, internal/storage/postgres/telegram_enrollment.go, internal/handler/account_sync.go и cmd/app/main.go.

Перед кодом добавь описание своего блока в docs/rfc/05-tg-connect-and-frontend-api.md.

Реализуй серверный HTTP-флоу подключения и отключения Telegram-аккаунта из веб-кабинета (FR-9, FR-13), не ломая существующий sync и cmd/telegram-dev:

1. Менеджер интерактивных сессий подключения (internal/telegram/connect.go):
   - Потокобезопасный (sync.Mutex) реестр активных попыток подключения по ключу (companyID, accountID) с жёстким общим таймаутом (например, 3 минуты) и отменой при graceful shutdown сервера.
   - Состояния (state): "waiting_qr", "code_required", "2fa_required", "active", "error", "canceled".
   - Поддержка режима "qr": запускает gotd-клиент с существующим createProxyDialer и in-memory session.StorageMemory, через qrlogin получает login token -> формирует tg://login?token=... и expires_at; при запросе 2FA переводит в "2fa_required" и ждёт пароль из канала.
   - Поддержка режима "phone": отправляет код на account.Phone, переводит в "code_required", принимает code (и при необходимости 2FA password) через канал.
   - После успешной авторизации проверяет совпадение Telegram-профиля (matchesEnrollmentIdentity), шифрует сессию через AccountSessionStorage.seal и сохраняет в БД через ActivateTelegramSession (status='active', telegram_user_id, session).
   - Методы: StartConnect(ctx, account, mode), GetStatus(companyID, accountID), Confirm(companyID, accountID, code, password), Cancel(companyID, accountID). При отключённом TelegramConfig возвращает usecase.ErrTelegramUnavailable (503).

2. Отключение аккаунта (FR-13):
   - Добавь в AccountStorage метод Disconnect(ctx, companyID, accountID): обнуляет session = NULL и ставит status = 'disconnected' (WHERE company_id = $1 AND id = $2), НЕ удаляя чаты, сообщения, контакты и статистику.
   - Если по этому аккаунту шла активная попытка подключения — отменяет её.

3. HTTP-ручки (internal/handler/account_connect.go, internal/handler/handler.go, cmd/app/main.go):
   - POST /accounts/:id/connect (body: {"mode": "qr" | "phone"}, по умолчанию "qr")
   - GET /accounts/:id/connect/status
   - POST /accounts/:id/confirm (body: {"code": "...", "password": "..."})
   - POST /accounts/:id/connect/cancel
   - POST /accounts/:id/disconnect
   - Все ручки используют parseCompanyID(c) и handleUsecaseError; пути в корне (без /api/v1).

4. Тесты и проверки:
   - Покрой unit-тестами state machine (старт, поллинг статуса, ввод 2FA, отмена, таймаут, повторный старт, Disconnect без удаления истории, 503 при отсутствии конфига).
   - Прогони gofmt -w ., go vet ./..., go test -race ./... и go build ./....
```

### Промпт 2B — Метрики в карточке аккаунта и докрутка аналитики

```text
Изучи internal/usecase/account.go, internal/usecase/account_daily_stats.go, internal/storage/postgres/account_daily_stats.go, internal/storage/postgres/alert.go, internal/handler/handler.go (GetByID, parseDateRange) и internal/handler/account_sync.go.

Нужно закрыть требование FR-12 (карточка аккаунта с метриками) и 3 точечных edge case из аудита, сохранив полную обратную совместимость со всеми существующими тестами и полями JSON:

1. Карточка аккаунта с метриками (FR-12):
   - В AccountUsecase (или через опциональную зависимость AccountDailyStatsRepository в AccountUsecase / отдельную структуру AccountDetails, встраивающую domain.Account) обогати ответ GET /accounts/:id необязательным блоком "metrics" (json:"metrics,omitempty"):
     * messages_in, messages_out, active_chats, new_contacts, avg_response_time (time.Duration — в наносекундах, как в DashboardStats и AccountRating!).
   - Поддержи опциональные query-параметры ?from=YYYY-MM-DD&to=YYYY-MM-DD в GET /accounts/:id (по умолчанию последние 30 дней). Если хранилище статистики не подключено (мок-режим в старых тестах), поле metrics просто опускается без поломки тестов.

2. Независимые дефолты from/to (internal/usecase/account_daily_stats.go):
   - Сейчас если передан только from или только to, оба параметра перезаписываются последними 30 днями. Сделай независимый fallback: если to.IsZero() -> time.Now().UTC(); если from.IsZero() -> to.AddDate(0, 0, -30). Затем проверяй from.After(to).

3. Защита от нулевого времени в AlertStorage.Create (internal/storage/postgres/alert.go):
   - Если alert.CreatedAt.IsZero(), подставляй time.Now().UTC() перед INSERT, чтобы в БД никогда не писалось 0001-01-01.

4. Корректный HTTP-статус таймаута в POST /accounts/:id/sync (internal/handler/account_sync.go):
   - Если SyncAccount вернул context.DeadlineExceeded (и клиент сам не отменил запрос), возвращай 504 Gateway Timeout вместо 500 Internal Server Error.

5. Тесты и проверки:
   - Добавь unit-тесты на GET /accounts/:id с метриками, независимые from/to, дефолт CreatedAt и 504 при таймауте синка.
   - Прогони gofmt -w ., go vet ./..., go test -race ./... и go build ./....
```

### Промпт 2C — Стыковка API с фронтендом (`PLAN.md`) и `api/openapi.yaml`

```text
Изучи internal/domain/chat.go, internal/storage/postgres/chat.go, internal/storage/postgres/company.go, internal/storage/postgres/alert.go, internal/usecase/chat.go, internal/usecase/company.go, internal/usecase/alert.go, internal/handler/handler.go и internal/handler/alert.go.

Перед кодом зафиксируй изменения контрактов в docs/rfc/05-tg-connect-and-frontend-api.md (включая явное обновление правила unanswered=true относительно старого RFC 02/03).

Выполни следующие доработки под готовый фронтенд, НЕ меняя формат наносекунд (NoReplyThreshold / avg_response_time), НЕ меняя сортировку GET /chats/:id/messages (sent_at ASC, id ASC) и сохраняя плоский формат ошибок {"error": "..."}:

1. Привести GET /chats?unanswered=true в соответствие с KPI дашборда:
   - В ChatStorage.ListByFilter при filter.UnansweredOnly == true делай JOIN companies comp ON comp.id = a.company_id и фильтруй по той же формуле, что и в GetDashboard / FindExpiredNoReplyChats:
     c.awaiting_reply_since IS NOT NULL AND comp.no_reply_threshold >= 0 AND NOW() - c.awaiting_reply_since > (comp.no_reply_threshold / 1000000000.0) * INTERVAL '1 second'.

2. Расширение GET /chats (FR-19, FR-21):
   - Добавь в domain.Chat опциональное поле LastMessageText string (json:"last_message_text,omitempty" db:"last_message_text") и заполняй его в ListByFilter через LATERAL JOIN / подзапрос последнего сообщения чата (ORDER BY m.sent_at DESC, m.id DESC LIMIT 1) без N+1 запросов.
   - Добавь в domain.ChatFilter опциональные фильтры:
     * From *time.Time, To *time.Time (?from=YYYY-MM-DD&to=YYYY-MM-DD — фильтр по c.last_message_at).
     * ProblematicOnly bool (?problematic=true — чаты с активным алертом EXISTS (SELECT 1 FROM alerts al WHERE al.chat_id = c.id AND al.resolved_at IS NULL) ИЛИ со статусом unhappy/lost/action_required ИЛИ с риском high в chat_summaries).
   - Поддержи парсинг и валидацию новых параметров в ListChats и ChatUsecase.ListByFilter (и в mock-ветке, чтобы старые тесты не ломались).

3. Редактирование компании PATCH /companies/:id:
   - Добавь CompanyStorage.Update, CompanyUsecase.Update и хендлер PATCH /companies/:id (принимает опциональные name, timezone, no_reply_threshold в наносекундах).
   - Валидируй непустое name (если передано), валидную IANA-зону через time.LoadLocation (если передана), no_reply_threshold >= 0 (если передан). Возвращай 200 и обновлённый domain.Company.

4. Серверный архив алертов GET /alerts?resolved=true|false|all:
   - Расширь AlertRepository / AlertStorage / AlertUsecase методом ListByCompany(ctx, companyID, filter) (сохранив ListActiveByCompany для обратной совместимости):
     * без параметра или ?resolved=false -> только активные (resolved_at IS NULL, текущее поведение по умолчанию);
     * ?resolved=true -> только закрытые (resolved_at IS NOT NULL, сортировка по resolved_at DESC, id DESC);
     * ?resolved=all -> все алерты компании;
     * любое другое значение -> 400 Bad Request.
   - Поддержи также опциональный фильтр ?type=no_reply|complaint|refusal|negative|urgent.

5. Спецификация OpenAPI (NFR-28):
   - Создай api/openapi.yaml (OpenAPI 3.0.3), описывающий все актуальные эндпоинты сервера (включая ручки Волны 2 и поле "result" при частичном сбое POST /accounts/:id/sync).

6. Тесты и проверки:
   - Покрой unit-тестами новые фильтры GET /chats, PATCH /companies/:id и GET /alerts?resolved=...&type=...
   - Прогони gofmt -w ., go vet ./..., go test -race ./... и go build ./....
```

---

## Критерии приёмки (Definition of Done)
- Через HTTP API можно инициировать QR- или телефонное подключение аккаунта, передать код/2FA, получить статус `active` с зашифрованной сессией в БД и отключить аккаунт без потери переписок.
- `GET /accounts/:id` возвращает метрики аккаунта за период; `GET /chats?unanswered=true` использует тот же порог `no_reply_threshold`, что и `GET /analytics/dashboard`.
- `GET /chats` отдаёт `last_message_text` и поддерживает `from`/`to`/`problematic=true`; работают `PATCH /companies/:id` и `GET /alerts?resolved=true|false|all`.
- В репозитории есть актуальный `api/openapi.yaml`; `go vet ./...`, `go test -race ./...` и `go build ./...` проходят без ошибок.
