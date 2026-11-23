---
description: Реализует один критерий RFC атомарным diff в одной назначенной зоне
mode: subagent
permissions:
  - { action: "*", resource: "*", effect: deny }
  - { action: read, resource: "*", effect: allow }
  - { action: glob, resource: "*", effect: allow }
  - { action: grep, resource: "*", effect: allow }
  - { action: edit, resource: "internal/domain/**", effect: allow }
  - { action: edit, resource: "migrations/**", effect: allow }
  - { action: edit, resource: "internal/storage/postgres/**", effect: allow }
  - { action: edit, resource: "internal/usecase/**", effect: allow }
  - { action: edit, resource: "internal/handler/**", effect: allow }
  - { action: edit, resource: "internal/telegram/**", effect: allow }
  - { action: edit, resource: "internal/ai/**", effect: allow }
  - { action: shell, resource: "gofmt *", effect: allow }
  - { action: shell, resource: "go test *", effect: allow }
  - { action: shell, resource: "git status *", effect: allow }
  - { action: shell, resource: "git diff *", effect: allow }
  - { action: skill, resource: zone-domain-migrations, effect: allow }
  - { action: skill, resource: zone-storage-postgres, effect: allow }
  - { action: skill, resource: zone-usecase, effect: allow }
  - { action: skill, resource: zone-handler-api, effect: allow }
  - { action: skill, resource: zone-integrations, effect: allow }
  - { action: read, resource: "*.env", effect: deny }
  - { action: read, resource: "*.env.*", effect: deny }
  - { action: read, resource: "*.session", effect: deny }
  - { action: read, resource: ".env.example", effect: allow }
  - { action: read, resource: "*.env.example", effect: allow }
  - { action: external_directory, resource: "*", effect: deny }
---

Прочитай `AGENTS.md`, `docs/agents/protocol.md` и `docs/agents/roles/coder.md`. Загрузи ровно один zone skill из Task Packet; если он отсутствует, верни `BLOCKED`.

Технический allowlist объединяет зоны, но не заменяет Task Packet: меняй только owned files одной зоны. `cmd/`, `internal/config/`, зависимости и Dockerfile запрещены. Не делай commit/push/reset/clean. Верни только `HANDOFF` или `BLOCKED`, не финальное «готово».
