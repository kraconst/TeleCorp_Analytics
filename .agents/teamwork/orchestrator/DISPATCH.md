# Dispatch Log

## 2026-11-10T13:53:37Z

Implement the Contact and Chat entities end-to-end for production in the GO_server codebase, following the specification in AGENTS.md lines 43-130.

Working directory: /home/asffe/3sem/GoLangCourse/GO_server
Integrity mode: development

## Requirements

### R1. Database Migrations
Create migration files `migrations/000003_create_contacts_and_chats.up.sql` and `migrations/000003_create_contacts_and_chats.down.sql`.
- The `contacts` table must contain: `id BIGSERIAL PRIMARY KEY`, `company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE`, `telegram_user_id BIGINT NOT NULL`, `name VARCHAR(255) NOT NULL`, `username VARCHAR(255) NOT NULL DEFAULT ''`, `UNIQUE (company_id, telegram_user_id)`, and index `idx_contacts_company_id ON contacts(company_id)`.
- The `chats` table must contain: `id BIGSERIAL PRIMARY KEY`, `account_id BIGINT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE`, `contact_id BIGINT NOT NULL REFERENCES contacts(id) ON DELETE CASCADE`, `status VARCHAR(32) NOT NULL DEFAULT 'new_lead'`, `status_source VARCHAR(16) NOT NULL DEFAULT 'auto' CHECK (status_source IN ('auto', 'manual'))`, `priority VARCHAR(16) NOT NULL DEFAULT 'normal' CHECK (priority IN ('low', 'normal', 'high'))`, `last_message_at TIMESTAMPTZ NOT NULL DEFAULT NOW()`, `awaiting_reply_since TIMESTAMPTZ`, `unread_count INT NOT NULL DEFAULT 0`, `UNIQUE (account_id, contact_id)`, and indexes on `account_id`, `contact_id`, `status`, and `last_message_at DESC`.
- The down migration must drop `chats` and `contacts` if they exist.

### R2. Domain Layer
In `internal/domain/contact.go` and `internal/domain/chat.go`, add `db` struct tags matching the column names. Provide a `ChatFilter` struct with `CompanyID int64`, `EmployeeID *int64`, and `Status *ChatStatus`.

### R3. Repository Contracts and PostgreSQL Storage
- Define `ContactRepository` and `ChatRepository` in `internal/usecase/interfaces.go` matching the signatures specified in AGENTS.md.
- Implement `ContactStorage` and `ChatStorage` in `internal/storage/postgres/` using `sqlx`.
- `ContactStorage.Create` must insert or update on conflict `(company_id, telegram_user_id)` and return the contact id.
- `ChatStorage.ListByCompany` must join `accounts` on `chats.account_id = accounts.id` and filter on `company_id` and optional `employee_id` and `status`, ordered by `last_message_at DESC, id DESC`.
- Translate `sql.ErrNoRows` (and 0 rows affected on status updates) to `domain.ErrNotFound`.

### R4. Usecase Layer
- Implement `ContactUsecase` and `ChatUsecase` in `internal/usecase/`.
- Validate required inputs (e.g., IDs > 0, non-empty trimmed contact name, valid allowed `domain.ChatStatus` values), wrapping invalid inputs with `domain.ErrValidation`.
- Implement `SetManualStatus` to update chat status with `StatusSourceManual`.

### R5. HTTP Handlers and Composition Root
- Extend `internal/handler/handler.go` to support `ContactUsecase` and `ChatUsecase`, registering routes:
  - `GET /chats`: query parameters `status` and `employee_id`, company ID 1.
  - `GET /chats/:id`: returns chat by ID.
  - `POST /chats/:id/status`: updates status manually from JSON body.
- When PostgreSQL is unavailable at startup (handlers receive `nil` usecases), fall back to in-memory `mockContacts` and `mockChats` slices.
- Map `domain.ErrValidation` to HTTP 400, `domain.ErrNotFound` to HTTP 404, and unexpected errors to HTTP 500.
- Wire new repositories, usecases, and handlers in `cmd/app/main.go`.

## Acceptance Criteria

### Schema and Migrations
- [ ] `migrations/000003_create_contacts_and_chats.up.sql` and `migrations/000003_create_contacts_and_chats.down.sql` exist and contain the required tables, columns, constraints, and indexes.

### Code Quality and Compilation
- [ ] `go fmt ./...` produces no diff.
- [ ] `go vet ./...` succeeds with exit code 0.
- [ ] `go test ./...` succeeds with exit code 0.
- [ ] `go build ./...` succeeds with exit code 0.

### Functional Verification
- [ ] HTTP endpoint `GET /chats` returns 200 with chat list, supporting optional `status` and `employee_id` query filters.
- [ ] HTTP endpoint `GET /chats/:id` returns 200 with the chat or 404 if not found, and 400 for invalid ID.
- [ ] HTTP endpoint `POST /chats/:id/status` returns 200 on status update, 400 on invalid status, and 404 if chat not found.
- [ ] When PostgreSQL is unavailable, endpoints respond using mock fallback data without crashing.
