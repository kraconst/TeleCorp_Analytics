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

type CompanyStorage struct {
	db *sqlx.DB
}

func NewCompanyStorage(db *sqlx.DB) *CompanyStorage {
	return &CompanyStorage{db: db}
}

func (s *CompanyStorage) Create(ctx context.Context, comp domain.Company) (int64, error) {
	query := `
		INSERT INTO companies (name, timezone, no_reply_threshold, created_at)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`
	createdAt := comp.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	tz := comp.Timezone
	if tz == "" {
		tz = "Europe/Moscow"
	}
	threshold := int64(comp.NoReplyThreshold)
	if threshold == 0 {
		threshold = int64(domain.DefaultNoReplyThreshold)
	}

	var id int64
	err := s.db.QueryRowContext(ctx, query, comp.Name, tz, threshold, createdAt).Scan(&id)
	if err != nil {
		return 0, mapPgError(fmt.Errorf("insert company: %w", err))
	}
	return id, nil
}

func (s *CompanyStorage) GetByID(ctx context.Context, id int64) (*domain.Company, error) {
	query := `
		SELECT id, name, timezone, no_reply_threshold, created_at
		FROM companies
		WHERE id = $1
	`
	var comp domain.Company
	err := s.db.GetContext(ctx, &comp, query, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("select company by id: %w", err)
	}
	return &comp, nil
}
