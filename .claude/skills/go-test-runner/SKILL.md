---
name: go-test-runner
description: Read-only Tester (Tier B) protocol for GO_server. Runs gofmt, go vet, go test -race, go build, and optional live HTTP curl smoke checks, returning a clean PASS/FAIL/BLOCKED report with raw command logs.
---

Прочитай и выполни канонический `.agents/skills/go-test-runner/SKILL.md`. Этот wrapper не содержит отдельной логики; если канонический файл отсутствует, верни `BLOCKED`.
