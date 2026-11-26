package telegram

import (
	"testing"

	"github.com/kraconst/GO_server/internal/domain"
)

func TestSessionEnrollmentRequiresMatchingPhoneAndKnownID(t *testing.T) {
	for _, tc := range []struct {
		accountPhone string
		knownID      int64
		telegramID   int64
		phone        string
		want         bool
	}{
		{"+1 (234) 567-8901", 0, 5, "12345678901", true},
		{"+12345678901", 5, 5, "12345678901", true},
		{"+12345678901", 6, 5, "12345678901", false},
		{"+12345678900", 5, 5, "12345678901", false},
		{"+12345678901", 0, 0, "12345678901", false},
		{"+1234ext5678901", 0, 5, "12345678901", false},
		{"", 0, 5, "", false},
	} {
		account := domain.Account{Phone: tc.accountPhone, TelegramUserID: tc.knownID}
		if got := matchesEnrollmentIdentity(account, tc.telegramID, tc.phone); got != tc.want {
			t.Errorf("unexpected identity match for test case %+v: %v", tc, got)
		}
	}
}
