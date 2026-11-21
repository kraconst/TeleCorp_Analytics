package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/kraconst/GO_server/internal/domain"
)

type MessageUsecase struct {
	messages MessageRepository
	chats    ChatRepository
	accounts AccountRepository
}

func NewMessageUsecase(messages MessageRepository, chats ChatRepository, accounts AccountRepository) *MessageUsecase {
	return &MessageUsecase{
		messages: messages,
		chats:    chats,
		accounts: accounts,
	}
}

func (u *MessageUsecase) Create(ctx context.Context, msg domain.Message) (int64, error) {
	if msg.ChatID <= 0 {
		return 0, fmt.Errorf("%w: invalid chat_id", domain.ErrValidation)
	}
	if msg.TelegramMessageID <= 0 {
		return 0, fmt.Errorf("%w: invalid telegram_message_id", domain.ErrValidation)
	}
	if msg.Direction != domain.MessageDirectionIn && msg.Direction != domain.MessageDirectionOut {
		return 0, fmt.Errorf("%w: direction must be 'in' or 'out'", domain.ErrValidation)
	}
	if msg.SentAt.IsZero() {
		msg.SentAt = time.Now().UTC()
	}

	if u.chats != nil {
		if _, err := u.chats.GetByID(ctx, msg.ChatID); err != nil {
			return 0, err
		}
	}

	id, err := u.messages.Create(ctx, msg)
	if err != nil {
		return 0, fmt.Errorf("usecase create message: %w", err)
	}
	return id, nil
}

func (u *MessageUsecase) ListByChat(ctx context.Context, companyID, chatID int64, limit, offset int) ([]domain.Message, error) {
	if companyID <= 0 {
		return nil, fmt.Errorf("%w: invalid company_id", domain.ErrValidation)
	}
	if chatID <= 0 {
		return nil, fmt.Errorf("%w: invalid chat_id", domain.ErrValidation)
	}
	if limit <= 0 || limit > 100 {
		return nil, fmt.Errorf("%w: limit must be between 1 and 100", domain.ErrValidation)
	}
	if offset < 0 {
		return nil, fmt.Errorf("%w: offset must be non-negative", domain.ErrValidation)
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

	return u.messages.ListByChat(ctx, companyID, chatID, limit, offset)
}
