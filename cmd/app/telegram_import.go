package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/kraconst/GO_server/internal/config"
	"github.com/kraconst/GO_server/internal/storage/postgres"
	"github.com/kraconst/GO_server/internal/telegram"
)

const maxSessionFileSize = 1 << 20

type telegramImportArgs struct {
	companyID int64
	accountID int64
	path      string
}

func parseTelegramImportArgs(args []string) (telegramImportArgs, error) {
	if len(args) != 4 || args[0] != "import-telegram-session" {
		return telegramImportArgs{}, fmt.Errorf("usage: app import-telegram-session <company-id> <account-id> <session-file>")
	}
	companyID, companyErr := strconv.ParseInt(args[1], 10, 64)
	accountID, accountErr := strconv.ParseInt(args[2], 10, 64)
	if companyErr != nil || accountErr != nil || companyID <= 0 || accountID <= 0 {
		return telegramImportArgs{}, fmt.Errorf("company and account IDs must be positive")
	}
	return telegramImportArgs{companyID: companyID, accountID: accountID, path: args[3]}, nil
}

func runTelegramImport(ctx context.Context, cfg config.TelegramConfig, accounts *postgres.AccountStorage, args telegramImportArgs) error {
	client, err := telegram.NewSyncClient(cfg, accounts)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	account, err := accounts.LoadTelegramEnrollment(ctx, args.companyID, args.accountID)
	if err != nil {
		return err
	}
	info, err := os.Stat(args.path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxSessionFileSize {
		return fmt.Errorf("Telegram session must be a regular file of at most 1 MiB")
	}
	file, err := os.Open(args.path)
	if err != nil {
		return fmt.Errorf("open Telegram session file failed")
	}
	defer file.Close()
	info, err = file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxSessionFileSize {
		return fmt.Errorf("Telegram session must be a regular file of at most 1 MiB")
	}
	raw, err := io.ReadAll(io.LimitReader(file, maxSessionFileSize+1))
	if err != nil || len(raw) == 0 || len(raw) > maxSessionFileSize {
		return fmt.Errorf("read Telegram session file failed or file size is invalid")
	}
	defer clear(raw)
	userID, sealed, err := client.PrepareEnrollment(ctx, account, raw)
	if err != nil {
		return err
	}
	if err := accounts.ActivateTelegramSession(ctx, account, userID, sealed); err != nil {
		return err
	}
	log.Printf("Telegram session imported for company %d account %d", args.companyID, args.accountID)
	return nil
}
