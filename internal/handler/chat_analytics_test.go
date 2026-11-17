package handler_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kraconst/GO_server/internal/domain"
	"github.com/kraconst/GO_server/internal/handler"
	"github.com/kraconst/GO_server/internal/usecase"
)

type analyticsChatRepo struct {
	usecase.ChatRepository
	missing bool
}

func (r analyticsChatRepo) GetByID(context.Context, int64) (*domain.Chat, error) {
	if r.missing {
		return nil, domain.ErrNotFound
	}
	return &domain.Chat{ID: 1, AccountID: 1}, nil
}

type analyticsAccountRepo struct{ usecase.AccountRepository }

func (analyticsAccountRepo) GetByID(context.Context, int64) (*domain.Account, error) {
	return &domain.Account{ID: 1, CompanyID: 1}, nil
}

type analyticsMessageRepo struct {
	usecase.MessageRepository
	calls    int
	messages []domain.Message
	err      error
}

func (r *analyticsMessageRepo) ListByChat(context.Context, int64, int64, int, int) ([]domain.Message, error) {
	r.calls++
	return r.messages, r.err
}

func TestChatAnalyticsHTTP(t *testing.T) {
	for _, tt := range []struct {
		name, path           string
		missing, unavailable bool
		status               int
	}{
		{"empty", "/chats/1/analytics", false, false, 200},
		{"foreign company", "/chats/1/analytics?company_id=2", false, false, 404},
		{"missing", "/chats/1/analytics", true, false, 404},
		{"invalid id", "/chats/no/analytics", false, false, 400},
		{"zero id", "/chats/0/analytics", false, false, 400},
		{"negative id", "/chats/-1/analytics", false, false, 400},
		{"overflow id", "/chats/9223372036854775808/analytics", false, false, 400},
		{"invalid company", "/chats/1/analytics?company_id=0", false, false, 400},
		{"unavailable", "/chats/1/analytics", false, true, 503},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := &analyticsMessageRepo{}
			var u *usecase.MessageUsecase
			if !tt.unavailable {
				u = usecase.NewMessageUsecase(repo, analyticsChatRepo{missing: tt.missing}, analyticsAccountRepo{})
			}
			router := handler.New(nil, nil, u, nil, nil, nil, nil, nil, nil, nil).InitRoutes()
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest("GET", tt.path, nil))
			if w.Code != tt.status {
				t.Fatalf("status %d: %s", w.Code, w.Body.String())
			}
			if tt.status != 200 && repo.calls != 0 {
				t.Fatal("read messages before access validation")
			}
			if tt.status == 200 {
				var a domain.ChatAnalytics
				if err := json.Unmarshal(w.Body.Bytes(), &a); err != nil {
					t.Fatal(err)
				}
				if a.ChatID != 1 || a.DailyActivity == nil || a.TotalMessages != 0 {
					t.Fatalf("response: %+v", a)
				}
			}
		})
	}
}

func TestChatAnalyticsHTTPPopulated(t *testing.T) {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	repo := &analyticsMessageRepo{messages: []domain.Message{
		{Direction: domain.MessageDirectionIn, SentAt: base},
		{Direction: domain.MessageDirectionOut, SentAt: base.Add(30 * time.Second)},
		{Direction: domain.MessageDirectionIn, SentAt: base.Add(time.Minute)},
	}}
	u := usecase.NewMessageUsecase(repo, analyticsChatRepo{}, analyticsAccountRepo{})
	router := handler.New(nil, nil, u, nil, nil, nil, nil, nil, nil, nil).InitRoutes()
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/chats/1/analytics?company_id=1", nil))
	if w.Code != 200 {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var a domain.ChatAnalytics
	if err := json.Unmarshal(w.Body.Bytes(), &a); err != nil {
		t.Fatal(err)
	}
	if a.TotalMessages != 3 || a.MessagesIn != 2 || a.MessagesOut != 1 || a.ResponseCount != 1 ||
		a.AvgResponseSeconds == nil || *a.AvgResponseSeconds != 30 || a.UnansweredMessages != 1 ||
		a.AwaitingReplySince == nil || !a.AwaitingReplySince.Equal(base.Add(time.Minute)) {
		t.Fatalf("response: %+v", a)
	}
}

func TestChatAnalyticsHTTPStorageError(t *testing.T) {
	repo := &analyticsMessageRepo{err: errors.New("database unavailable")}
	u := usecase.NewMessageUsecase(repo, analyticsChatRepo{}, analyticsAccountRepo{})
	router := handler.New(nil, nil, u, nil, nil, nil, nil, nil, nil, nil).InitRoutes()
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/chats/1/analytics", nil))
	if w.Code != 500 {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
}
