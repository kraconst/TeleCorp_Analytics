# Отчёт о валидации 3A + 3B + Kimi Code CLI

Дата: 2026-11-23. Базовый коммит: `c089b135b038ca711abd6182e5dea2c3f5e96590` (HEAD совпадал при проверках). Ветка: `feature/contact-chat`. Логи сокращены; secrets, DSN, session-данные и персональные пути не включены.

## 1. Итог

Девять скиллов 3B перенесены из `3B/telecorp-agent-skills-3b/` в канонический `.agents/skills/` и связаны со всеми adapters. Добавлена отдельная поддержка Kimi Code CLI `2.7.2`: пять профилей в `.kimi-code/agents/`, два ручных launcher в `.kimi-code/skills/`, прямое использование канонических skills без wrappers.

Интеграция прошла статические проверки и живые Kimi Researcher/Reviewer/Tester. Это не заявление о живой совместимости Antigravity/Codex/Claude Code: по решению владельца эти среды проверят другие члены команды. Полный feature-pipeline с изменением кода в одноразовом worktree в этой итерации не запускался.

Production-код, миграции, зависимости и Docker-файлы не изменялись. После проверки переноса исходная папка `3B/` была удалена по явному запросу владельца; канонические скиллы сохранены в `.agents/skills/`. `Plan_3A.md` и существующий `.agents/teamwork/` не редактировались.

## 2. Устранённые расхождения 3B ↔ 3A

При переносе канонических скиллов исправлены:

- `READY FOR REVIEW` → `HANDOFF`; Reviewer использует `HANDOFF`/`CHANGES_REQUESTED`/`BLOCKED`, Tester — `PASS`/`FAIL`/`SKIPPED` по каноническому протоколу.
- Удалены права handler zone менять `cmd/app/main.go` и integrations zone менять `internal/config/config.go`; такие потребности остаются integration blocker.
- Убраны жёстко заданный migration baseline `000008` и предположения о `RunMigrations`/`mapPgError`/handler helpers без проверки текущего checkout.
- Researcher возвращает полный Research Report из 3A; Coder skills требуют полный Coder handoff из 3A.
- `gofmt -l .` заменён на корректный `gofmt -l <changed Go files>`; неприменимый случай — `SKIPPED`.
- У всех девяти Claude wrappers `name` и `description` теперь точно равны canonical metadata.

## 3. Статические и Go-проверки

Environment: Linux, Python 3.12.3, PyYAML 6.0.1, Go `1.26.10`.

| Проверка | Exit code | Результат | Фактический результат |
|---|---:|---|---|
| Canonical skills/frontmatter + wrapper parity | 0 | PASS | `9/9`; directory ID = `name`; все descriptions wrappers совпадают. |
| Kimi profiles и launchers | 0 | PASS | 5 agents, 2 `type: flow` launchers; workers имеют `subagents: []`; coder имеет `override: true`. |
| `git check-ignore .kimi-code/local.toml` | 0 | PASS | Машинный local config исключён из Git. |
| YAML/TOML/JSON + локальные Markdown-ссылки | 0 | PASS | Все созданные config/frontmatter парсятся; битых локальных ссылок нет. |
| `go vet ./...` | 0 | PASS | Без диагностики. |
| `go test -race -count=1 ./...` | 0 | PASS | Handler, storage/postgres, usecase PASS; остальные пакеты без test files. |
| `go build ./...` | 0 | PASS | Успешно. |
| `gofmt -l <changed Go files>` | — | SKIPPED | Изменённых Go-файлов нет. |
| HTTP/PostgreSQL/migration checks | — | SKIPPED | Не относятся к инфраструктурному diff; тестовые сервисы не поднимались. |

`AGENTS.md` после интеграции содержит 88 строк, что ниже лимита 200. Прежнее значение 86 в старом отчёте относилось к версии до добавления Kimi; отчёт перезаписан по текущему состоянию.

## 4. Живая проверка Kimi Code CLI

Environment: `kimi version 2.7.2`.

| Сценарий | Роли/вызовы | Результат | Доказательства и ограничения |
|---|---|---|---|
| `kimi doctor` | CLI | PASS | Пользовательские TOML config валидны; пути в отчёт не включены. Проверка не заменяет project agent discovery. |
| Первый вызов `kimi -p --agent researcher …` | нет | FAIL | CLI вернул `unknown command 'researcher'`: у `-p` обязательный аргумент. Исправлен порядок аргументов. |
| `kimi --agent researcher -p …` | Researcher + `go-researcher` | HANDOFF | Возвращён полный Research Report; найденные пути/символы проверены в текущем checkout. Использованы только Read/Grep/Glob/Skill, без записи и shell. Base commit подтверждён. |
| `kimi --agent orchestrator -p …` — full-audit | Orchestrator → Reviewer + `code-auditor` | CHANGES_REQUESTED | Получён независимый отчёт по 66 файлам, 4 findings; файлы не изменены. |
| Тот же full-audit | Tester + `go-test-runner` | TEST_FAILED, см. анализ ниже | YAML/TOML/JSON, ссылки, 9/9 skills, post-check PASS. Общий FAIL основан на ложном требовании пятого Codex agent-файла. |
| Full-audit parent process | Orchestrator | Не завершён | После отсутствия нового stdout процесс был остановлен; последующее чтение лога показало уже завершённый Tester handoff. Финальный сводный ответ Orchestrator не получен и не выдаётся за полученный. |

### Findings Reviewer и их разрешение

1. **MEDIUM: тип `disableBypassPermissionsMode`.** Отклонено по официальной документации Claude Code: поле действительно должно быть строкой `"disable"`. `.claude/settings.json` не менялся. Live-проверка остаётся за членом команды с Claude Code.
2. **MEDIUM: у Claude Orchestrator/Reviewer нет Git-доступа.** Подтверждено и исправлено: добавлен `Bash` с явным разрешением только read-only `git status`/`git diff`/`git rev-parse` в инструкциях роли.
3. **LOW: 86 против 88 строк.** Подтверждено как устаревший отчёт после Kimi-изменения; отчёт перезаписан.
4. **LOW: недокументированный `${base_prompt}`.** Поддержка подтверждена официальной документацией Kimi; guide теперь объясняет, что переменная сохраняет базовый prompt, workspace instructions, skills и plugin sections.

Reviewer также честно сообщил о выходе за узкий shell allowlist для read-only `wc`/`ls`/`find`. Записи не было, git post-check чист; это зафиксировано как нарушение Task Packet, а не скрыто.

### Почему Tester TEST_FAILED не стал новым finding

Tester присвоил FAIL структурной проверке, потому что `.codex/agents/` содержит 4 файла вместо 5. Это не дефект: `Plan_3A.md` и `docs/agents-guide.md` намеренно определяют четыре Codex workers, а роль Orchestrator выполняет основной поток. Все остальные проверки Tester прошли. Исходный `TEST_FAILED` сохранён здесь, но не считается доказательством нового дефекта.

## 5. OpenCode после интеграции 3B

Environment: OpenCode `v2.2.6`; canonical skills уже присутствовали.

| Сценарий | Результат | Фактические доказательства |
|---|---|---|
| `opencode debug agents` | PASS | Пять project roles обнаружены с правильными primary/subagent modes. |
| Researcher + `go-researcher` | HANDOFF | Canonical skill загружен; полный Research Report с путями/строками, без записи и shell. |
| Coder negative в worktree | PASS | `zone-domain-migrations` загружен; запрос на вторую зону, `go.mod`, `cmd/` и unrelated refactoring дал `BLOCKED`; фактический diff пуст. |
| Coder positive в worktree | HANDOFF | Создан только owned `internal/domain/errors_test.go`; focused race-test PASS; production baseline не изменён. |
| Reviewer positive | BLOCKED | OpenCode завершился с `Free usage limit reached`; findings нельзя считать полученными. |
| Full-audit Orchestrator | BLOCKED | Run превысил timeout до получения Reviewer/Tester handoff. Прочитаны canonical docs и выполнен base discovery; полный аудит не завершён. |

Прежний negative read-only Researcher также не создавал sentinel. Ограничение free usage не обходилось и настройки провайдера не менялись.

## 6. Coder/Reviewer/Tester в одноразовом worktree

Для проверки был создан detached worktree от base `c089b135b038ca711abd6182e5dea2c3f5e96590`; в него скопирована только необходимая агентная инфраструктура. Тестовый RFC: `docs/rfc/99-domain-sentinel-test.md` внутри worktree. Он не входит в основной diff.

### Позитивный путь

1. OpenCode Coder создал один owned test для независимости четырёх sentinel errors и вернул `HANDOFF`.
2. Kimi Reviewer с `code-auditor` проверил RFC, границы и production-контракты: `HANDOFF` без findings.
3. Kimi Tester с `go-test-runner` выполнил `gofmt -l internal/domain/errors_test.go`, `go vet ./...`, `go test -race -count=1 ./...`, `go build ./...`: все exit code 0 / PASS. HTTP/DB — SKIPPED как неприменимые. Source tree unchanged: yes.

### Цикл исправлений

1. В owned test намеренно внесена инверсия условия `errors.Is` для безопасного сценария Reviewer.
2. Kimi Reviewer вернул `CHANGES_REQUESTED` с MEDIUM finding, точными строками, evidence, impact и Target Zone; файл не редактировал.
3. Kimi Coder с `zone-domain-migrations` исправил только условие и diagnostic message в owned file; focused race-test PASS.
4. Повторный Kimi Reviewer на актуальном файле: `HANDOFF` без findings.
5. Повторный Kimi Tester на актуальном diff: все четыре обязательные проверки PASS; pre/post `git status --short` совпали.

Тестовые изменения не переносились в основной проект; одноразовый worktree был удалён после фиксации результатов.

## 7. Оставшиеся проверки для команды

| Область | Статус | Комментарий |
|---|---|---|
| Antigravity discovery/audit/Coder/permissions | Не проверено локально | Выполняет член команды с установленным клиентом. |
| Codex trusted project/agents/skills/audit | Не проверено локально | Четыре worker файла намеренно; основной поток — Orchestrator. |
| Claude Code agents/wrappers/launchers/permissions | Не проверено локально | После исправления Bash-allowlist требуется live discovery. |
| OpenCode full-audit и Tester | BLOCKED | Timeout и free usage limit; успешные Research/Coder не заменяют эти этапы. |
| Единый end-to-end Orchestrator feature-pipeline | Не проверен полностью | Все отдельные роли и цикл проверены, но не одним непрерывным Orchestrator-run. |
| PR Групп 1 и 2 | SKIPPED | Конкретные PR/base refs не предоставлены. |

Файловая интеграция 3A+3B, Coder positive/negative и цикл исправлений готовы к просмотру человеком. Полная кросс-средовая приёмка и непрерывный Orchestrator feature-pipeline не заявляются как выполненные; `Plan_3A.md` сохраняется.
