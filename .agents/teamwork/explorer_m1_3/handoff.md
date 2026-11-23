# Handoff Report: Handler & Composition Exploration (Contact & Chat)

## 1. Observation

### Codebase Inspections
1. **`internal/handler/handler.go` lines 13-25**:
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
2. **`internal/handler/handler.go` lines 27-40**:
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
3. **`internal/handler/handler.go` lines 58-78**:
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

   	// Messages
   	r.GET("/chats/:id/messages", h.ListMessagesByChat)

   	return r.Run(addr)
   }
   ```
4. **`cmd/app/main.go` lines 12-38**:
   ```go
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
5. **Caller references search**:
   Command `grep -rn "handler.New" .` returned:
   `./cmd/app/main.go:36:	h := handler.New(accountUsecase, employeeUsecase, messageUsecase)`
   `handler.New` is exclusively invoked by `cmd/app/main.go`.
6. **Peer contracts alignment**:
   - `explorer_m1_1/analysis.md`: Storage methods `ContactStorage` and `ChatStorage` defined with `Create`, `GetByID`, `ListByCompany`, `UpdateStatus`.
   - `explorer_m1_2/analysis.md`: Usecase methods `NewContactUsecase`, `NewChatUsecase`, `ChatUsecase.SetManualStatus`, `ChatUsecase.ListByCompany(ctx, companyID, status, employeeID)`, `ChatUsecase.GetByID`, wrapping invalid inputs with `domain.ErrValidation` and propagating `domain.ErrNotFound`.

---

## 2. Logic Chain

1. **Usecase Ingestion in Handler**:
   - `Handler` struct and `New` constructor currently take 3 usecases (`accountUsecase`, `employeeUsecase`, `messageUsecase`) (Obs 1).
   - Adding `contactUsecase *usecase.ContactUsecase` and `chatUsecase *usecase.ChatUsecase` extends `Handler` to hold both domain services required for Task 1.
   - Since `handler.New` is only called in `cmd/app/main.go` (Obs 5), updating both files simultaneously causes no breaking API collisions elsewhere.

2. **Route Compatibility and Architecture**:
   - `GET /chats/:id/messages` is already mapped under `Run` (Obs 3).
   - Adding `GET /chats` (`h.ListChats`), `GET /chats/:id` (`h.GetChatByID`), and `POST /chats/:id/status` (`h.UpdateChatStatus`) integrates cleanly with Gin's radix-tree router without route conflicts.
   - Company ID is hardcoded to 1 across existing handlers (`accountUsecase.List(ctx, 1)`, `employeeUsecase.List(ctx, 1)`), so `h.ListChats` must pass `companyID = 1`.

3. **Mock Fallback Strategy**:
   - When database initialization fails (Obs 4), usecase pointers are `nil`. Handlers must fall back to in-memory slices without crashing.
   - In `mockAccounts` (Obs 2), `CompanyID` and `EmployeeID` were omitted (defaulting to 0). Setting `CompanyID: 1, EmployeeID: 1` on Account 1 and `CompanyID: 1, EmployeeID: 2` on Account 2 enables `ListChats` in mock mode to resolve `chat.AccountID -> account.EmployeeID` when filtering by `?employee_id=...`.
   - `mockChats` should contain at least 2 records with different statuses, priorities, accounts, and timestamps. Pre-sorting by `LastMessageAt DESC, ID DESC` via `sort.Slice` reproduces the database order `ORDER BY c.last_message_at DESC, c.id DESC`.
   - In `UpdateChatStatus`, validating `req.Status` against the 7 `ChatStatus` enums and updating `mockChats` in-place ensures stateful verification and matches database behavior (`404` if chat ID is not found, `400` if status is invalid).

4. **Error Sentinel Mapping**:
   - `domain.ErrNotFound` is translated to HTTP `404` with `{"error": "chat not found"}`.
   - `domain.ErrValidation` (and parameter parse/bind errors) is translated to HTTP `400` with `{"error": ...}`.
   - Any other error maps to HTTP `500` with `{"error": ...}`.

5. **Composition Root Wiring**:
   - In `cmd/app/main.go` (Obs 4), variables `contactUsecase` and `chatUsecase` are declared before the DB check.
   - In the `else` block, `ContactStorage` and `ChatStorage` are constructed from `db`, passed into `NewContactUsecase` and `NewChatUsecase`, and forwarded to `handler.New(...)`.

---

## 3. Caveats

1. **In-Memory Concurrency in Mock Mode**:
   Like `mockAccounts` and `mockEmployees`, `mockChats` is stored in a package-level slice without mutex locks. This is consistent with existing mock fallback design in `internal/handler/handler.go` and is appropriate for single-process development fallback.
2. **Contact Endpoints**:
   No HTTP routes were requested for contacts (e.g. `GET /contacts`), matching `CONTRIBUTING.md` Task 1 scope (`GET /chats`, `GET /chats/:id`, `POST /chats/:id/status`). However, `ContactUsecase` is wired into `Handler` and `mockContacts` is initialized to support future contact features.
3. **Tenant Context**:
   Tenant resolution continues to hardcode company ID 1 across all chat handlers, consistent with `Account` and `Employee` reference implementations.

---

## 4. Conclusion

The blueprints provided in `analysis.md` completely define the necessary changes for:
- `internal/handler/handler.go`:
  - `Handler` struct extension with `contactUsecase` and `chatUsecase`.
  - `New` constructor extension.
  - `mockAccounts` updated with `CompanyID: 1` and `EmployeeID: 1` / `2`.
  - `mockContacts` and `mockChats` slices initialized.
  - `updateChatStatusRequest` DTO defined.
  - Routes `GET /chats`, `GET /chats/:id`, `POST /chats/:id/status` registered.
  - Handler methods `ListChats`, `GetChatByID`, `UpdateChatStatus` fully implemented with mock fallback, parameter validation, and sentinel error mapping.
- `cmd/app/main.go`:
  - Storage construction: `postgres.NewContactStorage(db)`, `postgres.NewChatStorage(db)`.
  - Usecase construction: `usecase.NewContactUsecase(contactRepo)`, `usecase.NewChatUsecase(chatRepo)`.
  - Handler wiring: `handler.New(accountUsecase, employeeUsecase, messageUsecase, contactUsecase, chatUsecase)`.

---

## 5. Verification Method

### 1. Static Analysis and Compilation
Execute from the workspace root:
```sh
go fmt ./...
go vet ./...
go test ./...
go build ./...
```
Expected output: No diffs from `go fmt`, exit code 0 from `vet`, `test`, and `build`.

### 2. Functional Verification of Mock Fallback (PostgreSQL Offline)
Start server without Docker:
```sh
go run ./cmd/app
```
Test endpoints via curl:
- `curl -i http://localhost:8080/chats`
  * Expected: `200 OK`, JSON array with 2 chats.
- `curl -i http://localhost:8080/chats?status=new_lead`
  * Expected: `200 OK`, JSON array containing only chat 1.
- `curl -i http://localhost:8080/chats?employee_id=2`
  * Expected: `200 OK`, JSON array containing only chat 2.
- `curl -i http://localhost:8080/chats?status=bad_status`
  * Expected: `400 Bad Request`, `{"error": "invalid status"}`.
- `curl -i http://localhost:8080/chats?employee_id=abc`
  * Expected: `400 Bad Request`, `{"error": "employee_id must be a positive number"}`.
- `curl -i http://localhost:8080/chats/1`
  * Expected: `200 OK`, chat 1 JSON object.
- `curl -i http://localhost:8080/chats/0`
  * Expected: `400 Bad Request`, `{"error": "id must be a positive number"}`.
- `curl -i http://localhost:8080/chats/999`
  * Expected: `404 Not Found`, `{"error": "chat not found"}`.
- `curl -i -X POST http://localhost:8080/chats/1/status -H "Content-Type: application/json" -d '{"status": "negotiation"}'`
  * Expected: `200 OK`, `{"status": "updated"}`.
- `curl -i http://localhost:8080/chats/1`
  * Expected: `200 OK`, `status: "negotiation"`, `status_source: "manual"`.
- `curl -i -X POST http://localhost:8080/chats/1/status -H "Content-Type: application/json" -d '{"status": "invalid_status"}'`
  * Expected: `400 Bad Request`, `{"error": "invalid status"}`.
- `curl -i -X POST http://localhost:8080/chats/999/status -H "Content-Type: application/json" -d '{"status": "lost"}'`
  * Expected: `404 Not Found`, `{"error": "chat not found"}`.

### 3. Invalidation Conditions
- Any route handler failing to translate `domain.ErrNotFound` to 404 or `domain.ErrValidation` to 400.
- `ListChats` returning `null` instead of `[]` when no chats match.
- `handler.New` parameter mismatch between `main.go` and `internal/handler/handler.go`.
