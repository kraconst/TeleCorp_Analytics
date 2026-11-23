# Handoff Report: Usecase & Domain Logic (Contact & Chat)

**Agent**: Explorer 2 (`explorer_m1_2`)  
**Target Recipient**: Orchestrator (`bdf1e875-d4c5-41f0-9ccd-bdd7bf772375`) and Implementer  
**Status**: Complete (Hard Handoff)  
**Deliverable Path**: `/home/asffe/3sem/GoLangCourse/GO_server/.agents/teamwork/explorer_m1_2/analysis.md`

---

## 1. Observation

1. **Domain Sentinel Errors (`internal/domain/errors.go:5-10`)**:
   ```go
   var (
   	ErrNotFound   = errors.New("not found")
   	ErrForbidden  = errors.New("forbidden")
   	ErrConflict   = errors.New("conflict")
   	ErrValidation = errors.New("validation error")
   )
   ```
2. **Domain Models (`internal/domain/contact.go:4-10` and `internal/domain/chat.go:8-43`)**:
   - `Contact` has json tags but lacks `db` tags.
   - `Chat` defines constants:
     - Status: `new_lead`, `negotiation`, `waiting_reply`, `interested`, `unhappy`, `lost`, `action_required` (lines 8-15).
     - StatusSource: `auto`, `manual` (lines 20-21).
     - Priority: `low`, `normal`, `high` (lines 27-29).
     - `Chat` struct currently lacks `db` struct tags (lines 33-43).
     - `ChatFilter` struct is currently missing from `chat.go`.
3. **Usecase Interfaces (`internal/usecase/interfaces.go:1-38`)**:
   - Defines `AccountRepository`, `EmployeeRepository`, `CompanyRepository`, and `MessageRepository`.
   - `ContactRepository` and `ChatRepository` are not yet declared.
4. **Existing Usecase Implementations**:
   - `internal/usecase/account.go:30-33, 44-55`: Validates required strings with `strings.TrimSpace` and wraps `domain.ErrValidation` with `fmt.Errorf("%w: ...", domain.ErrValidation)`. In `Create`, it calls `u.accounts.Create(ctx, acc)` then re-queries `u.accounts.GetByID(ctx, id)`, falling back to `acc.ID = id; return &acc, nil` if `GetByID` returns an error.
   - `internal/usecase/employee.go:28-32, 65-71`: Validates `id <= 0` with `fmt.Errorf("%w: invalid id", domain.ErrValidation)`. Updates check `emp.ID <= 0` and `name == ""`.
   - `internal/usecase/message.go:22-30`: Validates positive IDs (`ChatID <= 0`, `TelegramMessageID <= 0`) and enum values (`Direction != domain.MessageDirectionIn && Direction != domain.MessageDirectionOut`), wrapping `domain.ErrValidation`.
5. **Existing Handlers (`internal/handler/handler.go:119-124, 199-204, 341-348`)**:
   - Handlers inspect errors using `errors.Is(err, domain.ErrNotFound)` -> 404, and `errors.Is(err, domain.ErrValidation)` -> 400.
6. **Build Status**:
   - `go build ./...` exits with code 0 (verified on Go 1.26.5 linux/amd64).

---

## 2. Logic Chain

1. **Validation & Error Handling Convention (from Observations 1, 4, 5)**:
   - Since handlers rely on `errors.Is(err, domain.ErrValidation)` to render HTTP 400, any invalid input identified at the usecase layer (e.g. non-positive IDs, empty contact name, invalid chat status string) must wrap `domain.ErrValidation` using `fmt.Errorf("%w: ...", domain.ErrValidation)`.
   - Since handlers rely on `errors.Is(err, domain.ErrNotFound)` to render HTTP 404, repository `domain.ErrNotFound` must be propagated cleanly through usecase methods (`GetByID`, `SetManualStatus`, `FindOrCreateByTelegramID`).
2. **Contact Usecase Design (from Observations 2, 4)**:
   - `Create` must validate `CompanyID > 0`, `TelegramUserID > 0`, and trimmed `Name != ""`.
   - `FindOrCreateByTelegramID` must validate `companyID > 0`, `tgUserID > 0`, and trimmed `name != ""` before calling `GetByTelegramID`. If `domain.ErrNotFound` is returned, it creates the record.
   - `GetByID` and `ListByCompany` must validate `id > 0` and `companyID > 0` respectively.
3. **Chat Usecase Design (from Observations 2, 4, 5)**:
   - There are exactly 7 valid `domain.ChatStatus` values. A private helper `isValidChatStatus(status domain.ChatStatus) bool` validates status values against the domain constants.
   - `SetManualStatus` must validate `chatID > 0` and `isValidChatStatus(status)`. It must call `chats.UpdateStatus(ctx, chatID, status, domain.StatusSourceManual)`.
   - `ListByCompany` must validate `companyID > 0`. If `status` is non-nil, it must validate `isValidChatStatus(*status)`. If `employeeID` is non-nil, it must validate `*employeeID > 0`.
   - `GetByID` must validate `id > 0`.
   - `Create` must validate `AccountID > 0`, `ContactID > 0`, and apply sensible defaults (`ChatStatusNewLead`, `StatusSourceAuto`, `ChatPriorityNormal`, `time.Now().UTC()`) or validate explicit values against allowed enums.

---

## 3. Caveats

- **Mock Handlers vs Database Mode**: In mock fallback mode (when PostgreSQL is unreachable at startup), usecase methods are not invoked because `Handler` fields are `nil`. Mock mode handlers must implement corresponding fallback validations locally (e.g., checking status strings).
- **Hard-coded Tenant in Handlers**: The API handlers currently hardcode `companyID = 1`. The usecase layer remains tenant-agnostic by accepting `companyID` as an explicit parameter in `ListByCompany` and `FindOrCreateByTelegramID`.
- **Concurrency in FindOrCreate**: The storage implementation of `ContactStorage.Create` relies on PostgreSQL `ON CONFLICT (company_id, telegram_user_id) DO UPDATE`. Thus, concurrent `FindOrCreateByTelegramID` calls in usecase will not trigger duplicate key errors at the database layer.

---

## 4. Conclusion

The domain and usecase design is fully mapped, verified against all existing codebase conventions, and ready for immediate implementation.

### Key Deliverables in `analysis.md`:
1. **Domain Models**:
   - `internal/domain/contact.go`: Add `db` tags.
   - `internal/domain/chat.go`: Add `db` tags to `Chat` and declare `ChatFilter` struct.
2. **Repository Interfaces**:
   - `internal/usecase/interfaces.go`: Add `ContactRepository` and `ChatRepository` with exact method signatures.
3. **Usecase Implementations**:
   - `internal/usecase/contact.go`: Full blueprint with `NewContactUsecase`, `Create`, `FindOrCreateByTelegramID`, `GetByID`, and `ListByCompany`, validating and wrapping `domain.ErrValidation`.
   - `internal/usecase/chat.go`: Full blueprint with `NewChatUsecase`, `SetManualStatus`, `ListByCompany`, `GetByID`, `Create`, and `isValidChatStatus` validating the 7 allowed statuses.

---

## 5. Verification Method

To independently verify the usecase layer once implemented:

1. **Compilation Check**:
   ```bash
   go build ./...
   ```
   Must succeed with exit code 0.
2. **Static Analysis & Formatting**:
   ```bash
   go fmt ./...
   go vet ./...
   ```
   Must produce zero diff and exit with code 0.
3. **Unit Tests (when test files are added)**:
   ```bash
   go test ./internal/usecase/...
   ```
4. **Invalidation Conditions**:
   - If `SetManualStatus` accepts an unrecognized status (e.g. `"foo"`) without returning an error that matches `errors.Is(err, domain.ErrValidation)`.
   - If `FindOrCreateByTelegramID` crashes or fails on empty contact name.
   - If `ListByCompany` fails to reject negative `employeeID` or invalid `status`.
