# Волна 2 — Команда 3 (2 человека): Наблюдаемость, CI, Docker + Caddy и финальная выкатка

## Цель команды
Довести инфраструктуру и эксплуатационную часть проекта до требований NFR (`docs/telecorp-spec.docx`: NFR-9, NFR-20, NFR-21, NFR-22, NFR-27) и подготовить коробочный запуск связки **Caddy + готовый Frontend (`dist/`) + Go Backend + PostgreSQL** для защиты и продакшена:
1. Реальный `db.Ping()` в `/health`, структурированные JSON-логи `log/slog` с `request_id` и метрики `/metrics` в формате Prometheus.
2. GitHub Actions CI (`gofmt`, `go vet`, `go test -race`, `docker build`), multi-stage non-root `Dockerfile`, единый `docker-compose.yml` с Caddy и `Makefile` (`reset-demo`).
3. Финальный прогон `code-auditor`, стыковка с готовым фронтендом и выкатка обновлённого стенда.

---

## Разделение работы внутри команды (2 разработчика)

| Роль | Зона ответственности | Основные файлы |
| :--- | :--- | :--- |
| **Участник 3A — `/health`, `log/slog` с `request_id` и `/metrics` (NFR-9, NFR-21, NFR-22)** | Проверка подключения к PostgreSQL (`db.PingContext` с таймаутом 2s) в `GET /health` (`503`, если БД недоступна), структурированные JSON-логи через стандартный `log/slog` + middleware `request_id` (`X-Request-ID`) без печати сырых `?q=` в логи, эндпоинт `GET /metrics` в Prometheus text format (счётчики/латентность HTTP-запросов, ошибки, длина очереди AI-саммари, счётчики синка Telegram), возврат ненулевой ошибки при таймауте остановки фоновых воркеров (`cmd/app/main.go`). | `internal/handler/handler.go` (`Health`, middleware), `internal/handler/observability.go`, `internal/handler/observability_test.go`, `cmd/app/main.go`, `internal/storage/postgres/db.go` (`SetConnMaxIdleTime`) |
| **Участник 3B — CI, multi-stage Docker, Caddy + Compose и финальная стыковка (NFR-20, NFR-27)** | `.github/workflows/ci.yml`, обновление `Dockerfile` (multi-stage build, non-root `USER`, без утечки лишних файлов), `Caddyfile` (`/` → статика `frontend/dist/` с SPA fallback, `/api/*` → reverse proxy на Go-бэк со снятием префикса `/api`), обновлённый `docker-compose.yml` (`caddy` + `app` + `postgres` на `127.0.0.1:5432` с healthcheck), `Makefile` (`reset-demo`, `test`, `build`), прогон `code-auditor` по PR всех команд и финальная стыковка с фронтом. | `.github/workflows/ci.yml`, `Dockerfile`, `Caddyfile`, `docker-compose.yml`, `Makefile`, `DEPLOY.md` |

### Точки синхронизации с другими командами
- **Не трогать:** бизнес-логику в `internal/usecase/` (кроме опциональных read-only геттеров вроде `QueueLen()` для метрик, если понадобятся), `internal/telegram/`, `internal/ai/`, `migrations/`.
- **Стыковка с Командой 1:** `GET /health` и `GET /metrics` должны оставаться доступными для healthcheck/мониторинга; в `Caddyfile` `/api/*` проксируется на Go-бэк с удалением префикса `/api`, чтобы пути `/api/auth/login` и `/api/chats` попадали в `/auth/login` и `/chats`.
- **Стыковка с готовым фронтом:** после мержа Команд 1 и 2 Участник 3B собирает `frontend/dist/`, подключает переключение фронта с Dev Session на JWT и проверяет все сценарии MVP на стенде.

---

## Короткий RFC (5 блоков — сохранить в `docs/rfc/06-observability-ci-deploy.md`)

1. **Фича:** реальный healthcheck с проверкой БД, структурированное JSON-логирование с `request_id`, метрики Prometheus, автоматический CI и коробочный запуск всей системы через Docker Compose + Caddy.
2. **Бизнес-требования:** команда и преподаватель могут поднять весь продукт одной командой `docker compose up -d` и за 5 секунд сбросить демо-данные перед защитой; сбои БД сразу видны по `/health` и логам.
3. **Технические требования:** стандартный `log/slog` (JSON), заголовок `X-Request-ID` в ответе, `/metrics` в формате Prometheus text exposition (без тяжёлых зависимостей или с `client_golang` по согласованию), multi-stage Docker-образ на Go `1.26-alpine` от непривилегированного пользователя, Postgres слушает только локальную сеть Compose / `127.0.0.1`.
4. **Контракты:**
   - `GET /health` → `200 {"status": "ok", "db": "ok"}` или `503 {"status": "degraded", "db": "error"}`
   - `GET /metrics` → `200 text/plain; version=0.0.4` (метрики HTTP, AI queue, Telegram sync)
5. **Схема БД:** изменений схемы БД нет.

---

## Готовые промпты для кодинг-агентов (Команда 3)

### Промпт 3A — `/health` с `db.Ping()`, `log/slog` + `request_id` и `/metrics`

```text
Изучи cmd/app/main.go, internal/handler/handler.go, internal/storage/postgres/db.go и internal/usecase/chat_summary.go.

Перед кодом добавь описание своего блока в docs/rfc/06-observability-ci-deploy.md.

Реализуй наблюдаемость и надёжность сервиса (NFR-9, NFR-21, NFR-22), НЕ меняя версию Go (1.26.5) в go.mod/Dockerfile и НЕ ломая существующие тесты (в мок-режиме без БД /health должен по-прежнему возвращать 200):

1. Реальный /health с проверкой БД (NFR-9):
   - Добавь в Handler опциональный HealthChecker func(context.Context) error (в cmd/app/main.go передаётся db.PingContext с таймаутом 2s).
   - Если проверка не задана (unit-тесты без БД) — возвращай 200 {"status": "ok"}.
   - Если задана и успешна — 200 {"status": "ok", "db": "ok"}.
   - Если ошибка — 503 {"status": "degraded", "db": "unavailable"} (без утечки DSN или внутренних деталей ошибки).
   - В internal/storage/postgres/db.go добавь db.SetConnMaxIdleTime(5 * time.Minute).

2. Структурированные JSON-логи (log/slog) и request_id (NFR-21):
   - В cmd/app/main.go настрой slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil))).
   - Замени gin.Default() в InitRoutes() на gin.New() с Recovery-middleware (возвращающим JSON {"error": "internal server error"} вместо пустого тела) и собственным slog-logger middleware:
     * Читает входящий X-Request-ID (если валидный и короткий) или генерирует новый (crypto/rand hex), кладёт в заголовок ответа X-Request-ID и в context.Context.
     * Логирует method, path (БЕЗ raw query string, чтобы ?q= и чувствительные параметры не попадали в логи!), status, duration_ms, request_id, client_ip.
   - Вызови r.SetTrustedProxies(nil) (или из конфига), чтобы Gin не доверял произвольным X-Forwarded-For.

3. Метрики Prometheus GET /metrics (NFR-22):
   - Реализуй легковесный сборщик метрик на стандартной библиотеке (sync/atomic + text exposition format text/plain; version=0.0.4, чтобы не раздувать go.mod) и смонтируй GET /metrics:
     * telecorp_http_requests_total{method, route, status}
     * telecorp_http_request_duration_seconds_sum / _count{method, route}
     * telecorp_http_errors_total{status_class}
     * telecorp_ai_summary_queue_depth (через метод QueueLen() в ChatSummaryUsecase)
     * telecorp_telegram_sync_runs_total{result="ok|error"}
   - Используй c.FullPath() в качестве label route (чтобы не было взрыва кардинальности от :id).

4. Graceful shutdown (cmd/app/main.go):
   - Если фоновые воркеры не успели завершиться за workerStopTimeout, возвращай ненулевую ошибку из run() вместо nil (чтобы процесс не выходил с кодом 0 при зависших воркерах).

5. Тесты и проверки:
   - Напиши тесты на GET /health (200 ok, 503 при ошибке ping), X-Request-ID, JSON-ответ при panic в Recovery-middleware и GET /metrics.
   - Прогони gofmt -w ., go vet ./..., go test -race ./... и go build ./....
```

### Промпт 3B — CI, multi-stage `Dockerfile`, `Caddyfile`, `docker-compose.yml` и `Makefile`

```text
Изучи Dockerfile, .dockerignore, docker-compose.yml, .env.example, cmd/app/main.go и migrations/.

Перед кодом дополни docs/rfc/06-observability-ci-deploy.md разделом про CI и Docker/Caddy.

Подготовь воспроизводимую инфраструктуру для CI и коробочного запуска стека (NFR-20, NFR-27), сохраняя Go 1.26-alpine в builder-этапе:

1. GitHub Actions CI (.github/workflows/ci.yml):
   - Запуск на push в main и на каждый pull_request.
   - Шаги: checkout, setup-go (из go.mod), проверка gofmt (test -z "$(gofmt -l .)"), go vet ./..., go test -race ./..., go build ./..., и сборка Docker-образа (docker build .).

2. Безопасный multi-stage Dockerfile:
   - Stage 1 (builder): FROM golang:1.26-alpine, копирование go.mod/go.sum, go mod download, сборка статического бинаря CGO_ENABLED=0 go build -o /out/server ./cmd/app.
   - Stage 2 (runtime): FROM alpine:3.21, установка ca-certificates и tzdata, создание непривилегированного пользователя appuser, копирование /out/server и ./migrations в /app, запуск от USER appuser, HEALTHCHECK на GET /health.
   - Убедись, что .dockerignore исключает .git, var/, *.session*, .env*, docs/, .agents/, .agent/, .claude/, .codex/, .kimi-code/, .opencode/, *.exe.

3. Caddyfile и единый docker-compose.yml:
   - Создай Caddyfile:
     * handle_path /api/* -> reverse_proxy app:8080 (срезает префикс /api, чтобы фронт работал без CORS на одном домене).
     * handle -> root * /srv/frontend, try_files {path} /index.html, file_server (для статики frontend/dist/).
   - Обнови docker-compose.yml, добавив сервисы:
     * postgres (postgres:16-alpine, порт привязан к 127.0.0.1:5432:5432, healthcheck, переменные из .env с безопасными дефолтами для локального dev).
     * app (сборка из Dockerfile, depends_on postgres: service_healthy, env из .env, HTTP_ADDR=:8080, DB_HOST=postgres, healthcheck).
     * caddy (caddy:2-alpine, порты 80:80 / 8080, монтирование ./Caddyfile и ./frontend/dist:/srv/frontend:ro, depends_on app).

4. Makefile и DEPLOY.md:
   - Создай Makefile с командами: test (go test -race ./...), vet, build, up (docker compose up -d --build), down, logs, reset-demo (пересоздание/сброс БД и повторный прогон миграций с seed-данными для показа).
   - Напиши краткий DEPLOY.md: запуск локально, запуск на VPS, список обязательных env-переменных (DB_*, JWT_SECRET, AUTH_REQUIRED, TELEGRAM_*, AI_*) и команда сброса демо-стенда.
```

---

## Критерии приёмки (Definition of Done)
- `GET /health` проверяет живое подключение к PostgreSQL и отдаёт `503` при остановленной БД; `GET /metrics` отдаёт валидные Prometheus-метрики; логи пишутся в JSON с `request_id` без утечки `?q=`.
- `.github/workflows/ci.yml` валиден и прогоняет `gofmt`, `go vet`, `go test -race` и `docker build`.
- `Dockerfile` собирается в multi-stage образ с non-root пользователем и не включает сессии/артефакты; `docker compose up -d --build` поднимает связку `postgres` + `app` + `caddy` целиком.
- Проведён финальный прогон `code-auditor` по веткам всех трёх команд, фронт подключён к боевому API и проверены сценарии US-1..US-7.
