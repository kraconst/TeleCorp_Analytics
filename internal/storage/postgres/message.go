package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/kraconst/GO_server/internal/domain"
)

type MessageStorage struct {
	db *sqlx.DB
}

func NewMessageStorage(db *sqlx.DB) *MessageStorage {
	return &MessageStorage{db: db}
}

func (s *MessageStorage) Create(ctx context.Context, msg domain.Message) (int64, error) {
	query := `
		INSERT INTO messages (chat_id, telegram_message_id, direction, text, media_type, sent_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (chat_id, telegram_message_id) DO NOTHING
		RETURNING id
	`
	var id int64
	err := s.db.QueryRowContext(ctx, query,
		msg.ChatID,
		msg.TelegramMessageID,
		msg.Direction,
		msg.Text,
		msg.MediaType,
		msg.SentAt,
	).Scan(&id)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		return 0, mapPgError(fmt.Errorf("insert message: %w", err))
	}
	return id, nil
}

func (s *MessageStorage) ListByChat(ctx context.Context, companyID, chatID int64, limit, offset int) ([]domain.Message, error) {
	query := `
		SELECT m.id, m.chat_id, m.telegram_message_id, m.direction, m.text, m.media_type, m.sent_at
		FROM messages m
		JOIN chats c ON c.id = m.chat_id
		JOIN accounts a ON a.id = c.account_id
		WHERE m.chat_id = $1 AND a.company_id = $2
		ORDER BY m.sent_at ASC, m.id ASC
		LIMIT $3 OFFSET $4
	`
	messages := make([]domain.Message, 0)
	err := s.db.SelectContext(ctx, &messages, query, chatID, companyID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list messages by chat: %w", err)
	}
	return messages, nil
}

// ListLatestByChat returns at most limit latest messages of the company's chat
// in chronological order. The latest rows are selected by (sent_at DESC, id DESC)
// and then re-ordered ascending. A chat of another company or a chat without
// messages yields an empty non-nil slice.
func (s *MessageStorage) ListLatestByChat(ctx context.Context, companyID, chatID int64, limit int) ([]domain.Message, error) {
	if companyID <= 0 {
		return nil, fmt.Errorf("%w: company id must be positive", domain.ErrValidation)
	}
	if chatID <= 0 {
		return nil, fmt.Errorf("%w: chat id must be positive", domain.ErrValidation)
	}
	if limit <= 0 {
		return nil, fmt.Errorf("%w: limit must be positive", domain.ErrValidation)
	}
	query := `
		SELECT latest.id, latest.chat_id, latest.telegram_message_id, latest.direction, latest.text, latest.media_type, latest.sent_at
		FROM (
			SELECT m.id, m.chat_id, m.telegram_message_id, m.direction, m.text, m.media_type, m.sent_at
			FROM messages m
			JOIN chats c ON c.id = m.chat_id
			JOIN accounts a ON a.id = c.account_id
			WHERE m.chat_id = $1 AND a.company_id = $2
			ORDER BY m.sent_at DESC, m.id DESC
			LIMIT $3
		) AS latest
		ORDER BY latest.sent_at ASC, latest.id ASC
	`
	messages := make([]domain.Message, 0)
	if err := s.db.SelectContext(ctx, &messages, query, chatID, companyID, limit); err != nil {
		return nil, mapPgError(fmt.Errorf("list latest messages by chat: %w", err))
	}
	return messages, nil
}
