# Руководство по агентной разработке

## 1. Назначение

Репозиторий содержит единый процесс, в котором задача проходит через пять ролей:

```text
RFC → Orchestrator → Researcher → Coder по зонам → Reviewer → Tester → человек
```

Канонические правила находятся в [AGENTS.md](../AGENTS.md), [protocol](agents/protocol.md), [role-файлах](agents/roles/) и [workflows](agents/workflows/). Конфигурации Kimi Code CLI, OpenCode, Antigravity, Codex и Claude Code — тонкие адаптеры, а не пять независимых наборов правил.

- **Orchestrator** декомпозирует и координирует, но не пишет код.
- **Researcher** read-only ищет фактические проектные паттерны.
- **Coder** меняет одну зону и owned files одним атомарным diff.
- **Reviewer** независимо проверяет diff, но не исправляет его.
- **Tester** запускает проверки и сообщает факты, но не меняет исходники.

Процесс не создаёт commit, push, merge или PR автоматически. Это сохраняет у человека контроль над историей, публикацией и окончательным принятием изменений.

## 2. Подготовка

1. Откройте доверенный репозиторий. Project-scoped настройки Codex применяются только к trusted project; другие среды также могут загружать project instructions и permissions после подтверждения доверия.
2. Запускайте клиент из корня репозитория, чтобы обнаружились корневые инструкции и project customizations.
3. Зафиксируйте `git status` и base commit. Не начинайте работу в дереве с неотличимыми чужими изменениями.
4. Заполните [RFC из пяти блоков](rfc/TEMPLATE.md): «Фича», «Бизнес-требования», «Технические требования», «Контракты», «Схема БД». Отдельных разделов «Тесты» и «План» нет; проверки входят в технические требования.
5. Подтвердите RFC и пользовательские ограничения. Для полного feature-pipeline нужны девять скиллов 3B, перечисленных ниже.

Секреты не входят во входные артефакты. Не передавайте агентам `.env`, токены, SSH-ключи, Telegram `*.session` и DSN с паролем; используйте `.env.example` и безопасные тестовые credentials. Не запускайте миграции на production или личной БД. Поведенческие тесты Coder выполняйте только в одноразовом worktree/копии.

## 3. Статусы и результат

- `HANDOFF` — роль закончила свой этап и вернула требуемый артефакт.
- `BLOCKED` — нужен человек/Orchestrator; причина и недостающий вход обязательны.
- `CHANGES_REQUESTED` — Reviewer нашёл конкретные проблемы; Coder исправляет только их, затем ревью повторяется.
- `PASS`/`FAIL` — результат конкретной команды Tester с exit code.
- `SKIPPED` — проверка не выполнялась; причина и непроверенная область обязательны. Это не PASS.
- `READY_FOR_HUMAN` — ревью и доступные обязательные проверки завершены; не означает merge.

Финальный отчёт Orchestrator содержит RFC, файлы, Research, findings и их устранение, команды/exit codes, SKIPPED, риски и ограничения. Фактическая приёмка этой инфраструктуры — в [validation-report](agents/validation-report.md).

## 4. Совместимость и запуск

В таблице «проверенная версия» относится только к локально доступному клиенту; отсутствие клиента не заменяется предположением о совместимости.

| Среда | Проверенная версия | Instructions / agents / skills | Feature-pipeline | Full-audit | Discovery |
|---|---|---|---|---|---|
| Kimi Code CLI | `2.7.2`, live Research/Review/Test | `AGENTS.md`; `.kimi-code/agents/`; `.agents/skills/` + `.kimi-code/skills/` | `kimi --agent orchestrator`, `/skill:feature-pipeline <RFC>` | `/skill:full-audit <цель>` | `/status`, `/tasks`, `/reload` |
| OpenCode V2 | `v2.2.6`, статическая схема | `AGENTS.md`; `.opencode/agents/`; `.agents/skills/` и `.claude/skills/` | Выбрать `orchestrator`, `/feature-pipeline <RFC>` | `/full-audit <цель>` | Список primary agents и `/`-команд в TUI |
| Antigravity | стандартный агент (1×) | `AGENTS.md`; встроенные `research`/`self`; `.agents/skills/` (без `.agents/agents/`) | Дефолтный агент: «Выполни feature-pipeline для `<RFC>`» | Дефолтный агент: «Выполни full-audit для …» | Settings → Projects → Customizations |
| Codex | клиент не установлен | `AGENTS.md`; `.codex/agents/`; `.agents/skills/` | Основной поток = Orchestrator, вызвать named workers | Read-only parent, вызвать Reviewer/Tester | Проверить instructions и agent threads |
| Claude Code | клиент не установлен | `CLAUDE.md` → `AGENTS.md`; `.claude/agents/`; `.claude/skills/` | `claude --agent orchestrator`, затем `/feature-pipeline <RFC>` | `/full-audit <цель>` | `/context all`, `/memory`, `/skills`, `/agents`/`/tasks` |

Поддерживаемые команды диагностики зависят от установленной версии. Если команды нет, используйте соответствующий UI и не считайте это ошибкой репозитория.

### Kimi Code CLI

1. Запустите `kimi --agent orchestrator` из корня доверенного репозитория. Проверьте `/status`, при необходимости примените изменения через `/reload`.
2. Выполните `/skill:feature-pipeline docs/rfc/<feature>.md` или `/skill:full-audit working tree` / `/skill:full-audit <branch> против <base>`.
3. Для ручного запуска отдельной роли доступны `/skill:go-researcher`, `/skill:code-auditor`, `/skill:go-test-runner` и пять `/skill:zone-*`. Автоматический выбор по description также поддерживается, кроме двух launchers с `type: flow`.
4. Отслеживайте дочерние вызовы через `/tasks`. Каждый subagent получает изолированный контекст и только Task Packet, а не историю родителя.
5. Используйте обычный manual permission mode и не включайте `--auto`/`--yolo` для приёмки. Subagents наследуют permissions родителя; tool allowlists и `subagents: []` ограничивают интерфейс, но не заменяют sandbox и post-check diff.

Kimi нативно читает девять `.agents/skills/` и корневой `AGENTS.md`; wrappers не нужны. Пять `.kimi-code/agents/*.md` имеют приоритет над одноимёнными Antigravity-файлами `.agents/agents/`, поэтому Kimi получает корректные case-sensitive tool names (`Read`, `Bash`, `Skill`, `Agent`). `coder.md` содержит `override: true`, чтобы заменить встроенный универсальный coder. Не удаляйте это поле без отдельной проверки discovery. В main profile `orchestrator.md` переменная `${base_prompt}` — поддерживаемая Kimi подстановка базового system prompt; она сохраняет workspace instructions, skills и plugin sections.

Проектные custom permission rules Kimi читает из `.kimi-code/local.toml` только для trusted workspace. Этот файл может содержать машинные настройки и не должен коммититься; базовая поставка использует tool allowlists и встроенные guards, без небезопасного `dangerous_command_guard = false`.

### OpenCode V2

1. Запустите `opencode` из корня и выберите primary agent `orchestrator`.
2. Выполните `/feature-pipeline docs/rfc/<feature>.md` либо `/full-audit working tree` / `/full-audit <branch> против <base>`.
3. Дождитесь результатов субагентов и просмотрите их отчёты и фактический diff. Команды выбирают `orchestrator` автоматически через frontmatter.
4. OpenCode перечитывает agents/commands автоматически; при stale UI используйте поддерживаемый `/reload` или `opencode reload`, затем повторите discovery.

Permissions V2 используют ordered `permissions` и last-match-wins. У read-only ролей edit/shell запрещены, у Orchestrator разрешены только четыре worker ID, у Coder технический edit allowlist объединяет пять зон. Последнее не является динамической изоляцией одной зоны: Task Packet и post-check diff обязательны. Внешние директории, `.env` и session-файлы запрещены; `.env.example` разрешён.

### Antigravity

1. **Запускайте Antigravity без `--agent` и без пользовательских агентов из `.agents/agents/*`:** кастомные агенты (`.agents/agents/*`, плагинные агенты и их субагенты) расходуют квоту в 4× быстрее. Директория `.agents/agents/` намеренно удалена; все правила перенесены в `AGENTS.md`, `docs/agents/roles/` и `.agents/skills/*`.
2. **Агент по умолчанию** выступает Orchestrator (или исполнителем для простых задач) и делегирует роли только **встроенным субагентам** (стандартный тариф 1×):
   - **Researcher:** встроенный `research` + `.agents/skills/go-researcher/SKILL.md` + `docs/agents/roles/researcher.md`.
   - **Coder / Reviewer / Tester:** встроенный `self` + Task Packet + нужный скилл `.agents/skills/{zone-*,code-auditor,go-test-runner}/SKILL.md` + соответствующий `docs/agents/roles/*.md`.
3. В чате передавайте задачу агенту по умолчанию: «Выполни feature-pipeline для `docs/rfc/<feature>.md`» или «Выполни full-audit для …».
4. Используйте project permission preset **Default** с Terminal sandbox и подтверждением внешних действий. Не используйте Turbo. Для поведенческих проверок Coder выбирайте изолированный worktree.
5. Проверяйте diff после роли (`git status --short`). `.agent/rules/` и `.agent/workflows/` сохранены как legacy compatibility.

### Codex

1. Отметьте проект trusted и запустите Codex из корня. Убедитесь, что загружен `AGENTS.md`; при необходимости попросите перечислить active instruction sources.
2. Основной поток выполняет `docs/agents/roles/orchestrator.md`. Отдельный `orchestrator.toml` намеренно отсутствует, чтобы не зависеть от nested orchestration.
3. Дайте основному потоку RFC и попросите последовательно вызвать project agents `researcher`, `coder`, `reviewer`, `tester` по canonical workflow. Дождитесь нужных результатов и откройте agent threads для проверки.
4. Для реализации используйте `workspace-write` в изолированном worktree. Для full-audit выберите read-only режим родительской сессии.
5. Custom agents наследуют sandbox/permissions родителя. `developer_instructions` не создаёт независимый read-only sandbox; у Researcher/Reviewer дополнительно отключён shell через `features.shell_tool = false`, но diff всё равно проверяется.

`.codex/config.toml` включает agents и ограничивает одновременные потоки четырьмя. Модель, reasoning effort, провайдер, токены и unsafe approval policy не закреплены.

### Claude Code

1. Запустите `claude --agent orchestrator` из корня либо выберите orchestrator в поддерживаемом UI.
2. Выполните `/feature-pipeline docs/rfc/<feature>.md` или `/full-audit <цель>`. Launchers используют `context: fork`, `agent: orchestrator` и ожидают результат.
3. Проверьте `/context all` или `/memory` для общих инструкций, `/skills` для wrappers, `/agents`/`/tasks` для ролей и дочерних задач — в пределах установленной версии.
4. Не используйте `bypassPermissions`; project settings отключают этот режим. Отсутствие Edit/Write не делает Bash read-only, поэтому ролевые правила и post-check diff остаются обязательными.
5. Девять `.claude/skills/<id>/SKILL.md` — wrappers: они читают `.agents/skills/<id>/SKILL.md`. Это не копии и не symlinks, поэтому checkout на Windows поддерживается. После появления нативной поддержки общей директории wrappers удаляются отдельной миграцией.

## 5. Контракт со скиллами 3B

Девять ID строго фиксированы:

```text
rfc-writer
go-researcher
code-auditor
go-test-runner
zone-domain-migrations
zone-storage-postgres
zone-usecase
zone-handler-api
zone-integrations
```

Канонический skill содержит `.agents/skills/<id>/SKILL.md` с переносимым Agent Skills Markdown, `name` равным directory ID и `description`, объясняющим назначение и вызов. Внутри нет proprietary tool names, `$skill`, dynamic injection или абсолютных пользовательских путей. 3A не создаёт заглушки этих файлов.

Researcher использует `go-researcher`, Reviewer — `code-auditor`, Tester — `go-test-runner`, Coder — ровно один `zone-*`. При изменении ID одновременно обновляются адаптеры и wrappers. При отсутствии обязательного скилла роль возвращает `BLOCKED`; полный live pipeline до появления 3B не считается принятым.

## 6. Сквозной пример

Допустим, нужен новый сигнал о жалобе клиента. Это пример чтения, а не инструкция изменить production-код сейчас.

1. Пользователь заполняет RFC: наблюдаемый сигнал, права компании, HTTP-контракт, up/down-миграция и проверки.
2. Orchestrator фиксирует базу и вызывает Researcher. Тот находит Account/Employee как эталоны слоёв и существующие sentinel errors.
3. Orchestrator разделяет работу на domain/migrations → storage → usecase → handler/API. Если нужна wiring-правка `cmd/app/main.go`, он отдельно возвращает integration blocker человеку.
4. Каждый Coder получает один критерий, owned files и один zone skill; после него Orchestrator проверяет diff.
5. Reviewer получает RFC, Research и полный diff, проверяет company scope и обработку ошибок. Findings возвращаются Coder, ревью повторяется.
6. Tester получает актуальный diff, выполняет `gofmt -l`, `go vet ./...`, `go test -race ./...`, `go build ./...` и применимые HTTP/PostgreSQL проверки.
7. Человек получает отчёт с результатами и рисками и сам решает о commit/PR/merge.

## 7. Troubleshooting

- **Instructions не загружены:** запускайте из корня, проверьте trusted project, размер/содержимое `AGENTS.md`, отсутствие override и активные sources. Перезапустите сессию, если клиент строит instruction chain один раз.
- **Custom agent не найден:** проверьте точный путь, глубину, `name`/filename, frontmatter и reload. Для OpenCode workers должны иметь `mode: subagent`, Orchestrator — `primary`.
- **Skill не найден:** проверьте `.agents/skills/<id>/SKILL.md`, точный case-sensitive ID и permissions. Для Claude проверьте wrapper. Отсутствие 3B — ожидаемый `BLOCKED`, а не повод создать заглушку.
- **Дублирующийся ID:** найдите пользовательские/global/project источники. Порядок override зависит от среды; удалите непреднамеренный дубль, не полагайтесь на случайный приоритет.
- **Неверный YAML/TOML/JSON:** проверьте разделители `---`, обязательные поля и парсинг. Для Antigravity неверное tool name может остановить субагента; используйте только подтверждённые имена.
- **Project не trusted:** project config Codex и часть project customizations других сред могут не применяться. Доверяйте только проверенному репозиторию.
- **Субагенту не хватает permission:** проверьте родительский sandbox, role allowlist, external directory и последнее совпавшее OpenCode-правило. Не включайте Turbo/bypass ради обхода.
- **Coder заблокирован из-за `cmd/` или `internal/config/`:** это предусмотренный integration blocker. Не относите файлы к другой зоне; решение принимает человек/владелец спецификации.
- **БД недоступна:** `/health` может работать на mock-данных. Проверьте startup logs и применённые миграции. DB-dependent проверки помечайте SKIPPED с ограничением, если безопасная БД недоступна.
- **Тесты SKIPPED:** сохраните причину и непроверенную область. Не переводите в PASS и не выдавай pipeline за полностью проверенный.
- **Worktree содержит чужие изменения:** остановитесь, зафиксируйте blocker и используйте новый одноразовый worktree. Не выполняйте reset/clean.
- **Отчёт содержит чувствительные данные:** не публикуйте его; замените секреты, DSN, персональные пути и session-данные sanitized summary. Удаление секрета из отчёта не отзывает уже раскрытый credential — нужен отдельный отзыв владельцем.

## 8. Официальные схемы

Перед изменением adapters повторно сверьте документацию установленных версий:

- [Kimi Code agents](https://www.kimi.com/code/docs/en/kimi-code-cli/customization/agents.html), [skills](https://www.kimi.com/code/docs/en/kimi-code-cli/customization/skills.html), [slash commands](https://www.kimi.com/code/docs/en/kimi-code-cli/reference/slash-commands.html), [configuration](https://www.kimi.com/code/docs/en/kimi-code-cli/configuration/config-files.html).
- [OpenCode agents](https://opencode.ai/docs/agents), [permissions](https://opencode.ai/docs/permissions), [commands](https://opencode.ai/docs/commands), [skills](https://opencode.ai/docs/skills).
- [Antigravity subagents](https://antigravity.google/docs/subagents/), [rules](https://antigravity.google/docs/rules/), [permissions](https://antigravity.google/docs/permissions/), [workflow migration](https://antigravity.google/docs/migration/workflows-to-skills/).
- [Codex subagents](https://developers.openai.com/codex/agent-configuration/subagents), [AGENTS.md](https://developers.openai.com/codex/agent-configuration/agents-md), [skills](https://developers.openai.com/codex/build-skills).
- [Claude Code subagents](https://code.claude.com/docs/en/sub-agents), [skills](https://code.claude.com/docs/en/skills), [permissions](https://code.claude.com/docs/en/permissions), [memory](https://code.claude.com/docs/en/memory).
