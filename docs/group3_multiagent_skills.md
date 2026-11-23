# Группа 3 (2 человека): Мультиагентная система и скиллы для автоматизации кодинга (Antigravity, Codex, OpenCode)

## Цель группы
Построить внутри репозитория `GO_server` воспроизводимую мультиагентную систему разработки по архитектуре из лекции преподавателя (`ai-v2.pdf`): **Оркестратор + 4 роли со строгими запретами + узкоспециализированные зонные субагенты-кодеры + формальный процесс через RFC из 5 блоков**, совместимую сразу с тремя средами: **Antigravity**, **Codex** и **OpenCode**.

---

## Как сделать систему универсальной для Antigravity, Codex и OpenCode

Вместо того чтобы поддерживать три разные копии инструкций, используем **единый источник истины (Single Source of Truth)** в репозитории:

1. **Корневой `AGENTS.md`** — главный файл правил проекта и ролевой модели. Его нативно читают **Codex** и **OpenCode**, а **Antigravity** подключает его через короткое правило-ссылку.
2. **Папка `.agents/skills/<skill-name>/SKILL.md`** — открытый стандарт Agent Skills (YAML frontmatter `name` + `description` + инструкции). Его поддерживают **Codex**, **OpenCode** и **Antigravity**.
3. **Тонкие адаптеры под конкретные IDE (без дублирования логики):**
   - **Antigravity:** `.agent/rules/project-rules.md` (ссылка на `AGENTS.md`) и `.agent/workflows/` (пошаговые слэш-команды запуска пайплайна Оркестратора и аудита).
   - **OpenCode:** `.opencode/agents/*.md` (декларация субагентов `orchestrator`, `researcher`, `coder`, `reviewer`, `tester` с ограничениями инструментов `read`/`edit`/`bash`, ссылающихся на общие инструкции).
   - **Codex:** читает `AGENTS.md` и `.agents/skills/` напрямую из коробки (при необходимости добавляется `.codex/config.toml` с описанием профилей).

---

## Архитектура ролей (строго по презентации `ai-v2.pdf`)

Ключевой принцип со слайда 15: *«Роли различаются тем, что им показывают и что им запрещено. Общая инструкция на всех превращает четвёрку в один голос»*.

| Уровень | Роль | Что делает | Жёсткий запрет (что НЕ имеет права делать) |
| :--- | :--- | :--- | :--- |
| **Tier A** | **Оркестратор** | Принимает RFC/задачу, декомпозирует, вызывает Ресерчера → Кодера → Ревьюера → Тестировщика, собирает итог. | **Код сам не пишет** и не правит файлы проекта. |
| **Tier B** | **Ресерчер** | Ищет эталонный образец в репозитории (`GO_server`) и приносит конкретные пути, строки и паттерны. | **Новое устройство и архитектуру не предлагает**, код не пишет. |
| **Tier B** | **Кодер** (+ зонные субагенты) | Пишет один точечный diff строго по критерию RFC и принесённому образцу. Делегирует работу в узкую зону (`storage`, `usecase`, `handler`, `telegram`, `ai`). | **Слово «готово» себе не оставляет**, не меняет файлы вне своей зоны и не трогает `go.mod`/`Dockerfile` без явного разрешения. |
| **Tier B** | **Ревьюер** | Сверяет diff против критерия RFC и правил проекта: ловит лишние файлы, чужие зависимости, дыры в изоляции `company_id`, заглушки и костыли. | Код сам не правит — выдаёт структурированный список замечаний с файлом, строками и критичностью. |
| **Tier B** | **Тестировщик** | Запускает `gofmt`, `go vet ./...`, `go test -race ./...`, `go build ./...` и curl-проверки; возвращает чистый лог. | **Код не правит и спор о дизайне не ведёт.** |

### Зонные субагенты под Кодером (слайд 17, адаптировано под `GO_server`)
Вместо одного «универсального кодера на весь сервис» под Кодером создаются узкие скиллы/профили на конкретные зоны:
1. `zone-domain-migrations` — `internal/domain/`, `migrations/` (DDL, индексы, `CHECK`/`UNIQUE`, `db`-теги).
2. `zone-storage-postgres` — `internal/storage/postgres/` (`sqlx`, `mapPgError`, транзакции, `FOR UPDATE`, пустые срезы вместо `nil`).
3. `zone-usecase` — `internal/usecase/` (валидация, `companyID`-изоляция, работа только через интерфейсы).
4. `zone-handler-api` — `internal/handler/` (Gin-роуты, `parseCompanyID`, `handleUsecaseError`, без SQL и бизнес-логики).
5. `zone-integrations` — `internal/telegram/` и `internal/ai/` (внешние клиенты, таймауты контекста, обработка `FLOOD_WAIT` и `429`).

---

## Разделение работы внутри группы (2 разработчика)

| Роль | Зона ответственности | Основные файлы |
| :--- | :--- | :--- |
| **Участник 3A — Ядро мультиагентной системы, RFC и адаптеры IDE** | Корневой `AGENTS.md`, ролевые промпты и матрица запретов (`Orchestrator`, `Researcher`, `Coder`, `Reviewer`, `Tester`), шаблон RFC из 5 блоков (`docs/rfc/TEMPLATE.md`), адаптеры под Antigravity (`.agent/`), OpenCode (`.opencode/agents/`) и Codex, инструкция по запуску команды агентов (`docs/agents-guide.md`). | `AGENTS.md`, `docs/rfc/TEMPLATE.md`, `docs/agents-guide.md`, `.agent/rules/*`, `.agent/workflows/*`, `.opencode/agents/*` |
| **Участник 3B — Библиотека скиллов в `.agents/skills/`** | Написание и отладка переносимых скиллов формата `SKILL.md`: `rfc-writer`, `code-auditor` (на базе нашего Промпта №4), `go-test-runner`, `go-researcher` и 5 зонных скиллов Кодера (`zone-domain-migrations`, `zone-storage-postgres`, `zone-usecase`, `zone-handler-api`, `zone-integrations`) с опорой на реальные эталоны `GO_server`. | `.agents/skills/*/SKILL.md` |

### Точки синхронизации с другими группами
- **Не трогать:** боевой Go-код в `cmd/`, `internal/`, `migrations/` — Группа 3 работает исключительно с агентной инфраструктурой (`AGENTS.md`, `.agents/`, `.agent/`, `.opencode/`, `docs/rfc/`, `docs/agents-guide.md`).
- **Практическая проверка:** прогнать готовый скилл `code-auditor` и `go-test-runner` на текущей ветке `main` (и на PR Групп 1 и 2), чтобы убедиться, что агенты реально работают в Antigravity, Codex и OpenCode.

---

## Готовый промпт для кодинг-агента (Группа 3)

```text
Изучи структуру репозитория GO_server (CONTRIBUTING.md, cmd/, internal/domain/, internal/storage/postgres/, internal/usecase/, internal/handler/, internal/telegram/, migrations/) и спроектируй универсальную мультиагентную систему разработки и библиотеку скиллов, совместимую с Antigravity, Codex и OpenCode.

Архитектура системы должна строго следовать ролевой модели:
- Единый источник истины: корневой AGENTS.md + переносимые скиллы в .agents/skills/<name>/SKILL.md (с YAML frontmatter: name, description).
- Тонкие адаптеры без дублирования правил: .agent/rules/ и .agent/workflows/ для Antigravity, .opencode/agents/ для OpenCode (Codex читает AGENTS.md и .agents/skills/ нативно).
- Не меняй ни одного .go файла, go.mod, Dockerfile или SQL-миграции.

Выполни следующие задачи:

1. Шаблон RFC из 5 фиксированных блоков (docs/rfc/TEMPLATE.md):
   Создай шаблон RFC строго с 5 разделами в фиксированном порядке:
   1) Фича (1–2 предложения: что появляется у пользователя, без ручек и таблиц);
   2) Бизнес-требования (экраны/сценарии, роли, что можно и чего нельзя);
   3) Технические требования (идентификаторы, слои, куда ходит usecase, ошибки, открытые пути);
   4) Контракты (HTTP-ручки, тела запросов и ответов, статусы);
   5) Схема БД (новые таблицы и миграции или список существующих таблиц).

2. Корневой файл правил и ролевой модели (AGENTS.md):
   - Кратко опиши архитектуру GO_server (handler -> usecase -> storage -> domain), инварианты (изоляция company_id, маппинг ошибок через mapPgError и handleUsecaseError, запрет менять версии в go.mod/Dockerfile без явного указания, обязательность gofmt/go vet/go test).
   - Опиши пайплайн работы из 5 ролей с жёсткими запретами для каждой:
     * Оркестратор (Tier A): принимает RFC, декомпозирует, вызывает 4 роли по очереди, КОД НЕ ПИШЕТ.
     * Ресерчер (Tier B): находит образец в репозитории и приносит пути/фрагменты, НОВОЕ УСТРОЙСТВО НЕ ПРЕДЛАГАЕТ, код не правит.
     * Кодер (Tier B): сдаёт один diff по критерию RFC и образцу в рамках одной зоны ответственности, СЛОВО «ГОТОВО» СЕБЕ НЕ ОСТАВЛЯЕТ (передаёт на ревью и тест).
     * Ревьюер (Tier B): сверяет diff с RFC и правилами проекта, ищет лишние файлы, чужие зависимости, заглушки, проглоченные ошибки, IDOR; КОД НЕ ПРАВИТ.
     * Тестировщик (Tier B): запускает gofmt, go vet, go test -race ./..., go build ./... и curl-проверки, возвращает лог; КОД НЕ ПРАВИТ И СПОР О ДИЗАЙНЕ НЕ ВЕДЁТ.

3. Библиотека переносимых скиллов в .agents/skills/<skill-name>/SKILL.md:
   Создай компактные, чёткие SKILL.md (без воды, с конкретными ссылками на эталонные файлы этого репозитория):
   - .agents/skills/rfc-writer/SKILL.md — помогает оформить задачу в формате docs/rfc/TEMPLATE.md.
   - .agents/skills/go-researcher/SKILL.md — протокол работы Ресерчера (поиск ближайшего эталона в GO_server, вывод только фактов и путей).
   - .agents/skills/zone-domain-migrations/SKILL.md — правила зоны internal/domain/ и migrations/ (нумерация, up/down, CHECK/UNIQUE, db-теги).
   - .agents/skills/zone-storage-postgres/SKILL.md — правила зоны internal/storage/postgres/ (sqlx, mapPgError, транзакции, FOR UPDATE, non-nil срезы).
   - .agents/skills/zone-usecase/SKILL.md — правила зоны internal/usecase/ (интерфейсы в interfaces.go, валидация, проверка companyID и IsActive, без импорта gin/sqlx).
   - .agents/skills/zone-handler-api/SKILL.md — правила зоны internal/handler/ (Gin, parseCompanyID, handleUsecaseError, request-структуры, без SQL).
   - .agents/skills/zone-integrations/SKILL.md — правила зоны internal/telegram/ и internal/ai/ (интерфейсы, таймауты контекста, обработка FLOOD_WAIT и 429, безопасное отключение без ключей).
   - .agents/skills/code-auditor/SKILL.md — полный чек-лист Ревьюера для аудита на костыли, заглушки, проглоченные ошибки, несоответствия БД/кода, IDOR, таймауты и мёртвый код (формат отчёта: файл, строки, критичность, причина, способ исправления, без правки файлов).
   - .agents/skills/go-test-runner/SKILL.md — протокол Тестировщика (команды проверки, формат отчёта с логом).

4. Адаптеры для IDE и документация запуска:
   - Для Antigravity: создай .agent/rules/telecorp-agents.md (подключение AGENTS.md) и воркфлоу .agent/workflows/feature-pipeline.md (запуск цепочки Оркестратор -> Ресерчер -> Кодер -> Ревьюер -> Тестировщик) и .agent/workflows/full-audit.md (запуск code-auditor).
   - Для OpenCode: создай .opencode/agents/orchestrator.md, researcher.md, coder.md, reviewer.md, tester.md с корректным YAML frontmatter (mode: subagent/primary, ограничения прав write/edit/bash согласно запретам роли) и ссылкой на соответствующие скиллы.
   - Напиши краткую инструкцию docs/agents-guide.md: как участнику команды запустить этот пайплайн в Antigravity, Codex и OpenCode на примере новой фичи.
```

---

## Критерии приёмки (Definition of Done)
- В корне есть `AGENTS.md`, в `docs/rfc/TEMPLATE.md` — шаблон RFC из 5 блоков, в `.agents/skills/` — все 9 скиллов с валидным YAML frontmatter (`name`, `description`).
- В `.agent/` и `.opencode/agents/` созданы тонкие адаптеры, не дублирующие текст правил.
- У каждой из 5 ролей явно прописаны вход, выход и **запреты** (Ресерчер не придумывает архитектуру, Кодер не ставит «готово», Ревьюер и Тестировщик не правят код, Оркестратор не пишет код).
- Проведён живой прогон минимум в двух средах (например, Antigravity + OpenCode или Codex): запущен `full-audit` / `code-auditor` и `go-test-runner` на текущей ветке, проверено, что агенты соблюдают свои запреты.
