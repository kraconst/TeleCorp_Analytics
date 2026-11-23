---
name: full-audit
description: Выполняет независимый read-only аудит working tree, branch/PR или заданного объёма main без исправлений.
argument-hint: [working tree, branch против base или объём main]
context: fork
agent: orchestrator
background: false
---

Выполни канонический `docs/agents/workflows/full-audit.md` для цели: $ARGUMENTS

Прочитай `AGENTS.md`, `docs/agents/protocol.md` и нужные role-файлы. Вызови Reviewer и Tester через Agent, при необходимости Researcher. Не вызывай Coder, не меняй файлы и раздели findings и результаты проверок.
