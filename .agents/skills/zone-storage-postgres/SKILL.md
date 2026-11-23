---
name: zone-storage-postgres
description: Zone Coder (Tier B) skill for GO_server PostgreSQL repositories (internal/storage/postgres/). Enforces sqlx conventions, verified project error mapping, transactions with FOR UPDATE, explicit column lists, and non-nil slices.
---

# Zone Coder: PostgreSQL Storage (`GO_server`)

Implement scoped repository methods strictly inside `internal/storage/postgres/` according to the active RFC, Research Brief, and contracts in `internal/usecase/interfaces.go`.

## Zone Boundaries & Hard Prohibitions

- **Owned Paths:** `internal/storage/postgres/*.go` (implementation and `*_test.go`).
- **Forbidden:**
  - Never modify `internal/domain/`, `internal/usecase/`, `internal/handler/`, `migrations/`, `cmd/`, `go.mod`, or `Dockerfile`. If an interface signature or schema column is missing, stop and escalate to the Orchestrator.
  - **Never declare the task "Done" / "Готово"** — always hand off as `Status: HANDOFF`; final readiness requires independent Reviewer and Tester.
  - Never put business rules or HTTP logic in storage; storage only persists, queries, locks, and maps DB errors.

## Pre-Flight Check

1. Confirm you have the active RFC, Research Brief, and target interface in `internal/usecase/interfaces.go`.
2. Verify every table and column name against `migrations/*.up.sql` and `db` tags in `internal/domain/`.

## Zone Rules & Repository Standards

- **Driver & Parameterization:** Use `github.com/jmoiron/sqlx` (`QueryRowContext`, `ExecContext`, `GetContext`, `SelectContext`) with positional `$1, $2, ...` placeholders. Always pass `ctx`.
- **Explicit Column Lists:** Never use `SELECT *`. Explicitly enumerate columns matching `internal/domain/` `db` tags. On list queries for `accounts`, omit heavy/sensitive session bytes via `NULL::bytea AS session` (mirror `internal/storage/postgres/account.go`).
- **Mandatory Error Mapping:**
  - Use the error-mapping mechanism confirmed by Research in the current checkout. If `internal/storage/postgres/errors.go` provides `mapPgError`, pass PostgreSQL write/constraint errors through it; do not invent or cite it when absent.
  - On single-row reads (`GetContext` / `Scan`), map `sql.ErrNoRows` to `domain.ErrNotFound`; preserve the project's mappings for conflict and foreign-key errors.
  - Never swallow errors with `_` (except documented `defer tx.Rollback()`).
- **Transactions & Locking:**
  - Any operation modifying 2+ tables or performing read-modify-write must use `tx, err := s.db.BeginTxx(ctx, nil)`, `defer tx.Rollback()`, and `tx.Commit()` (mirror `internal/storage/postgres/account.go`).
  - Lock rows with `FOR UPDATE` when preventing race conditions on state transitions or active assignments.
- **Non-Nil Slices & Deterministic Ordering:**
  - Always initialize list results with `items := make([]domain.T, 0)` so JSON serialization never produces `null`.
  - Always include a deterministic `ORDER BY` (and `LIMIT`/`OFFSET` when paginated).
- **Timestamps:** Normalize defaulted timestamps with `time.Now().UTC()`.
- **Tests:** Add or update focused storage tests in `internal/storage/postgres/*_test.go` (mirror `account_daily_stats_test.go`, `alert_test.go`, `chat_contact_test.go`).

## Coder Handoff Format

Use the complete handoff from `docs/agents/roles/coder.md`; the fields below are zone-specific additions, not a replacement.

- **Changed Files:** exact list of created/modified files.
- **RFC Compliance:** mapping of each method to RFC requirements and `interfaces.go`.
- **Tests Added/Updated:** exact test names and covered cases.
- **Status:** `HANDOFF` (or `BLOCKED` with the unmet precondition)
