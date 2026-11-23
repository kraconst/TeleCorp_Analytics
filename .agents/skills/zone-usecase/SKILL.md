---
name: zone-usecase
description: Zone Coder (Tier B) skill for GO_server business logic (internal/usecase/). Enforces repository interfaces, input validation, strict company_id isolation, IsActive checks, UTC timestamps, and mock-based unit tests.
---

# Zone Coder: Usecase / Business Logic (`GO_server`)

Implement scoped business logic and repository/integration interfaces strictly inside `internal/usecase/` according to the active RFC and Research Brief.

## Zone Boundaries & Hard Prohibitions

- **Owned Paths:** `internal/usecase/*.go` (including `internal/usecase/interfaces.go` and `*_test.go`).
- **Forbidden:**
  - Never import `github.com/gin-gonic/gin`, `net/http`, `github.com/jmoiron/sqlx`, `github.com/jackc/pgx/v5`, `database/sql`, or `github.com/gotd/td` in `internal/usecase/`.
  - Never modify files outside `internal/usecase/` (`domain`, `storage`, `handler`, `migrations`, `cmd/`, `go.mod`, `Dockerfile`). Escalate cross-zone needs to the Orchestrator.
  - **Never declare the task "Done" / "Готово"** — always hand off as `Status: HANDOFF`; final readiness requires independent Reviewer and Tester.
  - Never hardcode `companyID = 1` or `employeeID = 1` in business logic.

## Pre-Flight Check

1. Confirm you have the active RFC (Blocks 2 & 3) and Research Brief.
2. Check existing contracts in `internal/usecase/interfaces.go` before adding new methods or interfaces (`TelegramSyncClient`, `ChatSummarizer`, etc.).

## Zone Rules & Repository Standards

- **Interfaces Only:** Depend exclusively on interfaces declared in `internal/usecase/interfaces.go` and types in `internal/domain/`.
- **Input Validation First:** Validate all IDs (`> 0`), required strings (trimmed), enums, date ranges, and pagination bounds before touching repositories. Return wrapped `domain.ErrValidation` (`fmt.Errorf("%w: ...", domain.ErrValidation)`).
- **Strict Tenant Isolation (`company_id` / IDOR Prevention):**
  - Always verify that the target entity (or its owning account/chat) belongs to the caller's `companyID` **before** returning data or executing any mutation.
  - On cross-company access, return `domain.ErrForbidden`, using the exact reference path identified by Research in the current checkout.
- **Active Entity Checks:** When assigning or operating on employees/accounts, verify `IsActive` (and valid status transitions) before mutating state.
- **UTC & Non-Nil Slices:**
  - Default timestamps using `time.Now().UTC()`.
  - Guard returned slices (`if items == nil { items = make([]domain.T, 0) }`) so callers never receive `nil`.
- **Error Wrapping:** Preserve domain error sentinels with `%w` so `errors.Is` works with the handler's verified error-mapping mechanism. Never swallow errors.
- **Mandatory Unit Tests:** Add table-driven or focused unit tests in `internal/usecase/<entity>_test.go` using in-memory mocks of `interfaces.go` covering at minimum:
  1. Happy path
  2. Validation failure (`domain.ErrValidation`)
  3. Missing entity (`domain.ErrNotFound`)
  4. Cross-company access / IDOR (`domain.ErrForbidden`)
  5. Idempotency / caching / conflict edge cases from RFC Block 2

## Coder Handoff Format

Use the complete handoff from `docs/agents/roles/coder.md`; the fields below are zone-specific additions, not a replacement.

- **Changed Files:** exact list of created/modified files.
- **RFC Compliance:** mapping of implemented rules to RFC Blocks 2 & 3.
- **Tests Added/Updated:** exact test names and edge cases covered.
- **Status:** `HANDOFF` (or `BLOCKED` with the unmet precondition)
