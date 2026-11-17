package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kraconst/GO_server/internal/domain"
)

type ChatSummaryStorage struct {
	db *sqlx.DB
}

func NewChatSummaryStorage(db *sqlx.DB) *ChatSummaryStorage {
	return &ChatSummaryStorage{db: db}
}

func (s *ChatSummaryStorage) Upsert(ctx context.Context, summary domain.ChatSummary) error {
	query := `
		INSERT INTO chat_summaries (chat_id, topic, agreements, next_step, open_questions, risk, last_message_id, generated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (chat_id) DO UPDATE SET
			topic = EXCLUDED.topic,
			agreements = EXCLUDED.agreements,
			next_step = EXCLUDED.next_step,
			open_questions = EXCLUDED.open_questions,
			risk = EXCLUDED.risk,
			last_message_id = EXCLUDED.last_message_id,
			generated_at = EXCLUDED.generated_at
	`
	genAt := summary.GeneratedAt
	if genAt.IsZero() {
		genAt = time.Now().UTC()
	}
	risk := summary.Risk
	if risk == "" {
		risk = domain.RiskLow
	}

	_, err := s.db.ExecContext(ctx, query,
		summary.ChatID,
		summary.Topic,
		summary.Agreements,
		summary.NextStep,
		summary.OpenQuestions,
		risk,
		summary.LastMessageID,
		genAt,
	)
	if err != nil {
		return fmt.Errorf("upsert chat summary: %w", err)
	}
	return nil
}

func (s *ChatSummaryStorage) GetByChatID(ctx context.Context, companyID, chatID int64) (*domain.ChatSummary, error) {
	query := `
		SELECT s.chat_id, s.topic, s.agreements, s.next_step, s.open_questions, s.risk, s.last_message_id, s.generated_at
		FROM chat_summaries s
		JOIN chats c ON c.id = s.chat_id
		JOIN accounts a ON a.id = c.account_id
		WHERE s.chat_id = $1 AND a.company_id = $2
	`
	var summary domain.ChatSummary
	err := s.db.GetContext(ctx, &summary, query, chatID, companyID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("select chat summary by chat_id: %w", err)
	}
	return &summary, nil
}
