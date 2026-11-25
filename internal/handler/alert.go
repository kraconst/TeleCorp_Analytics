package handler

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kraconst/GO_server/internal/domain"
	"github.com/kraconst/GO_server/internal/usecase"
)

// The existing API uses company 1 until authentication supplies a company ID.
const alertCompanyID int64 = 1

func (h *Handler) alerts() *usecase.AlertUsecase {
	if h.alertUsecase != nil {
		return h.alertUsecase
	}
	return usecase.NewAlertUsecase(h.demoAlerts, nil, nil)
}

func (h *Handler) ListAlerts(c *gin.Context) {
	companyID, err := parseCompanyID(c)
	if err != nil {
		handleUsecaseError(c, err, "")
		return
	}
	alerts, err := h.alerts().ListActiveByCompany(c.Request.Context(), companyID)
	if err != nil {
		handleUsecaseError(c, err, "")
		return
	}
	c.JSON(200, alerts)
}

func (h *Handler) ResolveAlert(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(400, gin.H{"error": "id must be a positive number"})
		return
	}
	companyID, err := parseCompanyID(c)
	if err != nil {
		handleUsecaseError(c, err, "")
		return
	}

	if err := h.alerts().Resolve(c.Request.Context(), companyID, id); err != nil {
		handleUsecaseError(c, err, "alert not found")
		return
	}
	c.JSON(200, gin.H{"status": "resolved"})
}

// Each handler owns its demonstration state; concurrent reads and updates are safe.
type demoAlertStore struct {
	mu     sync.RWMutex
	alerts []domain.Alert
}

func newDemoAlertStore() *demoAlertStore {
	return &demoAlertStore{alerts: []domain.Alert{
		{ID: 2, CompanyID: alertCompanyID, ChatID: 2, Type: domain.AlertTypeComplaint, CreatedAt: time.Date(2026, 9, 15, 15, 0, 0, 0, time.UTC)},
		{ID: 1, CompanyID: alertCompanyID, ChatID: 1, Type: domain.AlertTypeNoReply, CreatedAt: time.Date(2026, 9, 1, 12, 5, 0, 0, time.UTC)},
	}}
}

func (s *demoAlertStore) Create(context.Context, domain.Alert) (int64, error) {
	return 0, errors.New("demo alerts are read-only except for resolution")
}

func (s *demoAlertStore) ExistsActive(_ context.Context, chatID int64, alertType domain.AlertType) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, alert := range s.alerts {
		if alert.ChatID == chatID && alert.Type == alertType && alert.ResolvedAt == nil {
			return true, nil
		}
	}
	return false, nil
}

func (s *demoAlertStore) FindExpiredNoReplyChats(context.Context) ([]domain.NoReplyChat, error) {
	return []domain.NoReplyChat{}, nil
}

func (s *demoAlertStore) ListActiveByCompany(_ context.Context, companyID int64) ([]domain.Alert, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	alerts := make([]domain.Alert, 0)
	for _, alert := range s.alerts {
		if alert.CompanyID == companyID && alert.ResolvedAt == nil {
			alerts = append(alerts, alert)
		}
	}
	return alerts, nil
}

func (s *demoAlertStore) Resolve(_ context.Context, companyID, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.alerts {
		alert := &s.alerts[i]
		if alert.ID == id && alert.CompanyID == companyID {
			if alert.ResolvedAt == nil {
				now := time.Now().UTC()
				alert.ResolvedAt = &now
			}
			return nil
		}
	}
	return domain.ErrNotFound
}
