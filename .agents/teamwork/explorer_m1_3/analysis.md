# Analysis Report: Handler & Composition Exploration (Contact & Chat)

**Date**: 2026-11-10  
**Explorer**: Explorer 3 (`explorer_m1_3`)  
**Scope**: HTTP handlers, routing, mock fallback architecture, and composition root wiring for `Contact` and `Chat` entities  
**Target Files Analyzed**:
- `internal/handler/handler.go`
- `cmd/app/main.go`
- `internal/domain/chat.go`, `contact.go`, `account.go`, `employee.go`, `errors.go`
- `internal/usecase/interfaces.go`, `usecase/chat.go`, `usecase/contact.go`

---

## 1. Executive Summary

This report delivers concrete architectural findings and exact Go code blueprints for:
1. Extending `internal/handler/handler.go` to support `ContactUsecase` and `ChatUsecase`.
2. Defining in-memory fallback datasets (`mockContacts`, `mockChats`, and enriched `mockAccounts`).
3. Adding three REST endpoints to `Handler.Run`:
   - `GET /chats` -> `h.ListChats` (filtering by optional `status` and `employee_id`, company ID 1).
   - `GET /chats/:id` -> `h.GetChatByID` (`id > 0`, `400` on invalid id, `404` on `domain.ErrNotFound`).
   - `POST /chats/:id/status` -> `h.UpdateChatStatus` (JSON `{"status": "..."}`, calls `usecase.SetManualStatus`, `400` on `domain.ErrValidation`, `404` on `domain.ErrNotFound`, `200` on success).
4. Updating `cmd/app/main.go` to instantiate `ContactStorage`, `ChatStorage`, `ContactUsecase`, `ChatUsecase`, and wire them into `handler.New(...)`.

All designs align strictly with existing codebase conventions: Clean Architecture layers, tenant hard-coding (company ID 1), sentinel domain error mapping (`domain.ErrNotFound` -> 404, `domain.ErrValidation` -> 400, unexpected -> 500), and robust mock fallback behavior when PostgreSQL is offline.

---

## 2. Codebase Investigation Findings

### 2.1 Current `internal/handler/handler.go` Architecture

#### Struct Definition & Constructor (lines 13-25):
```go
type Handler struct {
	accountUsecase  *usecase.AccountUsecase
	employeeUsecase *usecase.EmployeeUsecase
	messageUsecase  *usecase.MessageUsecase
}

func New(accountUsecase *usecase.AccountUsecase, employeeUsecase *usecase.EmployeeUsecase, messageUsecase *usecase.MessageUsecase) *Handler {
	return &Handler{
		accountUsecase:  accountUsecase,
		employeeUsecase: employeeUsecase,
		messageUsecase:  messageUsecase,
	}
}
```
- The handler holds pointers to usecases. When PostgreSQL fails to connect at startup, `cmd/app/main.go` passes `nil` pointers.
- Every handler method checks if its corresponding usecase pointer is `nil`. If `nil`, it executes in-memory fallback logic against package-level mock slices.

#### Existing Mock Data (lines 27-40):
```go
var mockAccounts = []domain.Account{
	{ID: 1, Phone: "+79991112233", Username: "@sales_anna", EmployeeName: "Анна", Status: domain.StatusActive, CreatedAt: time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)},
	{ID: 2, Phone: "+79994445566", Username: "@support_oleg", EmployeeName: "Олег", Status: domain.StatusDisconnected, CreatedAt: time.Date(2026, 9, 15, 14, 30, 0, 0, time.UTC)},
}

var mockEmployees = []domain.Employee{
	{ID: 1, CompanyID: 1, Name: "Анна", Position: "Менеджер по продажам", IsActive: true},
	{ID: 2, CompanyID: 1, Name: "Олег", Position: "Служба поддержки", IsActive: true},
}

var mockMessages = []domain.Message{
	{ID: 1, ChatID: 1, TelegramMessageID: 101, Direction: domain.MessageDirectionIn, Text: "Здравствуйте! Подскажите по условиям.", SentAt: time.Date(2026, 9, 1, 10, 5, 0, 0, time.UTC)},
	{ID: 2, ChatID: 1, TelegramMessageID: 102, Direction: domain.MessageDirectionOut, Text: "Добрый день! Сейчас направлю КП.", SentAt: time.Date(2026, 9, 1, 10, 6, 0, 0, time.UTC)},
}
```
**Key Observations**:
1. `mockAccounts` omits `CompanyID` and `EmployeeID`. In order to filter mock chats by `employee_id`, `mockAccounts` should explicitly set `CompanyID: 1, EmployeeID: 1` for Account 1 (Anna) and `CompanyID: 1, EmployeeID: 2` for Account 2 (Oleg).
2. `mockMessages` references `ChatID: 1` with a last message sent at `2026-09-01 10:06:00 UTC`.
3. `mockContacts` and `mockChats` do not yet exist and must be introduced.

#### Route Registrations in `Run` (lines 58-78):
- Current routes:
  - `GET /health`
  - `GET /accounts`, `GET /accounts/:id`, `POST /accounts`
  - `GET /employees`, `GET /employees/:id`, `POST /employees`, `POST /employees/:id`, `POST /employees/:id/deactivate`
  - `GET /chats/:id/messages`
- Adding:
  - `GET /chats` -> `h.ListChats`
  - `GET /chats/:id` -> `h.GetChatByID`
  - `POST /chats/:id/status` -> `h.UpdateChatStatus`
- Parametric Gin route compatibility: `/chats`, `/chats/:id`, `/chats/:id/status`, and `/chats/:id/messages` coexist without ambiguity under Gin's radix-tree router.

#### Error Mapping Conventions:
- Path parameter parsing: `strconv.ParseInt` or `strconv.Atoi`. Non-numbers or `<= 0` IDs return `400 Bad Request` with `gin.H{"error": "id must be a positive number"}`.
- JSON binding: `c.ShouldBindJSON(&req)`. Bind failure returns `400 Bad Request` with `gin.H{"error": err.Error()}`.
- Usecase error translation:
  - `errors.Is(err, domain.ErrNotFound)` -> `404 Not Found` with `gin.H{"error": "<entity> not found"}`.
  - `errors.Is(err, domain.ErrValidation)` -> `400 Bad Request` with `gin.H{"error": err.Error()}`.
  - Unexpected errors -> `500 Internal Server Error` with `gin.H{"error": err.Error()}`.

---

### 2.2 Current `cmd/app/main.go` Composition Root

```go
package main

import (
	"log"

	"github.com/kraconst/GO_server/internal/config"
	"github.com/kraconst/GO_server/internal/handler"
	"github.com/kraconst/GO_server/internal/storage/postgres"
	"github.com/kraconst/GO_server/internal/usecase"
)

func main() {
	cfg := config.Must()

	db, err := postgres.New(cfg.DB.DSN())
	var accountUsecase *usecase.AccountUsecase
	var employeeUsecase *usecase.EmployeeUsecase
	var messageUsecase *usecase.MessageUsecase

	if err != nil {
		log.Printf("PostgreSQL connection failed: %v (run 'docker compose up -d' to start database)", err)
		log.Println("Starting server with mock fallback...")
	} else {
		defer db.Close()
		log.Println("Connected to PostgreSQL successfully")

		accountRepo := postgres.NewAccountStorage(db)
		employeeRepo := postgres.NewEmployeeStorage(db)
		messageRepo := postgres.NewMessageStorage(db)

		accountUsecase = usecase.NewAccountUsecase(accountRepo)
		employeeUsecase = usecase.NewEmployeeUsecase(employeeRepo)
		messageUsecase = usecase.NewMessageUsecase(messageRepo)
	}

	h := handler.New(accountUsecase, employeeUsecase, messageUsecase)
	log.Fatal(h.Run(cfg.Addr))
}
```

**Key Observations**:
1. When DB ping fails: usecase pointers remain `nil`.
2. When DB ping succeeds: storages are instantiated with `*sqlx.DB`, then usecases are instantiated with storages.
3. `handler.New(...)` is called once at the end and passed to `h.Run(cfg.Addr)`.
4. Only `cmd/app/main.go` calls `handler.New(...)`. No tests or other packages invoke it.

---

## 3. Blueprint: `internal/handler/handler.go`

### 3.1 Imports
Add `"sort"` to imports (for sorting mock chats by `last_message_at DESC, id DESC`).

```go
package handler

import (
	"errors"
	"sort"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kraconst/GO_server/internal/domain"
	"github.com/kraconst/GO_server/internal/usecase"
)
```

### 3.2 Handler Struct and Constructor
```go
type Handler struct {
	accountUsecase  *usecase.AccountUsecase
	employeeUsecase *usecase.EmployeeUsecase
	messageUsecase  *usecase.MessageUsecase
	contactUsecase  *usecase.ContactUsecase
	chatUsecase     *usecase.ChatUsecase
}

func New(
	accountUsecase *usecase.AccountUsecase,
	employeeUsecase *usecase.EmployeeUsecase,
	messageUsecase *usecase.MessageUsecase,
	contactUsecase *usecase.ContactUsecase,
	chatUsecase *usecase.ChatUsecase,
) *Handler {
	return &Handler{
		accountUsecase:  accountUsecase,
		employeeUsecase: employeeUsecase,
		messageUsecase:  messageUsecase,
		contactUsecase:  contactUsecase,
		chatUsecase:     chatUsecase,
	}
}
```

### 3.3 Mock Data Definitions
Update `mockAccounts` with explicit `CompanyID: 1` and `EmployeeID: 1` / `2`, and add `mockContacts` and `mockChats`:

```go
var mockAccounts = []domain.Account{
	{ID: 1, CompanyID: 1, EmployeeID: 1, Phone: "+79991112233", Username: "@sales_anna", EmployeeName: "Анна", Status: domain.StatusActive, CreatedAt: time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)},
	{ID: 2, CompanyID: 1, EmployeeID: 2, Phone: "+79994445566", Username: "@support_oleg", EmployeeName: "Олег", Status: domain.StatusDisconnected, CreatedAt: time.Date(2026, 9, 15, 14, 30, 0, 0, time.UTC)},
}

var mockEmployees = []domain.Employee{
	{ID: 1, CompanyID: 1, Name: "Анна", Position: "Менеджер по продажам", IsActive: true},
	{ID: 2, CompanyID: 1, Name: "Олег", Position: "Служба поддержки", IsActive: true},
}

var mockMessages = []domain.Message{
	{ID: 1, ChatID: 1, TelegramMessageID: 101, Direction: domain.MessageDirectionIn, Text: "Здравствуйте! Подскажите по условиям.", SentAt: time.Date(2026, 9, 1, 10, 5, 0, 0, time.UTC)},
	{ID: 2, ChatID: 1, TelegramMessageID: 102, Direction: domain.MessageDirectionOut, Text: "Добрый день! Сейчас направлю КП.", SentAt: time.Date(2026, 9, 1, 10, 6, 0, 0, time.UTC)},
}

var mockContacts = []domain.Contact{
	{
		ID:             1,
		CompanyID:      1,
		TelegramUserID: 10001,
		Name:           "Иван Петров",
		Username:       "ivan_p",
	},
	{
		ID:             2,
		CompanyID:      1,
		TelegramUserID: 10002,
		Name:           "Мария Смирнова",
		Username:       "maria_s",
	},
}

var mockChats = []domain.Chat{
	{
		ID:                 1,
		AccountID:          1,
		ContactID:          1,
		Status:             domain.ChatStatusNewLead,
		StatusSource:       domain.StatusSourceAuto,
		Priority:           domain.ChatPriorityNormal,
		LastMessageAt:      time.Date(2026, 9, 1, 10, 6, 0, 0, time.UTC),
		AwaitingReplySince: nil,
		UnreadCount:        0,
	},
	{
		ID:                 2,
		AccountID:          2,
		ContactID:          2,
		Status:             domain.ChatStatusNegotiation,
		StatusSource:       domain.StatusSourceManual,
		Priority:           domain.ChatPriorityHigh,
		LastMessageAt:      time.Date(2026, 9, 15, 15, 0, 0, 0, time.UTC),
		AwaitingReplySince: nil,
		UnreadCount:        1,
	},
}
```

### 3.4 Request DTOs
```go
type updateChatStatusRequest struct {
	Status domain.ChatStatus `json:"status" binding:"required"`
}
```

### 3.5 Route Registrations in `Run`
```go
func (h *Handler) Run(addr string) error {
	r := gin.Default()
	r.GET("/health", h.Health)

	// Accounts
	r.GET("/accounts", h.List)
	r.GET("/accounts/:id", h.GetByID)
	r.POST("/accounts", h.Create)

	// Employees
	r.GET("/employees", h.ListEmployees)
	r.GET("/employees/:id", h.GetEmployeeByID)
	r.POST("/employees", h.CreateEmployee)
	r.POST("/employees/:id", h.UpdateEmployee)
	r.POST("/employees/:id/deactivate", h.DeactivateEmployee)

	// Chats
	r.GET("/chats", h.ListChats)
	r.GET("/chats/:id", h.GetChatByID)
	r.POST("/chats/:id/status", h.UpdateChatStatus)

	// Messages
	r.GET("/chats/:id/messages", h.ListMessagesByChat)

	return r.Run(addr)
}
```

### 3.6 Endpoint Handlers

#### 1. `ListChats` (`GET /chats`)
- **Query Parameters**:
  - `status`: optional filter string. Must match one of 7 valid `ChatStatus` enums or returns 400.
  - `employee_id`: optional filter integer. Must be positive or returns 400.
- **Tenant Context**: Hardcoded `companyID = 1`.
- **Mock Fallback**:
  - Filters `mockChats` by status.
  - Resolves responsible employee via `chat.AccountID == mockAccounts[i].ID` and matches `mockAccounts[i].EmployeeID == *employeeID`.
  - Sorts matches by `LastMessageAt DESC, ID DESC` (mirroring Postgres `ORDER BY c.last_message_at DESC, c.id DESC`).
  - Serializes as `[]domain.Chat` (returning `[]` when empty, never `null`).
- **Database Path**:
  - Calls `h.chatUsecase.ListByCompany(c.Request.Context(), 1, status, employeeID)`.
  - Maps `domain.ErrValidation` to 400, unexpected to 500.

```go
func (h *Handler) ListChats(c *gin.Context) {
	var status *domain.ChatStatus
	if s := c.Query("status"); s != "" {
		st := domain.ChatStatus(s)
		switch st {
		case domain.ChatStatusNewLead,
			domain.ChatStatusNegotiation,
			domain.ChatStatusWaitingReply,
			domain.ChatStatusInterested,
			domain.ChatStatusUnhappy,
			domain.ChatStatusLost,
			domain.ChatStatusActionRequired:
			status = &st
		default:
			c.JSON(400, gin.H{"error": "invalid status"})
			return
		}
	}

	var employeeID *int64
	if empStr := c.Query("employee_id"); empStr != "" {
		empID, err := strconv.ParseInt(empStr, 10, 64)
		if err != nil || empID <= 0 {
			c.JSON(400, gin.H{"error": "employee_id must be a positive number"})
			return
		}
		employeeID = &empID
	}

	if h.chatUsecase == nil {
		res := make([]domain.Chat, 0)
		for _, chat := range mockChats {
			if status != nil && chat.Status != *status {
				continue
			}
			if employeeID != nil {
				matched := false
				for _, acc := range mockAccounts {
					if acc.ID == chat.AccountID && acc.EmployeeID == *employeeID {
						matched = true
						break
					}
				}
				if !matched {
					continue
				}
			}
			res = append(res, chat)
		}

		sort.Slice(res, func(i, j int) bool {
			if res[i].LastMessageAt.Equal(res[j].LastMessageAt) {
				return res[i].ID > res[j].ID
			}
			return res[i].LastMessageAt.After(res[j].LastMessageAt)
		})

		c.JSON(200, res)
		return
	}

	chats, err := h.chatUsecase.ListByCompany(c.Request.Context(), 1, status, employeeID)
	if err != nil {
		if errors.Is(err, domain.ErrValidation) {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	if chats == nil {
		chats = make([]domain.Chat, 0)
	}
	c.JSON(200, chats)
}
```

#### 2. `GetChatByID` (`GET /chats/:id`)
- **Path Parameter**: `:id` parsed as positive `int64`. Returns `400` if not numeric or `<= 0`.
- **Mock Fallback**:
  - Searches `mockChats` for `ID == id`.
  - Returns `200` with the chat if found, or `404` with `gin.H{"error": "chat not found"}` if not found.
- **Database Path**:
  - Calls `h.chatUsecase.GetByID(c.Request.Context(), id)`.
  - Maps `domain.ErrNotFound` to `404`, `domain.ErrValidation` to `400`, unexpected to `500`.
  - Returns `200` with the chat.

```go
func (h *Handler) GetChatByID(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(400, gin.H{"error": "id must be a positive number"})
		return
	}

	if h.chatUsecase == nil {
		for _, chat := range mockChats {
			if chat.ID == id {
				c.JSON(200, chat)
				return
			}
		}
		c.JSON(404, gin.H{"error": "chat not found"})
		return
	}

	chat, err := h.chatUsecase.GetByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			c.JSON(404, gin.H{"error": "chat not found"})
			return
		}
		if errors.Is(err, domain.ErrValidation) {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, chat)
}
```

#### 3. `UpdateChatStatus` (`POST /chats/:id/status`)
- **Path Parameter**: `:id` parsed as positive `int64`. Returns `400` if not numeric or `<= 0`.
- **Body**: JSON `{"status": "..."}` bound to `updateChatStatusRequest`. Returns `400` on binding error.
- **Mock Fallback**:
  - Validates `req.Status` against the 7 allowed `ChatStatus` enums. Returns `400` on invalid enum.
  - Searches `mockChats` for `ID == id`.
  - If found, sets `mockChats[i].Status = req.Status` and `mockChats[i].StatusSource = domain.StatusSourceManual`, then returns `200` with `gin.H{"status": "updated"}`.
  - If not found, returns `404` with `gin.H{"error": "chat not found"}`.
- **Database Path**:
  - Calls `h.chatUsecase.SetManualStatus(c.Request.Context(), id, req.Status)`.
  - Maps `domain.ErrNotFound` to `404`, `domain.ErrValidation` to `400`, unexpected to `500`.
  - Returns `200` with `gin.H{"status": "updated"}`.

```go
func (h *Handler) UpdateChatStatus(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(400, gin.H{"error": "id must be a positive number"})
		return
	}

	var req updateChatStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	if h.chatUsecase == nil {
		switch req.Status {
		case domain.ChatStatusNewLead,
			domain.ChatStatusNegotiation,
			domain.ChatStatusWaitingReply,
			domain.ChatStatusInterested,
			domain.ChatStatusUnhappy,
			domain.ChatStatusLost,
			domain.ChatStatusActionRequired:
		default:
			c.JSON(400, gin.H{"error": "invalid status"})
			return
		}

		found := false
		for i := range mockChats {
			if mockChats[i].ID == id {
				mockChats[i].Status = req.Status
				mockChats[i].StatusSource = domain.StatusSourceManual
				found = true
				break
			}
		}
		if !found {
			c.JSON(404, gin.H{"error": "chat not found"})
			return
		}
		c.JSON(200, gin.H{"status": "updated"})
		return
	}

	err = h.chatUsecase.SetManualStatus(c.Request.Context(), id, req.Status)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			c.JSON(404, gin.H{"error": "chat not found"})
			return
		}
		if errors.Is(err, domain.ErrValidation) {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"status": "updated"})
}
```

---

## 4. Blueprint: `cmd/app/main.go` Composition Root

The composition root is updated to construct `ContactStorage`, `ChatStorage`, `ContactUsecase`, `ChatUsecase`, and pass them into `handler.New(...)`:

```go
package main

import (
	"log"

	"github.com/kraconst/GO_server/internal/config"
	"github.com/kraconst/GO_server/internal/handler"
	"github.com/kraconst/GO_server/internal/storage/postgres"
	"github.com/kraconst/GO_server/internal/usecase"
)

func main() {
	cfg := config.Must()

	db, err := postgres.New(cfg.DB.DSN())
	var accountUsecase *usecase.AccountUsecase
	var employeeUsecase *usecase.EmployeeUsecase
	var messageUsecase *usecase.MessageUsecase
	var contactUsecase *usecase.ContactUsecase
	var chatUsecase *usecase.ChatUsecase

	if err != nil {
		log.Printf("PostgreSQL connection failed: %v (run 'docker compose up -d' to start database)", err)
		log.Println("Starting server with mock fallback...")
	} else {
		defer db.Close()
		log.Println("Connected to PostgreSQL successfully")

		accountRepo := postgres.NewAccountStorage(db)
		employeeRepo := postgres.NewEmployeeStorage(db)
		messageRepo := postgres.NewMessageStorage(db)
		contactRepo := postgres.NewContactStorage(db)
		chatRepo := postgres.NewChatStorage(db)

		accountUsecase = usecase.NewAccountUsecase(accountRepo)
		employeeUsecase = usecase.NewEmployeeUsecase(employeeRepo)
		messageUsecase = usecase.NewMessageUsecase(messageRepo)
		contactUsecase = usecase.NewContactUsecase(contactRepo)
		chatUsecase = usecase.NewChatUsecase(chatRepo)
	}

	h := handler.New(accountUsecase, employeeUsecase, messageUsecase, contactUsecase, chatUsecase)
	log.Fatal(h.Run(cfg.Addr))
}
```

---

## 5. Error Mapping and HTTP Contract Matrix

| Endpoint | HTTP Method | Scenario | Handler Condition | Response Code | Response Body |
|---|---|---|---|---|---|
| `/chats` | `GET` | Valid request (no filters) | `status == nil && employeeID == nil` | `200 OK` | `[{"id": 2, ...}, {"id": 1, ...}]` |
| `/chats` | `GET` | Valid request with filters | `?status=negotiation&employee_id=2` | `200 OK` | `[{"id": 2, ...}]` |
| `/chats` | `GET` | No matching chats | Filter matches 0 chats | `200 OK` | `[]` |
| `/chats` | `GET` | Invalid `status` query | `?status=invalid_status` | `400 Bad Request` | `{"error": "invalid status"}` |
| `/chats` | `GET` | Non-numeric `employee_id` | `?employee_id=abc` | `400 Bad Request` | `{"error": "employee_id must be a positive number"}` |
| `/chats` | `GET` | Non-positive `employee_id` | `?employee_id=0` or `-1` | `400 Bad Request` | `{"error": "employee_id must be a positive number"}` |
| `/chats/:id` | `GET` | Valid existing chat ID | `id == 1` | `200 OK` | `{"id": 1, "status": "new_lead", ...}` |
| `/chats/:id` | `GET` | Non-numeric chat ID | `/chats/abc` | `400 Bad Request` | `{"error": "id must be a positive number"}` |
| `/chats/:id` | `GET` | Non-positive chat ID | `/chats/0` or `/chats/-5` | `400 Bad Request` | `{"error": "id must be a positive number"}` |
| `/chats/:id` | `GET` | Chat ID not found | `/chats/999` | `404 Not Found` | `{"error": "chat not found"}` |
| `/chats/:id/status` | `POST` | Valid status update | `id == 1`, `{"status": "negotiation"}` | `200 OK` | `{"status": "updated"}` |
| `/chats/:id/status` | `POST` | Non-positive chat ID | `/chats/0/status` | `400 Bad Request` | `{"error": "id must be a positive number"}` |
| `/chats/:id/status` | `POST` | Invalid JSON body | Body empty `{}` or missing `"status"` | `400 Bad Request` | `{"error": "<binding error>"}` |
| `/chats/:id/status` | `POST` | Invalid status enum | `{"status": "unknown"}` | `400 Bad Request` | `{"error": "invalid status"}` |
| `/chats/:id/status` | `POST` | Chat ID not found | `/chats/999/status` | `404 Not Found` | `{"error": "chat not found"}` |
| Any | Any | Database error / Panic | `err != nil` (unhandled) | `500 Internal Server Error` | `{"error": "<error text>"}` |

---

## 6. Verification and Integration Checklist

1. **Compilation and Static Analysis**:
   ```sh
   go fmt ./...
   go vet ./...
   go test ./...
   go build ./...
   ```
2. **Mock Fallback Verification** (when PostgreSQL is down):
   - Start app: `go run ./cmd/app`
   - Test `GET /health` -> `200 {"status": "ok"}`
   - Test `GET /chats` -> `200` with 2 mock chats, pre-sorted `LastMessageAt DESC`
   - Test `GET /chats?status=new_lead` -> `200` with chat 1
   - Test `GET /chats?employee_id=2` -> `200` with chat 2
   - Test `GET /chats?status=invalid` -> `400` with `{"error": "invalid status"}`
   - Test `GET /chats?employee_id=abc` -> `400` with `{"error": "employee_id must be a positive number"}`
   - Test `GET /chats/1` -> `200` with chat 1
   - Test `GET /chats/999` -> `404` with `{"error": "chat not found"}`
   - Test `GET /chats/0` -> `400` with `{"error": "id must be a positive number"}`
   - Test `POST /chats/1/status` with `{"status": "interested"}` -> `200 {"status": "updated"}`
   - Subsequent `GET /chats/1` -> reflects `status: "interested"` and `status_source: "manual"`
   - Test `POST /chats/1/status` with `{"status": "invalid"}` -> `400`
   - Test `POST /chats/999/status` with `{"status": "lost"}` -> `404`
3. **Database-Backed Verification** (when PostgreSQL is running with migration 000003):
   - Start database: `docker compose up -d postgres`
   - Run API: `go run ./cmd/app`
   - Verify connection log: `Connected to PostgreSQL successfully`
   - Exercise endpoints against seeded database records.
