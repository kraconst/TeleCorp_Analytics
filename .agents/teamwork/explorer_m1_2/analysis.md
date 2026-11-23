# Analysis Report: Usecase & Domain Logic Exploration (Contact & Chat)

**Date**: 2026-11-10  
**Explorer**: Explorer 2 (`explorer_m1_2`)  
**Scope**: Domain layer models & errors, Usecase layer interfaces & implementations for `Contact` and `Chat` entities  
**Target Files Analyzed**:
- `internal/domain/errors.go`
- `internal/domain/contact.go`
- `internal/domain/chat.go`
- `internal/domain/account.go`, `employee.go`, `message.go`
- `internal/usecase/interfaces.go`
- `internal/usecase/account.go`
- `internal/usecase/employee.go`
- `internal/usecase/message.go`
- `internal/handler/handler.go`
- `cmd/app/main.go`

---

## 1. Domain Layer Investigation

### 1.1 Existing Sentinel Errors (`internal/domain/errors.go`)
```go
package domain

import "errors"

var (
	ErrNotFound   = errors.New("not found")
	ErrForbidden  = errors.New("forbidden")
	ErrConflict   = errors.New("conflict")
	ErrValidation = errors.New("validation error")
)
```

#### Conventions & Error Semantics:
1. **`domain.ErrValidation`**:
   - Used across usecases whenever parameter validation fails (e.g. non-positive IDs, empty required strings, invalid enum values).
   - Wrapped via `fmt.Errorf("%w: <detail>", domain.ErrValidation, ...)` so that callers can inspect it using `errors.Is(err, domain.ErrValidation)`.
   - Handlers inspect `errors.Is(err, domain.ErrValidation)` to respond with HTTP `400 Bad Request` and `gin.H{"error": err.Error()}`.
2. **`domain.ErrNotFound`**:
   - Returned by repository/storage implementations when `sql.ErrNoRows` or 0 rows affected occurs.
   - Preserved through usecases so handlers can inspect `errors.Is(err, domain.ErrNotFound)` and return HTTP `404 Not Found`.
3. **`domain.ErrConflict` & `domain.ErrForbidden`**:
   - Reserved for business conflicts (e.g. duplicate resource) and permission violations.

---

### 1.2 Domain Models (`internal/domain/contact.go` & `internal/domain/chat.go`)

#### Current `internal/domain/contact.go`:
```go
package domain

// Contact — собеседник; TelegramUserID уникален в пределах CompanyID.
type Contact struct {
	ID             int64  `json:"id"`
	CompanyID      int64  `json:"company_id"`
	TelegramUserID int64  `json:"telegram_user_id"`
	Name           string `json:"name"`
	Username       string `json:"username"`
}
```
**Required Changes (R2)**:
Add `db` struct tags for sqlx scanning:
```go
type Contact struct {
	ID             int64  `json:"id" db:"id"`
	CompanyID      int64  `json:"company_id" db:"company_id"`
	TelegramUserID int64  `json:"telegram_user_id" db:"telegram_user_id"`
	Name           string `json:"name" db:"name"`
	Username       string `json:"username" db:"username"`
}
```

#### Current `internal/domain/chat.go`:
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
	ID                 int64        `json:"id"`
	AccountID          int64        `json:"account_id"`
	ContactID          int64        `json:"contact_id"`
	Status             ChatStatus   `json:"status"`
	StatusSource       StatusSource `json:"status_source"`
	Priority           ChatPriority `json:"priority"`
	LastMessageAt      time.Time    `json:"last_message_at"`
	AwaitingReplySince *time.Time   `json:"awaiting_reply_since"`
	UnreadCount        int          `json:"unread_count"`
}
```
**Required Changes (R2)**:
1. Add `db` struct tags to `Chat`:
```go
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
```
2. Add `ChatFilter` struct:
```go
type ChatFilter struct {
	CompanyID  int64
	EmployeeID *int64
	Status     *ChatStatus
}
```

---

## 2. Existing Usecase Layer Patterns

Analysis of `internal/usecase/account.go`, `internal/usecase/employee.go`, and `internal/usecase/message.go` reveals clear architectural conventions:

| Pattern | Observed Convention | Example in Codebase |
|---|---|---|
| **Constructor** | `New<Entity>Usecase(<entity>s <Entity>Repository) *<Entity>Usecase` | `NewAccountUsecase(accounts AccountRepository)` |
| **Encapsulation** | Private repository field on struct | `type AccountUsecase struct { accounts AccountRepository }` |
| **Validation Failures** | Wrap `domain.ErrValidation` with `%w` | `fmt.Errorf("%w: invalid id", domain.ErrValidation)` |
| **String Sanitization** | `strings.TrimSpace(input)` before checking emptiness | `name := strings.TrimSpace(input.Name); if name == "" { ... }` |
| **Creation & Reload** | Insert entity, get `id`, re-query via `GetByID(ctx, id)`. Fallback to returning entity with `ID = id` if re-query fails | In `account.go` lines 44-55, `employee.go` lines 40-50 |
| **Passthrough Errors** | Return repository error unchanged or wrapped with `%w` to preserve `domain.ErrNotFound` | In `employee.go` lines 73, 80; `account.go` lines 62, 73 |
| **Context Propagation** | `ctx context.Context` as first parameter in every method | Universal across all usecase methods |

---

## 3. Repository Interfaces (`internal/usecase/interfaces.go`)

To satisfy R3, the following repository interfaces must be declared in `internal/usecase/interfaces.go`:

```go
// ContactRepository определяет контракт доступа к данным собеседников.
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

## 4. Concrete Blueprint: `internal/usecase/contact.go`

### 4.1 Responsibilities & Validation Rules
- `NewContactUsecase(contacts ContactRepository)`: Constructor.
- `Create(ctx context.Context, c domain.Contact) (*domain.Contact, error)`:
  - Validation: `CompanyID > 0`, `TelegramUserID > 0`, trimmed `Name != ""`
  - Sanitization: trims `Name` and `Username`
  - Action: calls `contacts.Create`, then `contacts.GetByID(ctx, id)`
- `FindOrCreateByTelegramID(ctx context.Context, companyID, tgUserID int64, name, username string) (*domain.Contact, error)`:
  - Validation: `companyID > 0`, `tgUserID > 0`, trimmed `name != ""`
  - Sanitization: trims `name` and `username`
  - Action: calls `contacts.GetByTelegramID(ctx, companyID, tgUserID)`.
    - If found: returns existing contact.
    - If `domain.ErrNotFound`: calls `u.Create(ctx, domain.Contact{...})`.
    - Note on concurrency: Postgres `contacts` table has `UNIQUE(company_id, telegram_user_id)` and storage `Create` executes `ON CONFLICT (company_id, telegram_user_id) DO UPDATE SET name = EXCLUDED.name, username = EXCLUDED.username RETURNING id`. Thus, concurrent calls safely resolve without collision.
- `GetByID(ctx context.Context, id int64) (*domain.Contact, error)`:
  - Validation: `id > 0`
  - Action: calls `contacts.GetByID(ctx, id)`
- `ListByCompany(ctx context.Context, companyID int64) ([]domain.Contact, error)`:
  - Validation: `companyID > 0`
  - Action: calls `contacts.ListByCompany(ctx, companyID)`

### 4.2 Exact Code Blueprint

```go
package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/kraconst/GO_server/internal/domain"
)

type ContactUsecase struct {
	contacts ContactRepository
}

func NewContactUsecase(contacts ContactRepository) *ContactUsecase {
	return &ContactUsecase{
		contacts: contacts,
	}
}

func (u *ContactUsecase) Create(ctx context.Context, c domain.Contact) (*domain.Contact, error) {
	if c.CompanyID <= 0 {
		return nil, fmt.Errorf("%w: invalid company_id", domain.ErrValidation)
	}
	if c.TelegramUserID <= 0 {
		return nil, fmt.Errorf("%w: invalid telegram_user_id", domain.ErrValidation)
	}
	c.Name = strings.TrimSpace(c.Name)
	if c.Name == "" {
		return nil, fmt.Errorf("%w: name is required", domain.ErrValidation)
	}
	c.Username = strings.TrimSpace(c.Username)

	id, err := u.contacts.Create(ctx, c)
	if err != nil {
		return nil, fmt.Errorf("usecase create contact: %w", err)
	}

	created, err := u.contacts.GetByID(ctx, id)
	if err != nil {
		c.ID = id
		return &c, nil
	}
	return created, nil
}

func (u *ContactUsecase) FindOrCreateByTelegramID(ctx context.Context, companyID, tgUserID int64, name, username string) (*domain.Contact, error) {
	if companyID <= 0 {
		return nil, fmt.Errorf("%w: invalid company_id", domain.ErrValidation)
	}
	if tgUserID <= 0 {
		return nil, fmt.Errorf("%w: invalid telegram_user_id", domain.ErrValidation)
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("%w: name is required", domain.ErrValidation)
	}
	username = strings.TrimSpace(username)

	contact, err := u.contacts.GetByTelegramID(ctx, companyID, tgUserID)
	if err == nil {
		return contact, nil
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return nil, fmt.Errorf("usecase find contact: %w", err)
	}

	return u.Create(ctx, domain.Contact{
		CompanyID:      companyID,
		TelegramUserID: tgUserID,
		Name:           name,
		Username:       username,
	})
}

func (u *ContactUsecase) GetByID(ctx context.Context, id int64) (*domain.Contact, error) {
	if id <= 0 {
		return nil, fmt.Errorf("%w: invalid id", domain.ErrValidation)
	}
	return u.contacts.GetByID(ctx, id)
}

func (u *ContactUsecase) ListByCompany(ctx context.Context, companyID int64) ([]domain.Contact, error) {
	if companyID <= 0 {
		return nil, fmt.Errorf("%w: invalid company_id", domain.ErrValidation)
	}
	return u.contacts.ListByCompany(ctx, companyID)
}
```

---

## 5. Concrete Blueprint: `internal/usecase/chat.go`

### 5.1 Status Validation & Allowed Enum Values
Allowed `domain.ChatStatus` values (7 total):
1. `domain.ChatStatusNewLead` ("new_lead")
2. `domain.ChatStatusNegotiation` ("negotiation")
3. `domain.ChatStatusWaitingReply` ("waiting_reply")
4. `domain.ChatStatusInterested` ("interested")
5. `domain.ChatStatusUnhappy` ("unhappy")
6. `domain.ChatStatusLost` ("lost")
7. `domain.ChatStatusActionRequired` ("action_required")

Helper function:
```go
func isValidChatStatus(status domain.ChatStatus) bool {
	switch status {
	case domain.ChatStatusNewLead,
		domain.ChatStatusNegotiation,
		domain.ChatStatusWaitingReply,
		domain.ChatStatusInterested,
		domain.ChatStatusUnhappy,
		domain.ChatStatusLost,
		domain.ChatStatusActionRequired:
		return true
	default:
		return false
	}
}
```

### 5.2 Method Specifications
- `NewChatUsecase(chats ChatRepository)`: Constructor.
- `SetManualStatus(ctx context.Context, chatID int64, status domain.ChatStatus) error`:
  - Validation: `chatID > 0` (otherwise `domain.ErrValidation`)
  - Validation: `isValidChatStatus(status)` (otherwise `domain.ErrValidation`)
  - Calls `chats.UpdateStatus(ctx, chatID, status, domain.StatusSourceManual)`.
  - Storage returns `domain.ErrNotFound` if row does not exist (`RowsAffected == 0`), which passes cleanly up to the HTTP handler to produce HTTP 404.
- `ListByCompany(ctx context.Context, companyID int64, status *domain.ChatStatus, employeeID *int64) ([]domain.Chat, error)`:
  - Validation: `companyID > 0`
  - Validation: if `status != nil && !isValidChatStatus(*status)`, return `domain.ErrValidation`
  - Validation: if `employeeID != nil && *employeeID <= 0`, return `domain.ErrValidation`
  - Calls `chats.ListByCompany(ctx, companyID, status, employeeID)`.
- `GetByID(ctx context.Context, id int64) (*domain.Chat, error)`:
  - Validation: `id > 0` (otherwise `domain.ErrValidation`)
  - Calls `chats.GetByID(ctx, id)`.
- `Create(ctx context.Context, chat domain.Chat) (*domain.Chat, error)`:
  - Validation: `AccountID > 0`, `ContactID > 0`.
  - Defaults & Enum validation:
    - `Status`: defaults to `domain.ChatStatusNewLead`; if set, must be valid.
    - `StatusSource`: defaults to `domain.StatusSourceAuto`; if set, must be `auto` or `manual`.
    - `Priority`: defaults to `domain.ChatPriorityNormal`; if set, must be `low`, `normal`, or `high`.
    - `LastMessageAt`: defaults to `time.Now().UTC()` if zero.
  - Calls `chats.Create`, then `chats.GetByID(ctx, id)`.

### 5.3 Exact Code Blueprint

```go
package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/kraconst/GO_server/internal/domain"
)

type ChatUsecase struct {
	chats ChatRepository
}

func NewChatUsecase(chats ChatRepository) *ChatUsecase {
	return &ChatUsecase{
		chats: chats,
	}
}

func isValidChatStatus(status domain.ChatStatus) bool {
	switch status {
	case domain.ChatStatusNewLead,
		domain.ChatStatusNegotiation,
		domain.ChatStatusWaitingReply,
		domain.ChatStatusInterested,
		domain.ChatStatusUnhappy,
		domain.ChatStatusLost,
		domain.ChatStatusActionRequired:
		return true
	default:
		return false
	}
}

func isValidStatusSource(source domain.StatusSource) bool {
	return source == domain.StatusSourceAuto || source == domain.StatusSourceManual
}

func isValidChatPriority(priority domain.ChatPriority) bool {
	return priority == domain.ChatPriorityLow ||
		priority == domain.ChatPriorityNormal ||
		priority == domain.ChatPriorityHigh
}

func (u *ChatUsecase) SetManualStatus(ctx context.Context, chatID int64, status domain.ChatStatus) error {
	if chatID <= 0 {
		return fmt.Errorf("%w: invalid chat_id", domain.ErrValidation)
	}
	if !isValidChatStatus(status) {
		return fmt.Errorf("%w: invalid chat status '%s'", domain.ErrValidation, status)
	}

	return u.chats.UpdateStatus(ctx, chatID, status, domain.StatusSourceManual)
}

func (u *ChatUsecase) ListByCompany(ctx context.Context, companyID int64, status *domain.ChatStatus, employeeID *int64) ([]domain.Chat, error) {
	if companyID <= 0 {
		return nil, fmt.Errorf("%w: invalid company_id", domain.ErrValidation)
	}
	if status != nil && !isValidChatStatus(*status) {
		return nil, fmt.Errorf("%w: invalid chat status '%s'", domain.ErrValidation, *status)
	}
	if employeeID != nil && *employeeID <= 0 {
		return nil, fmt.Errorf("%w: invalid employee_id", domain.ErrValidation)
	}

	return u.chats.ListByCompany(ctx, companyID, status, employeeID)
}

func (u *ChatUsecase) GetByID(ctx context.Context, id int64) (*domain.Chat, error) {
	if id <= 0 {
		return nil, fmt.Errorf("%w: invalid id", domain.ErrValidation)
	}
	return u.chats.GetByID(ctx, id)
}

func (u *ChatUsecase) Create(ctx context.Context, chat domain.Chat) (*domain.Chat, error) {
	if chat.AccountID <= 0 {
		return nil, fmt.Errorf("%w: invalid account_id", domain.ErrValidation)
	}
	if chat.ContactID <= 0 {
		return nil, fmt.Errorf("%w: invalid contact_id", domain.ErrValidation)
	}

	if chat.Status == "" {
		chat.Status = domain.ChatStatusNewLead
	} else if !isValidChatStatus(chat.Status) {
		return nil, fmt.Errorf("%w: invalid chat status '%s'", domain.ErrValidation, chat.Status)
	}

	if chat.StatusSource == "" {
		chat.StatusSource = domain.StatusSourceAuto
	} else if !isValidStatusSource(chat.StatusSource) {
		return nil, fmt.Errorf("%w: invalid status_source '%s'", domain.ErrValidation, chat.StatusSource)
	}

	if chat.Priority == "" {
		chat.Priority = domain.ChatPriorityNormal
	} else if !isValidChatPriority(chat.Priority) {
		return nil, fmt.Errorf("%w: invalid priority '%s'", domain.ErrValidation, chat.Priority)
	}

	if chat.LastMessageAt.IsZero() {
		chat.LastMessageAt = time.Now().UTC()
	}

	id, err := u.chats.Create(ctx, chat)
	if err != nil {
		return nil, fmt.Errorf("usecase create chat: %w", err)
	}

	created, err := u.chats.GetByID(ctx, id)
	if err != nil {
		chat.ID = id
		return &chat, nil
	}
	return created, nil
}
```

---

## 6. Integration and Error Mapping Matrix

The table below outlines how usecase errors propagate through HTTP handlers:

| Scenario | Usecase Returns | Handler Checks | HTTP Response |
|---|---|---|---|
| Non-positive ID in path (`GET /chats/0` or `POST /chats/-1/status`) | `fmt.Errorf("%w: invalid id", domain.ErrValidation)` (or caught during `strconv.Atoi`) | `strconv` check or `errors.Is(err, domain.ErrValidation)` | `400 Bad Request` |
| Invalid status in body (`POST /chats/1/status` with `"status": "unknown"`) | `fmt.Errorf("%w: invalid chat status 'unknown'", domain.ErrValidation)` | `errors.Is(err, domain.ErrValidation)` | `400 Bad Request` |
| Chat ID not found in database (`GET /chats/999` or `POST /chats/999/status`) | `domain.ErrNotFound` | `errors.Is(err, domain.ErrNotFound)` | `404 Not Found` |
| Success status update (`POST /chats/1/status` with `"status": "negotiation"`) | `nil` | `err == nil` | `200 OK {"status": "updated"}` |
| Query filters with invalid status (`GET /chats?status=invalid`) | `fmt.Errorf("%w: ...", domain.ErrValidation)` | `errors.Is(err, domain.ErrValidation)` | `400 Bad Request` |
| Query filters with non-numeric or negative employee_id | `strconv` error or `domain.ErrValidation` | `errors.Is(err, domain.ErrValidation)` | `400 Bad Request` |
| Database query failure (syntax, connection drop) | `fmt.Errorf("...: %w", err)` | `default` | `500 Internal Server Error` |

---

## 7. Composition Root and Handler Wiring Implications

### In `internal/handler/handler.go`:
1. `Handler` struct must add:
   ```go
   contactUsecase *usecase.ContactUsecase
   chatUsecase    *usecase.ChatUsecase
   ```
2. `handler.New(...)` updated to accept `contactUsecase *usecase.ContactUsecase, chatUsecase *usecase.ChatUsecase`.
3. Add routes:
   - `GET /chats` -> `h.ListChats`
   - `GET /chats/:id` -> `h.GetChatByID`
   - `POST /chats/:id/status` -> `h.UpdateChatStatus`
4. Add in-memory mock fallback data:
   - `var mockContacts = []domain.Contact{...}`
   - `var mockChats = []domain.Chat{...}`

### In `cmd/app/main.go`:
```go
contactRepo := postgres.NewContactStorage(db)
chatRepo := postgres.NewChatStorage(db)

contactUsecase = usecase.NewContactUsecase(contactRepo)
chatUsecase = usecase.NewChatUsecase(chatRepo)

h := handler.New(accountUsecase, employeeUsecase, messageUsecase, contactUsecase, chatUsecase)
```
*(Note: handler.New parameter order to be aligned with Explorer 3 / implementer).*
