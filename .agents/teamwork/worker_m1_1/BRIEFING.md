# BRIEFING — 2026-11-10T14:15:00Z

## Mission
Implement Contact and Chat entities end-to-end for production in GO_server (migrations, domain, repositories, postgres storage, usecases, HTTP handlers, mock fallback, main wiring, and unit tests).

## 🔒 My Identity
- Archetype: implementer
- Roles: implementer, qa
- Working directory: /home/asffe/3sem/GoLangCourse/GO_server
- State folder: /home/asffe/3sem/GoLangCourse/GO_server/.agents/teamwork/worker_m1_1/
- Original parent: bdf1e875-d4c5-41f0-9ccd-bdd7bf772375
- Milestone: Milestone 1 (Contact + Chat End-to-End)

## 🔒 Key Constraints
- Conformance to AGENTS.md lines 43-130 and ORIGINAL_REQUEST.md requirements R1-R5.
- Migration numbering: `migrations/000003_create_contacts_and_chats.up.sql` and `down.sql`.
- Drop order in down migration: drop `chats` before `contacts`.
- Domain models: add `db` tags; define `ChatFilter`.
- Interfaces in `internal/usecase/interfaces.go`: `ContactRepository` and `ChatRepository`.
- Storage: postgres `ContactStorage` and `ChatStorage` using `sqlx`; map `sql.ErrNoRows` and 0 rows affected to `domain.ErrNotFound`.
- Contact upsert: `ON CONFLICT (company_id, telegram_user_id) DO UPDATE SET name = EXCLUDED.name, username = EXCLUDED.username RETURNING id`.
- Chat list: join `accounts` on `chats.account_id = accounts.id`, filter on company_id, optional status and employee_id, order by `last_message_at DESC, id DESC`.
- Usecases: validate required inputs wrapping `domain.ErrValidation`; implement `SetManualStatus`.
- Handlers: mock fallback for `mockContacts` and `mockChats` when postgres is offline; update `mockAccounts` with company/employee IDs.
- Error mapping: `domain.ErrValidation` -> 400, `domain.ErrNotFound` -> 404, unexpected -> 500.
- Mandatory verification: `go fmt ./...`, `go vet ./...`, `go test ./...`, `go build ./...`.

## Current Parent
- Conversation ID: bdf1e875-d4c5-41f0-9ccd-bdd7bf772375
- Updated: 2026-11-10T14:15:00Z

## Task Summary
- **What to build**: End-to-end Contact & Chat entities (DB schema, domain models, interfaces, storage, usecase, HTTP handler, main wiring, unit tests).
- **Success criteria**: All acceptance criteria in ORIGINAL_REQUEST.md met, tests pass, formatting clean, vet passes, build passes.
- **Interface contracts**: `.agents/teamwork/orchestrator/SCOPE.md`
- **Code layout**: Migrations in `migrations/`, domain in `internal/domain/`, interfaces & usecases in `internal/usecase/`, storage in `internal/storage/postgres/`, handlers in `internal/handler/`, entrypoint in `cmd/app/`.

## Key Decisions Made
- Down migration drops `chats` then `contacts` due to foreign key dependency.
- Slices initialized to `make([]T, 0)` so JSON responses serialize as `[]` instead of `null`.
- Contact upsert handles concurrency cleanly via Postgres `ON CONFLICT`.
- Chat listing filters via account join and nullable parameter casting `$2::varchar IS NULL OR ...`.
- Comprehensive unit test coverage added for usecases and handlers (both mock fallback and usecase-backed execution).

## Artifact Index
- `migrations/000003_create_contacts_and_chats.up.sql` — Schema migration for contacts and chats tables, constraints, indexes
- `migrations/000003_create_contacts_and_chats.down.sql` — Down migration dropping chats then contacts
- `internal/domain/contact.go` — Contact domain model with db tags
- `internal/domain/chat.go` — Chat domain model with db tags, enums, and ChatFilter
- `internal/usecase/interfaces.go` — ContactRepository and ChatRepository contracts
- `internal/storage/postgres/contact.go` — PostgreSQL ContactStorage implementation
- `internal/storage/postgres/chat.go` — PostgreSQL ChatStorage implementation
- `internal/usecase/contact.go` — Contact business logic and input validation
- `internal/usecase/chat.go` — Chat business logic and input validation
- `internal/handler/handler.go` — Gin HTTP handlers, route registration, and mock fallback
- `cmd/app/main.go` — Composition root wiring Contact and Chat storages, usecases, and handlers
- `internal/usecase/contact_test.go` — Unit tests for Contact usecase
- `internal/usecase/chat_test.go` — Unit tests for Chat usecase
- `internal/handler/handler_test.go` — Unit tests for HTTP handlers and mock fallback

## Change Tracker
- **Files modified**:
  - `migrations/000003_create_contacts_and_chats.up.sql` (created)
  - `migrations/000003_create_contacts_and_chats.down.sql` (created)
  - `internal/domain/contact.go` (added db tags)
  - `internal/domain/chat.go` (added db tags and ChatFilter)
  - `internal/usecase/interfaces.go` (added repository contracts)
  - `internal/storage/postgres/contact.go` (created)
  - `internal/storage/postgres/chat.go` (created)
  - `internal/usecase/contact.go` (created)
  - `internal/usecase/chat.go` (created)
  - `internal/handler/handler.go` (extended handler, routes, mocks)
  - `cmd/app/main.go` (wired storages, usecases, handlers)
  - `internal/usecase/contact_test.go` (created)
  - `internal/usecase/chat_test.go` (created)
  - `internal/handler/handler_test.go` (created)
- **Build status**: PASS
- **Pending issues**: None.

## Quality Status
- **Build/test result**: All unit tests pass, `go vet` passes with 0 issues.
- **Lint status**: Clean (`go fmt ./...` produced no diff).
- **Tests added/modified**: 23 new test cases across `internal/usecase` and `internal/handler`.
