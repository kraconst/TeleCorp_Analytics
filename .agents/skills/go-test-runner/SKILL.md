---
name: go-test-runner
description: Read-only Tester (Tier B) protocol for GO_server. Runs gofmt, go vet, go test -race, go build, and optional live HTTP curl smoke checks, returning a clean PASS/FAIL/BLOCKED report with raw command logs.
---

# Go Test Runner (`GO_server` — Tier B Tester)

Execute deterministic verification commands against `GO_server` and report the exact results without altering the codebase.

## Role & Hard Prohibitions

- **Allowed:** Run read-only Go checks, test suites, build checks (without leaving compiled binaries in the repo), `git status`, and `curl` smoke requests against a running local server.
- **Forbidden:**
  - **Never edit, format (`gofmt -w`), or fix any source file.** Use read-only `gofmt -l <changed Go files>`, never `gofmt -w` or non-recursive `gofmt -l .`.
  - **Never argue about architecture or code design** — report objective command output only.
  - **Never leave temporary binaries or scratch files** in the repository (build to `/tmp` or use `go build ./...`).

## Verification Gates

### Gate 1 — Static & Unit Gate (Mandatory, No DB Required)
Run in repository root in this exact order:
1. `gofmt -l <changed Go files>` — must return empty output and exit `0`; if no Go files changed, mark this check `SKIPPED` as not applicable. Do not use non-recursive `gofmt -l .` as a repository-wide check.
2. `go vet ./...` — must exit `0`.
3. `go test -race -count=1 ./...` — must exit `0` (note if any integration tests skip due to missing Postgres).
4. `go build ./...` — must exit `0`.

### Gate 2 — Live HTTP Smoke (Optional — Run Only If Server Is Running or Requested)
Check `GET http://localhost:8080/health` (or configured `HTTP_ADDR`) and any endpoints listed in RFC Block 4:
- Verify exact HTTP status codes (`200`, `201`, `400`, `403`, `404`, `409`, `503`).
- Verify list endpoints return `[]` rather than `null` when empty.
- Verify no sensitive fields (`session`, passwords, tokens) appear in JSON responses.

### Gate 3 — Workspace Cleanliness
Run `git status --short` to confirm the Tester introduced zero file modifications or untracked build artifacts.

## Required Output Format

Follow `docs/agents/roles/tester.md`. For every check return **Environment**, **Command**, **Exit code**, **Result: PASS | FAIL | SKIPPED**, **Relevant log excerpt**, and **Unverified areas**. Never convert `SKIPPED` to `PASS`.

Then report `Source tree unchanged: yes | no | unverified` with the `git status --short` result. Any mandatory `FAIL` means `TEST_FAILED`; a missing required input or unsafe/unavailable environment means `BLOCKED`. Preserve exact file:line references and the cause in trimmed failure logs, redacting secrets and personal paths.
