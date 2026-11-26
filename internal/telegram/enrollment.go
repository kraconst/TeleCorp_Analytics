package telegram

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gotd/td/tg"
	"github.com/kraconst/GO_server/internal/domain"
)

// PrepareEnrollment verifies a previously authorized session and returns the
// scoped ciphertext for the caller to persist together with the Telegram ID.
func (s *SyncClient) PrepareEnrollment(ctx context.Context, account domain.Account, raw []byte) (int64, []byte, error) {
	storage, err := NewAccountSessionStorage(s.accounts, account.CompanyID, account.ID, s.key)
	if err != nil {
		return 0, nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	var userID int64
	data, err := s.runSavedSession(ctx, raw, func(_ context.Context, self *tg.User, _ *tg.Client) error {
		if !matchesEnrollmentIdentity(account, self.ID, self.Phone) {
			return fmt.Errorf("%w: Telegram profile does not match the account", domain.ErrForbidden)
		}
		userID = self.ID
		return nil
	})
	if err != nil {
		return 0, nil, err
	}
	sealed, err := storage.seal(data)
	if err != nil {
		return 0, nil, err
	}
	return userID, sealed, nil
}

func matchesEnrollmentIdentity(account domain.Account, userID int64, phone string) bool {
	storedPhone := phoneDigits(account.Phone)
	return userID > 0 && (account.TelegramUserID == 0 || account.TelegramUserID == userID) &&
		storedPhone != "" && storedPhone == phoneDigits(phone)
}

func phoneDigits(phone string) string {
	var out strings.Builder
	for _, c := range strings.TrimPrefix(strings.TrimSpace(phone), "+") {
		switch {
		case c >= '0' && c <= '9':
			out.WriteRune(c)
		case strings.ContainsRune(" -()", c):
		default:
			return ""
		}
	}
	return out.String()
}
