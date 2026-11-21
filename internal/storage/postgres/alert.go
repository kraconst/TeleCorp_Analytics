package postgres

import (
	"context"
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
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`
	var id int64
	if err := s.db.QueryRowContext(ctx, query,
		alert.CompanyID, alert.ChatID, alert.Type, alert.CreatedAt,
	).Scan(&id); err != nil {
		return 0, mapPgError(fmt.Errorf("insert alert: %w", err))
	}
	return id, nil
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
