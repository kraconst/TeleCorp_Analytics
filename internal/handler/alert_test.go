package handler_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kraconst/GO_server/internal/domain"
	"github.com/kraconst/GO_server/internal/handler"
	"github.com/kraconst/GO_server/internal/usecase"
)

func alertTestRouter(u *usecase.AlertUsecase) *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := handler.New(nil, nil, nil, nil, nil, nil, u, nil, nil, nil)
	r := gin.New()
	r.GET("/alerts", h.ListAlerts)
	r.POST("/alerts/:id/resolve", h.ResolveAlert)
	return r
}

func alertRequest(r http.Handler, method, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(method, path, nil))
	return w
}

func TestAlertDemoResolutionChangesActiveFeed(t *testing.T) {
	r := alertTestRouter(nil)
	before := alertRequest(r, http.MethodGet, "/alerts")
	var alerts []domain.Alert
	if err := json.Unmarshal(before.Body.Bytes(), &alerts); err != nil || before.Code != http.StatusOK || len(alerts) != 2 {
		t.Fatalf("unexpected initial feed: %d %s (%v)", before.Code, before.Body, err)
	}
	for _, alert := range alerts {
		path := fmt.Sprintf("/alerts/%d/resolve", alert.ID)
		for range 2 {
			w := alertRequest(r, http.MethodPost, path)
			if w.Code != http.StatusOK || w.Body.String() != `{"status":"resolved"}` {
				t.Fatalf("resolve must be idempotent: %d %s", w.Code, w.Body)
			}
		}
	}
	after := alertRequest(r, http.MethodGet, "/alerts")
	if after.Code != http.StatusOK || after.Body.String() != "[]" {
		t.Fatalf("resolved alerts remain in feed: %d %s", after.Code, after.Body)
	}
	// A separate handler must start with its own demonstration data.
	fresh := alertRequest(alertTestRouter(nil), http.MethodGet, "/alerts")
	if err := json.Unmarshal(fresh.Body.Bytes(), &alerts); err != nil || len(alerts) != 2 {
		t.Fatalf("demo state leaked between handlers: %s", fresh.Body)
	}
}

func TestAlertResolveRejectsInvalidAndMissingIDs(t *testing.T) {
	r := alertTestRouter(nil)
	for _, id := range []string{"0", "-1", "abc", "1.5", "9223372036854775808"} {
		w := alertRequest(r, http.MethodPost, "/alerts/"+id+"/resolve")
		if w.Code != http.StatusBadRequest {
			t.Errorf("ID %q: expected 400, got %d", id, w.Code)
		}
	}
	if w := alertRequest(r, http.MethodPost, "/alerts/999/resolve"); w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing alert, got %d %s", w.Code, w.Body)
	}
}

type alertHandlerRepoStub struct {
	list    func(context.Context, int64) ([]domain.Alert, error)
	resolve func(context.Context, int64, int64) error
}

func (s alertHandlerRepoStub) Create(context.Context, domain.Alert) (int64, error) {
	return 0, errors.New("not used by HTTP API")
}

func (s alertHandlerRepoStub) ListActiveByCompany(ctx context.Context, companyID int64) ([]domain.Alert, error) {
	return s.list(ctx, companyID)
}

func (s alertHandlerRepoStub) Resolve(ctx context.Context, companyID, id int64) error {
	return s.resolve(ctx, companyID, id)
}

func TestAlertHTTPUsesCompanyScopeAndMapsErrors(t *testing.T) {
	storageErr := errors.New("private database connection details")
	u := usecase.NewAlertUsecase(alertHandlerRepoStub{
		list: func(_ context.Context, companyID int64) ([]domain.Alert, error) {
			if companyID != 1 {
				t.Fatalf("unexpected company: %d", companyID)
			}
			return nil, storageErr
		},
		resolve: func(_ context.Context, companyID, id int64) error {
			if companyID != 1 {
				t.Fatalf("unexpected company: %d", companyID)
			}
			switch id {
			case 1:
				return nil
			case 2:
				return fmt.Errorf("lookup: %w", domain.ErrNotFound)
			default:
				return storageErr
			}
		},
	}, nil, nil)
	r := alertTestRouter(u)
	for _, tc := range []struct {
		method string
		path   string
		status int
	}{
		{http.MethodGet, "/alerts", http.StatusInternalServerError},
		{http.MethodPost, "/alerts/1/resolve", http.StatusOK},
		{http.MethodPost, "/alerts/2/resolve", http.StatusNotFound},
		{http.MethodPost, "/alerts/3/resolve", http.StatusInternalServerError},
	} {
		w := alertRequest(r, tc.method, tc.path)
		if w.Code != tc.status || strings.Contains(w.Body.String(), storageErr.Error()) {
			t.Errorf("%s %s: unexpected response %d %s", tc.method, tc.path, w.Code, w.Body)
		}
	}
}

func TestAlertHTTPReturnsRepositoryDataAndEmptyArray(t *testing.T) {
	for _, alerts := range [][]domain.Alert{nil, {{ID: 42, CompanyID: 1, ChatID: 7, Type: domain.AlertTypeUrgent}}} {
		u := usecase.NewAlertUsecase(alertHandlerRepoStub{list: func(context.Context, int64) ([]domain.Alert, error) {
			return alerts, nil
		}}, nil, nil)
		w := alertRequest(alertTestRouter(u), http.MethodGet, "/alerts")
		var got []domain.Alert
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || w.Code != http.StatusOK || got == nil || len(got) != len(alerts) {
			t.Fatalf("unexpected feed: %d %s (%v)", w.Code, w.Body, err)
		}
		if len(got) > 0 && (got[0].ID != 42 || got[0].Type != domain.AlertTypeUrgent) {
			t.Fatalf("repository data was changed: %+v", got)
		}
	}
}

func TestAlertDemoSupportsConcurrentReadsAndResolution(t *testing.T) {
	r := alertTestRouter(nil)
	var workers sync.WaitGroup
	for i := range 32 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			if i%2 == 0 {
				if w := alertRequest(r, http.MethodGet, "/alerts"); w.Code != http.StatusOK {
					t.Errorf("list returned %d", w.Code)
				}
			} else if w := alertRequest(r, http.MethodPost, "/alerts/1/resolve"); w.Code != http.StatusOK {
				t.Errorf("resolve returned %d", w.Code)
			}
		}()
	}
	workers.Wait()
}
