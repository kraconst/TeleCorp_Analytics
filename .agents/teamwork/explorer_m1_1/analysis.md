# Schema & Storage Exploration Analysis: Contact & Chat Entities

## 1. Executive Summary

This report documents the architectural investigation and comprehensive code blueprints for the `Contact` and `Chat` entities in `GO_server`, covering:
- Database schema migrations (`migrations/000003_create_contacts_and_chats.up.sql` and `down.sql`).
- Domain struct tags and filter definitions (`internal/domain/contact.go` and `internal/domain/chat.go`).
- Repository interfaces (`internal/usecase/interfaces.go`).
- PostgreSQL storage implementations (`internal/storage/postgres/contact.go` and `internal/storage/postgres/chat.go`).

All designs adhere to Clean Architecture conventions established by reference entities (`Account`, `Employee`, `Message`) in the codebase, with strict error sentinel mapping (`sql.ErrNoRows` and 0 rows affected mapped to `domain.ErrNotFound`), proper foreign key cascades, and consistent Go/SQL type mappings.

---

## 2. Codebase Investigation Findings

### 2.1 Existing Schema Migrations Analysis
- **`migrations/000001_init_schema.up.sql`**:
  - Defines `companies`, `employees`, `users`, `accounts`, `account_assignments`.
  - `accounts`: Primary key `id BIGSERIAL`, `company_id BIGINT REFERENCES companies(id) ON DELETE CASCADE`, `employee_id BIGINT REFERENCES employees(id)`, `telegram_user_id BIGINT DEFAULT 0`, `status VARCHAR(32)`, `created_at TIMESTAMPTZ`.
  - Indexes follow the naming scheme `idx_<table_name>_<column_name>`.
  - All timestamps use `TIMESTAMPTZ NOT NULL DEFAULT NOW()`.
- **`migrations/000002_create_messages.up.sql`**:
  - Defines `messages` table with `chat_id BIGINT NOT NULL`, `telegram_message_id BIGINT NOT NULL`, `UNIQUE (chat_id, telegram_message_id)`.
  - Index: `CREATE INDEX IF NOT EXISTS idx_messages_chat_id_sent_at ON messages(chat_id, sent_at ASC)`.
  - Does NOT enforce foreign key on `chat_id`, allowing `chats` to be created in migration 3 without retrofitting migration 2.
- **Migration Numbering & Down File Ordering**:
  - The next migration pair is `000003_create_contacts_and_chats.up.sql` and `000003_create_contacts_and_chats.down.sql`.
  - `chats` references `contacts(id) ON DELETE CASCADE` and `accounts(id) ON DELETE CASCADE`.
  - Therefore, in the down migration, `chats` MUST be dropped before `contacts` to prevent foreign key constraint violations.

### 2.2 Domain Models Analysis
- **`internal/domain/contact.go`**:
  - Currently contains only `json` struct tags; missing `db` tags.
  - Fields: `ID` (int64), `CompanyID` (int64), `TelegramUserID` (int64), `Name` (string), `Username` (string).
- **`internal/domain/chat.go`**:
  - Contains enum definitions: `ChatStatus` (strings: `new_lead`, `negotiation`, `waiting_reply`, `interested`, `unhappy`, `lost`, `action_required`), `StatusSource` (strings: `auto`, `manual`), `ChatPriority` (strings: `low`, `normal`, `high`).
  - Struct `Chat`: currently has only `json` tags, missing `db` tags.
  - Struct `ChatFilter`: needs to be defined for querying chat lists by company, optional employee, and optional status.
- **`internal/domain/errors.go`**:
  - Sentinel errors: `domain.ErrNotFound`, `domain.ErrValidation`, `domain.ErrConflict`, `domain.ErrForbidden`.
  - Storage must translate `sql.ErrNoRows` and zero-rows-affected updates to `domain.ErrNotFound`.

### 2.3 Storage and Repository Patterns
- **`internal/usecase/interfaces.go`**:
  - Contracts are defined in the usecase package; storage implementations implement them without importing usecases (Dependency Inversion).
  - Storage returns domain models and maps errors to domain errors.
- **`internal/storage/postgres/account.go`, `employee.go`, `message.go`**:
  - Use `*sqlx.DB` with `db.GetContext`, `db.SelectContext`, `db.QueryRowContext`, and `db.ExecContext`.
  - Scan slices initialized to `make([]domain.T, 0)` so JSON serialization yields `[]` rather than `null`.
  - Wrap database errors with `fmt.Errorf("action: %w", err)`.
  - Use `errors.Is(err, sql.ErrNoRows)` to return `nil, domain.ErrNotFound`.
  - On update/deactivate: check `res.RowsAffected() == 0` -> return `domain.ErrNotFound`.

---

## 3. Blueprint 1: Database Migrations

### 3.1 `migrations/000003_create_contacts_and_chats.up.sql`

```sql
-- Contacts
CREATE TABLE IF NOT EXISTS contacts (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    telegram_user_id BIGINT NOT NULL,
    name VARCHAR(255) NOT NULL,
    username VARCHAR(255) NOT NULL DEFAULT '',
    UNIQUE (company_id, telegram_user_id)
);

CREATE INDEX IF NOT EXISTS idx_contacts_company_id ON contacts(company_id);

-- Chats
CREATE TABLE IF NOT EXISTS chats (
    id BIGSERIAL PRIMARY KEY,
    account_id BIGINT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    contact_id BIGINT NOT NULL REFERENCES contacts(id) ON DELETE CASCADE,
    status VARCHAR(32) NOT NULL DEFAULT 'new_lead',
    status_source VARCHAR(16) NOT NULL DEFAULT 'auto' CHECK (status_source IN ('auto', 'manual')),
    priority VARCHAR(16) NOT NULL DEFAULT 'normal' CHECK (priority IN ('low', 'normal', 'high')),
    last_message_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    awaiting_reply_since TIMESTAMPTZ,
    unread_count INT NOT NULL DEFAULT 0,
    UNIQUE (account_id, contact_id)
);

CREATE INDEX IF NOT EXISTS idx_chats_account_id ON chats(account_id);
CREATE INDEX IF NOT EXISTS idx_chats_contact_id ON chats(contact_id);
CREATE INDEX IF NOT EXISTS idx_chats_status ON chats(status);
CREATE INDEX IF NOT EXISTS idx_chats_last_message_at ON chats(last_message_at DESC);
```

### 3.2 `migrations/000003_create_contacts_and_chats.down.sql`

```sql
DROP TABLE IF EXISTS chats;
DROP TABLE IF EXISTS contacts;
```

---

## 4. Blueprint 2: Domain Layer

### 4.1 `internal/domain/contact.go`

```go
package domain

// Contact — собеседник; TelegramUserID уникален в пределах CompanyID.
type Contact struct {
	ID             int64  `json:"id" db:"id"`
	CompanyID      int64  `json:"company_id" db:"company_id"`
	TelegramUserID int64  `json:"telegram_user_id" db:"telegram_user_id"`
	Name           string `json:"name" db:"name"`
	Username       string `json:"username" db:"username"`
}
```

### 4.2 `internal/domain/chat.go`

```go
package domain

import "time"

type ChatStatus string

const (
	ChatStatusNewLead        ChatStatus = "new_lead"
	ChatStatusNegotiation    ChatStatus = "negotiation"
	ChatStatusWaitingReply   ChatStatus = "waiting_reply"
	ChatStatusInterested     ChatStatus = "interested"
	ChatStatusUnhappy        ChatStatus = "unhappy"
	ChatStatusLost           ChatStatus = "lost"
	ChatStatusActionRequired ChatStatus = "action_required"
)

type StatusSource string

const (
	StatusSourceAuto   StatusSource = "auto"
	StatusSourceManual StatusSource = "manual"
)

type ChatPriority string

const (
	ChatPriorityLow    ChatPriority = "low"
	ChatPriorityNormal ChatPriority = "normal"
	ChatPriorityHigh   ChatPriority = "high"
)

// Chat — личный диалог аккаунта с контактом; компания определяется через AccountID.
type Chat struct {
	ID                 int64        `json:"id" db:"id"`
	AccountID          int64        `json:"account_id" db:"account_id"`
	ContactID          int64        `json:"contact_id" db:"contact_id"`
	Status             ChatStatus   `json:"status" db:"status"`
	StatusSource       StatusSource `json:"status_source" db:"status_source"`
	Priority           ChatPriority `json:"priority" db:"priority"`
	LastMessageAt      time.Time    `json:"last_message_at" db:"last_message_at"`
	AwaitingReplySince *time.Time   `json:"awaiting_reply_since" db:"awaiting_reply_since"`
	UnreadCount        int          `json:"unread_count" db:"unread_count"`
}

// ChatFilter задаёт параметры фильтрации списка чатов компании.
type ChatFilter struct {
	CompanyID  int64       `json:"company_id"`
	EmployeeID *int64      `json:"employee_id,omitempty"`
	Status     *ChatStatus `json:"status,omitempty"`
}
```

---

## 5. Blueprint 3: Repository Contracts (`internal/usecase/interfaces.go`)

Append the following interface definitions to `internal/usecase/interfaces.go`:

```go
// ContactRepository определяет контракт доступа к данным контактов.
type ContactRepository interface {
	Create(ctx context.Context, c domain.Contact) (int64, error)
	GetByID(ctx context.Context, id int64) (*domain.Contact, error)
	GetByTelegramID(ctx context.Context, companyID, tgUserID int64) (*domain.Contact, error)
	ListByCompany(ctx context.Context, companyID int64) ([]domain.Contact, error)
}

// ChatRepository определяет контракт доступа к данным чатов.
type ChatRepository interface {
	Create(ctx context.Context, chat domain.Chat) (int64, error)
	GetByID(ctx context.Context, id int64) (*domain.Chat, error)
	ListByCompany(ctx context.Context, companyID int64, status *domain.ChatStatus, employeeID *int64) ([]domain.Chat, error)
	UpdateStatus(ctx context.Context, id int64, status domain.ChatStatus, source domain.StatusSource) error
}
```

---

## 6. Blueprint 4: PostgreSQL Storage Implementations

### 6.1 `internal/storage/postgres/contact.go`

```go
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/kraconst/GO_server/internal/domain"
)

type ContactStorage struct {
	db *sqlx.DB
}

func NewContactStorage(db *sqlx.DB) *ContactStorage {
	return &ContactStorage{db: db}
}

// Create inserts a new contact or updates name/username on (company_id, telegram_user_id) conflict, returning the ID.
func (s *ContactStorage) Create(ctx context.Context, c domain.Contact) (int64, error) {
	query := `
		INSERT INTO contacts (company_id, telegram_user_id, name, username)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (company_id, telegram_user_id)
		DO UPDATE SET name = EXCLUDED.name, username = EXCLUDED.username
		RETURNING id
	`
	var id int64
	err := s.db.QueryRowContext(ctx, query,
		c.CompanyID,
		c.TelegramUserID,
		c.Name,
		c.Username,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("upsert contact: %w", err)
	}
	return id, nil
}

// GetByID returns a contact by its primary key ID.
func (s *ContactStorage) GetByID(ctx context.Context, id int64) (*domain.Contact, error) {
	query := `
		SELECT id, company_id, telegram_user_id, name, username
		FROM contacts
		WHERE id = $1
	`
	var contact domain.Contact
	err := s.db.GetContext(ctx, &contact, query, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("select contact by id: %w", err)
	}
	return &contact, nil
}

// GetByTelegramID returns a contact by company ID and Telegram user ID.
func (s *ContactStorage) GetByTelegramID(ctx context.Context, companyID, tgUserID int64) (*domain.Contact, error) {
	query := `
		SELECT id, company_id, telegram_user_id, name, username
		FROM contacts
		WHERE company_id = $1 AND telegram_user_id = $2
	`
	var contact domain.Contact
	err := s.db.GetContext(ctx, &contact, query, companyID, tgUserID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("select contact by telegram id: %w", err)
	}
	return &contact, nil
}

// ListByCompany returns all contacts belonging to the given company.
func (s *ContactStorage) ListByCompany(ctx context.Context, companyID int64) ([]domain.Contact, error) {
	query := `
		SELECT id, company_id, telegram_user_id, name, username
		FROM contacts
		WHERE company_id = $1
		ORDER BY id ASC
	`
	contacts := make([]domain.Contact, 0)
	err := s.db.SelectContext(ctx, &contacts, query, companyID)
	if err != nil {
		return nil, fmt.Errorf("list contacts by company: %w", err)
	}
	return contacts, nil
}
```

### 6.2 `internal/storage/postgres/chat.go`

```go
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kraconst/GO_server/internal/domain"
)

type ChatStorage struct {
	db *sqlx.DB
}

func NewChatStorage(db *sqlx.DB) *ChatStorage {
	return &ChatStorage{db: db}
}

// Create inserts a new chat with default fallbacks and returns the generated ID.
func (s *ChatStorage) Create(ctx context.Context, chat domain.Chat) (int64, error) {
	status := chat.Status
	if status == "" {
		status = domain.ChatStatusNewLead
	}
	source := chat.StatusSource
	if source == "" {
		source = domain.StatusSourceAuto
	}
	priority := chat.Priority
	if priority == "" {
		priority = domain.ChatPriorityNormal
	}
	lastMessageAt := chat.LastMessageAt
	if lastMessageAt.IsZero() {
		lastMessageAt = time.Now().UTC()
	}

	query := `
		INSERT INTO chats (
			account_id, contact_id, status, status_source, priority,
			last_message_at, awaiting_reply_since, unread_count
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id
	`
	var id int64
	err := s.db.QueryRowContext(ctx, query,
		chat.AccountID,
		chat.ContactID,
		status,
		source,
		priority,
		lastMessageAt,
		chat.AwaitingReplySince,
		chat.UnreadCount,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert chat: %w", err)
	}
	return id, nil
}

// GetByID returns a chat by ID or domain.ErrNotFound if not found.
func (s *ChatStorage) GetByID(ctx context.Context, id int64) (*domain.Chat, error) {
	query := `
		SELECT id, account_id, contact_id, status, status_source, priority,
		       last_message_at, awaiting_reply_since, unread_count
		FROM chats
		WHERE id = $1
	`
	var chat domain.Chat
	err := s.db.GetContext(ctx, &chat, query, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("select chat by id: %w", err)
	}
	return &chat, nil
}

// ListByCompany joins accounts to filter by company and optional employee and status.
func (s *ChatStorage) ListByCompany(ctx context.Context, companyID int64, status *domain.ChatStatus, employeeID *int64) ([]domain.Chat, error) {
	query := `
		SELECT c.id, c.account_id, c.contact_id, c.status, c.status_source, c.priority,
		       c.last_message_at, c.awaiting_reply_since, c.unread_count
		FROM chats c
		JOIN accounts a ON c.account_id = a.id
		WHERE a.company_id = $1
		  AND ($2::varchar IS NULL OR c.status = $2)
		  AND ($3::bigint IS NULL OR a.employee_id = $3)
		ORDER BY c.last_message_at DESC, c.id DESC
	`
	var statusVal *string
	if status != nil {
		str := string(*status)
		statusVal = &str
	}

	chats := make([]domain.Chat, 0)
	err := s.db.SelectContext(ctx, &chats, query, companyID, statusVal, employeeID)
	if err != nil {
		return nil, fmt.Errorf("list chats by company: %w", err)
	}
	return chats, nil
}

// UpdateStatus updates chat status and source, returning domain.ErrNotFound if no rows matched.
func (s *ChatStorage) UpdateStatus(ctx context.Context, id int64, status domain.ChatStatus, source domain.StatusSource) error {
	query := `
		UPDATE chats
		SET status = $1, status_source = $2
		WHERE id = $3
	`
	res, err := s.db.ExecContext(ctx, query, status, source, id)
	if err != nil {
		return fmt.Errorf("update chat status: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}
	if rows == 0 {
		return domain.ErrNotFound
	}
	return nil
}
```

---

## 7. Key Decisions & Architectural Verification

1. **Resolution of Company & Employee on `Chat`**:
   - `chats` does not store `company_id` or `employee_id`. The linkage is:
     `chats.account_id -> accounts.id`, where `accounts.company_id` identifies the company and `accounts.employee_id` identifies the responsible employee.
   - The query in `ChatStorage.ListByCompany` joins `accounts a ON c.account_id = a.id` and applies predicates `a.company_id = $1`, `($2::varchar IS NULL OR c.status = $2)`, and `($3::bigint IS NULL OR a.employee_id = $3)`.
2. **Nullable Query Parameters**:
   - In `ChatStorage.ListByCompany`, `status` is converted from `*domain.ChatStatus` to `*string` (or `nil`), and `employeeID` is passed as `*int64` (or `nil`).
   - PostgreSQL casts `$2::varchar` and `$3::bigint` allow `$X IS NULL OR column = $X` to execute correctly regardless of whether the parameter is `NULL` or a concrete value.
3. **Upsert Semantics on `Contact`**:
   - The unique constraint `(company_id, telegram_user_id)` on `contacts` powers `ON CONFLICT (company_id, telegram_user_id) DO UPDATE SET name = EXCLUDED.name, username = EXCLUDED.username RETURNING id`.
   - This provides atomic find-or-create/update behavior without race conditions.
4. **Error Mapping Consistency**:
   - Every `SELECT` using `GetContext` maps `sql.ErrNoRows` to `domain.ErrNotFound`.
   - Every `UPDATE` checks `RowsAffected() == 0` and returns `domain.ErrNotFound`.
   - Handlers can reliably map `domain.ErrNotFound` to HTTP 404.
5. **Slice Initialization Discipline**:
   - All list queries (`ListByCompany` for contacts and chats) initialize slices with `make([]T, 0)` so that empty results serialize as `[]` in JSON instead of `null`.
