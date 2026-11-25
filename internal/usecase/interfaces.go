package usecase

import (
	"context"
	"errors"
	"time"

	"github.com/kraconst/GO_server/internal/domain"
)

// AccountRepository определяет контракт доступа к данным аккаунтов.
type AccountRepository interface {
	Create(ctx context.Context, acc domain.Account) (int64, error)
	GetByID(ctx context.Context, id int64) (*domain.Account, error)
	ListByCompany(ctx context.Context, companyID int64) ([]domain.Account, error)
	ListByEmployee(ctx context.Context, employeeID int64) ([]domain.Account, error)
	UpdateStatus(ctx context.Context, id int64, status domain.AccountStatus) error
	AssignEmployee(ctx context.Context, accountID, employeeID int64) error
}

// EmployeeRepository определяет контракт доступа к данным сотрудников.
type EmployeeRepository interface {
	Create(ctx context.Context, emp domain.Employee) (int64, error)
	GetByID(ctx context.Context, id int64) (*domain.Employee, error)
	ListByCompany(ctx context.Context, companyID int64) ([]domain.Employee, error)
	Update(ctx context.Context, emp domain.Employee) error
	Deactivate(ctx context.Context, id int64) error
}

// CompanyRepository определяет контракт доступа к данным компании.
type CompanyRepository interface {
	Create(ctx context.Context, comp domain.Company) (int64, error)
	GetByID(ctx context.Context, id int64) (*domain.Company, error)
}

// UserRepository определяет контракт доступа к пользователям системы.
type UserRepository interface {
	Create(ctx context.Context, user domain.User) (int64, error)
	GetByID(ctx context.Context, id int64) (*domain.User, error)
	GetByEmail(ctx context.Context, email string) (*domain.User, error)
	ListByCompany(ctx context.Context, companyID int64) ([]domain.User, error)
}

// MessageRepository определяет контракт доступа к сообщениям диалогов.
type MessageRepository interface {
	Create(ctx context.Context, msg domain.Message) (int64, error)
	ListByChat(ctx context.Context, companyID, chatID int64, limit, offset int) ([]domain.Message, error)
}

// ContactRepository определяет контракт доступа к данным контактов.
type ContactRepository interface {
	Create(ctx context.Context, c domain.Contact) (int64, error)
	GetByID(ctx context.Context, id int64) (*domain.Contact, error)
	GetByTelegramID(ctx context.Context, companyID, tgUserID int64) (*domain.Contact, error)
	ListByCompany(ctx context.Context, companyID int64) ([]domain.Contact, error)
}

// ChatRepository определяет контракт доступа к данным чатов.
type ChatRepository interface {
	Create(ctx context.Context, chat domain.Chat) (int64, error)
	GetByID(ctx context.Context, id int64) (*domain.Chat, error)
	ListByCompany(ctx context.Context, companyID int64, status *domain.ChatStatus, employeeID *int64) ([]domain.Chat, error)
	UpdateStatus(ctx context.Context, id int64, status domain.ChatStatus, source domain.StatusSource) error
}

// AccountDailyStatsRepository определяет контракт доступа к дневной аналитике аккаунтов.
type AccountDailyStatsRepository interface {
	Upsert(ctx context.Context, stats domain.AccountDailyStats) error
	IncrementCounters(ctx context.Context, accountID int64, day time.Time, messagesIn, messagesOut, newContacts int) error
	GetByAccountAndDateRange(ctx context.Context, companyID, accountID int64, from, to time.Time) ([]domain.AccountDailyStats, error)
	GetByCompanyAndDateRange(ctx context.Context, companyID int64, from, to time.Time) ([]domain.AccountDailyStats, error)
	GetDashboard(ctx context.Context, companyID int64, from, to time.Time) (*domain.DashboardStats, error)
	GetAccountRatings(ctx context.Context, companyID int64, from, to time.Time) ([]domain.AccountRating, error)
}

// AlertRepository хранит уведомления и ограничивает операции компанией.
type AlertRepository interface {
	Create(ctx context.Context, alert domain.Alert) (int64, error)
	ListActiveByCompany(ctx context.Context, companyID int64) ([]domain.Alert, error)
	Resolve(ctx context.Context, companyID, id int64) error
}

// ChatSummaryRepository определяет контракт доступа к AI-выжимкам диалогов.
type ChatSummaryRepository interface {
	Upsert(ctx context.Context, summary domain.ChatSummary) error
	GetByChatID(ctx context.Context, companyID, chatID int64) (*domain.ChatSummary, error)
}

// AnalyzeChatInput — контекст AI-анализа диалога (RFC 02, D3).
type AnalyzeChatInput struct {
	Messages []domain.Message    // хронологически, не более AI_MAX_MESSAGES
	Previous *domain.ChatSummary // nil, если саммари ещё нет
}

// AIAnalysisResult — провалидированный результат AI-анализа диалога.
type AIAnalysisResult struct {
	Topic, Agreements, NextStep, OpenQuestions string
	Risk                                       domain.RiskLevel
	SuggestedStatus                            *domain.ChatStatus // nil — не менять статус
	AlertTypes                                 []domain.AlertType // после фильтрации в internal/ai: только complaint|refusal|negative|urgent, без дублей
}

// ChatSummarizer строит AI-выжимку диалога. Реализуется internal/ai.
type ChatSummarizer interface {
	AnalyzeChat(ctx context.Context, in AnalyzeChatInput) (*AIAnalysisResult, error)
}

// ChatAnalysisHandler применяет результат анализа (автостатусы и алерты, трек 2B).
type ChatAnalysisHandler interface {
	HandleAnalysis(ctx context.Context, companyID, chatID int64, res AIAnalysisResult) error
}

// ErrAIRetryable — retryable-маркер для фоновой очереди (D6); оборачивается вместе с domain.ErrUnavailable.
var ErrAIRetryable = errors.New("ai: retryable")

// RetryAfterError exposes the provider's Retry-After hint on retryable AI errors.
// A negative duration means the hint is absent or invalid. Zero means retry immediately.
type RetryAfterError interface {
	error
	RetryAfter() time.Duration
}

// RecentMessageRepository — узкий интерфейс последних сообщений чата (2A).
type RecentMessageRepository interface {
	// Последние limit сообщений чата компании в хронологическом порядке (ASC).
	ListLatestByChat(ctx context.Context, companyID, chatID int64, limit int) ([]domain.Message, error)
}
