---
name: full-audit
description: Запускает canonical read-only аудит working tree, branch/PR или заданного объёма main в Kimi orchestrator.
type: flow
disableModelInvocation: true
arguments: target
---

Выполни `docs/agents/workflows/full-audit.md` для цели: $target

Текущий main Agent должен быть `orchestrator`; если это не так, попроси пользователя перезапустить сессию через `kimi --agent orchestrator`. Вызови Reviewer и Tester, при необходимости Researcher. Не вызывай Coder, не меняй файлы и раздели findings и результаты проверок.
