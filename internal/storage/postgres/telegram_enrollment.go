package postgres

import (
	"context"
	"fmt"

	"github.com/kraconst/GO_server/internal/domain"
)

func (s *AccountStorage) LoadTelegramEnrollment(ctx context.Context, companyID, accountID int64) (domain.Account, error) {
	var account domain.Account
	if err := validateTelegramAccountIDs(companyID, accountID); err != nil {
		return account, err
	}
	const query = `SELECT id, company_id, phone, telegram_user_id FROM accounts WHERE company_id = $1 AND id = $2`
	if err := s.db.GetContext(ctx, &account, query, companyID, accountID); err != nil {
		return account, mapPgError(fmt.Errorf("load account for Telegram enrollment: %w", err))
	}
	return account, nil
}

// ActivateTelegramSession atomically saves a verified session against the
// account identity used during verification.
func (s *AccountStorage) ActivateTelegramSession(ctx context.Context, account domain.Account, userID int64, ciphertext []byte) error {
	if err := validateTelegramAccountIDs(account.CompanyID, account.ID); err != nil {
		return err
	}
	if userID <= 0 || len(ciphertext) == 0 || (account.TelegramUserID != 0 && account.TelegramUserID != userID) {
		return fmt.Errorf("%w: invalid Telegram enrollment", domain.ErrValidation)
	}
	const query = `
		UPDATE accounts SET session = $5, telegram_user_id = $6, status = 'active'
		WHERE company_id = $1 AND id = $2 AND phone = $3 AND telegram_user_id = $4
	`
	result, err := s.db.ExecContext(ctx, query, account.CompanyID, account.ID, account.Phone,
		account.TelegramUserID, ciphertext, userID)
	if err != nil {
		return mapPgError(fmt.Errorf("activate Telegram session: %w", err))
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count activated Telegram accounts: %w", err)
	}
	if count == 0 {
		return fmt.Errorf("%w: account changed during Telegram session verification", domain.ErrConflict)
	}
	return nil
}
