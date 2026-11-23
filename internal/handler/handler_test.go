package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kraconst/GO_server/internal/domain"
	"github.com/kraconst/GO_server/internal/handler"
	"github.com/kraconst/GO_server/internal/usecase"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func setupTestRouter(h *handler.Handler) *gin.Engine {
	return h.InitRoutes()
}

func TestMockHandler_Health(t *testing.T) {
	h := handler.New(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	r := setupTestRouter(h)

	req, _ := http.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
}

func TestMockHandler_ListChats(t *testing.T) {
	h := handler.New(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	r := setupTestRouter(h)

	t.Run("list all mock chats (sorted by LastMessageAt desc)", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/chats", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", w.Code)
		}

		var chats []domain.Chat
		if err := json.Unmarshal(w.Body.Bytes(), &chats); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if len(chats) < 2 {
			t.Fatalf("expected at least 2 mock chats, got %d", len(chats))
		}
		// Chat 2 is Sept 15, Chat 1 is Sept 1 -> Chat 2 must be first
		if chats[0].ID != 2 || chats[1].ID != 1 {
			t.Fatalf("expected sorted order [2, 1], got [%d, %d]", chats[0].ID, chats[1].ID)
		}
	})

	t.Run("filter by status", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/chats?status=new_lead", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", w.Code)
		}
		var chats []domain.Chat
		if err := json.Unmarshal(w.Body.Bytes(), &chats); err != nil {
			t.Fatalf("failed to decode: %v", err)
		}
		for _, c := range chats {
			if c.Status != domain.ChatStatusNewLead {
				t.Fatalf("expected status new_lead, got %s", c.Status)
			}
		}
	})

	t.Run("filter by employee_id", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/chats?employee_id=2", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", w.Code)
		}
		var chats []domain.Chat
		if err := json.Unmarshal(w.Body.Bytes(), &chats); err != nil {
			t.Fatalf("failed to decode: %v", err)
		}
		if len(chats) != 1 || chats[0].ID != 2 {
			t.Fatalf("expected chat 2 for employee 2, got %+v", chats)
		}
	})

	t.Run("invalid status query", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/chats?status=invalid_status", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected status 400, got %d", w.Code)
		}
	})

	t.Run("invalid employee_id non-numeric", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/chats?employee_id=abc", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected status 400, got %d", w.Code)
		}
	})

	t.Run("invalid employee_id non-positive", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/chats?employee_id=0", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected status 400, got %d", w.Code)
		}
	})
}

func TestMockHandler_GetChatByID(t *testing.T) {
	h := handler.New(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	r := setupTestRouter(h)

	t.Run("existing chat", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/chats/1", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", w.Code)
		}
		var chat domain.Chat
		if err := json.Unmarshal(w.Body.Bytes(), &chat); err != nil {
			t.Fatalf("failed to decode: %v", err)
		}
		if chat.ID != 1 {
			t.Fatalf("expected chat ID 1, got %d", chat.ID)
		}
	})

	t.Run("not found", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/chats/999", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected status 404, got %d", w.Code)
		}
	})

	t.Run("invalid id non-positive", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/chats/0", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected status 400, got %d", w.Code)
		}
	})

	t.Run("invalid id non-numeric", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/chats/not-an-id", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected status 400, got %d", w.Code)
		}
	})
}

func TestMockHandler_UpdateChatStatus(t *testing.T) {
	h := handler.New(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	r := setupTestRouter(h)

	t.Run("successful status update", func(t *testing.T) {
		body := `{"status": "interested"}`
		req, _ := http.NewRequest(http.MethodPost, "/chats/1/status", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
		}

		// Verify state update in mock
		reqGet, _ := http.NewRequest(http.MethodGet, "/chats/1", nil)
		wGet := httptest.NewRecorder()
		r.ServeHTTP(wGet, reqGet)
		var chat domain.Chat
		if err := json.Unmarshal(wGet.Body.Bytes(), &chat); err != nil {
			t.Fatalf("failed to decode: %v", err)
		}
		if chat.Status != domain.ChatStatusInterested {
			t.Fatalf("expected status interested, got %s", chat.Status)
		}
		if chat.StatusSource != domain.StatusSourceManual {
			t.Fatalf("expected status_source manual, got %s", chat.StatusSource)
		}
	})

	t.Run("invalid status enum", func(t *testing.T) {
		body := `{"status": "invalid_status"}`
		req, _ := http.NewRequest(http.MethodPost, "/chats/1/status", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected status 400, got %d", w.Code)
		}
	})

	t.Run("chat not found", func(t *testing.T) {
		body := `{"status": "negotiation"}`
		req, _ := http.NewRequest(http.MethodPost, "/chats/999/status", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected status 404, got %d", w.Code)
		}
	})

	t.Run("malformed json body", func(t *testing.T) {
		body := `{"status":`
		req, _ := http.NewRequest(http.MethodPost, "/chats/1/status", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected status 400, got %d", w.Code)
		}
	})
}

// Tests for database/usecase-backed mode:
type handlerMockChatRepo struct {
	getByIDFn       func(ctx context.Context, id int64) (*domain.Chat, error)
	listByCompanyFn func(ctx context.Context, companyID int64, status *domain.ChatStatus, employeeID *int64) ([]domain.Chat, error)
	updateStatusFn  func(ctx context.Context, id int64, status domain.ChatStatus, source domain.StatusSource) error
}

func (m *handlerMockChatRepo) Create(ctx context.Context, chat domain.Chat) (int64, error) {
	return 0, nil
}
func (m *handlerMockChatRepo) GetByID(ctx context.Context, id int64) (*domain.Chat, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
	}
	return nil, domain.ErrNotFound
}
func (m *handlerMockChatRepo) ListByCompany(ctx context.Context, companyID int64, status *domain.ChatStatus, employeeID *int64) ([]domain.Chat, error) {
	if m.listByCompanyFn != nil {
		return m.listByCompanyFn(ctx, companyID, status, employeeID)
	}
	return []domain.Chat{}, nil
}
func (m *handlerMockChatRepo) UpdateStatus(ctx context.Context, id int64, status domain.ChatStatus, source domain.StatusSource) error {
	if m.updateStatusFn != nil {
		return m.updateStatusFn(ctx, id, status, source)
	}
	return nil
}

func TestUsecaseBackedHandler_Endpoints(t *testing.T) {
	repo := &handlerMockChatRepo{
		getByIDFn: func(ctx context.Context, id int64) (*domain.Chat, error) {
			if id == 55 {
				return &domain.Chat{ID: 55, Status: domain.ChatStatusWaitingReply, LastMessageAt: time.Now().UTC()}, nil
			}
			return nil, domain.ErrNotFound
		},
		listByCompanyFn: func(ctx context.Context, companyID int64, status *domain.ChatStatus, employeeID *int64) ([]domain.Chat, error) {
			return []domain.Chat{{ID: 55, Status: domain.ChatStatusWaitingReply}}, nil
		},
		updateStatusFn: func(ctx context.Context, id int64, status domain.ChatStatus, source domain.StatusSource) error {
			if id == 55 {
				return nil
			}
			return domain.ErrNotFound
		},
	}

	chatUsecase := usecase.NewChatUsecase(repo, nil, nil, nil)
	h := handler.New(nil, nil, nil, nil, chatUsecase, nil, nil, nil, nil, nil)
	r := setupTestRouter(h)

	t.Run("GET /chats with usecase", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/chats", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
		var chats []domain.Chat
		if err := json.Unmarshal(w.Body.Bytes(), &chats); err != nil {
			t.Fatalf("failed to decode: %v", err)
		}
		if len(chats) != 1 || chats[0].ID != 55 {
			t.Fatalf("expected chat 55, got %+v", chats)
		}
	})

	t.Run("GET /chats/:id with usecase success", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/chats/55", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
	})

	t.Run("GET /chats/:id with usecase not found", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/chats/77", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})

	t.Run("POST /chats/:id/status with usecase success", func(t *testing.T) {
		body := `{"status": "lost"}`
		req, _ := http.NewRequest(http.MethodPost, "/chats/55/status", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("POST /chats/:id/status with usecase not found", func(t *testing.T) {
		body := `{"status": "lost"}`
		req, _ := http.NewRequest(http.MethodPost, "/chats/77/status", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})
}

type handlerMockContactRepo struct {
	getByIDFn func(ctx context.Context, id int64) (*domain.Contact, error)
	listFn    func(ctx context.Context, companyID int64) ([]domain.Contact, error)
}

func (m *handlerMockContactRepo) Create(ctx context.Context, c domain.Contact) (int64, error) {
	return 0, nil
}
func (m *handlerMockContactRepo) GetByID(ctx context.Context, id int64) (*domain.Contact, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
	}
	return nil, domain.ErrNotFound
}
func (m *handlerMockContactRepo) GetByTelegramID(ctx context.Context, companyID, tgUserID int64) (*domain.Contact, error) {
	return nil, domain.ErrNotFound
}
func (m *handlerMockContactRepo) ListByCompany(ctx context.Context, companyID int64) ([]domain.Contact, error) {
	if m.listFn != nil {
		return m.listFn(ctx, companyID)
	}
	return []domain.Contact{}, nil
}

type handlerMockSummaryRepo struct {
	getByChatIDFn func(ctx context.Context, companyID, chatID int64) (*domain.ChatSummary, error)
}

func (m *handlerMockSummaryRepo) Upsert(ctx context.Context, s domain.ChatSummary) error {
	return nil
}
func (m *handlerMockSummaryRepo) GetByChatID(ctx context.Context, companyID, chatID int64) (*domain.ChatSummary, error) {
	if m.getByChatIDFn != nil {
		return m.getByChatIDFn(ctx, companyID, chatID)
	}
	return nil, domain.ErrNotFound
}

func TestContacts_Handler(t *testing.T) {
	t.Run("GET /contacts mock mode", func(t *testing.T) {
		h := handler.New(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
		r := setupTestRouter(h)

		req, _ := http.NewRequest(http.MethodGet, "/contacts", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
		var contacts []domain.Contact
		if err := json.Unmarshal(w.Body.Bytes(), &contacts); err != nil {
			t.Fatalf("failed to decode: %v", err)
		}
		if len(contacts) != 2 {
			t.Fatalf("expected 2 contacts, got %d", len(contacts))
		}
	})

	t.Run("GET /contacts/:id mock mode success", func(t *testing.T) {
		h := handler.New(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
		r := setupTestRouter(h)

		req, _ := http.NewRequest(http.MethodGet, "/contacts/1", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
		var contact domain.Contact
		if err := json.Unmarshal(w.Body.Bytes(), &contact); err != nil {
			t.Fatalf("failed to decode: %v", err)
		}
		if contact.ID != 1 {
			t.Fatalf("expected contact id 1, got %d", contact.ID)
		}
	})

	t.Run("GET /contacts/:id mock mode not found", func(t *testing.T) {
		h := handler.New(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
		r := setupTestRouter(h)

		req, _ := http.NewRequest(http.MethodGet, "/contacts/999", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})

	t.Run("GET /contacts with usecase", func(t *testing.T) {
		repo := &handlerMockContactRepo{
			listFn: func(ctx context.Context, companyID int64) ([]domain.Contact, error) {
				return []domain.Contact{{ID: 42, Name: "Test Contact"}}, nil
			},
		}
		uc := usecase.NewContactUsecase(repo)
		h := handler.New(nil, nil, nil, uc, nil, nil, nil, nil, nil, nil)
		r := setupTestRouter(h)

		req, _ := http.NewRequest(http.MethodGet, "/contacts", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
		var contacts []domain.Contact
		if err := json.Unmarshal(w.Body.Bytes(), &contacts); err != nil || len(contacts) != 1 || contacts[0].ID != 42 {
			t.Fatalf("unexpected contacts: %+v, %v", contacts, err)
		}
	})
}

func TestChatCompositeDetails_Handler(t *testing.T) {
	chatRepo := &handlerMockChatRepo{
		getByIDFn: func(ctx context.Context, id int64) (*domain.Chat, error) {
			if id == 100 {
				return &domain.Chat{
					ID:            100,
					AccountID:     1,
					ContactID:     200,
					Status:        domain.ChatStatusNegotiation,
					LastMessageAt: time.Now().UTC(),
				}, nil
			}
			return nil, domain.ErrNotFound
		},
	}
	contactRepo := &handlerMockContactRepo{
		getByIDFn: func(ctx context.Context, id int64) (*domain.Contact, error) {
			if id == 200 {
				return &domain.Contact{
					ID:             200,
					CompanyID:      1,
					TelegramUserID: 12345,
					Name:           "Ivan Ivanov",
					Username:       "ivan",
				}, nil
			}
			return nil, domain.ErrNotFound
		},
	}
	summaryRepo := &handlerMockSummaryRepo{
		getByChatIDFn: func(ctx context.Context, companyID, chatID int64) (*domain.ChatSummary, error) {
			if chatID == 100 {
				return &domain.ChatSummary{
					ChatID:   100,
					Topic:    "Enterprise Contract",
					Risk:     domain.RiskMedium,
					NextStep: "Send NDA",
				}, nil
			}
			return nil, domain.ErrNotFound
		},
	}

	chatUsecase := usecase.NewChatUsecase(chatRepo, contactRepo, summaryRepo, nil)
	h := handler.New(nil, nil, nil, nil, chatUsecase, nil, nil, nil, nil, nil)
	r := setupTestRouter(h)

	req, _ := http.NewRequest(http.MethodGet, "/chats/100", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var details usecase.ChatDetails
	if err := json.Unmarshal(w.Body.Bytes(), &details); err != nil {
		t.Fatalf("failed to decode ChatDetails: %v", err)
	}

	if details.ID != 100 {
		t.Errorf("expected chat ID 100, got %d", details.ID)
	}
	if details.Contact == nil || details.Contact.Name != "Ivan Ivanov" {
		t.Errorf("expected contact Ivan Ivanov, got %+v", details.Contact)
	}
	if details.Summary == nil || details.Summary.Topic != "Enterprise Contract" {
		t.Errorf("expected summary topic 'Enterprise Contract', got %+v", details.Summary)
	}
}

func TestHandler_AssignEmployee(t *testing.T) {
	t.Run("mock mode success", func(t *testing.T) {
		h := handler.New(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
		r := setupTestRouter(h)

		body := bytes.NewBufferString(`{"employee_id": 2}`)
		req, _ := http.NewRequest(http.MethodPost, "/accounts/1/assign", body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("invalid account id", func(t *testing.T) {
		h := handler.New(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
		r := setupTestRouter(h)

		body := bytes.NewBufferString(`{"employee_id": 2}`)
		req, _ := http.NewRequest(http.MethodPost, "/accounts/abc/assign", body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("invalid employee id", func(t *testing.T) {
		h := handler.New(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
		r := setupTestRouter(h)

		body := bytes.NewBufferString(`{"employee_id": 0}`)
		req, _ := http.NewRequest(http.MethodPost, "/accounts/1/assign", body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("account not found in mock mode", func(t *testing.T) {
		h := handler.New(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
		r := setupTestRouter(h)

		body := bytes.NewBufferString(`{"employee_id": 2}`)
		req, _ := http.NewRequest(http.MethodPost, "/accounts/999/assign", body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})
}
