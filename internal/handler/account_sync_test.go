package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kraconst/GO_server/internal/domain"
	"github.com/kraconst/GO_server/internal/usecase"
)

type accountSyncFunc func(context.Context, int64, int64) (usecase.SyncResult, error)

func (f accountSyncFunc) SyncAccount(ctx context.Context, companyID, accountID int64) (usecase.SyncResult, error) {
	return f(ctx, companyID, accountID)
}

func TestSyncAccountEndpointScopeAndErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name, path string
		result     usecase.SyncResult
		err        error
		want       int
		body       string
		calls      bool
	}{
		{"invalid account", "/accounts/0/sync?company_id=3", usecase.SyncResult{}, nil, http.StatusBadRequest, "invalid account id", false},
		{"invalid company", "/accounts/9/sync?company_id=0", usecase.SyncResult{}, nil, http.StatusBadRequest, "invalid company_id", false},
		{"success", "/accounts/9/sync?company_id=3", usecase.SyncResult{ChatsProcessed: 2, NewMessages: 7}, nil, http.StatusOK, `"new_messages":7`, true},
		{"missing", "/accounts/9/sync?company_id=3", usecase.SyncResult{}, domain.ErrNotFound, http.StatusNotFound, "account not found", true},
		{"busy", "/accounts/9/sync?company_id=3", usecase.SyncResult{}, domain.ErrConflict, http.StatusConflict, "already syncing", true},
		{"unconfigured", "/accounts/9/sync?company_id=3", usecase.SyncResult{}, usecase.ErrTelegramUnavailable, http.StatusServiceUnavailable, "not configured", true},
		{"revoked", "/accounts/9/sync?company_id=3", usecase.SyncResult{}, usecase.ErrTelegramSessionInvalid, http.StatusConflict, "reconnect", true},
		{"partial", "/accounts/9/sync?company_id=3", usecase.SyncResult{ChatsProcessed: 1, NewMessages: 2}, errors.New("private details"), http.StatusInternalServerError, `"new_messages":2`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			h := New(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
			h.SetAccountSync(accountSyncFunc(func(ctx context.Context, companyID, accountID int64) (usecase.SyncResult, error) {
				calls++
				if _, ok := ctx.Deadline(); !ok || companyID != 3 || accountID != 9 {
					t.Fatalf("wrong deadline or scope: %d %d", companyID, accountID)
				}
				return tc.result, tc.err
			}))
			response := httptest.NewRecorder()
			h.InitRoutes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, tc.path, nil))
			if (calls == 1) != tc.calls || response.Code != tc.want || !strings.Contains(response.Body.String(), tc.body) || strings.Contains(response.Body.String(), "private details") {
				t.Fatalf("calls=%d status=%d body=%s", calls, response.Code, response.Body)
			}
		})
	}
}

func TestSyncAccountEndpointReturns503WithoutConfiguredService(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := New(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	response := httptest.NewRecorder()
	h.InitRoutes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/accounts/1/sync", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", response.Code, response.Body)
	}
}
