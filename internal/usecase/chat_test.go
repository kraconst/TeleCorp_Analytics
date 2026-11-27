package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kraconst/GO_server/internal/domain"
	"github.com/kraconst/GO_server/internal/usecase"
)

type mockChatRepo struct {
	createFn           func(ctx context.Context, chat domain.Chat) (int64, error)
	getByIDFn          func(ctx context.Context, id int64) (*domain.Chat, error)
	listByCompanyFn    func(ctx context.Context, companyID int64, status *domain.ChatStatus, employeeID *int64) ([]domain.Chat, error)
	listByFilterFn     func(ctx context.Context, filter domain.ChatFilter) ([]domain.Chat, error)
	updateStatusFn     func(ctx context.Context, id int64, status domain.ChatStatus, source domain.StatusSource) error
	updateAutoStatusFn func(ctx context.Context, companyID, chatID int64, status domain.ChatStatus) error
}

func (m *mockChatRepo) Create(ctx context.Context, chat domain.Chat) (int64, error) {
	if m.createFn != nil {
		return m.createFn(ctx, chat)
	}
	return 0, nil
}

func (m *mockChatRepo) GetByID(ctx context.Context, id int64) (*domain.Chat, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
	}
	return nil, domain.ErrNotFound
}

func (m *mockChatRepo) ListByCompany(ctx context.Context, companyID int64, status *domain.ChatStatus, employeeID *int64) ([]domain.Chat, error) {
	if m.listByCompanyFn != nil {
		return m.listByCompanyFn(ctx, companyID, status, employeeID)
	}
	return []domain.Chat{}, nil
}

func (m *mockChatRepo) ListByFilter(ctx context.Context, filter domain.ChatFilter) ([]domain.Chat, error) {
	if m.listByFilterFn != nil {
		return m.listByFilterFn(ctx, filter)
	}
	return []domain.Chat{}, nil
}

func (m *mockChatRepo) UpdateStatus(ctx context.Context, id int64, status domain.ChatStatus, source domain.StatusSource) error {
	if m.updateStatusFn != nil {
		return m.updateStatusFn(ctx, id, status, source)
	}
	return nil
}

func (m *mockChatRepo) UpdateAutoStatus(ctx context.Context, companyID, chatID int64, status domain.ChatStatus) error {
	if m.updateAutoStatusFn != nil {
		return m.updateAutoStatusFn(ctx, companyID, chatID, status)
	}
	return nil
}

func TestChatUsecase_SetManualStatus(t *testing.T) {
	ctx := context.Background()

	t.Run("invalid company id", func(t *testing.T) {
		uc := usecase.NewChatUsecase(&mockChatRepo{}, nil, nil, nil)
		err := uc.SetManualStatus(ctx, 0, 10, domain.ChatStatusNegotiation)
		if !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("expected ErrValidation, got %v", err)
		}
	})

	t.Run("invalid chat id", func(t *testing.T) {
		uc := usecase.NewChatUsecase(&mockChatRepo{}, nil, nil, nil)
		err := uc.SetManualStatus(ctx, 1, 0, domain.ChatStatusNegotiation)
		if !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("expected ErrValidation, got %v", err)
		}
	})

	t.Run("invalid chat status", func(t *testing.T) {
		uc := usecase.NewChatUsecase(&mockChatRepo{}, nil, nil, nil)
		err := uc.SetManualStatus(ctx, 1, 1, domain.ChatStatus("unknown_status"))
		if !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("expected ErrValidation, got %v", err)
		}
	})

	t.Run("not found in repository", func(t *testing.T) {
		repo := &mockChatRepo{
			getByIDFn: func(ctx context.Context, id int64) (*domain.Chat, error) {
				return nil, domain.ErrNotFound
			},
			updateStatusFn: func(ctx context.Context, id int64, status domain.ChatStatus, source domain.StatusSource) error {
				return domain.ErrNotFound
			},
		}
		uc := usecase.NewChatUsecase(repo, nil, nil, nil)
		err := uc.SetManualStatus(ctx, 1, 999, domain.ChatStatusInterested)
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("successful manual status update with all valid statuses", func(t *testing.T) {
		validStatuses := []domain.ChatStatus{
			domain.ChatStatusNewLead,
			domain.ChatStatusNegotiation,
			domain.ChatStatusWaitingReply,
			domain.ChatStatusInterested,
			domain.ChatStatusUnhappy,
			domain.ChatStatusLost,
			domain.ChatStatusActionRequired,
		}

		for _, st := range validStatuses {
			called := false
			repo := &mockChatRepo{
				getByIDFn: func(ctx context.Context, id int64) (*domain.Chat, error) {
					return &domain.Chat{ID: id, AccountID: 1}, nil
				},
				updateStatusFn: func(ctx context.Context, id int64, status domain.ChatStatus, source domain.StatusSource) error {
					called = true
					if id != 10 {
						t.Errorf("expected id 10, got %d", id)
					}
					if status != st {
						t.Errorf("expected status %s, got %s", st, status)
					}
					if source != domain.StatusSourceManual {
						t.Errorf("expected source %s, got %s", domain.StatusSourceManual, source)
					}
					return nil
				},
			}
			uc := usecase.NewChatUsecase(repo, nil, nil, nil)
			err := uc.SetManualStatus(ctx, 1, 10, st)
			if err != nil {
				t.Fatalf("unexpected error for %s: %v", st, err)
			}
			if !called {
				t.Fatalf("updateStatusFn not called for %s", st)
			}
		}
	})
}

func TestChatUsecase_ListByCompany(t *testing.T) {
	ctx := context.Background()

	t.Run("invalid company_id", func(t *testing.T) {
		uc := usecase.NewChatUsecase(&mockChatRepo{}, nil, nil, nil)
		_, err := uc.ListByCompany(ctx, 0, nil, nil)
		if !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("expected ErrValidation, got %v", err)
		}
	})

	t.Run("invalid status", func(t *testing.T) {
		uc := usecase.NewChatUsecase(&mockChatRepo{}, nil, nil, nil)
		st := domain.ChatStatus("bogus")
		_, err := uc.ListByCompany(ctx, 1, &st, nil)
		if !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("expected ErrValidation, got %v", err)
		}
	})

	t.Run("invalid employee_id", func(t *testing.T) {
		uc := usecase.NewChatUsecase(&mockChatRepo{}, nil, nil, nil)
		empID := int64(-5)
		_, err := uc.ListByCompany(ctx, 1, nil, &empID)
		if !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("expected ErrValidation, got %v", err)
		}
	})

	t.Run("success with filters passed through", func(t *testing.T) {
		st := domain.ChatStatusInterested
		empID := int64(3)
		repo := &mockChatRepo{
			listByCompanyFn: func(ctx context.Context, companyID int64, status *domain.ChatStatus, employeeID *int64) ([]domain.Chat, error) {
				if companyID != 1 {
					t.Errorf("expected companyID 1, got %d", companyID)
				}
				if status == nil || *status != st {
					t.Errorf("expected status %v, got %v", st, status)
				}
				if employeeID == nil || *employeeID != empID {
					t.Errorf("expected employeeID %v, got %v", empID, employeeID)
				}
				return []domain.Chat{{ID: 101, Status: st}}, nil
			},
		}

		uc := usecase.NewChatUsecase(repo, nil, nil, nil)
		chats, err := uc.ListByCompany(ctx, 1, &st, &empID)
		if err != nil || len(chats) != 1 {
			t.Fatalf("unexpected result: %v, len=%d", err, len(chats))
		}
	})
}

func TestChatUsecase_ListByFilter(t *testing.T) {
	ctx := context.Background()

	t.Run("invalid company_id", func(t *testing.T) {
		uc := usecase.NewChatUsecase(&mockChatRepo{}, nil, nil, nil)
		for _, cid := range []int64{0, -1} {
			_, err := uc.ListByFilter(ctx, domain.ChatFilter{CompanyID: cid})
			if !errors.Is(err, domain.ErrValidation) {
				t.Fatalf("company_id=%d: expected ErrValidation, got %v", cid, err)
			}
		}
	})

	t.Run("invalid status", func(t *testing.T) {
		uc := usecase.NewChatUsecase(&mockChatRepo{}, nil, nil, nil)
		st := domain.ChatStatus("bogus")
		_, err := uc.ListByFilter(ctx, domain.ChatFilter{CompanyID: 1, Status: &st})
		if !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("expected ErrValidation, got %v", err)
		}
	})

	t.Run("invalid employee_id", func(t *testing.T) {
		uc := usecase.NewChatUsecase(&mockChatRepo{}, nil, nil, nil)
		for _, eid := range []int64{0, -5} {
			id := eid
			_, err := uc.ListByFilter(ctx, domain.ChatFilter{CompanyID: 1, EmployeeID: &id})
			if !errors.Is(err, domain.ErrValidation) {
				t.Fatalf("employee_id=%d: expected ErrValidation, got %v", eid, err)
			}
		}
	})

	t.Run("invalid account_id", func(t *testing.T) {
		uc := usecase.NewChatUsecase(&mockChatRepo{}, nil, nil, nil)
		for _, aid := range []int64{0, -3} {
			id := aid
			_, err := uc.ListByFilter(ctx, domain.ChatFilter{CompanyID: 1, AccountID: &id})
			if !errors.Is(err, domain.ErrValidation) {
				t.Fatalf("account_id=%d: expected ErrValidation, got %v", aid, err)
			}
		}
	})

	t.Run("invalid priority", func(t *testing.T) {
		uc := usecase.NewChatUsecase(&mockChatRepo{}, nil, nil, nil)
		pr := domain.ChatPriority("urgent")
		_, err := uc.ListByFilter(ctx, domain.ChatFilter{CompanyID: 1, Priority: &pr})
		if !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("expected ErrValidation, got %v", err)
		}
	})

	t.Run("invalid sort", func(t *testing.T) {
		uc := usecase.NewChatUsecase(&mockChatRepo{}, nil, nil, nil)
		_, err := uc.ListByFilter(ctx, domain.ChatFilter{CompanyID: 1, SortBy: "oldest"})
		if !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("expected ErrValidation, got %v", err)
		}
	})

	t.Run("invalid limit", func(t *testing.T) {
		uc := usecase.NewChatUsecase(&mockChatRepo{}, nil, nil, nil)
		for _, lim := range []int{-1, 101} {
			_, err := uc.ListByFilter(ctx, domain.ChatFilter{CompanyID: 1, Limit: lim})
			if !errors.Is(err, domain.ErrValidation) {
				t.Fatalf("limit=%d: expected ErrValidation, got %v", lim, err)
			}
		}
	})

	t.Run("invalid offset", func(t *testing.T) {
		uc := usecase.NewChatUsecase(&mockChatRepo{}, nil, nil, nil)
		_, err := uc.ListByFilter(ctx, domain.ChatFilter{CompanyID: 1, Offset: -1})
		if !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("expected ErrValidation, got %v", err)
		}
	})

	t.Run("default normalization and non-nil empty slice", func(t *testing.T) {
		var gotFilter domain.ChatFilter
		repo := &mockChatRepo{
			listByFilterFn: func(ctx context.Context, filter domain.ChatFilter) ([]domain.Chat, error) {
				gotFilter = filter
				return nil, nil
			},
		}
		uc := usecase.NewChatUsecase(repo, nil, nil, nil)
		chats, err := uc.ListByFilter(ctx, domain.ChatFilter{
			CompanyID: 2,
			Query:     "  abc  ",
			Limit:     0,
			SortBy:    "",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if chats == nil {
			t.Fatalf("expected non-nil empty slice, got nil")
		}
		if len(chats) != 0 {
			t.Fatalf("expected empty slice, got len=%d", len(chats))
		}
		if gotFilter.Limit != 50 {
			t.Errorf("expected default limit 50, got %d", gotFilter.Limit)
		}
		if gotFilter.SortBy != domain.ChatSortLastMessage {
			t.Errorf("expected default sort %q, got %q", domain.ChatSortLastMessage, gotFilter.SortBy)
		}
		if gotFilter.Query != "abc" {
			t.Errorf("expected trimmed query %q, got %q", "abc", gotFilter.Query)
		}
	})

	t.Run("success passes all filters to repository", func(t *testing.T) {
		empID := int64(7)
		accID := int64(11)
		st := domain.ChatStatusWaitingReply
		pr := domain.ChatPriorityHigh

		var gotFilter domain.ChatFilter
		repo := &mockChatRepo{
			listByFilterFn: func(ctx context.Context, filter domain.ChatFilter) ([]domain.Chat, error) {
				gotFilter = filter
				return []domain.Chat{{ID: 55, AccountID: accID, Status: st, Priority: pr}}, nil
			},
		}
		uc := usecase.NewChatUsecase(repo, nil, nil, nil)
		chats, err := uc.ListByFilter(ctx, domain.ChatFilter{
			CompanyID:      3,
			EmployeeID:     &empID,
			AccountID:      &accID,
			Status:         &st,
			Priority:       &pr,
			UnansweredOnly: true,
			Query:          " invoice ",
			SortBy:         domain.ChatSortAwaitingReply,
			Limit:          25,
			Offset:         10,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(chats) != 1 || chats[0].ID != 55 {
			t.Fatalf("unexpected chats: %+v", chats)
		}
		if gotFilter.CompanyID != 3 ||
			gotFilter.EmployeeID == nil || *gotFilter.EmployeeID != empID ||
			gotFilter.AccountID == nil || *gotFilter.AccountID != accID ||
			gotFilter.Status == nil || *gotFilter.Status != st ||
			gotFilter.Priority == nil || *gotFilter.Priority != pr ||
			!gotFilter.UnansweredOnly ||
			gotFilter.Query != "invoice" ||
			gotFilter.SortBy != domain.ChatSortAwaitingReply ||
			gotFilter.Limit != 25 ||
			gotFilter.Offset != 10 {
			t.Fatalf("filter mismatch: %+v", gotFilter)
		}
	})

	t.Run("propagates repository error", func(t *testing.T) {
		repoErr := errors.New("db connection lost")
		repo := &mockChatRepo{
			listByFilterFn: func(ctx context.Context, filter domain.ChatFilter) ([]domain.Chat, error) {
				return nil, repoErr
			},
		}
		uc := usecase.NewChatUsecase(repo, nil, nil, nil)
		_, err := uc.ListByFilter(ctx, domain.ChatFilter{CompanyID: 1})
		if !errors.Is(err, repoErr) {
			t.Fatalf("expected repoErr, got %v", err)
		}
	})
}

func TestChatUsecase_GetByID(t *testing.T) {
	ctx := context.Background()

	t.Run("invalid id", func(t *testing.T) {
		uc := usecase.NewChatUsecase(&mockChatRepo{}, nil, nil, nil)
		_, err := uc.GetByID(ctx, 1, 0)
		if !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("expected ErrValidation, got %v", err)
		}
	})

	t.Run("not found", func(t *testing.T) {
		repo := &mockChatRepo{
			getByIDFn: func(ctx context.Context, id int64) (*domain.Chat, error) {
				return nil, domain.ErrNotFound
			},
		}
		uc := usecase.NewChatUsecase(repo, nil, nil, nil)
		_, err := uc.GetByID(ctx, 1, 42)
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("success", func(t *testing.T) {
		repo := &mockChatRepo{
			getByIDFn: func(ctx context.Context, id int64) (*domain.Chat, error) {
				return &domain.Chat{ID: id, Status: domain.ChatStatusNewLead}, nil
			},
		}
		uc := usecase.NewChatUsecase(repo, nil, nil, nil)
		chat, err := uc.GetByID(ctx, 1, 42)
		if err != nil || chat.ID != 42 {
			t.Fatalf("unexpected result: %v, %+v", err, chat)
		}
	})
}

func TestChatUsecase_Create(t *testing.T) {
	ctx := context.Background()

	t.Run("invalid account_id or contact_id", func(t *testing.T) {
		uc := usecase.NewChatUsecase(&mockChatRepo{}, nil, nil, nil)
		if _, err := uc.Create(ctx, domain.Chat{AccountID: 0, ContactID: 1}); !errors.Is(err, domain.ErrValidation) {
			t.Errorf("expected ErrValidation, got %v", err)
		}
		if _, err := uc.Create(ctx, domain.Chat{AccountID: 1, ContactID: 0}); !errors.Is(err, domain.ErrValidation) {
			t.Errorf("expected ErrValidation, got %v", err)
		}
	})

	t.Run("invalid enum values", func(t *testing.T) {
		uc := usecase.NewChatUsecase(&mockChatRepo{}, nil, nil, nil)
		if _, err := uc.Create(ctx, domain.Chat{AccountID: 1, ContactID: 1, Status: "bad_status"}); !errors.Is(err, domain.ErrValidation) {
			t.Errorf("expected ErrValidation for status, got %v", err)
		}
		if _, err := uc.Create(ctx, domain.Chat{AccountID: 1, ContactID: 1, StatusSource: "bad_source"}); !errors.Is(err, domain.ErrValidation) {
			t.Errorf("expected ErrValidation for source, got %v", err)
		}
		if _, err := uc.Create(ctx, domain.Chat{AccountID: 1, ContactID: 1, Priority: "bad_priority"}); !errors.Is(err, domain.ErrValidation) {
			t.Errorf("expected ErrValidation for priority, got %v", err)
		}
	})

	t.Run("success with default values", func(t *testing.T) {
		repo := &mockChatRepo{
			createFn: func(ctx context.Context, chat domain.Chat) (int64, error) {
				if chat.Status != domain.ChatStatusNewLead {
					t.Errorf("expected default status %s, got %s", domain.ChatStatusNewLead, chat.Status)
				}
				if chat.StatusSource != domain.StatusSourceAuto {
					t.Errorf("expected default source %s, got %s", domain.StatusSourceAuto, chat.StatusSource)
				}
				if chat.Priority != domain.ChatPriorityNormal {
					t.Errorf("expected default priority %s, got %s", domain.ChatPriorityNormal, chat.Priority)
				}
				if chat.LastMessageAt.IsZero() {
					t.Errorf("expected LastMessageAt to be set")
				}
				return 77, nil
			},
			getByIDFn: func(ctx context.Context, id int64) (*domain.Chat, error) {
				return &domain.Chat{
					ID:            id,
					AccountID:     1,
					ContactID:     2,
					Status:        domain.ChatStatusNewLead,
					StatusSource:  domain.StatusSourceAuto,
					Priority:      domain.ChatPriorityNormal,
					LastMessageAt: time.Now().UTC(),
				}, nil
			},
		}

		uc := usecase.NewChatUsecase(repo, nil, nil, nil)
		chat, err := uc.Create(ctx, domain.Chat{AccountID: 1, ContactID: 2})
		if err != nil || chat.ID != 77 {
			t.Fatalf("unexpected result: %v, %+v", err, chat)
		}
	})

	t.Run("cross company account and contact returns ErrForbidden", func(t *testing.T) {
		repo := &mockChatRepo{}
		accRepo := &mockAccountRepoForChat{
			getByIDFn: func(ctx context.Context, id int64) (*domain.Account, error) {
				return &domain.Account{ID: id, CompanyID: 1}, nil
			},
		}
		contactRepo := &mockContactRepoForChat{
			getByIDFn: func(ctx context.Context, id int64) (*domain.Contact, error) {
				return &domain.Contact{ID: id, CompanyID: 2}, nil
			},
		}
		uc := usecase.NewChatUsecase(repo, contactRepo, nil, accRepo)
		_, err := uc.Create(ctx, domain.Chat{AccountID: 1, ContactID: 2})
		if !errors.Is(err, domain.ErrForbidden) {
			t.Fatalf("expected ErrForbidden, got %v", err)
		}
	})

	t.Run("GetDetails returns composite chat, contact and summary", func(t *testing.T) {
		repo := &mockChatRepo{
			getByIDFn: func(ctx context.Context, id int64) (*domain.Chat, error) {
				return &domain.Chat{ID: 10, ContactID: 20}, nil
			},
		}
		contactRepo := &mockContactRepoForChat{
			getByIDFn: func(ctx context.Context, id int64) (*domain.Contact, error) {
				return &domain.Contact{ID: 20, CompanyID: 1, Name: "Test Contact"}, nil
			},
		}
		summaryRepo := &mockSummaryRepoForChat{
			getByChatIDFn: func(ctx context.Context, companyID, chatID int64) (*domain.ChatSummary, error) {
				return &domain.ChatSummary{ChatID: 10, Topic: "AI Discussion"}, nil
			},
		}
		uc := usecase.NewChatUsecase(repo, contactRepo, summaryRepo, nil)
		details, err := uc.GetDetails(ctx, 1, 10)
		if err != nil {
			t.Fatalf("GetDetails failed: %v", err)
		}
		if details.ID != 10 || details.Contact == nil || details.Contact.Name != "Test Contact" || details.Summary == nil || details.Summary.Topic != "AI Discussion" {
			t.Fatalf("unexpected details: %+v", details)
		}
	})

	t.Run("GetDetails ignores contact if company_id does not match", func(t *testing.T) {
		repo := &mockChatRepo{
			getByIDFn: func(ctx context.Context, id int64) (*domain.Chat, error) {
				return &domain.Chat{ID: 10, ContactID: 20}, nil
			},
		}
		contactRepo := &mockContactRepoForChat{
			getByIDFn: func(ctx context.Context, id int64) (*domain.Contact, error) {
				return &domain.Contact{ID: 20, CompanyID: 999, Name: "Other Company Contact"}, nil
			},
		}
		uc := usecase.NewChatUsecase(repo, contactRepo, nil, nil)
		details, err := uc.GetDetails(ctx, 1, 10)
		if err != nil {
			t.Fatalf("GetDetails failed: %v", err)
		}
		if details.Contact != nil {
			t.Fatalf("expected details.Contact to be nil for mismatched company, got %+v", details.Contact)
		}
	})
}

type mockAccountRepoForChat struct {
	usecase.TelegramAccountStateRepository
	getByIDFn func(ctx context.Context, id int64) (*domain.Account, error)
}

func (m *mockAccountRepoForChat) Create(ctx context.Context, acc domain.Account) (int64, error) {
	return 0, nil
}
func (m *mockAccountRepoForChat) GetByID(ctx context.Context, id int64) (*domain.Account, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
	}
	return nil, domain.ErrNotFound
}
func (m *mockAccountRepoForChat) ListByCompany(ctx context.Context, companyID int64) ([]domain.Account, error) {
	return nil, nil
}
func (m *mockAccountRepoForChat) ListByEmployee(ctx context.Context, employeeID int64) ([]domain.Account, error) {
	return nil, nil
}
func (m *mockAccountRepoForChat) UpdateStatus(ctx context.Context, id int64, status domain.AccountStatus) error {
	return nil
}
func (m *mockAccountRepoForChat) AssignEmployee(ctx context.Context, accountID, employeeID int64) error {
	return nil
}

type mockContactRepoForChat struct {
	getByIDFn func(ctx context.Context, id int64) (*domain.Contact, error)
}

func (m *mockContactRepoForChat) Create(ctx context.Context, c domain.Contact) (int64, error) {
	return 0, nil
}
func (m *mockContactRepoForChat) GetByID(ctx context.Context, id int64) (*domain.Contact, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
	}
	return nil, domain.ErrNotFound
}
func (m *mockContactRepoForChat) GetByTelegramID(ctx context.Context, companyID, tgUserID int64) (*domain.Contact, error) {
	return nil, nil
}
func (m *mockContactRepoForChat) ListByCompany(ctx context.Context, companyID int64) ([]domain.Contact, error) {
	return nil, nil
}

type mockSummaryRepoForChat struct {
	getByChatIDFn func(ctx context.Context, companyID, chatID int64) (*domain.ChatSummary, error)
}

func (m *mockSummaryRepoForChat) Upsert(ctx context.Context, s domain.ChatSummary) error {
	return nil
}
func (m *mockSummaryRepoForChat) GetByChatID(ctx context.Context, companyID, chatID int64) (*domain.ChatSummary, error) {
	if m.getByChatIDFn != nil {
		return m.getByChatIDFn(ctx, companyID, chatID)
	}
	return nil, domain.ErrNotFound
}
