package usecase

import (
	"context"
	"errors"
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

func isValidAlertType(t domain.AlertType) bool {
	switch t {
	case domain.AlertTypeNoReply, domain.AlertTypeComplaint, domain.AlertTypeRefusal,
		domain.AlertTypeNegative, domain.AlertTypeUrgent:
		return true
	default:
		return false
	}
}

func isValidAIAlertType(t domain.AlertType) bool {
	switch t {
	case domain.AlertTypeComplaint, domain.AlertTypeRefusal,
		domain.AlertTypeNegative, domain.AlertTypeUrgent:
		return true
	default:
		return false
	}
}

// Create вызывается обработчиками событий, когда в диалоге обнаружена проблема.
func (u *AlertUsecase) Create(ctx context.Context, alert domain.Alert) (int64, error) {
	return u.createAlert(ctx, alert, true)
}

func (u *AlertUsecase) createAlert(ctx context.Context, alert domain.Alert, verifyOwnership bool) (int64, error) {
	if alert.CompanyID <= 0 || alert.ChatID <= 0 {
		return 0, fmt.Errorf("%w: company_id and chat_id must be positive", domain.ErrValidation)
	}
	if !isValidAlertType(alert.Type) {
		return 0, fmt.Errorf("%w: invalid alert type", domain.ErrValidation)
	}
	if alert.ResolvedAt != nil {
		return 0, fmt.Errorf("%w: a new alert must be unresolved", domain.ErrValidation)
	}
	if alert.CreatedAt.IsZero() {
		alert.CreatedAt = time.Now().UTC()
	}

	if verifyOwnership {
		if err := u.verifyChatCompany(ctx, alert.CompanyID, alert.ChatID); err != nil {
			return 0, err
		}
	}

	exists, err := u.alerts.ExistsActive(ctx, alert.CompanyID, alert.ChatID, alert.Type)
	if err != nil {
		return 0, fmt.Errorf("usecase check active alert: %w", err)
	}
	if exists {
		return 0, nil
	}

	id, err := u.alerts.Create(ctx, alert)
	if err != nil {
		if errors.Is(err, domain.ErrConflict) {
			return 0, nil
		}
		return 0, fmt.Errorf("usecase create alert: %w", err)
	}
	return id, nil
}

func (u *AlertUsecase) verifyChatCompany(ctx context.Context, companyID, chatID int64) error {
	if u.chats == nil {
		return nil
	}
	if u.accounts == nil {
		return errors.New("alert usecase: account repository is not configured")
	}
	chat, err := u.chats.GetByID(ctx, chatID)
	if err != nil {
		return err
	}
	acc, err := u.accounts.GetByID(ctx, chat.AccountID)
	if err != nil {
		return err
	}
	if acc.CompanyID != companyID {
		return fmt.Errorf("%w: chat belongs to a different company", domain.ErrForbidden)
	}
	return nil
}

// ApplyAIAnalysis обновляет автостатус диалога (если он не задан вручную) и
// создаёт алерты по флагам AI без дублирования уже активных сигналов.
func (u *AlertUsecase) ApplyAIAnalysis(ctx context.Context, companyID, chatID int64, result *AIAnalysisResult) error {
	if companyID <= 0 {
		return fmt.Errorf("%w: invalid company_id", domain.ErrValidation)
	}
	if chatID <= 0 {
		return fmt.Errorf("%w: invalid chat_id", domain.ErrValidation)
	}
	if result == nil {
		return fmt.Errorf("%w: analysis result is required", domain.ErrValidation)
	}
	if result.SuggestedStatus != nil && !isValidChatStatus(*result.SuggestedStatus) {
		return fmt.Errorf("%w: invalid suggested chat status '%s'", domain.ErrValidation, *result.SuggestedStatus)
	}
	for _, alertType := range result.AlertTypes {
		if !isValidAIAlertType(alertType) {
			return fmt.Errorf("%w: invalid AI alert type '%s'", domain.ErrValidation, alertType)
		}
	}
	if u.chats == nil || u.accounts == nil {
		return errors.New("alert usecase: chat or account repository is not configured")
	}

	if err := u.verifyChatCompany(ctx, companyID, chatID); err != nil {
		return err
	}

	var errs []error
	if result.SuggestedStatus != nil {
		if err := u.chats.UpdateAutoStatus(ctx, companyID, chatID, *result.SuggestedStatus); err != nil {
			errs = append(errs, fmt.Errorf("usecase apply auto status: %w", err))
		}
	}

	if u.alerts != nil && len(result.AlertTypes) > 0 {
		now := time.Now().UTC()
		seen := make(map[domain.AlertType]struct{}, len(result.AlertTypes))
		for _, alertType := range result.AlertTypes {
			if _, ok := seen[alertType]; ok {
				continue
			}
			seen[alertType] = struct{}{}
			if _, err := u.createAlert(ctx, domain.Alert{
				CompanyID: companyID,
				ChatID:    chatID,
				Type:      alertType,
				CreatedAt: now,
			}, false); err != nil {
				errs = append(errs, fmt.Errorf("alert type %s: %w", alertType, err))
			}
		}
	}

	return errors.Join(errs...)
}

// CheckNoReplyAlerts находит диалоги, превысившие порог ожидания ответа компании,
// и создаёт для них алерты типа no_reply без дублирования активных сигналов.
func (u *AlertUsecase) CheckNoReplyAlerts(ctx context.Context) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if u.alerts == nil {
		return 0, fmt.Errorf("%w: alert repository is unavailable", domain.ErrValidation)
	}

	candidates, err := u.alerts.FindExpiredNoReplyChats(ctx)
	if err != nil {
		return 0, fmt.Errorf("usecase find expired no_reply chats: %w", err)
	}

	now := time.Now().UTC()
	created := 0
	var errs []error
	for _, c := range candidates {
		if err := ctx.Err(); err != nil {
			return created, err
		}
		id, err := u.createAlert(ctx, domain.Alert{
			CompanyID: c.CompanyID,
			ChatID:    c.ChatID,
			Type:      domain.AlertTypeNoReply,
			CreatedAt: now,
		}, false)
		if err != nil {
			errs = append(errs, fmt.Errorf("chat %d (company %d): %w", c.ChatID, c.CompanyID, err))
			continue
		}
		if id > 0 {
			created++
		}
	}
	if len(errs) > 0 {
		return created, errors.Join(errs...)
	}
	return created, nil
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
