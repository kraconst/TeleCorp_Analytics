package handler_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/kraconst/GO_server/internal/domain"
	"github.com/kraconst/GO_server/internal/handler"
	"github.com/kraconst/GO_server/internal/usecase"
)

func TestListChats_FilterValidation_InvalidParams(t *testing.T) {
	called := false
	repo := &handlerMockChatRepo{
		listByFilterFn: func(ctx context.Context, filter domain.ChatFilter) ([]domain.Chat, error) {
			called = true
			return []domain.Chat{}, nil
		},
	}
	chatUC := usecase.NewChatUsecase(repo, nil, nil, nil)
	r := setupTestRouter(handler.New(nil, nil, nil, nil, chatUC, nil, nil, nil, nil, nil))

	cases := []struct {
		name  string
		query string
	}{
		{name: "company_id zero", query: "company_id=0"},
		{name: "company_id negative", query: "company_id=-1"},
		{name: "company_id non-numeric", query: "company_id=abc"},
		{name: "account_id zero", query: "account_id=0"},
		{name: "account_id negative", query: "account_id=-5"},
		{name: "account_id non-numeric", query: "account_id=abc"},
		{name: "account_id float", query: "account_id=1.5"},
		{name: "account_id with spaces", query: "account_id=" + url.QueryEscape(" 1 ")},
		{name: "priority unknown", query: "priority=urgent"},
		{name: "priority uppercase", query: "priority=HIGH"},
		{name: "priority leading space", query: "priority=" + url.QueryEscape(" high")},
		{name: "priority trailing space", query: "priority=" + url.QueryEscape("low ")},
		{name: "unanswered numeric 1", query: "unanswered=1"},
		{name: "unanswered numeric 0", query: "unanswered=0"},
		{name: "unanswered yes", query: "unanswered=yes"},
		{name: "unanswered uppercase TRUE", query: "unanswered=TRUE"},
		{name: "unanswered padded", query: "unanswered=" + url.QueryEscape(" true")},
		{name: "sort unknown", query: "sort=created_at"},
		{name: "sort uppercase", query: "sort=LAST_MESSAGE"},
		{name: "sort trailing space", query: "sort=" + url.QueryEscape("last_message ")},
		{name: "limit zero", query: "limit=0"},
		{name: "limit negative", query: "limit=-1"},
		{name: "limit over max", query: "limit=101"},
		{name: "limit non-numeric", query: "limit=ten"},
		{name: "limit padded", query: "limit=" + url.QueryEscape(" 10")},
		{name: "offset negative", query: "offset=-1"},
		{name: "offset large negative", query: "offset=-100"},
		{name: "offset non-numeric", query: "offset=abc"},
		{name: "offset padded", query: "offset=" + url.QueryEscape(" 0")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			called = false
			req, err := http.NewRequest(http.MethodGet, "/chats?"+tc.query, nil)
			if err != nil {
				t.Fatalf("unexpected request error: %v", err)
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400 for query %q, got %d: %s", tc.query, w.Code, w.Body.String())
			}
			if called {
				t.Fatalf("expected repository not to be called for invalid query %q", tc.query)
			}
		})
	}
}

func TestListChats_UsecaseBacked_FilterPropagationAndBoundaries(t *testing.T) {
	t.Run("defaults when no query params provided", func(t *testing.T) {
		var gotFilter domain.ChatFilter
		repo := &handlerMockChatRepo{
			listByFilterFn: func(ctx context.Context, filter domain.ChatFilter) ([]domain.Chat, error) {
				gotFilter = filter
				return nil, nil // verify nil slice is serialized as []
			},
		}
		chatUC := usecase.NewChatUsecase(repo, nil, nil, nil)
		r := setupTestRouter(handler.New(nil, nil, nil, nil, chatUC, nil, nil, nil, nil, nil))

		req, _ := http.NewRequest(http.MethodGet, "/chats", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		if w.Body.String() != "[]" {
			t.Fatalf("expected JSON '[]', got %q", w.Body.String())
		}
		if gotFilter.CompanyID != 1 {
			t.Errorf("expected CompanyID=1, got %d", gotFilter.CompanyID)
		}
		if gotFilter.Status != nil || gotFilter.EmployeeID != nil || gotFilter.AccountID != nil || gotFilter.Priority != nil {
			t.Errorf("expected nil optional pointers, got %+v", gotFilter)
		}
		if gotFilter.UnansweredOnly {
			t.Errorf("expected UnansweredOnly=false")
		}
		if gotFilter.Query != "" {
			t.Errorf("expected empty Query, got %q", gotFilter.Query)
		}
		if gotFilter.SortBy != domain.ChatSortLastMessage {
			t.Errorf("expected SortBy=%q, got %q", domain.ChatSortLastMessage, gotFilter.SortBy)
		}
		if gotFilter.Limit != 50 || gotFilter.Offset != 0 {
			t.Errorf("expected Limit=50 Offset=0, got Limit=%d Offset=%d", gotFilter.Limit, gotFilter.Offset)
		}
	})

	t.Run("empty query params are ignored as unset", func(t *testing.T) {
		var gotFilter domain.ChatFilter
		repo := &handlerMockChatRepo{
			listByFilterFn: func(ctx context.Context, filter domain.ChatFilter) ([]domain.Chat, error) {
				gotFilter = filter
				return []domain.Chat{}, nil
			},
		}
		chatUC := usecase.NewChatUsecase(repo, nil, nil, nil)
		r := setupTestRouter(handler.New(nil, nil, nil, nil, chatUC, nil, nil, nil, nil, nil))

		req, _ := http.NewRequest(http.MethodGet, "/chats?company_id=&status=&employee_id=&account_id=&priority=&unanswered=&q=&sort=&limit=&offset=", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		if gotFilter.CompanyID != 1 || gotFilter.SortBy != domain.ChatSortLastMessage || gotFilter.Limit != 50 || gotFilter.Offset != 0 {
			t.Fatalf("expected default filter values, got %+v", gotFilter)
		}
		if gotFilter.Status != nil || gotFilter.EmployeeID != nil || gotFilter.AccountID != nil || gotFilter.Priority != nil || gotFilter.UnansweredOnly || gotFilter.Query != "" {
			t.Fatalf("expected empty optional filters, got %+v", gotFilter)
		}
	})

	t.Run("full filter combination reaches usecase with exact values", func(t *testing.T) {
		var gotFilter domain.ChatFilter
		repo := &handlerMockChatRepo{
			listByFilterFn: func(ctx context.Context, filter domain.ChatFilter) ([]domain.Chat, error) {
				gotFilter = filter
				return []domain.Chat{{ID: 42, AccountID: 12, Status: domain.ChatStatusNegotiation, Priority: domain.ChatPriorityHigh}}, nil
			},
		}
		chatUC := usecase.NewChatUsecase(repo, nil, nil, nil)
		r := setupTestRouter(handler.New(nil, nil, nil, nil, chatUC, nil, nil, nil, nil, nil))

		rawURL := "/chats?company_id=3&status=negotiation&employee_id=7&account_id=12&priority=high&unanswered=true&q=" +
			url.QueryEscape("  КП 100%  ") + "&sort=awaiting_reply&limit=100&offset=25"
		req, _ := http.NewRequest(http.MethodGet, rawURL, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		if gotFilter.CompanyID != 3 {
			t.Errorf("expected CompanyID=3, got %d", gotFilter.CompanyID)
		}
		if gotFilter.Status == nil || *gotFilter.Status != domain.ChatStatusNegotiation {
			t.Errorf("expected Status=negotiation, got %v", gotFilter.Status)
		}
		if gotFilter.EmployeeID == nil || *gotFilter.EmployeeID != 7 {
			t.Errorf("expected EmployeeID=7, got %v", gotFilter.EmployeeID)
		}
		if gotFilter.AccountID == nil || *gotFilter.AccountID != 12 {
			t.Errorf("expected AccountID=12, got %v", gotFilter.AccountID)
		}
		if gotFilter.Priority == nil || *gotFilter.Priority != domain.ChatPriorityHigh {
			t.Errorf("expected Priority=high, got %v", gotFilter.Priority)
		}
		if !gotFilter.UnansweredOnly {
			t.Errorf("expected UnansweredOnly=true")
		}
		if gotFilter.Query != "КП 100%" {
			t.Errorf("expected trimmed Query=%q, got %q", "КП 100%", gotFilter.Query)
		}
		if gotFilter.SortBy != domain.ChatSortAwaitingReply {
			t.Errorf("expected SortBy=%q, got %q", domain.ChatSortAwaitingReply, gotFilter.SortBy)
		}
		if gotFilter.Limit != 100 || gotFilter.Offset != 25 {
			t.Errorf("expected Limit=100 Offset=25, got Limit=%d Offset=%d", gotFilter.Limit, gotFilter.Offset)
		}

		var chats []domain.Chat
		if err := json.Unmarshal(w.Body.Bytes(), &chats); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if len(chats) != 1 || chats[0].ID != 42 {
			t.Fatalf("expected chat 42, got %+v", chats)
		}
	})

	t.Run("boundary valid values limit=1 offset=0 priority=low unanswered=false sort=last_message", func(t *testing.T) {
		var gotFilter domain.ChatFilter
		repo := &handlerMockChatRepo{
			listByFilterFn: func(ctx context.Context, filter domain.ChatFilter) ([]domain.Chat, error) {
				gotFilter = filter
				return []domain.Chat{}, nil
			},
		}
		chatUC := usecase.NewChatUsecase(repo, nil, nil, nil)
		r := setupTestRouter(handler.New(nil, nil, nil, nil, chatUC, nil, nil, nil, nil, nil))

		req, _ := http.NewRequest(http.MethodGet, "/chats?priority=low&unanswered=false&sort=last_message&limit=1&offset=0", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		if gotFilter.Priority == nil || *gotFilter.Priority != domain.ChatPriorityLow {
			t.Errorf("expected Priority=low, got %v", gotFilter.Priority)
		}
		if gotFilter.UnansweredOnly {
			t.Errorf("expected UnansweredOnly=false")
		}
		if gotFilter.SortBy != domain.ChatSortLastMessage {
			t.Errorf("expected SortBy=last_message, got %q", gotFilter.SortBy)
		}
		if gotFilter.Limit != 1 || gotFilter.Offset != 0 {
			t.Errorf("expected Limit=1 Offset=0, got Limit=%d Offset=%d", gotFilter.Limit, gotFilter.Offset)
		}
	})

	t.Run("usecase errors are mapped properly", func(t *testing.T) {
		errCases := []struct {
			name           string
			repoErr        error
			expectedStatus int
			forbidSubstr   string
		}{
			{name: "forbidden", repoErr: domain.ErrForbidden, expectedStatus: http.StatusForbidden},
			{name: "not found", repoErr: domain.ErrNotFound, expectedStatus: http.StatusNotFound},
			{name: "internal db error", repoErr: errors.New("pq: secret connection details"), expectedStatus: http.StatusInternalServerError, forbidSubstr: "secret connection details"},
		}

		for _, tc := range errCases {
			t.Run(tc.name, func(t *testing.T) {
				repo := &handlerMockChatRepo{
					listByFilterFn: func(ctx context.Context, filter domain.ChatFilter) ([]domain.Chat, error) {
						return nil, tc.repoErr
					},
				}
				chatUC := usecase.NewChatUsecase(repo, nil, nil, nil)
				r := setupTestRouter(handler.New(nil, nil, nil, nil, chatUC, nil, nil, nil, nil, nil))

				req, _ := http.NewRequest(http.MethodGet, "/chats", nil)
				w := httptest.NewRecorder()
				r.ServeHTTP(w, req)

				if w.Code != tc.expectedStatus {
					t.Fatalf("expected %d, got %d: %s", tc.expectedStatus, w.Code, w.Body.String())
				}
				if tc.forbidSubstr != "" && strings.Contains(w.Body.String(), tc.forbidSubstr) {
					t.Fatalf("response leaked internal error: %s", w.Body.String())
				}
			})
		}
	})
}

func TestListChats_MockMode_FiltersSearchSortPagination(t *testing.T) {
	r := setupTestRouter(handler.New(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil))

	getChats := func(t *testing.T, path string) ([]domain.Chat, string) {
		t.Helper()
		req, err := http.NewRequest(http.MethodGet, path, nil)
		if err != nil {
			t.Fatalf("unexpected request error: %v", err)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 for %s, got %d: %s", path, w.Code, w.Body.String())
		}
		var chats []domain.Chat
		if err := json.Unmarshal(w.Body.Bytes(), &chats); err != nil {
			t.Fatalf("failed to decode %s: %v", path, err)
		}
		return chats, w.Body.String()
	}

	t.Run("filter by account_id", func(t *testing.T) {
		chats, _ := getChats(t, "/chats?account_id=1")
		if len(chats) != 1 || chats[0].ID != 1 {
			t.Fatalf("expected [1], got %+v", chats)
		}

		chats, body := getChats(t, "/chats?account_id=999")
		if len(chats) != 0 || body != "[]" {
			t.Fatalf("expected [], got %s", body)
		}
	})

	t.Run("filter by priority", func(t *testing.T) {
		chats, _ := getChats(t, "/chats?priority=high")
		if len(chats) != 1 || chats[0].ID != 2 {
			t.Fatalf("expected chat 2 for priority=high, got %+v", chats)
		}

		chats, _ = getChats(t, "/chats?priority=normal")
		if len(chats) != 1 || chats[0].ID != 1 {
			t.Fatalf("expected chat 1 for priority=normal, got %+v", chats)
		}

		chats, body := getChats(t, "/chats?priority=low")
		if len(chats) != 0 || body != "[]" {
			t.Fatalf("expected [] for priority=low, got %s", body)
		}
	})

	t.Run("filter by unanswered", func(t *testing.T) {
		chats, body := getChats(t, "/chats?unanswered=true")
		if len(chats) != 0 || body != "[]" {
			t.Fatalf("expected [] for unanswered=true in mock data, got %s", body)
		}

		chats, _ = getChats(t, "/chats?unanswered=false")
		if len(chats) != 2 {
			t.Fatalf("expected 2 chats for unanswered=false, got %d", len(chats))
		}
	})

	t.Run("search q across contact name, username, and message text", func(t *testing.T) {
		chats, _ := getChats(t, "/chats?q="+url.QueryEscape("иван"))
		if len(chats) != 1 || chats[0].ID != 1 {
			t.Fatalf("expected chat 1 for contact name query, got %+v", chats)
		}

		chats, _ = getChats(t, "/chats?q="+url.QueryEscape("MARIA_S"))
		if len(chats) != 1 || chats[0].ID != 2 {
			t.Fatalf("expected chat 2 for contact username query, got %+v", chats)
		}

		chats, _ = getChats(t, "/chats?q="+url.QueryEscape("  кп  "))
		if len(chats) != 1 || chats[0].ID != 1 {
			t.Fatalf("expected chat 1 for message text query, got %+v", chats)
		}

		chats, body := getChats(t, "/chats?q="+url.QueryEscape("no-such-match"))
		if len(chats) != 0 || body != "[]" {
			t.Fatalf("expected [] for non-matching query, got %s", body)
		}
	})

	t.Run("sort and pagination", func(t *testing.T) {
		chats, _ := getChats(t, "/chats?sort=awaiting_reply")
		if len(chats) != 2 || chats[0].ID != 2 || chats[1].ID != 1 {
			t.Fatalf("expected [2, 1] for sort=awaiting_reply (NULLS LAST, ID DESC), got %+v", chats)
		}

		chats, _ = getChats(t, "/chats?limit=1&offset=0")
		if len(chats) != 1 || chats[0].ID != 2 {
			t.Fatalf("expected [2] for limit=1&offset=0, got %+v", chats)
		}

		chats, _ = getChats(t, "/chats?limit=1&offset=1")
		if len(chats) != 1 || chats[0].ID != 1 {
			t.Fatalf("expected [1] for limit=1&offset=1, got %+v", chats)
		}

		chats, body := getChats(t, "/chats?limit=10&offset=5")
		if len(chats) != 0 || body != "[]" {
			t.Fatalf("expected [] for offset beyond length, got %s", body)
		}
	})

	t.Run("company_id isolation in mock mode", func(t *testing.T) {
		chats, body := getChats(t, "/chats?company_id=2")
		if len(chats) != 0 || body != "[]" {
			t.Fatalf("expected [] for foreign company_id=2, got %s", body)
		}
	})
}
