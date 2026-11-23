## 2026-11-10T14:04:54Z
You are Worker 1.
Working directory: /home/asffe/3sem/GoLangCourse/GO_server
Your state folder: /home/asffe/3sem/GoLangCourse/GO_server/.agents/teamwork/worker_m1_1/
Scope document: /home/asffe/3sem/GoLangCourse/GO_server/.agents/teamwork/orchestrator/SCOPE.md
Original user request file (MANDATORY TO READ FIRST): /home/asffe/3sem/GoLangCourse/GO_server/.agents/teamwork/ORIGINAL_REQUEST.md

MANDATORY INTEGRITY WARNING:
DO NOT CHEAT. All implementations must be genuine. DO NOT hardcode test results, create dummy/facade implementations, or circumvent the intended task. A teamwork_auditor will independently verify your work. Integrity violations WILL be detected and your work WILL be rejected.

The Explorers have completed exhaustive investigations and provided complete, copy-paste ready blueprints in:
- Explorer 1 (Schema & Storage): /home/asffe/3sem/GoLangCourse/GO_server/.agents/teamwork/explorer_m1_1/analysis.md
- Explorer 2 (Usecase & Domain): /home/asffe/3sem/GoLangCourse/GO_server/.agents/teamwork/explorer_m1_2/analysis.md
- Explorer 3 (Handler & Main): /home/asffe/3sem/GoLangCourse/GO_server/.agents/teamwork/explorer_m1_3/analysis.md

You have exclusive write ownership of these files:
- migrations/000003_create_contacts_and_chats.up.sql
- migrations/000003_create_contacts_and_chats.down.sql
- internal/domain/contact.go
- internal/domain/chat.go
- internal/usecase/interfaces.go
- internal/storage/postgres/contact.go
- internal/storage/postgres/chat.go
- internal/usecase/contact.go
- internal/usecase/chat.go
- internal/handler/handler.go
- cmd/app/main.go
- unit test files (e.g., in internal/usecase/ or internal/handler/)

Your implementation tasks:
1. Create `migrations/000003_create_contacts_and_chats.up.sql` and `migrations/000003_create_contacts_and_chats.down.sql` with exact tables, columns, constraints, unique keys, and indexes.
2. In `internal/domain/contact.go` and `internal/domain/chat.go`:
   - Add `db` struct tags matching database column names.
   - Add `ChatFilter` struct with `CompanyID int64`, `EmployeeID *int64`, and `Status *ChatStatus`.
3. In `internal/usecase/interfaces.go`:
   - Add `ContactRepository` and `ChatRepository` interface contracts.
4. Implement PostgreSQL storage:
   - `internal/storage/postgres/contact.go`: `ContactStorage`, `NewContactStorage`, `Create` (with ON CONFLICT upsert RETURNING id), `GetByID`, `GetByTelegramID`, `ListByCompany` (mapping `sql.ErrNoRows` to `domain.ErrNotFound`).
   - `internal/storage/postgres/chat.go`: `ChatStorage`, `NewChatStorage`, `Create`, `GetByID`, `ListByCompany` (JOIN accounts on chats.account_id = accounts.id, filter on company_id, optional status and employee_id, ORDER BY last_message_at DESC, id DESC), `UpdateStatus` (mapping `RowsAffected() == 0` to `domain.ErrNotFound`).
5. Implement usecases:
   - `internal/usecase/contact.go`: `ContactUsecase`, `NewContactUsecase`, `Create`, `FindOrCreateByTelegramID`, `GetByID`, `ListByCompany` with validation wrapping `domain.ErrValidation`.
   - `internal/usecase/chat.go`: `ChatUsecase`, `NewChatUsecase`, `SetManualStatus` (calling chats.UpdateStatus with `domain.StatusSourceManual`), `ListByCompany`, `GetByID`, `Create` with validation of allowed 7 statuses wrapping `domain.ErrValidation`.
6. Implement handler and composition root:
   - In `internal/handler/handler.go`:
     * Extend `Handler` and `New` with `ContactUsecase` and `ChatUsecase`.
     * Update `mockAccounts` with `CompanyID: 1`, `EmployeeID: 1` / `2`.
     * Add `mockContacts` and `mockChats` fallback slices.
     * Register routes in `Run`: `GET /chats`, `GET /chats/:id`, `POST /chats/:id/status`.
     * Implement handlers `ListChats`, `GetChatByID`, `UpdateChatStatus` with validation, mock fallback, and error mapping (400 for `domain.ErrValidation`, 404 for `domain.ErrNotFound`, 500 otherwise).
   - In `cmd/app/main.go`:
     * Instantiate `ContactStorage`, `ChatStorage`, `ContactUsecase`, `ChatUsecase`, and wire into `handler.New`.
7. Write unit tests to thoroughly verify the usecase logic, validations, and handler behavior.
8. Run verification commands:
   - `go fmt ./...`
   - `go vet ./...`
   - `go test ./...`
   - `go build ./...`

Write your handoff report to `/home/asffe/3sem/GoLangCourse/GO_server/.agents/teamwork/worker_m1_1/handoff.md`.
Notify me via `send_message` when done.
