---
name: zone-domain-migrations
description: Zone Coder (Tier B) skill for GO_server domain models and SQL migrations (internal/domain/ and migrations/). Enforces sequential idempotent migrations, constraints, indexes, and zero-dependency domain structs.
---

# Zone Coder: Domain & Migrations (`GO_server`)

Implement scoped changes strictly inside `internal/domain/` and `migrations/` according to the active RFC and Research Brief.

## Zone Boundaries & Hard Prohibitions

- **Owned Paths:** `internal/domain/*.go`, `migrations/*.up.sql`, `migrations/*.down.sql`.
- **Forbidden:**
  - Never modify files outside `internal/domain/` and `migrations/` (no storage SQL, usecases, handlers, `cmd/`, `go.mod`, or `Dockerfile`). If a change requires another zone, stop and escalate to the Orchestrator.
  - **Never declare the task "Done" / "Готово"** — always hand off as `Status: HANDOFF`; final readiness requires independent Reviewer and Tester.
  - Never import `sqlx`, `pgx`, `database/sql`, `gin`, `net/http`, or external SDKs inside `internal/domain/`.

## Pre-Flight Check

1. Confirm you have the active RFC (Blocks 3 & 5) and Research Brief.
2. Check the current highest migration number in `migrations/` on the actual base. Use the next sequential 6-digit number and verify no duplicate prefix exists; coordinate numbering across concurrent branches.

## Zone Rules & Repository Standards

### SQL Migrations (`migrations/`)
- **Paired files:** Always create both `NNNNNN_<snake_case>.up.sql` and `NNNNNN_<snake_case>.down.sql`.
- **Migration runner and idempotency:** Research must establish how the current checkout applies migrations and whether re-runs are possible. Match that mechanism instead of assuming a `RunMigrations` helper.
  - Where compatible with the verified runner and RFC, use idempotent guards such as `CREATE TABLE IF NOT EXISTS`, `CREATE INDEX IF NOT EXISTS`, and `ALTER TABLE ... ADD COLUMN IF NOT EXISTS`.
  - Guard constraints using the closest verified migration pattern in the current base; do not invent a baseline or duplicate constraint names.
- **Constraints & Indexes:**
  - Enforce domain enums with explicit `CHECK` constraints (e.g., `chk_accounts_status`).
  - Enforce tenant/entity uniqueness with `UNIQUE` constraints or partial unique indexes (`WHERE to_time IS NULL`, `WHERE resolved_at IS NULL`).
  - Add indexes for every frequent filter, join, or sort key (`company_id`, `chat_id`, `account_id`, `telegram_message_id`, timestamps).
- **Symmetric `.down.sql`:** Drop indexes, constraints, columns, and tables in strict reverse dependency order using `IF EXISTS`, following a verified migration from the current base.

### Domain Models (`internal/domain/`)
- **Zero external dependencies:** Only Go standard library (e.g., `time`, `errors`).
- **Struct tags:** Every persisted field must have matching `json:"..."` and `db:"..."` tags aligned 1:1 with migration column names.
- **Sensitive fields:** Hide secrets/sessions from JSON serialization using `json:"-"` (mirror `Session []byte \`json:"-" db:"session"\`` in `internal/domain/account.go`).
- **Domain errors:** Reuse sentinel errors in `internal/domain/errors.go` (`ErrNotFound`, `ErrForbidden`, `ErrConflict`, `ErrValidation`). Add new sentinels only if explicitly specified in RFC Block 3.

## Coder Handoff Format

Use the complete handoff from `docs/agents/roles/coder.md`; the fields below are zone-specific additions, not a replacement.

- **Changed Files:** exact list of created/modified files.
- **RFC Compliance:** mapping of each change to RFC Block 3 / Block 5 items.
- **Next Zone Needed:** downstream zone(s) that should pick up the new models/schema (typically `zone-storage-postgres`).
- **Status:** `HANDOFF` (or `BLOCKED` with the unmet precondition)
