package handler_test

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/kraconst/GO_server/internal/domain"
	"github.com/kraconst/GO_server/internal/handler"
	"github.com/kraconst/GO_server/internal/usecase"
)

type mockUserRepoForHandler struct {
	create func(ctx context.Context, u domain.User) (int64, error)
}

func (m *mockUserRepoForHandler) Create(ctx context.Context, u domain.User) (int64, error) {
	if m.create != nil {
		return m.create(ctx, u)
	}
	return 1, nil
}
func (m *mockUserRepoForHandler) GetByID(ctx context.Context, id int64) (*domain.User, error) {
	return nil, nil
}
func (m *mockUserRepoForHandler) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	return nil, nil
}
func (m *mockUserRepoForHandler) ListByCompany(ctx context.Context, companyID int64) ([]domain.User, error) {
	return nil, nil
}

type mockSummaryRepoForHandler struct {
	lastSaved domain.ChatSummary
}

func (m *mockSummaryRepoForHandler) Upsert(ctx context.Context, summary domain.ChatSummary) error {
	m.lastSaved = summary
	return nil
}
func (m *mockSummaryRepoForHandler) GetByChatID(ctx context.Context, companyID, chatID int64) (*domain.ChatSummary, error) {
	return nil, nil
}

// TestMinor1_UpsertChatSummary_PreservesLastMessageID verifies LastMessageID is not overwritten to 0
func TestMinor1_UpsertChatSummary_PreservesLastMessageID(t *testing.T) {
	summaryRepo := &mockSummaryRepoForHandler{}
	sumUC := usecase.NewChatSummaryUsecase(summaryRepo, nil, nil)

	h := handler.New(nil, nil, nil, nil, nil, nil, nil, nil, sumUC, nil)
	r := h.InitRoutes()

	body := []byte(`{
		"topic": "Discussed roadmap",
		"agreements": "Agreed on Q1 release",
		"next_step": "Deploy staging",
		"open_questions": "Budget",
		"risk": "medium",
		"last_message_id": 456
	}`)
	req, _ := http.NewRequest(http.MethodPost, "/chats/10/summary?company_id=1", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d, body: %s", w.Code, w.Body.String())
	}

	if summaryRepo.lastSaved.LastMessageID != 456 {
		t.Fatalf("expected LastMessageID 456 to be preserved, got %d", summaryRepo.lastSaved.LastMessageID)
	}
}

// TestMedium2_CreateUser_ConflictReturns409 verifies duplicate email returns 409 Conflict
func TestMedium2_CreateUser_ConflictReturns409(t *testing.T) {
	userRepo := &mockUserRepoForHandler{
		create: func(ctx context.Context, u domain.User) (int64, error) {
			return 0, fmt.Errorf("%w: user with email %s already exists", domain.ErrConflict, u.Email)
		},
	}
	userUC := usecase.NewUserUsecase(userRepo, nil)

	h := handler.New(nil, nil, nil, nil, nil, nil, nil, nil, nil, userUC)
	r := h.InitRoutes()

	body := []byte(`{
		"company_id": 1,
		"email": "existing@example.com",
		"password_hash": "hashed123",
		"role": "manager"
	}`)
	req, _ := http.NewRequest(http.MethodPost, "/users", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestMinor2_ConcurrentMockAccess tests that concurrent reads and writes to mock data do not race
func TestMinor2_ConcurrentMockAccess(t *testing.T) {
	h := handler.New(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	r := h.InitRoutes()

	var wg sync.WaitGroup
	iterations := 50

	for i := 0; i < iterations; i++ {
		wg.Add(4)

		// Read employees
		go func() {
			defer wg.Done()
			req, _ := http.NewRequest(http.MethodGet, "/employees?company_id=1", nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
		}()

		// Update employee
		go func(idx int) {
			defer wg.Done()
			body := []byte(fmt.Sprintf(`{"name": "Name_%d", "position": "Pos"}`, idx))
			req, _ := http.NewRequest(http.MethodPut, "/employees/1?company_id=1", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
		}(i)

		// Read chats
		go func() {
			defer wg.Done()
			req, _ := http.NewRequest(http.MethodGet, "/chats?company_id=1", nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
		}()

		// Update chat status
		go func() {
			defer wg.Done()
			body := []byte(`{"status": "negotiation"}`)
			req, _ := http.NewRequest(http.MethodPost, "/chats/1/status?company_id=1", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
		}()
	}

	wg.Wait()
}
