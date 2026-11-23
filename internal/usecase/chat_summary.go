package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/kraconst/GO_server/internal/domain"
)

type ChatSummaryUsecase struct {
	summaries ChatSummaryRepository
	chats     ChatRepository
	accounts  AccountRepository
}

func NewChatSummaryUsecase(summaries ChatSummaryRepository, chats ChatRepository, accounts AccountRepository) *ChatSummaryUsecase {
	return &ChatSummaryUsecase{
		summaries: summaries,
		chats:     chats,
		accounts:  accounts,
	}
}

func (u *ChatSummaryUsecase) Upsert(ctx context.Context, companyID int64, summary domain.ChatSummary) error {
	if companyID <= 0 {
		return fmt.Errorf("%w: invalid company_id", domain.ErrValidation)
	}
	if summary.ChatID <= 0 {
		return fmt.Errorf("%w: invalid chat_id", domain.ErrValidation)
	}
	if summary.LastMessageID < 0 {
		return fmt.Errorf("%w: invalid last_message_id", domain.ErrValidation)
	}
	if u.chats != nil {
		chat, err := u.chats.GetByID(ctx, summary.ChatID)
		if err != nil {
			return err
		}
		if u.accounts != nil {
			acc, err := u.accounts.GetByID(ctx, chat.AccountID)
			if err != nil {
				return err
			}
			if acc.CompanyID != companyID {
				return fmt.Errorf("%w: chat not found", domain.ErrNotFound)
			}
		}
	}

	summary.Topic = strings.TrimSpace(summary.Topic)
	summary.Agreements = strings.TrimSpace(summary.Agreements)
	summary.NextStep = strings.TrimSpace(summary.NextStep)
	summary.OpenQuestions = strings.TrimSpace(summary.OpenQuestions)
	if summary.Risk == "" {
		summary.Risk = domain.RiskLow
	} else {
		switch summary.Risk {
		case domain.RiskLow, domain.RiskMedium, domain.RiskHigh:
		default:
			return fmt.Errorf("%w: invalid risk '%s'", domain.ErrValidation, summary.Risk)
		}
	}
	summary.GeneratedAt = time.Now().UTC()

	return u.summaries.Upsert(ctx, summary)
}

func (u *ChatSummaryUsecase) GetByChatID(ctx context.Context, companyID, chatID int64) (*domain.ChatSummary, error) {
	if companyID <= 0 {
		return nil, fmt.Errorf("%w: invalid company_id", domain.ErrValidation)
	}
	if chatID <= 0 {
		return nil, fmt.Errorf("%w: invalid chat_id", domain.ErrValidation)
	}
	if u.chats != nil {
		chat, err := u.chats.GetByID(ctx, chatID)
		if err != nil {
			return nil, err
		}
		if u.accounts != nil {
			acc, err := u.accounts.GetByID(ctx, chat.AccountID)
			if err != nil {
				return nil, err
			}
			if acc.CompanyID != companyID {
				return nil, fmt.Errorf("%w: chat not found", domain.ErrNotFound)
			}
		}
	}
	return u.summaries.GetByChatID(ctx, companyID, chatID)
}
