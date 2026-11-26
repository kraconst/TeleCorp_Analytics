package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/kraconst/GO_server/internal/usecase"
)

func TestTelegramImportValidatesBeforeOpeningDatabase(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"unknown"}, "usage:"},
		{[]string{"import-telegram-session", "0", "10", "example.session"}, "IDs must be positive"},
	} {
		if err := run(t.Context(), tc.args); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("unexpected validation result: %v", err)
		}
	}
	t.Setenv("TELEGRAM_API_ID", "")
	if err := run(t.Context(), []string{"import-telegram-session", "1", "10", "example.session"}); !errors.Is(err, usecase.ErrTelegramUnavailable) {
		t.Fatalf("unexpected error without Telegram configuration: %v", err)
	}
}
