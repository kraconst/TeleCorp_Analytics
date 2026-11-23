# Handoff Report: Schema & Storage Exploration for Contact & Chat

## 1. Observation

- **Existing Migrations**:
  - `migrations/000001_init_schema.up.sql` lines 32-47: `accounts` table contains `id BIGSERIAL PRIMARY KEY`, `company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE`, `employee_id BIGINT NOT NULL REFERENCES employees(id)`, `status VARCHAR(32)`, `created_at TIMESTAMPTZ`.
  - `migrations/000002_create_messages.up.sql` lines 1-12: `messages` table contains `chat_id BIGINT NOT NULL`, `telegram_message_id BIGINT NOT NULL`, with `UNIQUE (chat_id, telegram_message_id)` and index `idx_messages_chat_id_sent_at` on `(chat_id, sent_at ASC)`.
  - `migrations/000001_init_schema.down.sql` lines 1-5 and `000002_create_messages.down.sql` line 1: drop tables in reverse dependency order using `DROP TABLE IF EXISTS <table_name>;`.
- **Existing Domain Models**:
  - `internal/domain/contact.go` lines 4-10: `Contact` struct has JSON tags (`id`, `company_id`, `telegram_user_id`, `name`, `username`) but lacks `db` struct tags.
  - `internal/domain/chat.go` lines 5-43: defines enums `ChatStatus` (lines 8-15), `StatusSource` (lines 20-22), `ChatPriority` (lines 27-30), and struct `Chat` (lines 33-43) which has JSON tags but lacks `db` tags. `ChatFilter` is not yet defined.
  - `internal/domain/errors.go` lines 6-9: sentinel errors `ErrNotFound`, `ErrForbidden`, `ErrConflict`, `ErrValidation`.
- **Existing Repository Interfaces**:
  - `internal/usecase/interfaces.go` lines 10-38: contracts for `AccountRepository`, `EmployeeRepository`, `CompanyRepository`, and `MessageRepository`. Lacks `ContactRepository` and `ChatRepository`.
- **Existing Storage Implementations**:
  - `internal/storage/postgres/account.go` lines 60-78: `GetByID` maps `sql.ErrNoRows` to `domain.ErrNotFound`.
  - `internal/storage/postgres/account.go` lines 116-130: `UpdateStatus` checks `rows == 0` from `RowsAffected()` and returns `domain.ErrNotFound`.
  - `internal/storage/postgres/message.go` lines 21-45: `Create` uses `QueryRowContext` and returns generated `id`.
  - `internal/storage/postgres/message.go` lines 47-60: `ListByChat` initializes empty slice `make([]domain.Message, 0)` so it never returns nil slice.
- **Specification and Requirements**:
  - `AGENTS.md` lines 43-130 and `ORIGINAL_REQUEST.md` lines 12-40: specify table schema for `contacts` and `chats`, struct tags, repository methods, and storage query patterns.

## 2. Logic Chain

1. From **Observation** (`migrations/000001_init_schema.up.sql` and `000002_create_messages.up.sql`), migrations use `000001_` and `000002_` prefixes. Therefore, the new migrations must be named `migrations/000003_create_contacts_and_chats.up.sql` and `migrations/000003_create_contacts_and_chats.down.sql`.
2. `chats` table has foreign key `contact_id BIGINT REFERENCES contacts(id) ON DELETE CASCADE`. Thus, in `000003_create_contacts_and_chats.up.sql`, `contacts` must be created first; in `000003_create_contacts_and_chats.down.sql`, `chats` must be dropped before `contacts`.
3. In `internal/domain/contact.go` and `internal/domain/chat.go`, sqlx relies on `db` struct tags matching the SQL column names (`id`, `company_id`, `telegram_user_id`, `name`, `username` for Contact; `id`, `account_id`, `contact_id`, `status`, `status_source`, `priority`, `last_message_at`, `awaiting_reply_since`, `unread_count` for Chat). Adding these tags is required for `s.db.GetContext` and `s.db.SelectContext` to populate struct fields.
4. `chats` does not store `company_id` or `employee_id`. As observed in `000001_init_schema.up.sql`, `accounts` has `company_id` and `employee_id`. Therefore, `ChatStorage.ListByCompany` must join `accounts a ON chats.account_id = a.id` and filter on `a.company_id = $1`, `($2::varchar IS NULL OR c.status = $2)`, and `($3::bigint IS NULL OR a.employee_id = $3)` ordered by `c.last_message_at DESC, c.id DESC`.
5. For error consistency, storage methods must translate `sql.ErrNoRows` on `GetContext` and `RowsAffected() == 0` on `UpdateStatus` to `domain.ErrNotFound`, matching the existing pattern in `internal/storage/postgres/account.go` and `employee.go`.
6. To satisfy R1-R3, blueprints have been synthesized and documented in `analysis.md` with complete, copy-paste ready implementations.

## 3. Caveats

- Docker daemon is currently inaccessible in the sandbox environment, so live PostgreSQL execution was not run during this read-only exploration turn. However, syntax and query semantics were verified against PostgreSQL 14+ documentation and existing codebase patterns.
- `ChatFilter` is provided in `internal/domain/chat.go` for convenient usage by handlers and usecase layers; the `ChatRepository.ListByCompany` contract takes `(ctx, companyID, status, employeeID)` matching the explicit signature in `AGENTS.md` and `SCOPE.md`.

## 4. Conclusion

The schema migrations, domain struct tags, repository contracts, and PostgreSQL storage components are fully designed and aligned with the existing Clean Architecture patterns:
1. `migrations/000003_create_contacts_and_chats.up.sql` and `down.sql` provide exact DDL, cascade constraints, and indexes.
2. `internal/domain/contact.go` and `internal/domain/chat.go` blueprints provide matching `db` tags and `ChatFilter`.
3. `internal/usecase/interfaces.go` defines `ContactRepository` and `ChatRepository`.
4. `internal/storage/postgres/contact.go` and `internal/storage/postgres/chat.go` provide complete sqlx implementations with atomic upsert, join querying, nullable filter parameters, and sentinel error translation.

The complete code blueprints are documented in `.agents/teamwork/explorer_m1_1/analysis.md`.

## 5. Verification Method

To verify the blueprints upon implementation:
1. Check Go syntax and compilation:
   ```sh
   go fmt ./...
   go vet ./...
   go test ./...
   go build ./...
   ```
2. Verify migration syntax and execution with PostgreSQL:
   - Ensure `migrations/000003_create_contacts_and_chats.up.sql` applies cleanly without foreign key or constraint errors.
   - Ensure `migrations/000003_create_contacts_and_chats.down.sql` drops `chats` then `contacts` cleanly.
3. Invalidation conditions:
   - Any column mismatch between `contacts`/`chats` DDL and `db` tags in `internal/domain/`.
   - `ChatStorage.ListByCompany` failing to order by `last_message_at DESC, id DESC`.
   - `UpdateStatus` returning `nil` when the chat ID does not exist instead of `domain.ErrNotFound`.
