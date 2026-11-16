package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kraconst/GO_server/internal/domain"
	"github.com/kraconst/GO_server/internal/usecase"
)

type alertRepoStub struct {
	create  func(context.Context, domain.Alert) (int64, error)
	list    func(context.Context, int64) ([]domain.Alert, error)
	resolve func(context.Context, int64, int64) error
}

func (s alertRepoStub) Create(ctx context.Context, a domain.Alert) (int64, error) {
	return s.create(ctx, a)
}

func (s alertRepoStub) ListActiveByCompany(ctx context.Context, companyID int64) ([]domain.Alert, error) {
	return s.list(ctx, companyID)
}

func (s alertRepoStub) Resolve(ctx context.Context, companyID, id int64) error {
	return s.resolve(ctx, companyID, id)
}

func TestAlertCreateRejectsInvalidInput(t *testing.T) {
	now := time.Now()
	tests := []domain.Alert{
		{CompanyID: 0, ChatID: 1, Type: domain.AlertTypeNoReply},
		{CompanyID: -1, ChatID: 1, Type: domain.AlertTypeNoReply},
		{CompanyID: 1, ChatID: 0, Type: domain.AlertTypeNoReply},
		{CompanyID: 1, ChatID: -1, Type: domain.AlertTypeNoReply},
		{CompanyID: 1, ChatID: 1},
		{CompanyID: 1, ChatID: 1, Type: "unknown"},
		{CompanyID: 1, ChatID: 1, Type: domain.AlertTypeUrgent, ResolvedAt: &now},
	}
	u := usecase.NewAlertUsecase(alertRepoStub{create: func(context.Context, domain.Alert) (int64, error) {
		t.Fatal("invalid alerts must not reach storage")
		return 0, nil
	}})
	for _, alert := range tests {
		if _, err := u.Create(context.Background(), alert); !errors.Is(err, domain.ErrValidation) {
			t.Errorf("Create(%+v): expected validation error, got %v", alert, err)
		}
	}
}

func TestAlertCreateAcceptsSupportedTypesAndSetsTimestamp(t *testing.T) {
	ctx := context.Background()
	for _, kind := range []domain.AlertType{domain.AlertTypeNoReply, domain.AlertTypeComplaint,
		domain.AlertTypeRefusal, domain.AlertTypeNegative, domain.AlertTypeUrgent} {
		t.Run(string(kind), func(t *testing.T) {
			before := time.Now().UTC()
			u := usecase.NewAlertUsecase(alertRepoStub{create: func(gotCtx context.Context, a domain.Alert) (int64, error) {
				if gotCtx != ctx || a.CompanyID != 7 || a.ChatID != 12 || a.Type != kind || a.ResolvedAt != nil {
					t.Fatalf("unexpected repository input: %+v", a)
				}
				if a.CreatedAt.Before(before) || a.CreatedAt.After(time.Now().UTC()) || a.CreatedAt.Location() != time.UTC {
					t.Fatalf("expected current UTC timestamp, got %v", a.CreatedAt)
				}
				return 42, nil
			}})
			id, err := u.Create(ctx, domain.Alert{CompanyID: 7, ChatID: 12, Type: kind})
			if err != nil || id != 42 {
				t.Fatalf("expected ID 42, got %d, %v", id, err)
			}
		})
	}
}

func TestAlertCreatePreservesEventTimeAndRepositoryError(t *testing.T) {
	wantErr := errors.New("storage unavailable")
	eventTime := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	u := usecase.NewAlertUsecase(alertRepoStub{create: func(_ context.Context, a domain.Alert) (int64, error) {
		if a.CreatedAt != eventTime {
			t.Fatalf("event time was changed: %v", a.CreatedAt)
		}
		return 0, wantErr
	}})
	_, err := u.Create(context.Background(), domain.Alert{
		CompanyID: 1, ChatID: 2, Type: domain.AlertTypeNoReply, CreatedAt: eventTime,
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected wrapped storage error, got %v", err)
	}
}

func TestAlertListValidatesCompanyAndNormalizesEmptyResult(t *testing.T) {
	u := usecase.NewAlertUsecase(alertRepoStub{list: func(_ context.Context, companyID int64) ([]domain.Alert, error) {
		if companyID != 7 {
			t.Fatalf("expected company 7, got %d", companyID)
		}
		return nil, nil
	}})
	for _, companyID := range []int64{0, -1} {
		if _, err := u.ListActiveByCompany(context.Background(), companyID); !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("expected validation error, got %v", err)
		}
	}
	alerts, err := u.ListActiveByCompany(context.Background(), 7)
	if err != nil || alerts == nil || len(alerts) != 0 {
		t.Fatalf("expected non-nil empty result, got %+v, %v", alerts, err)
	}
}

func TestAlertListPropagatesRepositoryError(t *testing.T) {
	wantErr := context.Canceled
	u := usecase.NewAlertUsecase(alertRepoStub{list: func(context.Context, int64) ([]domain.Alert, error) {
		return nil, wantErr
	}})
	if _, err := u.ListActiveByCompany(context.Background(), 1); !errors.Is(err, wantErr) {
		t.Fatalf("expected wrapped repository error, got %v", err)
	}
}

func TestAlertResolveValidatesIDsAndPreservesCompanyScope(t *testing.T) {
	u := usecase.NewAlertUsecase(alertRepoStub{resolve: func(_ context.Context, companyID, id int64) error {
		if companyID != 7 || id != 42 {
			t.Fatalf("unexpected company/alert IDs: %d/%d", companyID, id)
		}
		return domain.ErrNotFound
	}})
	for _, ids := range [][2]int64{{0, 1}, {-1, 1}, {1, 0}, {1, -1}} {
		if err := u.Resolve(context.Background(), ids[0], ids[1]); !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("expected validation error for %v, got %v", ids, err)
		}
	}
	if err := u.Resolve(context.Background(), 7, 42); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected wrapped not found, got %v", err)
	}
}
