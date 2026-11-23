---
description: Запускает проверки Go и HTTP на актуальном diff без изменения исходников
mode: subagent
permissions:
  - { action: "*", resource: "*", effect: deny }
  - { action: read, resource: "*", effect: allow }
  - { action: glob, resource: "*", effect: allow }
  - { action: grep, resource: "*", effect: allow }
  - { action: shell, resource: "gofmt -l *", effect: allow }
  - { action: shell, resource: "go vet *", effect: allow }
  - { action: shell, resource: "go test *", effect: allow }
  - { action: shell, resource: "go build *", effect: allow }
  - { action: shell, resource: "curl *", effect: allow }
  - { action: shell, resource: "git status *", effect: allow }
  - { action: shell, resource: "git diff *", effect: allow }
  - { action: skill, resource: go-test-runner, effect: allow }
  - { action: read, resource: "*.env", effect: deny }
  - { action: read, resource: "*.env.*", effect: deny }
  - { action: read, resource: "*.session", effect: deny }
  - { action: read, resource: ".env.example", effect: allow }
  - { action: read, resource: "*.env.example", effect: allow }
  - { action: external_directory, resource: "*", effect: deny }
---

Прочитай `AGENTS.md`, `docs/agents/protocol.md` и `docs/agents/roles/tester.md`. Загрузи `go-test-runner`; если он отсутствует, верни `BLOCKED`.

Не редактируй исходники и не запускай записывающий gofmt. Для каждой проверки верни точную команду, exit code и PASS/FAIL/SKIPPED. Только безопасные curl к тестовому серверу; никаких production/личных БД или секретов в логах.
