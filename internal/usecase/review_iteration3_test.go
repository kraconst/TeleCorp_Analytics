package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kraconst/GO_server/internal/domain"
	"github.com/kraconst/GO_server/internal/usecase"
)

type mockAccountRepoStub struct {
	usecase.TelegramAccountStateRepository
	getByID func(ctx context.Context, id int64) (*domain.Account, error)
	assign  func(ctx context.Context, accountID, employeeID int64) error
}

func (m *mockAccountRepoStub) Create(ctx context.Context, acc domain.Account) (int64, error) {
	return 1, nil
}
func (m *mockAccountRepoStub) GetByID(ctx context.Context, id int64) (*domain.Account, error) {
	if m.getByID != nil {
		return m.getByID(ctx, id)
	}
	return &domain.Account{ID: id, CompanyID: 1}, nil
}
func (m *mockAccountRepoStub) ListByCompany(ctx context.Context, companyID int64) ([]domain.Account, error) {
	return nil, nil
}
func (m *mockAccountRepoStub) ListByEmployee(ctx context.Context, employeeID int64) ([]domain.Account, error) {
	return nil, nil
}
func (m *mockAccountRepoStub) UpdateStatus(ctx context.Context, id int64, status domain.AccountStatus) error {
	return nil
}
func (m *mockAccountRepoStub) AssignEmployee(ctx context.Context, accountID, employeeID int64) error {
	if m.assign != nil {
		return m.assign(ctx, accountID, employeeID)
	}
	return nil
}

type mockChatRepoStub struct {
	getByID func(ctx context.Context, id int64) (*domain.Chat, error)
}

func (m *mockChatRepoStub) Create(ctx context.Context, chat domain.Chat) (int64, error) {
	return 1, nil
}
func (m *mockChatRepoStub) GetByID(ctx context.Context, id int64) (*domain.Chat, error) {
	if m.getByID != nil {
		return m.getByID(ctx, id)
	}
	return &domain.Chat{ID: id, AccountID: 1}, nil
}
func (m *mockChatRepoStub) ListByCompany(ctx context.Context, companyID int64, status *domain.ChatStatus, employeeID *int64) ([]domain.Chat, error) {
	return nil, nil
}
func (m *mockChatRepoStub) UpdateStatus(ctx context.Context, id int64, status domain.ChatStatus, source domain.StatusSource) error {
	return nil
}

type mockSummaryRepoStub struct {
	upsert func(ctx context.Context, summary domain.ChatSummary) error
}

func (m *mockSummaryRepoStub) Upsert(ctx context.Context, summary domain.ChatSummary) error {
	if m.upsert != nil {
		return m.upsert(ctx, summary)
	}
	return nil
}
func (m *mockSummaryRepoStub) GetByChatID(ctx context.Context, companyID, chatID int64) (*domain.ChatSummary, error) {
	return nil, nil
}

type mockEmployeeRepoStub struct {
	getByID func(ctx context.Context, id int64) (*domain.Employee, error)
}

func (m *mockEmployeeRepoStub) Create(ctx context.Context, emp domain.Employee) (int64, error) {
	return 1, nil
}
func (m *mockEmployeeRepoStub) GetByID(ctx context.Context, id int64) (*domain.Employee, error) {
	if m.getByID != nil {
		return m.getByID(ctx, id)
	}
	return &domain.Employee{ID: id, CompanyID: 1, IsActive: true}, nil
}
func (m *mockEmployeeRepoStub) ListByCompany(ctx context.Context, companyID int64) ([]domain.Employee, error) {
	return nil, nil
}
func (m *mockEmployeeRepoStub) Update(ctx context.Context, emp domain.Employee) error {
	return nil
}
func (m *mockEmployeeRepoStub) Deactivate(ctx context.Context, id int64) error {
	return nil
}

type mockMessageRepoStub struct {
	listByChat func(ctx context.Context, companyID, chatID int64, limit, offset int) ([]domain.Message, error)
}

func (m *mockMessageRepoStub) Create(ctx context.Context, msg domain.Message) (int64, error) {
	return 1, nil
}
func (m *mockMessageRepoStub) ListByChat(ctx context.Context, companyID, chatID int64, limit, offset int) ([]domain.Message, error) {
	if m.listByChat != nil {
		return m.listByChat(ctx, companyID, chatID, limit, offset)
	}
	return []domain.Message{}, nil
}

// 1. Critical 2: ChatSummaryUsecase.Upsert IDOR check
func TestChatSummaryUsecase_Upsert_CompanyOwnership(t *testing.T) {
	ctx := context.Background()
	chatRepo := &mockChatRepoStub{
		getByID: func(ctx context.Context, id int64) (*domain.Chat, error) {
			return &domain.Chat{ID: id, AccountID: 100}, nil
		},
	}
	accountRepo := &mockAccountRepoStub{
		getByID: func(ctx context.Context, id int64) (*domain.Account, error) {
			// Account 100 belongs to company 1
			return &domain.Account{ID: id, CompanyID: 1}, nil
		},
	}
	summaryRepo := &mockSummaryRepoStub{}

	uc := usecase.NewChatSummaryUsecase(summaryRepo, chatRepo, accountRepo)

	// Company 1 can upsert summary for chat in company 1
	err := uc.Upsert(ctx, 1, domain.ChatSummary{ChatID: 10, Topic: "Test"})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}

	// Negative last_message_id
	err = uc.Upsert(ctx, 1, domain.ChatSummary{ChatID: 10, Topic: "Test", LastMessageID: -1})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected ErrValidation for negative last_message_id, got %v", err)
	}

	// Company 2 attempting to upsert summary for chat in company 1 must fail with ErrNotFound
	err = uc.Upsert(ctx, 2, domain.ChatSummary{ChatID: 10, Topic: "Hacked"})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for wrong company, got %v", err)
	}
}

// 2. Medium 1: ListMessagesByChat returns 404 for non-existent or other company chat
func TestMessageUsecase_ListByChat_NotFound(t *testing.T) {
	ctx := context.Background()
	chatRepo := &mockChatRepoStub{
		getByID: func(ctx context.Context, id int64) (*domain.Chat, error) {
			if id == 999 {
				return nil, domain.ErrNotFound
			}
			return &domain.Chat{ID: id, AccountID: 100}, nil
		},
	}
	accountRepo := &mockAccountRepoStub{
		getByID: func(ctx context.Context, id int64) (*domain.Account, error) {
			return &domain.Account{ID: id, CompanyID: 1}, nil
		},
	}
	msgRepo := &mockMessageRepoStub{}

	uc := usecase.NewMessageUsecase(msgRepo, chatRepo, accountRepo)

	// Non-existent chat
	_, err := uc.ListByChat(ctx, 1, 999, 10, 0)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for missing chat, got %v", err)
	}

	// Chat belonging to company 1 requested by company 2
	_, err = uc.ListByChat(ctx, 2, 10, 10, 0)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for wrong company, got %v", err)
	}

	// Valid company
	msgs, err := uc.ListByChat(ctx, 1, 10, 10, 0)
	if err != nil || msgs == nil {
		t.Fatalf("expected success, got err=%v", err)
	}

	// Limit validation
	for _, l := range []int{0, -1, 101, 500} {
		_, err := uc.ListByChat(ctx, 1, 10, l, 0)
		if !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("expected ErrValidation for limit=%d, got %v", l, err)
		}
	}
	// Offset validation
	if _, err := uc.ListByChat(ctx, 1, 10, 50, -1); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected ErrValidation for negative offset, got %v", err)
	}
}

// 3. Medium 2: UserUsecase validation (Role, EmployeeID check) & CompanyUsecase timezone validation
func TestUserUsecase_Validation(t *testing.T) {
	ctx := context.Background()
	empRepo := &mockEmployeeRepoStub{
		getByID: func(ctx context.Context, id int64) (*domain.Employee, error) {
			if id == 1 {
				return &domain.Employee{ID: 1, CompanyID: 1, IsActive: true}, nil
			}
			if id == 2 {
				return &domain.Employee{ID: 2, CompanyID: 2, IsActive: true}, nil
			}
			if id == 3 {
				return &domain.Employee{ID: 3, CompanyID: 1, IsActive: false}, nil
			}
			return nil, domain.ErrNotFound
		},
	}

	uc := usecase.NewUserUsecase(nil, empRepo)

	// Invalid role
	_, err := uc.Create(ctx, domain.User{
		CompanyID:    1,
		Email:        "user@example.com",
		PasswordHash: "secret",
		Role:         "admin",
	})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected ErrValidation for role admin, got %v", err)
	}

	// Employee not found
	empNotFound := int64(999)
	_, err = uc.Create(ctx, domain.User{
		CompanyID:    1,
		Email:        "user@example.com",
		PasswordHash: "secret",
		Role:         domain.RoleManager,
		EmployeeID:   &empNotFound,
	})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for non-existent employee, got %v", err)
	}

	// Inactive employee cannot be linked
	empInactive := int64(3)
	_, err = uc.Create(ctx, domain.User{
		CompanyID:    1,
		Email:        "inactive@example.com",
		PasswordHash: "secret",
		Role:         domain.RoleManager,
		EmployeeID:   &empInactive,
	})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected ErrValidation for inactive employee, got %v", err)
	}

	// Employee lookup database error must be returned directly, not masked as ErrNotFound
	dbErr := errors.New("connection reset by peer")
	empRepoWithDbErr := &mockEmployeeRepoStub{
		getByID: func(ctx context.Context, id int64) (*domain.Employee, error) {
			return nil, dbErr
		},
	}
	ucWithDbErr := usecase.NewUserUsecase(nil, empRepoWithDbErr)
	empID := int64(1)
	_, err = ucWithDbErr.Create(ctx, domain.User{
		CompanyID:    1,
		Email:        "user@example.com",
		PasswordHash: "secret",
		Role:         domain.RoleManager,
		EmployeeID:   &empID,
	})
	if !errors.Is(err, dbErr) {
		t.Fatalf("expected direct dbErr, got %v", err)
	}
}

func TestCompanyUsecase_TimezoneValidation(t *testing.T) {
	ctx := context.Background()
	uc := usecase.NewCompanyUsecase(nil)

	// Invalid timezone
	_, err := uc.Create(ctx, domain.Company{
		Name:     "Test Corp",
		Timezone: "Invalid/Not_A_Timezone_123",
	})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected ErrValidation for invalid timezone, got %v", err)
	}

	// Negative threshold
	_, err = uc.Create(ctx, domain.Company{
		Name:             "Test Corp",
		Timezone:         "Europe/Moscow",
		NoReplyThreshold: -1,
	})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected ErrValidation for negative threshold, got %v", err)
	}
}

// 4. Medium 3: AlertUsecase chat company validation & AccountUsecase.AssignEmployee companyID check
func TestAlertUsecase_Create_CompanyIsolation(t *testing.T) {
	ctx := context.Background()
	chatRepo := &mockChatRepoStub{
		getByID: func(ctx context.Context, id int64) (*domain.Chat, error) {
			return &domain.Chat{ID: id, AccountID: 100}, nil
		},
	}
	accountRepo := &mockAccountRepoStub{
		getByID: func(ctx context.Context, id int64) (*domain.Account, error) {
			// Account 100 belongs to company 1
			return &domain.Account{ID: id, CompanyID: 1}, nil
		},
	}

	uc := usecase.NewAlertUsecase(nil, chatRepo, accountRepo)

	// Alert for company 2 pointing to chat of company 1 must fail with ErrForbidden
	_, err := uc.Create(ctx, domain.Alert{
		CompanyID: 2,
		ChatID:    10,
		Type:      domain.AlertTypeNoReply,
	})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for cross-company alert, got %v", err)
	}
}

func TestAccountUsecase_AssignEmployee_CompanyIsolation(t *testing.T) {
	ctx := context.Background()
	accountRepo := &mockAccountRepoStub{
		getByID: func(ctx context.Context, id int64) (*domain.Account, error) {
			if id == 1 {
				return &domain.Account{ID: 1, CompanyID: 1}, nil
			}
			return &domain.Account{ID: id, CompanyID: 2}, nil
		},
	}
	empRepo := &mockEmployeeRepoStub{
		getByID: func(ctx context.Context, id int64) (*domain.Employee, error) {
			if id == 1 {
				return &domain.Employee{ID: 1, CompanyID: 1, IsActive: true}, nil
			}
			if id == 2 {
				return &domain.Employee{ID: 2, CompanyID: 2, IsActive: true}, nil
			}
			if id == 3 {
				return &domain.Employee{ID: 3, CompanyID: 1, IsActive: false}, nil
			}
			return &domain.Employee{ID: id, CompanyID: 2, IsActive: true}, nil
		},
	}

	uc := usecase.NewAccountUsecase(accountRepo, empRepo)

	// Inactive employee cannot be assigned
	err := uc.AssignEmployee(ctx, 1, 1, 3)
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected ErrValidation for assigning inactive employee, got %v", err)
	}

	// Inactive employee cannot be used in Create
	_, err = uc.Create(ctx, usecase.CreateAccountInput{
		CompanyID:  1,
		EmployeeID: 3,
		Phone:      "+79991112233",
	})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected ErrValidation for creating account with inactive employee, got %v", err)
	}

	// Caller company 1 cannot assign account 2 (which belongs to company 2)
	err = uc.AssignEmployee(ctx, 1, 2, 2)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for account belonging to other company, got %v", err)
	}

	// Caller company 1 cannot assign employee 2 (from company 2) to account 1 (company 1)
	err = uc.AssignEmployee(ctx, 1, 1, 2)
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for employee from another company, got %v", err)
	}

	// Valid assignment within company 1
	err = uc.AssignEmployee(ctx, 1, 1, 1)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
}

func TestAccountDailyStatsUsecase_AccountCompanyCheck(t *testing.T) {
	ctx := context.Background()
	accountRepo := &mockAccountRepoStub{
		getByID: func(ctx context.Context, id int64) (*domain.Account, error) {
			if id == 999 {
				return nil, domain.ErrNotFound
			}
			return &domain.Account{ID: id, CompanyID: 1}, nil
		},
	}

	uc := usecase.NewAccountDailyStatsUsecase(nil, accountRepo)

	// Upsert with non-existent account
	err := uc.Upsert(ctx, domain.AccountDailyStats{
		AccountID: 999,
		Day:       time.Now().UTC(),
	})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for missing account in Upsert, got %v", err)
	}

	// Increment with non-existent account
	err = uc.IncrementCounters(ctx, 999, time.Now().UTC(), 1, 1, 0)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for missing account in IncrementCounters, got %v", err)
	}

	// GetByAccountAndDateRange for wrong company
	_, err = uc.GetByAccountAndDateRange(ctx, 2, 1, time.Now().Add(-24*time.Hour), time.Now())
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for wrong company, got %v", err)
	}
}
