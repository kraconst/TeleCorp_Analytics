# Общий протокол ролевой разработки

Документ задаёт единый протокол для всех сред. Краткие обязательные правила находятся в [AGENTS.md](../../AGENTS.md), детали ролей — в [roles/](roles/). Адаптеры IDE не заменяют этот документ.

## 1. Состояния и переходы

```text
RFC_REQUIRED → RFC_READY
RFC_READY → RESEARCHING
RESEARCHING → RESEARCHED
RESEARCHED → CODING
CODING → REVIEWING
REVIEWING → CHANGES_REQUESTED | TESTING
CHANGES_REQUESTED → CODING
TESTING → TEST_FAILED | READY_FOR_HUMAN
TEST_FAILED → CODING
Любой этап → BLOCKED
```

- `RFC_REQUIRED`: нет подтверждённого RFC либо пять его блоков недостаточны для работы.
- `RFC_READY`: RFC полон, критерии наблюдаемы, ограничения зафиксированы.
- `RESEARCHING`/`RESEARCHED`: исследование выполняется/завершено с фактическими ссылками.
- `CODING`: Coder выполняет одну атомарную зонную задачу.
- `REVIEWING`: независимый Reviewer проверяет актуальный полный diff.
- `CHANGES_REQUESTED`: есть конкретные findings; повторяется Coder только в исходной зоне.
- `TESTING`: Reviewer вернул `HANDOFF`, Tester проверяет актуальный diff.
- `TEST_FAILED`: хотя бы одна обязательная проверка завершилась `FAIL`.
- `BLOCKED`: нужен человек, отсутствует обязательный вход или требуется файл вне разрешённой зоны.
- `READY_FOR_HUMAN`: ревью пройдено и все обязательные доступные проверки выполнены. Это не commit, push, merge или одобрение PR.

Orchestrator ведёт журнал `состояние → состояние`, Task ID, ссылку на артефакт и причину каждого `BLOCKED`/повтора.

## 2. Task Packet

Каждая роль получает самодостаточное задание в новом контексте. История родительского чата не считается входом.

```text
Task ID:
RFC path:
RFC criterion / acceptance target:
Current state:
Base commit or base diff:
Owned zone:
Owned files:
Input artifacts from previous roles:
Allowed operations:
Explicit exceptions, if any:
Expected output format:
```

Для Reviewer и Tester дополнительно обязательны:

- фактически изменённые файлы;
- diff или однозначная команда/база для его получения;
- результаты предыдущего ревью и исправлений;
- требуемые проверки, окружение и известные ограничения.

Пустые поля заполняются `none`/`not applicable`, а не опускаются молча. Несогласованное исключение не даёт права выйти за зону.

## 3. Статусы результатов

- `HANDOFF` — роль завершила этап и передала указанный формат результата.
- `BLOCKED` — продолжение требует решения человека или Orchestrator; причина и недостающий вход обязательны.
- `CHANGES_REQUESTED` — Reviewer нашёл проблемы и описал каждую с доказательствами.
- `PASS`/`FAIL` — результат одной конкретной проверки Tester с точной командой и exit code.
- `SKIPPED` — проверка не выполнялась; причина и непроверенная область обязательны.

Coder никогда не использует `READY_FOR_HUMAN` и не объявляет продукт окончательно готовым. `HANDOFF` от Reviewer не означает, что тесты пройдены.

## 4. Зонная декомпозиция

| Зона | Разрешённые пути | Skill |
|---|---|---|
| domain/migrations | `internal/domain/`, `migrations/` | `zone-domain-migrations` |
| storage | `internal/storage/postgres/` | `zone-storage-postgres` |
| usecase | `internal/usecase/` | `zone-usecase` |
| handler/API | `internal/handler/` | `zone-handler-api` |
| integrations | `internal/telegram/`, будущий `internal/ai/` | `zone-integrations` |

Один Coder получает один критерий, одну зону и явный owned files. Он загружает ровно один zone skill. `cmd/`, `internal/config/`, `go.mod`, `go.sum` и `Dockerfile` не принадлежат зонам. Если они нужны, Coder возвращает `BLOCKED`, а Orchestrator создаёт integration blocker для человека. Нельзя неявно отнести composition root к handler или config к integrations.

Последовательность по умолчанию: Research → Code → Review → Test. Параллельны только независимые исследования. Параллельная запись допустима лишь в независимых файловых областях, предпочтительно в worktrees; для первой версии она не используется по умолчанию.

## 5. Цикл исправлений

1. Reviewer формирует упорядоченные findings с файлом, строками, правилом, доказательством и влиянием.
2. Orchestrator передаёт исходный RFC, Research, finding и актуальный diff исходному Coder.
3. Coder исправляет только замечание, не расширяя зону и owned files.
4. Orchestrator проверяет новый diff и повторно вызывает Reviewer.
5. После `HANDOFF` Tester получает именно актуальный diff; после `TEST_FAILED` цикл возвращается к Coder.
6. Три непродуктивных цикла одной проблемы → `BLOCKED` и эскалация человеку.
7. `BLOCKED`, `CHANGES_REQUESTED`, `FAIL` и обязательный `SKIPPED` нельзя игнорировать ради формального завершения.

## 6. Независимость и enforcement

Orchestrator не пишет код, Coder не ревьюит себя, Reviewer не исправляет findings, Tester не редактирует исходники. Для каждого запрета учитываются четыре уровня: инструкция, tool permissions, sandbox/worktree и post-check фактического diff/логов. Текст «не редактируй» не является технической read-only гарантией. Если среда наследует общий sandbox, это явно указывается в отчёте.

## 7. Итог Orchestrator

Финальный отчёт содержит:

- пользовательское изменение и ссылку на RFC;
- фактический список файлов;
- краткий Research Report;
- findings Reviewer и статус устранения;
- точные команды Tester, exit codes и PASS/FAIL/SKIPPED;
- пропущенные проверки, риски и ограничения;
- состояние `READY_FOR_HUMAN` либо `BLOCKED`;
- явное «commit/merge/push выполняет человек».

Не включайте токены, DSN с паролем, `.env`, Telegram-сессии, персональные пути или большие сырые логи.
