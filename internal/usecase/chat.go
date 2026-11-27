package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kraconst/GO_server/internal/domain"
)

type ChatDetails struct {
	domain.Chat
	Contact *domain.Contact     `json:"contact,omitempty"`
	Summary *domain.ChatSummary `json:"summary,omitempty"`
}

type ChatUsecase struct {
	chats     ChatRepository
	contacts  ContactRepository
	summaries ChatSummaryRepository
	accounts  AccountRepository
}

func NewChatUsecase(
	chats ChatRepository,
	contacts ContactRepository,
	summaries ChatSummaryRepository,
	accounts AccountRepository,
) *ChatUsecase {
	return &ChatUsecase{
		chats:     chats,
		contacts:  contacts,
		summaries: summaries,
		accounts:  accounts,
	}
}

func isValidChatStatus(status domain.ChatStatus) bool {
	switch status {
	case domain.ChatStatusNewLead,
		domain.ChatStatusNegotiation,
		domain.ChatStatusWaitingReply,
		domain.ChatStatusInterested,
		domain.ChatStatusUnhappy,
		domain.ChatStatusLost,
		domain.ChatStatusActionRequired:
		return true
	default:
		return false
	}
}

func isValidStatusSource(source domain.StatusSource) bool {
	return source == domain.StatusSourceAuto || source == domain.StatusSourceManual
}

func isValidChatPriority(priority domain.ChatPriority) bool {
	return priority == domain.ChatPriorityLow ||
		priority == domain.ChatPriorityNormal ||
		priority == domain.ChatPriorityHigh
}

func (u *ChatUsecase) SetManualStatus(ctx context.Context, companyID, chatID int64, status domain.ChatStatus) error {
	if companyID <= 0 {
		return fmt.Errorf("%w: invalid company_id", domain.ErrValidation)
	}
	if chatID <= 0 {
		return fmt.Errorf("%w: invalid chat_id", domain.ErrValidation)
	}
	if !isValidChatStatus(status) {
		return fmt.Errorf("%w: invalid chat status '%s'", domain.ErrValidation, status)
	}
	if _, err := u.GetByID(ctx, companyID, chatID); err != nil {
		return err
	}

	return u.chats.UpdateStatus(ctx, chatID, status, domain.StatusSourceManual)
}

func (u *ChatUsecase) ListByCompany(ctx context.Context, companyID int64, status *domain.ChatStatus, employeeID *int64) ([]domain.Chat, error) {
	if companyID <= 0 {
		return nil, fmt.Errorf("%w: invalid company_id", domain.ErrValidation)
	}
	if status != nil && !isValidChatStatus(*status) {
		return nil, fmt.Errorf("%w: invalid chat status '%s'", domain.ErrValidation, *status)
	}
	if employeeID != nil && *employeeID <= 0 {
		return nil, fmt.Errorf("%w: invalid employee_id", domain.ErrValidation)
	}

	return u.chats.ListByCompany(ctx, companyID, status, employeeID)
}

func (u *ChatUsecase) ListByFilter(ctx context.Context, filter domain.ChatFilter) ([]domain.Chat, error) {
	if filter.CompanyID <= 0 {
		return nil, fmt.Errorf("%w: invalid company_id", domain.ErrValidation)
	}
	if filter.Status != nil && !isValidChatStatus(*filter.Status) {
		return nil, fmt.Errorf("%w: invalid chat status '%s'", domain.ErrValidation, *filter.Status)
	}
	if filter.EmployeeID != nil && *filter.EmployeeID <= 0 {
		return nil, fmt.Errorf("%w: invalid employee_id", domain.ErrValidation)
	}
	if filter.AccountID != nil && *filter.AccountID <= 0 {
		return nil, fmt.Errorf("%w: invalid account_id", domain.ErrValidation)
	}
	if filter.Priority != nil && !isValidChatPriority(*filter.Priority) {
		return nil, fmt.Errorf("%w: invalid priority '%s'", domain.ErrValidation, *filter.Priority)
	}

	filter.Query = strings.TrimSpace(filter.Query)

	if filter.SortBy == "" {
		filter.SortBy = domain.ChatSortLastMessage
	} else if filter.SortBy != domain.ChatSortLastMessage && filter.SortBy != domain.ChatSortAwaitingReply {
		return nil, fmt.Errorf("%w: invalid sort '%s'", domain.ErrValidation, filter.SortBy)
	}

	if filter.Limit == 0 {
		filter.Limit = 50
	} else if filter.Limit < 0 || filter.Limit > 100 {
		return nil, fmt.Errorf("%w: invalid limit %d", domain.ErrValidation, filter.Limit)
	}

	if filter.Offset < 0 {
		return nil, fmt.Errorf("%w: invalid offset %d", domain.ErrValidation, filter.Offset)
	}

	chats, err := u.chats.ListByFilter(ctx, filter)
	if err != nil {
		return nil, err
	}
	if chats == nil {
		return make([]domain.Chat, 0), nil
	}
	return chats, nil
}

func (u *ChatUsecase) GetByID(ctx context.Context, companyID, id int64) (*domain.Chat, error) {
	if companyID <= 0 {
		return nil, fmt.Errorf("%w: invalid company_id", domain.ErrValidation)
	}
	if id <= 0 {
		return nil, fmt.Errorf("%w: invalid id", domain.ErrValidation)
	}
	chat, err := u.chats.GetByID(ctx, id)
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
	return chat, nil
}

func (u *ChatUsecase) GetDetails(ctx context.Context, companyID, id int64) (*ChatDetails, error) {
	chat, err := u.GetByID(ctx, companyID, id)
	if err != nil {
		return nil, err
	}

	details := &ChatDetails{
		Chat: *chat,
	}

	if u.contacts != nil && chat.ContactID > 0 {
		contact, err := u.contacts.GetByID(ctx, chat.ContactID)
		if err != nil && !errors.Is(err, domain.ErrNotFound) {
			return nil, fmt.Errorf("get contact for chat details: %w", err)
		}
		if err == nil && contact.CompanyID == companyID {
			details.Contact = contact
		}
	}

	if u.summaries != nil {
		summary, err := u.summaries.GetByChatID(ctx, companyID, id)
		if err != nil && !errors.Is(err, domain.ErrNotFound) {
			return nil, fmt.Errorf("get summary for chat details: %w", err)
		}
		if err == nil {
			details.Summary = summary
		}
	}

	return details, nil
}

func (u *ChatUsecase) Create(ctx context.Context, chat domain.Chat) (*domain.Chat, error) {
	if chat.AccountID <= 0 {
		return nil, fmt.Errorf("%w: invalid account_id", domain.ErrValidation)
	}
	if chat.ContactID <= 0 {
		return nil, fmt.Errorf("%w: invalid contact_id", domain.ErrValidation)
	}

	if u.accounts != nil && u.contacts != nil {
		acc, err := u.accounts.GetByID(ctx, chat.AccountID)
		if err != nil {
			return nil, err
		}
		contact, err := u.contacts.GetByID(ctx, chat.ContactID)
		if err != nil {
			return nil, err
		}
		if acc.CompanyID != contact.CompanyID {
			return nil, fmt.Errorf("%w: account and contact belong to different companies", domain.ErrForbidden)
		}
	}

	if chat.Status == "" {
		chat.Status = domain.ChatStatusNewLead
	} else if !isValidChatStatus(chat.Status) {
		return nil, fmt.Errorf("%w: invalid chat status '%s'", domain.ErrValidation, chat.Status)
	}

	if chat.StatusSource == "" {
		chat.StatusSource = domain.StatusSourceAuto
	} else if !isValidStatusSource(chat.StatusSource) {
		return nil, fmt.Errorf("%w: invalid status_source '%s'", domain.ErrValidation, chat.StatusSource)
	}

	if chat.Priority == "" {
		chat.Priority = domain.ChatPriorityNormal
	} else if !isValidChatPriority(chat.Priority) {
		return nil, fmt.Errorf("%w: invalid priority '%s'", domain.ErrValidation, chat.Priority)
	}

	if chat.LastMessageAt.IsZero() {
		chat.LastMessageAt = time.Now().UTC()
	}

	id, err := u.chats.Create(ctx, chat)
	if err != nil {
		return nil, fmt.Errorf("usecase create chat: %w", err)
	}

	created, err := u.chats.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get created chat: %w", err)
	}
	return created, nil
}
