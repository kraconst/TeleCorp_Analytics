package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/kraconst/GO_server/internal/domain"
)

type AlertStorage struct {
	db *sqlx.DB
}

func NewAlertStorage(db *sqlx.DB) *AlertStorage {
	return &AlertStorage{db: db}
}

func (s *AlertStorage) Create(ctx context.Context, alert domain.Alert) (int64, error) {
	const query = `
		INSERT INTO alerts (company_id, chat_id, type, created_at)
		SELECT $1::bigint, $2::bigint, $3::varchar, $4::timestamptz
		WHERE NOT EXISTS (
			SELECT 1
			FROM alerts
			WHERE chat_id = $2 AND type = $3 AND resolved_at IS NULL
		)
		RETURNING id
	`
	var id int64
	err := s.db.QueryRowContext(ctx, query,
		alert.CompanyID, alert.ChatID, alert.Type, alert.CreatedAt,
	).Scan(&id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		mapped := mapPgError(fmt.Errorf("insert alert: %w", err))
		if errors.Is(mapped, domain.ErrConflict) {
			return 0, nil
		}
		return 0, mapped
	}
	return id, nil
}

func (s *AlertStorage) ExistsActive(ctx context.Context, chatID int64, alertType domain.AlertType) (bool, error) {
	const query = `
		SELECT EXISTS (
			SELECT 1
			FROM alerts
			WHERE chat_id = $1 AND type = $2 AND resolved_at IS NULL
		)
	`
	var exists bool
	if err := s.db.GetContext(ctx, &exists, query, chatID, alertType); err != nil {
		return false, fmt.Errorf("check active alert exists: %w", err)
	}
	return exists, nil
}

func (s *AlertStorage) FindExpiredNoReplyChats(ctx context.Context) ([]domain.NoReplyChat, error) {
	const query = `
		SELECT a.company_id, c.id AS chat_id
		FROM chats c
		JOIN accounts a ON a.id = c.account_id
		JOIN companies comp ON comp.id = a.company_id
		WHERE c.awaiting_reply_since IS NOT NULL
		  AND comp.no_reply_threshold >= 0
		  AND NOW() - c.awaiting_reply_since > (comp.no_reply_threshold / 1000000000.0) * INTERVAL '1 second'
		  AND NOT EXISTS (
		      SELECT 1
		      FROM alerts al
		      WHERE al.chat_id = c.id
		        AND al.type = $1
		        AND al.resolved_at IS NULL
		  )
		ORDER BY c.awaiting_reply_since ASC, c.id ASC
	`
	chats := make([]domain.NoReplyChat, 0)
	if err := s.db.SelectContext(ctx, &chats, query, domain.AlertTypeNoReply); err != nil {
		return nil, fmt.Errorf("find expired no_reply chats: %w", err)
	}
	return chats, nil
}

func (s *AlertStorage) ListActiveByCompany(ctx context.Context, companyID int64) ([]domain.Alert, error) {
	const query = `
		SELECT id, company_id, chat_id, type, created_at, resolved_at
		FROM alerts
		WHERE company_id = $1 AND resolved_at IS NULL
		ORDER BY created_at DESC, id DESC
	`
	alerts := make([]domain.Alert, 0)
	if err := s.db.SelectContext(ctx, &alerts, query, companyID); err != nil {
		return nil, fmt.Errorf("list active alerts: %w", err)
	}
	return alerts, nil
}

func (s *AlertStorage) Resolve(ctx context.Context, companyID, id int64) error {
	const query = `
		UPDATE alerts
		SET resolved_at = COALESCE(resolved_at, NOW())
		WHERE company_id = $1 AND id = $2
	`
	res, err := s.db.ExecContext(ctx, query, companyID, id)
	if err != nil {
		return mapPgError(fmt.Errorf("resolve alert: %w", err))
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("resolve alert rows affected: %w", err)
	}
	if rows == 0 {
		return domain.ErrNotFound
	}
	return nil
}
