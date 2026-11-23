# TeleCorp Analytics: общие правила и агентный процесс

## Назначение и архитектура

Go HTTP API для контроля корпоративных Telegram-переписок. Module path — case-sensitive `github.com/kraconst/GO_server`; требуется Go 1.26.5 по `go.mod`.

```text
Gin Handler → Usecase → Storage (PostgreSQL/sqlx) → Domain
cmd/app/main.go — composition root
```

Repository contracts: `internal/usecase/interfaces.go`; модели и sentinel errors: `internal/domain/`. Account и Employee — основные эталоны; порядок добавления сущности — `CONTRIBUTING.md`. `internal/ai/` — только будущая зона, сейчас модуль не существует.

## Обязательные инварианты

1. Ограничивайте данные `company_id`/`companyID` на каждом соответствующем уровне. Получения по глобальному ID недостаточно, если RFC требует изоляции.
2. Handler не содержит SQL/бизнес-логики; Usecase зависит от интерфейсов, не от Gin/sqlx; Domain остаётся dependency-free.
3. Storage сохраняет проектное преобразование PostgreSQL/`sql.ErrNoRows` в доменные ошибки (`ErrNotFound`, `ErrForbidden`, `ErrConflict`, `ErrValidation`).
4. Изменения БД требуют согласованных up/down-миграций, ограничений и индексов.
5. Не проглатывайте ошибки; явно маркируйте частичный успех. Внешние вызовы получают context и таймауты.
6. Отсутствие внешних ключей не должно ронять сервер, если архитектура предусматривает отключение интеграции.
7. Не меняйте зависимости, их версии, Dockerfile и Docker-окружение без явного разрешения.
8. Текущий hard-coded company ID 1 в handlers — известное ограничение, а не доказательство tenant isolation.
9. Ни один агент не объявляет результат готовым без независимых Reviewer и Tester.

## Команды

```bash
go fmt ./...
go vet ./...
go test -race ./...
go build ./...
```

Для read-only Tester: `gofmt -l <изменённые Go-файлы>` вместо записывающего форматирования. `gofmt -w .` из старых README/CONTRIBUTING не рекурсивен; используйте `go fmt ./...` или явный список файлов.

PostgreSQL: `docker compose up -d postgres`; API: `go run ./cmd/app` (`:8080`). Процесс не загружает `.env` автоматически. При неудачном DB ping сервер может стартовать с mock-данными, поэтому `/health` не доказывает работу PostgreSQL. Compose монтирует только начальную миграцию при инициализации volume; новые миграции в существующий volume автоматически не применяются.

## Единый pipeline

```text
RFC → Orchestrator → Researcher → Coder (по зонам) → Reviewer → Tester → человек
```

Состояния: `RFC_REQUIRED`, `RFC_READY`, `RESEARCHING`, `RESEARCHED`, `CODING`, `REVIEWING`, `CHANGES_REQUESTED`, `TESTING`, `TEST_FAILED`, `BLOCKED`, `READY_FOR_HUMAN`. Переходы, Task Packet и повторные итерации: `docs/agents/protocol.md`. По умолчанию запись последовательна; Research можно параллелить только для независимых вопросов. `READY_FOR_HUMAN` не означает merge.

| Роль | Может | Не может |
|---|---|---|
| Orchestrator | декомпозировать, вызывать четыре роли, собирать отчёт | писать/исправлять код |
| Researcher | читать, искать, цитировать факты | менять файлы, проектировать новую архитектуру |
| Coder | менять одну назначенную зону атомарным diff | выходить за owned files, объявлять финальную готовность |
| Reviewer | анализировать diff и выдавать findings | править код |
| Tester | запускать проверки и возвращать факты | править код, решать вопросы дизайна |

Подробные протоколы: `docs/agents/roles/{orchestrator,researcher,coder,reviewer,tester}.md`. Каждый вызов получает самодостаточный Task Packet. Reviewer и Tester обязательны после Coder; после исправлений повторяются на актуальном diff. Три непродуктивных цикла одной проблемы → эскалация человеку.

## Зоны и скиллы

| Зона Coder | Пути | Zone skill |
|---|---|---|
| domain/migrations | `internal/domain/`, `migrations/` | `zone-domain-migrations` |
| storage | `internal/storage/postgres/` | `zone-storage-postgres` |
| usecase | `internal/usecase/` | `zone-usecase` |
| handler/API | `internal/handler/` | `zone-handler-api` |
| integrations | `internal/telegram/`, будущий `internal/ai/` | `zone-integrations` |

Один Coder загружает ровно один zone skill. `cmd/` и `internal/config/` не входят в зоны: необходимость их изменения → `BLOCKED` и integration blocker человеку, а не неявное расширение зоны.

Девять общих скиллов: `rfc-writer` (RFC), `go-researcher` (Researcher), `code-auditor` (Reviewer), `go-test-runner` (Tester) и пять `zone-*` из таблицы. Канонические файлы 3B: `.agents/skills/<id>/SKILL.md`. Отсутствие обязательного скилла → `BLOCKED`, а не универсальный fallback. В Claude Code используются одноимённые тонкие wrappers.

Kimi Code CLI читает канонические skills напрямую. Используйте `.kimi-code/agents/` вместо несовместимых Antigravity-профилей из `.agents/agents/` и ручные launchers `.kimi-code/skills/`; Kimi наследует permissions родителя, поэтому нужны tool allowlists и post-check diff.

## Глобальные запреты и безопасность

- Не читайте и не выводите `.env`, токены, DSN с паролем, SSH-ключи и Telegram `*.session`; не включайте их в отчёты, Git или Docker build. `.env.example` безопасен как шаблон.
- Не делайте commit, push, reset, clean, merge или публикацию PR по умолчанию; решение и действие остаются за человеком.
- Read-only роль не получает права писать через shell. Tool allowlist, sandbox/worktree и post-check diff дополняют инструкции; prompt-only запрет не считается технической гарантией.
- Не запускайте миграции на production/личной БД. Поведенческие тесты Coder выполняйте только в одноразовом worktree/копии.
- Не скрывайте FAIL/SKIPPED, чужие изменения, непроверенные области и наследование permissions конкретной средой.

## Канонические документы и сценарии

- RFC: `docs/rfc/TEMPLATE.md` — ровно пять основных блоков.
- Протокол: `docs/agents/protocol.md`.
- Feature: `docs/agents/workflows/feature-pipeline.md`.
- Read-only аудит: `docs/agents/workflows/full-audit.md`.
- Установка, запуск, диагностика и ограничения сред: `docs/agents-guide.md`.
- Фактические результаты приёмки: `docs/agents/validation-report.md`.
