---
name: feature-pipeline
description: Запускает canonical пятиуровневый feature-pipeline для подтверждённого RFC в сессии Kimi orchestrator.
type: flow
disableModelInvocation: true
arguments: target
---

Выполни `docs/agents/workflows/feature-pipeline.md` для RFC и ограничений: $target

Если вход не содержит путь к RFC, верни `RFC_REQUIRED`. Текущий main Agent должен быть `orchestrator`; если это не так, попроси пользователя перезапустить сессию через `kimi --agent orchestrator`. Используй Agent только для researcher, coder, reviewer и tester. Не реализуй код сам и не делай commit/push/merge.
