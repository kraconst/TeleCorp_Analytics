package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jmoiron/sqlx"
	"github.com/kraconst/GO_server/internal/domain"
)

type UserStorage struct {
	db *sqlx.DB
}

func NewUserStorage(db *sqlx.DB) *UserStorage {
	return &UserStorage{db: db}
}

func (s *UserStorage) Create(ctx context.Context, u domain.User) (int64, error) {
	query := `
		INSERT INTO users (company_id, email, password_hash, role, employee_id, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`
	createdAt := u.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	role := u.Role
	if role == "" {
		role = domain.RoleManager
	}

	var id int64
	err := s.db.QueryRowContext(ctx, query,
		u.CompanyID,
		u.Email,
		u.PasswordHash,
		role,
		u.EmployeeID,
		createdAt,
	).Scan(&id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return 0, fmt.Errorf("%w: user with email %s already exists", domain.ErrConflict, u.Email)
		}
		if strings.Contains(err.Error(), "users_email_key") || strings.Contains(err.Error(), "23505") {
			return 0, fmt.Errorf("%w: user with email %s already exists", domain.ErrConflict, u.Email)
		}
		return 0, mapPgError(fmt.Errorf("insert user: %w", err))
	}
	return id, nil
}

func (s *UserStorage) GetByID(ctx context.Context, id int64) (*domain.User, error) {
	query := `
		SELECT id, company_id, email, password_hash, role, employee_id, created_at
		FROM users
		WHERE id = $1
	`
	var u domain.User
	err := s.db.GetContext(ctx, &u, query, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("select user by id: %w", err)
	}
	return &u, nil
}

func (s *UserStorage) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	query := `
		SELECT id, company_id, email, password_hash, role, employee_id, created_at
		FROM users
		WHERE email = $1
	`
	var u domain.User
	err := s.db.GetContext(ctx, &u, query, email)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("select user by email: %w", err)
	}
	return &u, nil
}

func (s *UserStorage) ListByCompany(ctx context.Context, companyID int64) ([]domain.User, error) {
	query := `
		SELECT id, company_id, email, password_hash, role, employee_id, created_at
		FROM users
		WHERE company_id = $1
		ORDER BY id ASC
	`
	users := make([]domain.User, 0)
	err := s.db.SelectContext(ctx, &users, query, companyID)
	if err != nil {
		return nil, fmt.Errorf("list users by company: %w", err)
	}
	return users, nil
}
