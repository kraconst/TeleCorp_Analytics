---
name: zone-handler-api
description: Zone Coder (Tier B) skill for GO_server HTTP handlers in internal/handler/. Enforces thin Gin handlers, verified project helpers, non-nil JSON lists, and httptest coverage.
---

# Zone Coder: HTTP Handler & Wiring (`GO_server`)

Implement scoped HTTP routes and request/response DTOs strictly inside `internal/handler/` according to RFC Block 4 and the Research Brief.

## Zone Boundaries & Hard Prohibitions

- **Owned Paths:** only files explicitly assigned under `internal/handler/`, including `*_test.go`.
- **Forbidden:**
  - Never modify `cmd/app/main.go` or `internal/config/`; wiring needs are an integration blocker for the Orchestrator and human, not part of this zone.
  - Never write SQL queries, import `sqlx`/`pgx`, or implement business/domain rules inside handlers.
  - Never modify `internal/domain/`, `internal/storage/`, `internal/usecase/`, `migrations/`, `go.mod`, or `Dockerfile`. Escalate cross-zone needs to the Orchestrator.
  - **Never declare the task "Done" / "Готово"** — always hand off as `Status: HANDOFF`; final readiness requires independent Reviewer and Tester.
  - Never return fake `200`/`201` responses or mock fallback data when a usecase fails.

## Pre-Flight Check

1. Confirm you have RFC Block 4 (exact HTTP routes, query params, request/response schemas, status codes) and Research Brief.
2. Check the actual route registration and helper functions identified by Research. Names such as `InitRoutes`, `parseCompanyID`, or `handleUsecaseError` are candidates only; never assume they exist on the current branch.

## Zone Rules & Repository Standards

- **Thin Gin Handlers:** Parse path/query/JSON inputs -> call usecase with `c.Request.Context()` -> map errors -> write JSON response.
- **Unified `company_id` Extraction:** Use the company-scoping helper confirmed by Research, if present; never invent `parseCompanyID`. If the current branch still hard-codes company ID, report this limitation and follow the RFC rather than presenting it as tenant isolation.
- **Dedicated Request Structs:** Define explicit request structs (e.g., `createXRequest`, `updateXRequest`) instead of binding directly onto `domain.*` structs. Validate malformed JSON or invalid path IDs and return `400 Bad Request`.
- **Unified Error Mapping:** Use the shared handler error mapper confirmed by Research, if present; otherwise follow the current verified mapping without inventing `handleUsecaseError`:
  - `domain.ErrValidation` -> `400`
  - `domain.ErrForbidden` -> `403`
  - `domain.ErrNotFound` -> `404`
  - `domain.ErrConflict` -> `409`
  - Unmapped errors -> `500` (with generic `"internal server error"` — never leak raw DB errors or stack traces).
  - If an optional integration (Telegram/AI) is disabled by config, return `503 Service Unavailable` as specified in the RFC.
- **JSON Safety:**
  - Empty collections must serialize as `[]`, never `null`.
  - Never expose sensitive fields (`session`, tokens, passwords) in response payloads.
- **Route Organization:** Group new endpoints cleanly in `InitRoutes()` (`internal/handler/handler.go`); place large feature handlers in dedicated files (mirror `internal/handler/alert.go`, `internal/handler/chat_analytics.go`).
- **Mandatory `httptest` Coverage:** For every new or modified endpoint, add tests in `internal/handler/*_test.go` covering:
  - Happy path (`200`/`201`) and empty list returning `[]`
  - Invalid path ID / JSON (`400`)
  - Mapped usecase errors (`403`, `404`, `409`, `503` where applicable)

## Coder Handoff Format

Use the complete handoff from `docs/agents/roles/coder.md`; the fields below are zone-specific additions, not a replacement.

- **Changed Files:** exact list of created/modified files.
- **RFC Contract Check:** table/list confirming each route, status code, and payload matches RFC Block 4.
- **Tests Added/Updated:** exact `httptest` test names and scenarios.
- **Status:** `HANDOFF` (or `BLOCKED` with the unmet precondition)
