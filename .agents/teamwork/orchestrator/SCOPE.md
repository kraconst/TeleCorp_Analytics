# Scope: Contact and Chat Entities

## Architecture
Clean Architecture conforming to repository reference implementations (Account, Employee, Message).
- Migrations: `migrations/000003_create_contacts_and_chats.up.sql` & `.down.sql`
- Domain: `internal/domain/contact.go`, `internal/domain/chat.go`
- Repository Contracts: `internal/usecase/interfaces.go`
- Storage: `internal/storage/postgres/contact.go`, `internal/storage/postgres/chat.go`
- Usecase: `internal/usecase/contact.go`, `internal/usecase/chat.go`
- HTTP Handler: `internal/handler/handler.go` (including mock fallback)
- Composition Root: `cmd/app/main.go`

## Feature Inventory
| # | Feature | Description | Milestone | Source |
|---|---------|-------------|-----------|--------|
| 1 | Database Migrations | Contacts and chats tables with up/down SQL, foreign keys, indexes | M1 | AGENTS.md § 1 |
| 2 | Domain Layer | Struct tags db and ChatFilter struct | M1 | AGENTS.md § 2 |
| 3 | Repository Contracts | ContactRepository and ChatRepository interfaces | M1 | AGENTS.md § 3 |
| 4 | Postgres Storage | sqlx ContactStorage and ChatStorage implementations with error mapping | M1 | AGENTS.md § 4 |
| 5 | Usecase Layer | ContactUsecase and ChatUsecase with input validation | M1 | AGENTS.md § 5 |
| 6 | HTTP Handlers & Mock Fallback | GET /chats, GET /chats/:id, POST /chats/:id/status, mocks | M1 | AGENTS.md § 6 |
| 7 | Main Wiring | Wiring in cmd/app/main.go | M1 | AGENTS.md § 7 |

## Milestones
| # | Name | Scope | Dependencies | Status |
|---|------|-------|-------------|--------|
| 1 | Contact + Chat End-to-End | Complete implementation of R1-R5 and verification | none | DONE |

## Interface Contracts
- `ContactRepository`:
  - `Create(ctx context.Context, c domain.Contact) (int64, error)`
  - `GetByID(ctx context.Context, id int64) (*domain.Contact, error)`
  - `GetByTelegramID(ctx context.Context, companyID, tgUserID int64) (*domain.Contact, error)`
  - `ListByCompany(ctx context.Context, companyID int64) ([]domain.Contact, error)`
- `ChatRepository`:
  - `Create(ctx context.Context, chat domain.Chat) (int64, error)`
  - `GetByID(ctx context.Context, id int64) (*domain.Chat, error)`
  - `ListByCompany(ctx context.Context, companyID int64, status *domain.ChatStatus, employeeID *int64) ([]domain.Chat, error)`
  - `UpdateStatus(ctx context.Context, id int64, status domain.ChatStatus, source domain.StatusSource) error`

## Code Layout
- `migrations/000003_create_contacts_and_chats.up.sql`
- `migrations/000003_create_contacts_and_chats.down.sql`
- `internal/domain/contact.go`
- `internal/domain/chat.go`
- `internal/usecase/interfaces.go`
- `internal/storage/postgres/contact.go`
- `internal/storage/postgres/chat.go`
- `internal/usecase/contact.go`
- `internal/usecase/chat.go`
- `internal/handler/handler.go`
- `cmd/app/main.go`
