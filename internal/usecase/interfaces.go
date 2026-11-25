package usecase

import (
	"context"
	"errors"
	"time"

	"github.com/kraconst/GO_server/internal/domain"
)

// AccountRepository определяет контракт доступа к данным аккаунтов.
type AccountRepository interface {
	TelegramAccountStateRepository
	Create(ctx context.Context, acc domain.Account) (int64, error)
	GetByID(ctx context.Context, id int64) (*domain.Account, error)
	ListByCompany(ctx context.Context, companyID int64) ([]domain.Account, error)
	ListByEmployee(ctx context.Context, employeeID int64) ([]domain.Account, error)
	UpdateStatus(ctx context.Context, id int64, status domain.AccountStatus) error
	AssignEmployee(ctx context.Context, accountID, employeeID int64) error
}

// TelegramAccountStateRepository ограничивает работу с сессией и её статусом компанией.
// LoadSession возвращает пустой срез для аккаунта без сессии и ErrNotFound при отсутствии строки.
type TelegramAccountStateRepository interface {
	LoadSession(ctx context.Context, companyID, accountID int64) ([]byte, error)
	UpdateSession(ctx context.Context, companyID, accountID int64, data []byte) error
	UpdateLastActivity(ctx context.Context, companyID, accountID int64, at time.Time) error
	UpdateSessionStatus(ctx context.Context, companyID, accountID int64, status domain.AccountStatus) error
}

var (
	ErrTelegramUnavailable    = errors.New("Telegram sync is not configured")
	ErrTelegramSessionInvalid = errors.New("Telegram session is missing or invalid")
)

// TelegramDialog содержит метаданные собеседника для чтения истории той же сессией.
type TelegramDialog struct {
	TelegramUserID int64
	Name           string
	Username       string
	UnreadCount    int
}

// TelegramAccountReader действует только внутри вызова TelegramSyncClient.WithAccount.
type TelegramAccountReader interface {
	ListDialogs(ctx context.Context) ([]TelegramDialog, error)
	// ListHistory читает диалог из ListDialogs той же сессии и возвращает полную
	// выборку после afterID в порядке возрастания ID.
	// Внутренний ChatID заполняет вызывающий код при сохранении сообщений.
	ListHistory(ctx context.Context, dialog TelegramDialog, afterID int64) ([]domain.Message, error)
}

type TelegramSyncClient interface {
	WithAccount(ctx context.Context, companyID, accountID int64, fn func(context.Context, TelegramAccountReader) error) error
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
