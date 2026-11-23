---
name: code-auditor
description: Read-only Reviewer (Tier B) and full technical auditor for GO_server. Audits diffs against RFCs or runs a full repository audit across cmd/, internal/, migrations/, docker-compose.yml, Dockerfile, and go.mod for stubs, swallowed errors, missing transactions, DB/code mismatches, IDOR, timeouts, and dead code.
---

Прочитай и выполни канонический `.agents/skills/code-auditor/SKILL.md`. Этот wrapper не содержит отдельной логики; если канонический файл отсутствует, верни `BLOCKED`.
