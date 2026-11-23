package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/kraconst/GO_server/internal/domain"
)

type AlertUsecase struct {
	alerts   AlertRepository
	chats    ChatRepository
	accounts AccountRepository
}

func NewAlertUsecase(alerts AlertRepository, chats ChatRepository, accounts AccountRepository) *AlertUsecase {
	return &AlertUsecase{
		alerts:   alerts,
		chats:    chats,
		accounts: accounts,
	}
}

// Create вызывается обработчиками событий, когда в диалоге обнаружена проблема.
func (u *AlertUsecase) Create(ctx context.Context, alert domain.Alert) (int64, error) {
	if alert.CompanyID <= 0 || alert.ChatID <= 0 {
		return 0, fmt.Errorf("%w: company_id and chat_id must be positive", domain.ErrValidation)
	}
	switch alert.Type {
	case domain.AlertTypeNoReply, domain.AlertTypeComplaint, domain.AlertTypeRefusal,
		domain.AlertTypeNegative, domain.AlertTypeUrgent:
	default:
		return 0, fmt.Errorf("%w: invalid alert type", domain.ErrValidation)
	}
	if alert.ResolvedAt != nil {
		return 0, fmt.Errorf("%w: a new alert must be unresolved", domain.ErrValidation)
	}
	if alert.CreatedAt.IsZero() {
		alert.CreatedAt = time.Now().UTC()
	}

	if u.chats != nil {
		chat, err := u.chats.GetByID(ctx, alert.ChatID)
		if err != nil {
			return 0, err
		}
		if u.accounts != nil {
			acc, err := u.accounts.GetByID(ctx, chat.AccountID)
			if err != nil {
				return 0, err
			}
			if acc.CompanyID != alert.CompanyID {
				return 0, fmt.Errorf("%w: chat belongs to a different company", domain.ErrForbidden)
			}
		}
	}

	id, err := u.alerts.Create(ctx, alert)
	if err != nil {
		return 0, fmt.Errorf("usecase create alert: %w", err)
	}
	return id, nil
}

func (u *AlertUsecase) ListActiveByCompany(ctx context.Context, companyID int64) ([]domain.Alert, error) {
	if companyID <= 0 {
		return nil, fmt.Errorf("%w: company_id must be positive", domain.ErrValidation)
	}
	alerts, err := u.alerts.ListActiveByCompany(ctx, companyID)
	if err != nil {
		return nil, fmt.Errorf("usecase list active alerts: %w", err)
	}
	if alerts == nil {
		alerts = make([]domain.Alert, 0)
	}
	return alerts, nil
}

// Resolve сохраняет время первого закрытия; повторный вызов безопасен.
func (u *AlertUsecase) Resolve(ctx context.Context, companyID, id int64) error {
	if companyID <= 0 || id <= 0 {
		return fmt.Errorf("%w: company_id and id must be positive", domain.ErrValidation)
	}
	if err := u.alerts.Resolve(ctx, companyID, id); err != nil {
		return fmt.Errorf("usecase resolve alert: %w", err)
	}
	return nil
}
