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
	create       func(context.Context, domain.Alert) (int64, error)
	existsActive func(context.Context, int64, domain.AlertType) (bool, error)
	findExpired  func(context.Context) ([]domain.NoReplyChat, error)
	list         func(context.Context, int64) ([]domain.Alert, error)
	resolve      func(context.Context, int64, int64) error
}

func (s alertRepoStub) Create(ctx context.Context, a domain.Alert) (int64, error) {
	return s.create(ctx, a)
}

func (s alertRepoStub) ExistsActive(ctx context.Context, chatID int64, alertType domain.AlertType) (bool, error) {
	if s.existsActive != nil {
		return s.existsActive(ctx, chatID, alertType)
	}
	return false, nil
}

func (s alertRepoStub) FindExpiredNoReplyChats(ctx context.Context) ([]domain.NoReplyChat, error) {
	if s.findExpired != nil {
		return s.findExpired(ctx)
	}
	return nil, nil
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
	}}, nil, nil)
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
			}}, nil, nil)
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
	}}, nil, nil)
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
	}}, nil, nil)
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
	}}, nil, nil)
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
	}}, nil, nil)
	for _, ids := range [][2]int64{{0, 1}, {-1, 1}, {1, 0}, {1, -1}} {
		if err := u.Resolve(context.Background(), ids[0], ids[1]); !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("expected validation error for %v, got %v", ids, err)
		}
	}
	if err := u.Resolve(context.Background(), 7, 42); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected wrapped not found, got %v", err)
	}
}

type inMemoryAlertRepo struct {
	nextID  int64
	alerts  []domain.Alert
	expired []domain.NoReplyChat
}

func (r *inMemoryAlertRepo) Create(_ context.Context, a domain.Alert) (int64, error) {
	for _, existing := range r.alerts {
		if existing.ChatID == a.ChatID && existing.Type == a.Type && existing.ResolvedAt == nil {
			return 0, nil
		}
	}
	r.nextID++
	a.ID = r.nextID
	r.alerts = append(r.alerts, a)
	return a.ID, nil
}

func (r *inMemoryAlertRepo) ExistsActive(_ context.Context, chatID int64, alertType domain.AlertType) (bool, error) {
	for _, existing := range r.alerts {
		if existing.ChatID == chatID && existing.Type == alertType && existing.ResolvedAt == nil {
			return true, nil
		}
	}
	return false, nil
}

func (r *inMemoryAlertRepo) FindExpiredNoReplyChats(context.Context) ([]domain.NoReplyChat, error) {
	out := make([]domain.NoReplyChat, 0, len(r.expired))
	for _, c := range r.expired {
		active := false
		for _, a := range r.alerts {
			if a.ChatID == c.ChatID && a.Type == domain.AlertTypeNoReply && a.ResolvedAt == nil {
				active = true
				break
			}
		}
		if !active {
			out = append(out, c)
		}
	}
	return out, nil
}

func (r *inMemoryAlertRepo) ListActiveByCompany(_ context.Context, companyID int64) ([]domain.Alert, error) {
	out := make([]domain.Alert, 0)
	for _, a := range r.alerts {
		if a.CompanyID == companyID && a.ResolvedAt == nil {
			out = append(out, a)
		}
	}
	return out, nil
}

func (r *inMemoryAlertRepo) Resolve(_ context.Context, companyID, id int64) error {
	now := time.Now().UTC()
	for i := range r.alerts {
		if r.alerts[i].ID == id && r.alerts[i].CompanyID == companyID {
			if r.alerts[i].ResolvedAt == nil {
				r.alerts[i].ResolvedAt = &now
			}
			return nil
		}
	}
	return domain.ErrNotFound
}

func TestAlertCreateDeduplicatesActiveAlerts(t *testing.T) {
	ctx := context.Background()
	repo := &inMemoryAlertRepo{}
	u := usecase.NewAlertUsecase(repo, nil, nil)

	firstID, err := u.Create(ctx, domain.Alert{CompanyID: 1, ChatID: 10, Type: domain.AlertTypeComplaint})
	if err != nil || firstID == 0 {
		t.Fatalf("expected first alert to be created, got id=%d, err=%v", firstID, err)
	}

	// Duplicate active alert of the same type for the same chat must be skipped.
	dupID, err := u.Create(ctx, domain.Alert{CompanyID: 1, ChatID: 10, Type: domain.AlertTypeComplaint})
	if err != nil || dupID != 0 {
		t.Fatalf("expected duplicate active alert to be skipped (id=0, err=nil), got id=%d, err=%v", dupID, err)
	}

	// Different alert type on the same chat is allowed.
	otherTypeID, err := u.Create(ctx, domain.Alert{CompanyID: 1, ChatID: 10, Type: domain.AlertTypeUrgent})
	if err != nil || otherTypeID == 0 {
		t.Fatalf("expected different type alert to be created, got id=%d, err=%v", otherTypeID, err)
	}

	// Same alert type on a different chat is allowed.
	otherChatID, err := u.Create(ctx, domain.Alert{CompanyID: 1, ChatID: 20, Type: domain.AlertTypeComplaint})
	if err != nil || otherChatID == 0 {
		t.Fatalf("expected alert for another chat to be created, got id=%d, err=%v", otherChatID, err)
	}

	// Once resolved, a new alert of the same type can be created again.
	if err := u.Resolve(ctx, 1, firstID); err != nil {
		t.Fatalf("resolve first alert: %v", err)
	}
	recreatedID, err := u.Create(ctx, domain.Alert{CompanyID: 1, ChatID: 10, Type: domain.AlertTypeComplaint})
	if err != nil || recreatedID == 0 || recreatedID == firstID {
		t.Fatalf("expected new alert after resolution, got id=%d, err=%v", recreatedID, err)
	}
}

func TestAlertCreatePropagatesExistsActiveError(t *testing.T) {
	wantErr := errors.New("db failure")
	u := usecase.NewAlertUsecase(alertRepoStub{
		existsActive: func(context.Context, int64, domain.AlertType) (bool, error) {
			return false, wantErr
		},
		create: func(context.Context, domain.Alert) (int64, error) {
			t.Fatal("Create must not be called when ExistsActive fails")
			return 0, nil
		},
	}, nil, nil)

	if _, err := u.Create(context.Background(), domain.Alert{CompanyID: 1, ChatID: 5, Type: domain.AlertTypeUrgent}); !errors.Is(err, wantErr) {
		t.Fatalf("expected wrapped ExistsActive error, got %v", err)
	}
}

func TestAlertApplyAIAnalysis_ProtectsManualStatusAndDeduplicatesAlerts(t *testing.T) {
	ctx := context.Background()

	chatsByID := map[int64]*domain.Chat{
		10: {ID: 10, AccountID: 1, Status: domain.ChatStatusNewLead, StatusSource: domain.StatusSourceAuto},
		11: {ID: 11, AccountID: 1, Status: domain.ChatStatusLost, StatusSource: domain.StatusSourceManual},
	}
	chatRepo := &mockChatRepo{
		getByIDFn: func(_ context.Context, id int64) (*domain.Chat, error) {
			c, ok := chatsByID[id]
			if !ok {
				return nil, domain.ErrNotFound
			}
			cp := *c
			return &cp, nil
		},
		updateAutoStatusFn: func(_ context.Context, chatID int64, status domain.ChatStatus) error {
			c, ok := chatsByID[chatID]
			if !ok {
				return nil
			}
			if c.StatusSource == domain.StatusSourceManual {
				return nil
			}
			c.Status = status
			c.StatusSource = domain.StatusSourceAuto
			return nil
		},
	}
	accountRepo := &mockAccountRepoStub{
		getByID: func(_ context.Context, id int64) (*domain.Account, error) {
			return &domain.Account{ID: id, CompanyID: 1}, nil
		},
	}
	alertRepo := &inMemoryAlertRepo{}

	alertUC := usecase.NewAlertUsecase(alertRepo, chatRepo, accountRepo)
	summaryUC := usecase.NewChatSummaryUsecase(&mockSummaryRepoStub{}, chatRepo, accountRepo)
	summaryUC.SetAnalysisHandler(alertUC.ApplyAIAnalysis)

	suggestedUnhappy := domain.ChatStatusUnhappy
	suggestedInterested := domain.ChatStatusInterested

	// Auto chat status is updated and AI alerts are created without duplicates.
	err := summaryUC.ApplyAnalysisResult(ctx, 1, 10, &usecase.AIAnalysisResult{
		Topic:           "Complaint about delay",
		Risk:            domain.RiskHigh,
		SuggestedStatus: &suggestedUnhappy,
		AlertTypes: []domain.AlertType{
			domain.AlertTypeComplaint,
			domain.AlertTypeNegative,
			domain.AlertTypeComplaint, // duplicate in same payload
		},
	})
	if err != nil {
		t.Fatalf("ApplyAnalysisResult for auto chat failed: %v", err)
	}
	if chatsByID[10].Status != domain.ChatStatusUnhappy || chatsByID[10].StatusSource != domain.StatusSourceAuto {
		t.Fatalf("expected auto status to become unhappy/auto, got %+v", chatsByID[10])
	}

	// Re-running AI analysis for the same chat must not create duplicate active alerts.
	err = summaryUC.ApplyAnalysisResult(ctx, 1, 10, &usecase.AIAnalysisResult{
		SuggestedStatus: &suggestedInterested,
		AlertTypes:      []domain.AlertType{domain.AlertTypeComplaint, domain.AlertTypeUrgent},
	})
	if err != nil {
		t.Fatalf("second ApplyAnalysisResult failed: %v", err)
	}
	active, err := alertUC.ListActiveByCompany(ctx, 1)
	if err != nil {
		t.Fatalf("ListActiveByCompany failed: %v", err)
	}
	if len(active) != 3 {
		t.Fatalf("expected 3 deduplicated alerts (complaint, negative, urgent), got %d: %+v", len(active), active)
	}

	// Manual chat status must NOT be overwritten by AI analysis, while alerts are still created.
	err = summaryUC.ApplyAnalysisResult(ctx, 1, 11, &usecase.AIAnalysisResult{
		SuggestedStatus: &suggestedInterested,
		AlertTypes:      []domain.AlertType{domain.AlertTypeRefusal},
	})
	if err != nil {
		t.Fatalf("ApplyAnalysisResult for manual chat failed: %v", err)
	}
	if chatsByID[11].Status != domain.ChatStatusLost || chatsByID[11].StatusSource != domain.StatusSourceManual {
		t.Fatalf("manual status was overwritten: %+v", chatsByID[11])
	}
	active, err = alertUC.ListActiveByCompany(ctx, 1)
	if err != nil {
		t.Fatalf("ListActiveByCompany failed: %v", err)
	}
	if len(active) != 4 {
		t.Fatalf("expected 4 active alerts total, got %d", len(active))
	}
}

func TestAlertApplyAIAnalysis_ValidationAndIsolation(t *testing.T) {
	ctx := context.Background()
	chatRepo := &mockChatRepo{
		getByIDFn: func(_ context.Context, id int64) (*domain.Chat, error) {
			return &domain.Chat{ID: id, AccountID: 1}, nil
		},
		updateAutoStatusFn: func(context.Context, int64, domain.ChatStatus) error {
			t.Fatal("UpdateAutoStatus must not be called on validation or IDOR failure")
			return nil
		},
	}
	accountRepo := &mockAccountRepoStub{
		getByID: func(_ context.Context, id int64) (*domain.Account, error) {
			return &domain.Account{ID: id, CompanyID: 1}, nil
		},
	}
	u := usecase.NewAlertUsecase(&inMemoryAlertRepo{}, chatRepo, accountRepo)

	badStatus := domain.ChatStatus("bad_status")
	for _, tc := range []struct {
		name      string
		companyID int64
		chatID    int64
		res       *usecase.AIAnalysisResult
		wantErr   error
	}{
		{"invalid company", 0, 1, &usecase.AIAnalysisResult{}, domain.ErrValidation},
		{"invalid chat", 1, 0, &usecase.AIAnalysisResult{}, domain.ErrValidation},
		{"nil result", 1, 1, nil, domain.ErrValidation},
		{"invalid suggested status", 1, 1, &usecase.AIAnalysisResult{SuggestedStatus: &badStatus}, domain.ErrValidation},
		{"invalid alert type", 1, 1, &usecase.AIAnalysisResult{AlertTypes: []domain.AlertType{"unknown"}}, domain.ErrValidation},
		{"no_reply not allowed from AI", 1, 1, &usecase.AIAnalysisResult{AlertTypes: []domain.AlertType{domain.AlertTypeNoReply}}, domain.ErrValidation},
		{"cross company forbidden", 2, 1, &usecase.AIAnalysisResult{}, domain.ErrForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := u.ApplyAIAnalysis(ctx, tc.companyID, tc.chatID, tc.res)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("expected %v, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestAlertCheckNoReplyAlerts(t *testing.T) {
	ctx := context.Background()

	t.Run("creates no_reply alerts and deduplicates repeat runs", func(t *testing.T) {
		repo := &inMemoryAlertRepo{
			expired: []domain.NoReplyChat{
				{CompanyID: 1, ChatID: 101},
				{CompanyID: 2, ChatID: 202},
			},
		}
		u := usecase.NewAlertUsecase(repo, nil, nil)

		created, err := u.CheckNoReplyAlerts(ctx)
		if err != nil {
			t.Fatalf("first CheckNoReplyAlerts failed: %v", err)
		}
		if created != 2 {
			t.Fatalf("expected 2 created alerts, got %d", created)
		}

		// Repeat run must not create duplicate active alerts.
		createdAgain, err := u.CheckNoReplyAlerts(ctx)
		if err != nil {
			t.Fatalf("second CheckNoReplyAlerts failed: %v", err)
		}
		if createdAgain != 0 {
			t.Fatalf("expected 0 created alerts on repeat run, got %d", createdAgain)
		}

		// Even if repository returns an already-active chat (race simulation), Create deduplicates it.
		repo.alerts = append(repo.alerts[:0], domain.Alert{
			ID:        1,
			CompanyID: 1,
			ChatID:    101,
			Type:      domain.AlertTypeNoReply,
		})
		createdRace, err := u.CheckNoReplyAlerts(ctx)
		if err != nil {
			t.Fatalf("race CheckNoReplyAlerts failed: %v", err)
		}
		if createdRace != 1 {
			t.Fatalf("expected only 1 new alert (chat 202), got %d", createdRace)
		}
	})

	t.Run("propagates query error and partial creation errors", func(t *testing.T) {
		wantErr := errors.New("query failed")
		u := usecase.NewAlertUsecase(alertRepoStub{
			findExpired: func(context.Context) ([]domain.NoReplyChat, error) {
				return nil, wantErr
			},
		}, nil, nil)
		if _, err := u.CheckNoReplyAlerts(ctx); !errors.Is(err, wantErr) {
			t.Fatalf("expected query error, got %v", err)
		}

		partialErr := errors.New("insert failed for chat 2")
		uPartial := usecase.NewAlertUsecase(alertRepoStub{
			findExpired: func(context.Context) ([]domain.NoReplyChat, error) {
				return []domain.NoReplyChat{{CompanyID: 1, ChatID: 1}, {CompanyID: 1, ChatID: 2}}, nil
			},
			create: func(_ context.Context, a domain.Alert) (int64, error) {
				if a.ChatID == 2 {
					return 0, partialErr
				}
				return 10, nil
			},
		}, nil, nil)

		created, err := uPartial.CheckNoReplyAlerts(ctx)
		if created != 1 || !errors.Is(err, partialErr) {
			t.Fatalf("expected created=1 and partialErr, got created=%d err=%v", created, err)
		}
	})

	t.Run("respects cancelled context", func(t *testing.T) {
		cancelCtx, cancel := context.WithCancel(ctx)
		cancel()
		u := usecase.NewAlertUsecase(&inMemoryAlertRepo{}, nil, nil)
		if _, err := u.CheckNoReplyAlerts(cancelCtx); !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	})
}
