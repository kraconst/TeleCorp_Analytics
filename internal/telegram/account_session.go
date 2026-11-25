package telegram

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/gotd/td/session"
	"github.com/kraconst/GO_server/internal/domain"
	"github.com/kraconst/GO_server/internal/usecase"
)

const accountSessionVersion byte = 1

var errUnreadableAccountSession = errors.New("cannot read encrypted Telegram session")

// AccountSessionStorage stores gotd session bytes for one company and account.
type AccountSessionStorage struct {
	accounts  usecase.TelegramAccountStateRepository
	companyID int64
	accountID int64
	cipher    cipher.AEAD
}

var _ session.Storage = (*AccountSessionStorage)(nil)

// NewAccountSessionStorage creates storage for session data belonging to this account.
// Enrollment must verify Telegram identity before persisting an authorized session.
func NewAccountSessionStorage(accounts usecase.TelegramAccountStateRepository, companyID, accountID int64, key []byte) (*AccountSessionStorage, error) {
	if accounts == nil || companyID <= 0 || accountID <= 0 {
		return nil, fmt.Errorf("%w: Telegram storage needs a repository and positive company and account IDs", domain.ErrValidation)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("%w: Telegram session encryption requires a 32-byte key", domain.ErrValidation)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("initialize Telegram session encryption: %w", err)
	}
	aead, err := cipher.NewGCMWithRandomNonce(block)
	if err != nil {
		return nil, fmt.Errorf("initialize Telegram session encryption: %w", err)
	}
	return &AccountSessionStorage{accounts: accounts, companyID: companyID, accountID: accountID, cipher: aead}, nil
}

func (s *AccountSessionStorage) LoadSession(ctx context.Context) ([]byte, error) {
	data, err := s.accounts.LoadSession(ctx, s.companyID, s.accountID)
	if err != nil {
		return nil, fmt.Errorf("load account session: %w", err)
	}
	if len(data) == 0 {
		return nil, session.ErrNotFound
	}
	if data[0] != accountSessionVersion || len(data) <= 1+s.cipher.Overhead() {
		return nil, errUnreadableAccountSession
	}
	aad := s.associatedData()
	plain, err := s.cipher.Open(nil, nil, data[1:], aad[:])
	if err != nil {
		return nil, errUnreadableAccountSession
	}
	return plain, nil
}

func (s *AccountSessionStorage) StoreSession(ctx context.Context, data []byte) error {
	if len(data) == 0 {
		return fmt.Errorf("%w: cannot store an empty Telegram session", domain.ErrValidation)
	}
	aad := s.associatedData()
	encrypted := s.cipher.Seal([]byte{accountSessionVersion}, nil, data, aad[:])
	if err := s.accounts.UpdateSession(ctx, s.companyID, s.accountID, encrypted); err != nil {
		return fmt.Errorf("store account session: %w", err)
	}
	return nil
}

func (s *AccountSessionStorage) associatedData() [17]byte {
	var aad [17]byte
	aad[0] = accountSessionVersion
	binary.BigEndian.PutUint64(aad[1:9], uint64(s.companyID))
	binary.BigEndian.PutUint64(aad[9:], uint64(s.accountID))
	return aad
}
