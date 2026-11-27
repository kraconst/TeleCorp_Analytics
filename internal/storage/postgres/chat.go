package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kraconst/GO_server/internal/domain"
)

var likeEscaper = strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_")

type ChatStorage struct {
	db *sqlx.DB
}

func NewChatStorage(db *sqlx.DB) *ChatStorage {
	return &ChatStorage{db: db}
}

// Create inserts a new chat with default fallbacks and returns the generated ID.
func (s *ChatStorage) Create(ctx context.Context, chat domain.Chat) (int64, error) {
	status := chat.Status
	if status == "" {
		status = domain.ChatStatusNewLead
	}
	source := chat.StatusSource
	if source == "" {
		source = domain.StatusSourceAuto
	}
	priority := chat.Priority
	if priority == "" {
		priority = domain.ChatPriorityNormal
	}
	lastMessageAt := chat.LastMessageAt
	if lastMessageAt.IsZero() {
		lastMessageAt = time.Now().UTC()
	}

	query := `
		INSERT INTO chats (
			account_id, contact_id, status, status_source, priority,
			last_message_at, awaiting_reply_since, unread_count
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id
	`
	var id int64
	err := s.db.QueryRowContext(ctx, query,
		chat.AccountID,
		chat.ContactID,
		status,
		source,
		priority,
		lastMessageAt,
		chat.AwaitingReplySince,
		chat.UnreadCount,
	).Scan(&id)
	if err != nil {
		return 0, mapPgError(fmt.Errorf("insert chat: %w", err))
	}
	return id, nil
}

// GetByID returns a chat by ID or domain.ErrNotFound if not found.
func (s *ChatStorage) GetByID(ctx context.Context, id int64) (*domain.Chat, error) {
	query := `
		SELECT id, account_id, contact_id, status, status_source, priority,
		       last_message_at, awaiting_reply_since, unread_count
		FROM chats
		WHERE id = $1
	`
	var chat domain.Chat
	err := s.db.GetContext(ctx, &chat, query, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("select chat by id: %w", err)
	}
	return &chat, nil
}

// ListByCompany joins accounts to filter by company and optional employee and status.
func (s *ChatStorage) ListByCompany(ctx context.Context, companyID int64, status *domain.ChatStatus, employeeID *int64) ([]domain.Chat, error) {
	query := `
		SELECT c.id, c.account_id, c.contact_id, c.status, c.status_source, c.priority,
		       c.last_message_at, c.awaiting_reply_since, c.unread_count
		FROM chats c
		JOIN accounts a ON c.account_id = a.id
		WHERE a.company_id = $1
		  AND ($2::varchar IS NULL OR c.status = $2)
		  AND ($3::bigint IS NULL OR a.employee_id = $3)
		ORDER BY c.last_message_at DESC, c.id DESC
	`
	var statusVal *string
	if status != nil {
		str := string(*status)
		statusVal = &str
	}

	chats := make([]domain.Chat, 0)
	err := s.db.SelectContext(ctx, &chats, query, companyID, statusVal, employeeID)
	if err != nil {
		return nil, fmt.Errorf("list chats by company: %w", err)
	}
	return chats, nil
}

// ListByFilter returns company chats matching optional filters, search query, sorting, and pagination.
func (s *ChatStorage) ListByFilter(ctx context.Context, filter domain.ChatFilter) ([]domain.Chat, error) {
	var qb strings.Builder
	qb.WriteString(`SELECT c.id, c.account_id, c.contact_id, c.status, c.status_source, c.priority,
       c.last_message_at, c.awaiting_reply_since, c.unread_count
FROM chats c
JOIN accounts a ON c.account_id = a.id
JOIN contacts ct ON c.contact_id = ct.id AND ct.company_id = a.company_id
WHERE a.company_id = $1`)

	args := []any{filter.CompanyID}

	if filter.Status != nil {
		args = append(args, string(*filter.Status))
		fmt.Fprintf(&qb, " AND c.status = $%d", len(args))
	}
	if filter.EmployeeID != nil {
		args = append(args, *filter.EmployeeID)
		fmt.Fprintf(&qb, " AND a.employee_id = $%d", len(args))
	}
	if filter.AccountID != nil {
		args = append(args, *filter.AccountID)
		fmt.Fprintf(&qb, " AND c.account_id = $%d", len(args))
	}
	if filter.Priority != nil {
		args = append(args, string(*filter.Priority))
		fmt.Fprintf(&qb, " AND c.priority = $%d", len(args))
	}
	if filter.UnansweredOnly {
		qb.WriteString(" AND c.awaiting_reply_since IS NOT NULL")
	}
	if q := strings.TrimSpace(filter.Query); q != "" {
		pattern := "%" + likeEscaper.Replace(q) + "%"
		args = append(args, pattern)
		idx := len(args)
		fmt.Fprintf(&qb,
			" AND (ct.name ILIKE $%d ESCAPE '\\' OR ct.username ILIKE $%d ESCAPE '\\' OR EXISTS (SELECT 1 FROM messages m WHERE m.chat_id = c.id AND m.text ILIKE $%d ESCAPE '\\'))",
			idx, idx, idx,
		)
	}

	switch filter.SortBy {
	case domain.ChatSortAwaitingReply:
		qb.WriteString(" ORDER BY c.awaiting_reply_since ASC NULLS LAST, c.id DESC")
	default:
		qb.WriteString(" ORDER BY c.last_message_at DESC, c.id DESC")
	}

	if filter.Limit > 0 {
		args = append(args, filter.Limit)
		fmt.Fprintf(&qb, " LIMIT $%d", len(args))
	}
	if filter.Offset > 0 {
		args = append(args, filter.Offset)
		fmt.Fprintf(&qb, " OFFSET $%d", len(args))
	}

	chats := make([]domain.Chat, 0)
	if err := s.db.SelectContext(ctx, &chats, qb.String(), args...); err != nil {
		return nil, mapPgError(fmt.Errorf("list chats by filter: %w", err))
	}
	return chats, nil
}

// UpdateStatus updates chat status and source, returning domain.ErrNotFound if no rows matched.
func (s *ChatStorage) UpdateStatus(ctx context.Context, id int64, status domain.ChatStatus, source domain.StatusSource) error {
	query := `
		UPDATE chats
		SET status = $1, status_source = $2
		WHERE id = $3
	`
	res, err := s.db.ExecContext(ctx, query, status, source, id)
	if err != nil {
		return mapPgError(fmt.Errorf("update chat status: %w", err))
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}
	if rows == 0 {
		return domain.ErrNotFound
	}
	return nil
}
