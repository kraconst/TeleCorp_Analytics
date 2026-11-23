---
name: feature-pipeline
description: Проводит RFC через Researcher, зонных Coder, Reviewer и Tester без автоматического commit/merge.
argument-hint: [docs/rfc/feature.md и ограничения]
context: fork
agent: orchestrator
background: false
---

Выполни канонический `docs/agents/workflows/feature-pipeline.md` для входа: $ARGUMENTS

Если не передан путь к RFC, верни `RFC_REQUIRED`. Прочитай `AGENTS.md`, `docs/agents/protocol.md` и нужные role-файлы. Вызывай специализированных agents через Agent, не реализуй код сам и не делай commit/push/merge.
