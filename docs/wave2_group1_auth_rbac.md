# Волна 2 — Команда 1 (2 человека): Аутентификация, JWT, RBAC и CORS

## Цель команды
Закрыть требования безопасности MVP (`docs/telecorp-spec.docx`: FR-1, FR-2, FR-3, FR-4, FR-5, NFR-10, NFR-14):
1. Реализовать регистрацию компании, вход по email/паролю (`bcrypt`) с выдачей JWT, `GET /me` и приглашение менеджеров (`POST /users/invite`).
2. Перевести API с временного `?company_id=` на контекст из JWT и разграничить права `owner` / `manager` (с возвратом `403` на чужие данные и закрытием `owner`-only ручек).
3. Добавить `CORS`-middleware, безопасный флаг перехода `AUTH_REQUIRED` (чтобы не сломать Dev Session готового фронта до финальной стыковки) и журнал доступа к перепискам.

**Важные ограничения для стыковки с фронтом (`PLAN.md`):**
- **Пути остаются в корне** (`/auth/register`, `/auth/login`, `/me`, `/users/invite`, `/chats`, `/accounts`) — **без** префикса `/api/v1` (Vite/Caddy сами режут `/api` до корня).
- **Формат ошибок остаётся плоским** `{"error": "текст"}` (при необходимости можно добавить соседнее поле `"code"`), чтобы не ломать существующие тесты и `ApiError` на фронте.
- **Пакет `golang.org/x/crypto/bcrypt`** уже присутствует в `go.mod` (`golang.org/x/crypto v0.54.0`) — для хэширования паролей новые зависимости не нужны. Для JWT можно использовать `github.com/golang-jwt/jwt/v5` (по согласованию) или компактную реализацию HS256 на стандартной библиотеке `crypto/hmac` + `crypto/sha256` без изменения `go.mod`. Не меняйте версию Go `1.26.5` и существующие версии зависимостей.

---

## Разделение работы внутри команды (2 разработчика)

| Роль | Зона ответственности | Основные файлы |
| :--- | :--- | :--- |
| **Участник 1A — Auth, JWT, `bcrypt`, CORS и миграция `000011`** | Секция `AuthConfig` (`JWT_SECRET`, `JWT_TTL`, `AUTH_REQUIRED`, `CORS_ALLOWED_ORIGINS`), генерация/валидация JWT, `bcrypt`-хэширование, транзакционный `POST /auth/register` (`company` + `employee` + `owner`), `POST /auth/login`, `GET /me`, `POST /users/invite`, закрытие небезопасного `POST /users`, JWT- и CORS-middleware, миграция `000011` (`LOWER(email)` + таблица `access_logs`). | `migrations/000011_auth_and_access_logs.{up,down}.sql`, `internal/config/config.go`, `internal/domain/auth.go`, `internal/usecase/auth.go`, `internal/storage/postgres/auth.go`, `internal/storage/postgres/user.go`, `internal/handler/auth.go`, `internal/handler/middleware.go`, `cmd/app/main.go` |
| **Участник 1B — RBAC (`owner`/`manager`), сквозная изоляция `company_id` и журнал доступа** | Перевод всех хендлеров на `AuthContext` (при `AUTH_REQUIRED=true` клиентские `?company_id=` и `req.CompanyID` игнорируются), разграничение `owner` vs `manager` (менеджер видит только закреплённые за ним аккаунты, их чаты, сообщения, саммари и алерты; `403` на чужие), закрытие `owner`-only ручек, `company_id` в SQL-мутациях (`AssignEmployee`, `ChatStorage.UpdateStatus`, `AlertStorage.Create`), fail-closed при `nil`-репозиториях, запись в `access_logs` при чтении переписок. | `internal/handler/handler.go`, `internal/handler/alert.go`, `internal/handler/account_sync.go`, `internal/handler/chat_analytics.go`, `internal/usecase/account.go`, `internal/usecase/chat.go`, `internal/usecase/message.go`, `internal/usecase/alert.go`, `internal/storage/postgres/account.go`, `internal/storage/postgres/chat.go`, `internal/storage/postgres/alert.go` |

### Точки синхронизации с другими командами
- **Номер миграции:** за Командой 1 закреплена **`000011`** (`migrations/000011_auth_and_access_logs.{up,down}.sql`).
- **Не трогать:** `internal/telegram/`, `internal/ai/`, `internal/storage/postgres/telegram_sync.go`, `Dockerfile`, `docker-compose.yml` — это зоны Команд 2 и 3.
- **Стыковка с фронтом и Командой 2:** в День 1 Участник 1A фиксирует JSON-формат ответов `/auth/register`, `/auth/login` и `/me` в RFC `docs/rfc/04-auth-rbac.md`.

---

## Короткий RFC (5 блоков — сохранить в `docs/rfc/04-auth-rbac.md`)

1. **Фича:** регистрация компании, вход по email/паролю с выдачей JWT, приглашение менеджеров и разграничение доступа `owner`/`manager` с изоляцией данных компаний.
2. **Бизнес-требования:** `owner` видит всю компанию и управляет сотрудниками/аккаунтами/настройками; `manager` видит только закреплённые за ним аккаунты и их чаты/алерты, при попытке открыть чужие данные получает `403`; email регистронезависим и уникален.
3. **Технические требования:** `bcrypt` для паролей, HS256 JWT (`JWT_SECRET`, `JWT_TTL`), флаг `AUTH_REQUIRED` (по умолчанию `false` для локального dev / совместимости со старыми тестами, `true` в проде), CORS-middleware, журнал доступа `access_logs` при чтении сообщений чужих/корпоративных чатов.
4. **Контракты:**
   - `POST /auth/register` — `{company_name, owner_name, email, password, timezone?, no_reply_threshold?}` → `201 {token, user, company}`
   - `POST /auth/login` — `{email, password}` → `200 {token, user}`
   - `GET /me` — `200 {user, company}`
   - `POST /users/invite` (`owner`) — `{email, password, employee_id}` → `201 {id, email, role, employee_id}`
5. **Схема БД:** миграция `000011` — уникальный функциональный индекс `users(LOWER(email))` и таблица `access_logs (id, company_id, user_id, chat_id, action, created_at)`.

---

## Готовые промпты для кодинг-агентов (Команда 1)

### Промпт 1A — Auth, JWT, `bcrypt`, CORS и миграция `000011`

```text
Изучи docs/telecorp-spec.docx (FR-1..FR-5, NFR-10, NFR-14), internal/config/config.go, internal/domain/user.go, internal/domain/company.go, internal/storage/postgres/user.go, internal/storage/postgres/company.go, internal/usecase/user.go, internal/handler/handler.go и cmd/app/main.go.

Перед кодом зафиксируй короткий 5-блочный RFC в docs/rfc/04-auth-rbac.md.

Реализуй базовый слой аутентификации, JWT, CORS и миграцию 000011, НЕ меняя версии Go (1.26.5) и существующие версии в go.mod/Dockerfile, оставляя все пути в корне (без /api/v1) и сохраняя плоский формат ошибок {"error": "..."}:

1. Миграция 000011 (migrations/000011_auth_and_access_logs.up.sql и .down.sql):
   - Добавь уникальный индекс по LOWER(email) в таблице users (предварительно проверь совместимость с существующим UNIQUE(email)).
   - Создай таблицу access_logs (id BIGSERIAL PRIMARY KEY, company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE, user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE, chat_id BIGINT REFERENCES chats(id) ON DELETE SET NULL, action VARCHAR(64) NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()) с индексом по (company_id, created_at DESC).

2. Конфигурация (internal/config/config.go):
   - Добавь секцию AuthConfig: JWT_SECRET (обязателен, если AUTH_REQUIRED=true; маскировать в String/GoString как [REDACTED]), JWT_TTL (по умолчанию 24h), AUTH_REQUIRED (bool, по умолчанию false для обратной совместимости со старыми тестами и Dev Session), CORS_ALLOWED_ORIGINS (список через запятую, по умолчанию http://localhost:5173).
   - Обнови .env.example (без секретных значений).

3. Бизнес-логика Auth (internal/usecase/auth.go + internal/storage/postgres/auth.go):
   - Используй golang.org/x/crypto/bcrypt (уже есть в go.mod) для хэширования и проверки паролей (минимальная длина пароля — 8 символов).
   - Реализуй нормализацию email (strings.ToLower(strings.TrimSpace(email))) при создании и поиске пользователя.
   - Реализуй транзакционный Register(ctx, input): в ОДНОЙ SQL-транзакции создаёт company, employee (owner_name, position="Владелец", is_active=true) и user (role=owner, employee_id=&empID, bcrypt-хэш), затем выдаёт подписанный JWT (HS256 с claims: sub=user_id, company_id, role, employee_id, exp, iat).
   - Реализуй Login(ctx, email, password) -> (token, user) с единой ошибкой "invalid email or password" (401) при неверном логине/пароле.
   - Реализуй InviteManager(ctx, companyID, input): проверяет, что employee_id существует, активен и принадлежит companyID, хэширует пароль и создаёт user с role=manager.
   - В UserUsecase.Create запрети сохранение произвольной пустой/пробельной строки в PasswordHash.

4. Middleware и HTTP-ручки (internal/handler/middleware.go, internal/handler/auth.go, internal/handler/handler.go):
   - Реализуй CORS-middleware (Access-Control-Allow-Origin для разрешённых origin, Allow-Headers: Authorization, Content-Type, Allow-Methods: GET, POST, PUT, PATCH, DELETE, OPTIONS, корректный ответ 204 на OPTIONS).
   - Реализуй JWT Auth-middleware: парсит Authorization: Bearer <token>, кладёт AuthClaims (UserID, CompanyID, Role, EmployeeID) в gin.Context и context.Context. Открытые пути: GET /health, POST /auth/register, POST /auth/login. При AUTH_REQUIRED=false отсутствие токена пропускает запрос дальше (для Dev Session и старых тестов), но если токен передан — валидирует его (битый токен всегда -> 401).
   - Добавь ручки: POST /auth/register, POST /auth/login, GET /me, POST /users/invite.
   - Старую ручку POST /users закрой для неавторизованных при AUTH_REQUIRED=true и не принимай сырой password_hash (либо переведи на password + bcrypt).

5. Тесты и проверки:
   - Покрой unit-тестами bcrypt/JWT, Register, Login, InviteManager, CORS preflight и JWT-middleware (валидный токен, истёкший, битая подпись, AUTH_REQUIRED=true/false).
   - Прогони gofmt -w ., go vet ./..., go test -race ./... и go build ./....
```

### Промпт 1B — RBAC (`owner`/`manager`), сквозная изоляция и журнал доступа

```text
Изучи ветку Участника 1A (internal/handler/middleware.go, internal/handler/auth.go, docs/rfc/04-auth-rbac.md), а также internal/handler/handler.go, internal/handler/alert.go, internal/handler/account_sync.go, internal/handler/chat_analytics.go, internal/usecase/ и internal/storage/postgres/.

Нужно внедрить строгую проверку ролей owner/manager, сквозную изоляцию company_id и журнал доступа (FR-4, FR-5, NFR-14), не сломав существующие тесты (при AUTH_REQUIRED=false и отсутствии JWT в контексте сохраняется текущее поведение parseCompanyID):

1. Извлечение tenant-контекста в хендлерах:
   - Добавь единый хелпер requestAuth(c *gin.Context): если в контексте есть AuthClaims из JWT, ВСЕГДА брать companyID, role и employeeID только из токена (игнорируя ?company_id= и req.CompanyID в теле запроса). Если токена нет и AUTH_REQUIRED=false — fallback на parseCompanyID(c) с ролью owner.
   - Убери молчаливую подстановку company_id=1 / employee_id=1 при передаче отрицательных значений (<= 0) в JSON-телах: возвращай 400 Bad Request.

2. Разграничение ролей owner и manager (FR-5):
   - Добавь middleware RequireRole(domain.RoleOwner) на owner-only маршруты:
     * POST /users/invite, GET /users, GET /users/:id, POST /users
     * GET/POST/PUT/PATCH /employees, POST /employees/:id/deactivate
     * POST /accounts, POST /accounts/:id/assign, POST /accounts/:id/sync
     * GET /analytics/dashboard, GET /analytics/accounts
     * POST /companies (и будущий PATCH /companies/:id)
     При попытке manager вызвать эти ручки возвращать 403 Forbidden.
   - Для manager на общих ручках:
     * GET /accounts — возвращать только аккаунты, закреплённые за его employee_id (ListByEmployee с проверкой company_id).
     * GET /accounts/:id — 403 Forbidden, если account.EmployeeID != claims.EmployeeID.
     * GET /chats — принудительно выставлять filter.EmployeeID = claims.EmployeeID (перекрывая любой query-параметр).
     * GET /chats/:id, POST /chats/:id/status, GET /chats/:id/messages, GET /chats/:id/analytics, GET /chats/:id/summary, POST /chats/:id/summary, POST /chats/:id/summary/generate — проверять, что чат принадлежит аккаунту этого менеджера, иначе 403 Forbidden.
     * GET /alerts, POST /alerts/:id/resolve — фильтровать/проверять доступность только по чатам аккаунтов этого менеджера (403 на чужие).

3. Защита на уровне usecase и SQL (defense-in-depth):
   - Убери fail-open проверки: в ChatUsecase, AccountUsecase, AlertUsecase, ChatSummaryUsecase, MessageUsecase при nil обязательных репозиториев владения возвращать внутреннюю ошибку, а не пропускать проверку.
   - Добавь company_id в SQL WHERE для AccountStorage.AssignEmployee, ChatStorage.UpdateStatus и проверку принадлежности chat_id компании в AlertStorage.Create.

4. Журнал доступа (NFR-14):
   - Реализуй AccessLogRepository поверх таблицы access_logs (из миграции 000011).
   - При чтении истории сообщений (GET /chats/:id/messages) авторизованным пользователем асинхронно или синхронно записывай факт доступа (company_id, user_id, chat_id, action="read_messages").

5. Тесты и проверки:
   - Напиши тесты на RBAC: owner имеет доступ ко всему в своей компании и 404/403 на чужую компанию; manager получает 403 на /analytics/dashboard, /employees, чужой /accounts/:id, чужой /chats/:id/messages; запись в access_logs создаётся при чтении сообщений.
   - Прогони gofmt -w ., go vet ./..., go test -race ./... и go build ./....
```

---

## Критерии приёмки (Definition of Done)
- `POST /auth/register` атомарно создаёт компанию, сотрудника и владельца, возвращая рабочий JWT; повторная регистрация с тем же email в другом регистре возвращает `409 Conflict`.
- При `AUTH_REQUIRED=true` любой запрос без токена (кроме `/health`, `/auth/register`, `/auth/login`) возвращает `401 Unauthorized`; `manager` получает `403 Forbidden` на чужие аккаунты/чаты и на `owner`-ручки.
- CORS preflight (`OPTIONS`) отвечает `204` с корректными заголовками для разрешённых origin.
- Все существующие и новые тесты проходят: `go vet ./...`, `go test -race ./...`, `go build ./...`.
