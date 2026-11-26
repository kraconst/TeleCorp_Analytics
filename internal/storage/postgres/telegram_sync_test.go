package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
	"github.com/kraconst/GO_server/internal/domain"
	"github.com/kraconst/GO_server/internal/storage/postgres"
	"github.com/kraconst/GO_server/internal/usecase"
)

func TestTelegramSyncStorageImportAndRetry(t *testing.T) {
	ctx, db := newTelegramSyncTestDB(t)
	repo := postgres.NewTelegramSyncStorage(db)
	accounts, err := repo.ListSyncAccounts(ctx)
	if err != nil || len(accounts) != 3 || accounts[0] != (usecase.SyncAccountRef{CompanyID: 1, AccountID: 10}) ||
		accounts[1] != (usecase.SyncAccountRef{CompanyID: 1, AccountID: 14}) ||
		accounts[2] != (usecase.SyncAccountRef{CompanyID: 2, AccountID: 20}) {
		t.Fatalf("eligible accounts: %v, %v", accounts, err)
	}
	if _, err := repo.SyncAccountStatus(ctx, 1, 20); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("foreign account status: %v", err)
	}
	if _, err := repo.DialogCheckpoint(ctx, 1, 20, 100); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("foreign account checkpoint: %v", err)
	}
	if checkpoint, err := repo.DialogCheckpoint(ctx, 1, 10, 100); err != nil || checkpoint != 0 {
		t.Fatalf("empty dialog checkpoint: %d, %v", checkpoint, err)
	}

	dialog := usecase.TelegramDialog{TelegramUserID: 100, Name: "Contact", UnreadCount: 5}
	first := time.Date(2026, 11, 26, 20, 50, 0, 0, time.UTC)
	firstWaiting := first.Add(15 * time.Minute)
	latest := first.Add(16 * time.Minute)
	messages := []domain.Message{
		{TelegramMessageID: 4, Direction: domain.MessageDirectionIn, SentAt: latest},
		{TelegramMessageID: 1, Direction: domain.MessageDirectionIn, SentAt: first},
		{TelegramMessageID: 2, Direction: domain.MessageDirectionOut, SentAt: first.Add(10 * time.Minute)},
		{TelegramMessageID: 3, Direction: domain.MessageDirectionIn, SentAt: firstWaiting},
	}
	created, err := repo.ImportDialog(ctx, 1, 10, dialog, messages)
	if err != nil || created.ChatID == 0 || created.NewMessages != 4 {
		t.Fatalf("first import: %+v, %v", created, err)
	}
	if checkpoint, err := repo.DialogCheckpoint(ctx, 1, 10, 100); err != nil || checkpoint != 4 {
		t.Fatalf("dialog checkpoint: %d, %v", checkpoint, err)
	}
	assertTelegramChat := func(wantUnread int, wantLatest time.Time, wantWaiting *time.Time, wantStatus domain.ChatStatus, wantSource domain.StatusSource) {
		t.Helper()
		var chat domain.Chat
		if err := db.GetContext(ctx, &chat, `SELECT unread_count, last_message_at, awaiting_reply_since, status, status_source FROM chats WHERE id = $1`, created.ChatID); err != nil {
			t.Fatal(err)
		}
		if chat.UnreadCount != wantUnread || !chat.LastMessageAt.Equal(wantLatest) || chat.Status != wantStatus || chat.StatusSource != wantSource ||
			(wantWaiting == nil && chat.AwaitingReplySince != nil) ||
			(wantWaiting != nil && (chat.AwaitingReplySince == nil || !chat.AwaitingReplySince.Equal(*wantWaiting))) {
			t.Fatalf("unexpected chat state: %+v", chat)
		}
	}
	assertTelegramChat(5, latest, &firstWaiting, domain.ChatStatusNewLead, domain.StatusSourceAuto)
	if _, err := db.ExecContext(ctx, `UPDATE chats SET status = 'lost', status_source = 'manual' WHERE id = $1`, created.ChatID); err != nil {
		t.Fatal(err)
	}
	dialog.Name = "Renamed"
	dialog.UnreadCount = 1
	retry, err := repo.ImportDialog(ctx, 1, 10, dialog, messages)
	if err != nil || retry.ChatID != created.ChatID || retry.NewMessages != 0 {
		t.Fatalf("retry import: %+v, %v", retry, err)
	}
	assertTelegramChat(1, latest, &firstWaiting, domain.ChatStatusLost, domain.StatusSourceManual)
	var name string
	if err := db.GetContext(ctx, &name, `SELECT name FROM contacts WHERE company_id = 1 AND telegram_user_id = 100`); err != nil || name != dialog.Name {
		t.Fatalf("updated contact name: %q, %v", name, err)
	}
	var stats []domain.AccountDailyStats
	if err := db.SelectContext(ctx, &stats, `SELECT day, messages_in, messages_out, new_contacts FROM account_daily_stats WHERE account_id = 10 ORDER BY day`); err != nil {
		t.Fatal(err)
	}
	if len(stats) != 2 || stats[0].Day.Format(time.DateOnly) != "2026-11-26" || stats[0].MessagesIn != 1 || stats[0].MessagesOut != 0 || stats[0].NewContacts != 1 ||
		stats[1].Day.Format(time.DateOnly) != "2026-11-27" || stats[1].MessagesIn != 2 || stats[1].MessagesOut != 1 || stats[1].NewContacts != 0 {
		t.Fatalf("company-local daily counters after retry: %+v", stats)
	}
	answeredAt := latest.Add(time.Minute)
	if _, err := repo.ImportDialog(ctx, 1, 10, dialog, []domain.Message{{TelegramMessageID: 5, Direction: domain.MessageDirectionOut, SentAt: answeredAt}}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ImportDialog(ctx, 1, 10, dialog, []domain.Message{{TelegramMessageID: 6, Direction: domain.MessageDirectionIn, SentAt: first.Add(-time.Minute)}}); err != nil {
		t.Fatal(err)
	}
	assertTelegramChat(1, answeredAt, nil, domain.ChatStatusLost, domain.StatusSourceManual)
	var activity time.Time
	if err := db.GetContext(ctx, &activity, `SELECT last_activity_at FROM accounts WHERE id = 10`); err != nil || !activity.Equal(answeredAt) {
		t.Fatalf("account activity after older import: %v, %v", activity, err)
	}
	otherChat, err := repo.ImportDialog(ctx, 1, 14, dialog, nil)
	if err != nil || otherChat.ChatID == 0 || otherChat.ChatID == created.ChatID || otherChat.NewMessages != 0 {
		t.Fatalf("existing contact on second account: %+v, %v", otherChat, err)
	}
	var newContacts int
	if err := db.GetContext(ctx, &newContacts, `SELECT COALESCE(SUM(new_contacts), 0) FROM account_daily_stats`); err != nil || newContacts != 1 {
		t.Fatalf("same-company contact counted %d times: %v", newContacts, err)
	}
}

func TestTelegramSyncStorageRollsBackDialogOnMessageError(t *testing.T) {
	ctx, db := newTelegramSyncTestDB(t)
	repo := postgres.NewTelegramSyncStorage(db)
	dialog := usecase.TelegramDialog{TelegramUserID: 100, Name: "Contact"}
	if _, err := repo.ImportDialog(ctx, 2, 10, dialog, nil); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("foreign account import: %v", err)
	}
	if _, err := repo.ImportDialog(ctx, 1, 13, dialog, nil); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("inactive account import: %v", err)
	}
	sent := time.Now().UTC()
	messages := []domain.Message{
		{TelegramMessageID: 1, Direction: domain.MessageDirectionIn, SentAt: sent},
		{TelegramMessageID: 2, Direction: "invalid", SentAt: sent},
	}
	if result, err := repo.ImportDialog(ctx, 1, 10, dialog, messages); err == nil || result != (usecase.SyncDialogResult{}) {
		t.Fatalf("failed dialog reported progress: %+v, %v", result, err)
	}
	const countPartial = `
		SELECT (SELECT COUNT(*) FROM contacts) + (SELECT COUNT(*) FROM chats) +
			(SELECT COUNT(*) FROM messages) + (SELECT COUNT(*) FROM account_daily_stats) +
			(SELECT COUNT(*) FROM accounts WHERE last_activity_at IS NOT NULL)
	`
	var partial int
	if err := db.GetContext(ctx, &partial, countPartial); err != nil || partial != 0 {
		t.Fatalf("partial import survived rollback: %d rows, %v", partial, err)
	}
}

func newTelegramSyncTestDB(t *testing.T) (context.Context, *sqlx.DB) {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set TEST_POSTGRES_DSN to run PostgreSQL integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	admin, err := sqlx.ConnectContext(ctx, "pgx", dsn)
	if err != nil {
		t.Fatal("could not connect to test PostgreSQL")
	}
	t.Cleanup(func() { admin.Close() })
	schema := fmt.Sprintf("telegram_sync_test_%d", time.Now().UnixNano())
	if _, err := admin.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatalf("create test schema: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if _, err := admin.ExecContext(cleanupCtx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Errorf("remove test schema: %v", err)
		}
	})
	config, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal("could not parse test PostgreSQL connection settings")
	}
	config.RuntimeParams["search_path"] = schema
	db := sqlx.NewDb(stdlib.OpenDB(*config), "pgx")
	t.Cleanup(func() { db.Close() })
	for _, migration := range []string{"000001_init_schema.up.sql", "000002_create_messages.up.sql", "000003_create_contacts_and_chats.up.sql", "000004_create_account_daily_stats.up.sql"} {
		content, err := os.ReadFile(filepath.Join("..", "..", "..", "migrations", migration))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, string(content)); err != nil {
			t.Fatalf("apply %s: %v", migration, err)
		}
	}
	const fixtures = `
		INSERT INTO companies (id, name, timezone) VALUES (1, 'First', 'Europe/Moscow'), (2, 'Second', 'UTC');
		INSERT INTO employees (id, company_id, name) VALUES (1, 1, 'First employee'), (2, 2, 'Second employee');
		INSERT INTO accounts (id, company_id, employee_id, phone, status, session) VALUES
			(10, 1, 1, '+10000000010', 'active', decode('01', 'hex')),
			(11, 1, 1, '+10000000011', 'active', NULL),
			(12, 1, 1, '+10000000012', 'active', decode('', 'hex')),
			(13, 1, 1, '+10000000013', 'disconnected', decode('01', 'hex')),
			(14, 1, 1, '+10000000014', 'active', decode('01', 'hex')),
			(20, 2, 2, '+10000000020', 'active', decode('01', 'hex'));
	`
	if _, err := db.ExecContext(ctx, fixtures); err != nil {
		t.Fatalf("seed test schema: %v", err)
	}
	return ctx, db
}
