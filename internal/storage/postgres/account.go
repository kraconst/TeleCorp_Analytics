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

type AccountStorage struct {
	db *sqlx.DB
}

func NewAccountStorage(db *sqlx.DB) *AccountStorage {
	return &AccountStorage{db: db}
}

func (s *AccountStorage) Create(ctx context.Context, acc domain.Account) (int64, error) {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	query := `
		INSERT INTO accounts (company_id, employee_id, phone, username, telegram_user_id, status, session, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id
	`
	var id int64
	createdAt := acc.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}

	err = tx.QueryRowContext(ctx, query,
		acc.CompanyID,
		acc.EmployeeID,
		acc.Phone,
		acc.Username,
		acc.TelegramUserID,
		acc.Status,
		acc.Session,
		createdAt,
	).Scan(&id)
	if err != nil {
		return 0, mapPgError(fmt.Errorf("insert account: %w", err))
	}

	// Записываем первоначальное назначение ответственного сотрудника
	if acc.EmployeeID > 0 {
		assignQuery := `
			INSERT INTO account_assignments (account_id, employee_id, from_time)
			VALUES ($1, $2, $3)
		`
		if _, err := tx.ExecContext(ctx, assignQuery, id, acc.EmployeeID, createdAt); err != nil {
			return 0, mapPgError(fmt.Errorf("insert initial assignment: %w", err))
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit tx: %w", err)
	}
	return id, nil
}

func (s *AccountStorage) GetByID(ctx context.Context, id int64) (*domain.Account, error) {
	query := `
		SELECT a.id, a.company_id, a.employee_id, a.phone, a.username,
		       COALESCE(e.name, '') AS employee_name,
		       a.telegram_user_id, a.status, NULL::bytea AS session, a.last_activity_at, a.created_at
		FROM accounts a
		LEFT JOIN employees e ON a.employee_id = e.id
		WHERE a.id = $1
	`
	var acc domain.Account
	err := s.db.GetContext(ctx, &acc, query, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("select account by id: %w", err)
	}
	return &acc, nil
}

func (s *AccountStorage) ListByCompany(ctx context.Context, companyID int64) ([]domain.Account, error) {
	query := `
		SELECT a.id, a.company_id, a.employee_id, a.phone, a.username,
		       COALESCE(e.name, '') AS employee_name,
		       a.telegram_user_id, a.status, NULL::bytea AS session, a.last_activity_at, a.created_at
		FROM accounts a
		LEFT JOIN employees e ON a.employee_id = e.id
		WHERE a.company_id = $1
		ORDER BY a.id ASC
	`
	accounts := make([]domain.Account, 0)
	err := s.db.SelectContext(ctx, &accounts, query, companyID)
	if err != nil {
		return nil, fmt.Errorf("list accounts by company: %w", err)
	}
	return accounts, nil
}

func (s *AccountStorage) ListByEmployee(ctx context.Context, employeeID int64) ([]domain.Account, error) {
	query := `
		SELECT a.id, a.company_id, a.employee_id, a.phone, a.username,
		       COALESCE(e.name, '') AS employee_name,
		       a.telegram_user_id, a.status, NULL::bytea AS session, a.last_activity_at, a.created_at
		FROM accounts a
		LEFT JOIN employees e ON a.employee_id = e.id
		WHERE a.employee_id = $1
		ORDER BY a.id ASC
	`
	accounts := make([]domain.Account, 0)
	err := s.db.SelectContext(ctx, &accounts, query, employeeID)
	if err != nil {
		return nil, fmt.Errorf("list accounts by employee: %w", err)
	}
	return accounts, nil
}

func (s *AccountStorage) UpdateStatus(ctx context.Context, companyID, id int64, status domain.AccountStatus) error {
	query := `UPDATE accounts SET status = $1 WHERE id = $2 AND company_id = $3`
	res, err := s.db.ExecContext(ctx, query, status, id, companyID)
	if err != nil {
		return fmt.Errorf("update account status: %w", err)
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

func (s *AccountStorage) LoadSession(ctx context.Context, companyID, accountID int64) ([]byte, error) {
	if err := validateTelegramAccountIDs(companyID, accountID); err != nil {
		return nil, err
	}
	const query = `SELECT session FROM accounts WHERE company_id = $1 AND id = $2`
	var data []byte
	if err := s.db.GetContext(ctx, &data, query, companyID, accountID); err != nil {
		return nil, mapPgError(fmt.Errorf("load account session: %w", err))
	}
	return data, nil
}

func (s *AccountStorage) UpdateSession(ctx context.Context, companyID, accountID int64, data []byte) error {
	const query = `UPDATE accounts SET session = $3 WHERE company_id = $1 AND id = $2 RETURNING id`
	return s.updateTelegramAccount(ctx, query, "update account session", companyID, accountID, data)
}

func (s *AccountStorage) UpdateLastActivity(ctx context.Context, companyID, accountID int64, at time.Time) error {
	if at.IsZero() {
		return fmt.Errorf("%w: missing account activity time", domain.ErrValidation)
	}
	const query = `
		UPDATE accounts
		SET last_activity_at = GREATEST(last_activity_at, $3)
		WHERE company_id = $1 AND id = $2
		RETURNING id
	`
	return s.updateTelegramAccount(ctx, query, "update account activity", companyID, accountID, at.UTC())
}

func (s *AccountStorage) UpdateSessionStatus(ctx context.Context, companyID, accountID int64, status domain.AccountStatus) error {
	if status != domain.StatusDisconnected && status != domain.StatusBanned {
		return fmt.Errorf("%w: invalid Telegram session status", domain.ErrValidation)
	}
	const query = `UPDATE accounts SET status = $3 WHERE company_id = $1 AND id = $2 RETURNING id`
	return s.updateTelegramAccount(ctx, query, "update account session status", companyID, accountID, status)
}

func (s *AccountStorage) updateTelegramAccount(ctx context.Context, query, operation string, companyID, accountID int64, value any) error {
	if err := validateTelegramAccountIDs(companyID, accountID); err != nil {
		return err
	}
	var id int64
	if err := s.db.GetContext(ctx, &id, query, companyID, accountID, value); err != nil {
		return mapPgError(fmt.Errorf("%s: %w", operation, err))
	}
	return nil
}

func validateTelegramAccountIDs(companyID, accountID int64) error {
	if companyID <= 0 || accountID <= 0 {
		return fmt.Errorf("%w: invalid company_id or account_id", domain.ErrValidation)
	}
	return nil
}

func (s *AccountStorage) AssignEmployee(ctx context.Context, accountID, employeeID int64) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	now := time.Now().UTC()

	// Проверяем существование аккаунта и блокируем его для предотвращения параллельных назначений
	var accID int64
	if err := tx.GetContext(ctx, &accID, `SELECT id FROM accounts WHERE id = $1 FOR UPDATE`, accountID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.ErrNotFound
		}
		return mapPgError(fmt.Errorf("lock account for update: %w", err))
	}

	// Закрываем предыдущее активное назначение
	closeQuery := `
		UPDATE account_assignments
		SET to_time = $1
		WHERE account_id = $2 AND to_time IS NULL
	`
	if _, err := tx.ExecContext(ctx, closeQuery, now, accountID); err != nil {
		return mapPgError(fmt.Errorf("close previous assignment: %w", err))
	}

	// Создаем новую запись назначения
	insertQuery := `
		INSERT INTO account_assignments (account_id, employee_id, from_time)
		VALUES ($1, $2, $3)
	`
	if _, err := tx.ExecContext(ctx, insertQuery, accountID, employeeID, now); err != nil {
		return mapPgError(fmt.Errorf("insert new assignment: %w", err))
	}

	// Обновляем текущего ответственного в таблице accounts
	updateAccQuery := `UPDATE accounts SET employee_id = $1 WHERE id = $2`
	if _, err := tx.ExecContext(ctx, updateAccQuery, employeeID, accountID); err != nil {
		return mapPgError(fmt.Errorf("update account employee_id: %w", err))
	}

	if err := tx.Commit(); err != nil {
		return mapPgError(fmt.Errorf("commit tx: %w", err))
	}
	return nil
}
