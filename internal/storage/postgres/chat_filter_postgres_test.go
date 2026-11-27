package postgres_test

import (
	"context"
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
)

func TestChatStorage_ListByFilter_Postgres(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set TEST_POSTGRES_DSN to run PostgreSQL integration tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	admin, err := sqlx.ConnectContext(ctx, "pgx", dsn)
	if err != nil {
		t.Fatalf("connect to test PostgreSQL: %v", err)
	}
	defer admin.Close()

	schema := fmt.Sprintf("chat_filter_test_%d", time.Now().UnixNano())
	if _, err := admin.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatalf("create test schema: %v", err)
	}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		if _, err := admin.ExecContext(cleanupCtx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Errorf("clean test schema: %v", err)
		}
	}()

	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse test DSN: %v", err)
	}
	cfg.RuntimeParams["search_path"] = schema
	db := sqlx.NewDb(stdlib.OpenDB(*cfg), "pgx")
	defer db.Close()

	apply := func(name string) {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join("..", "..", "..", "migrations", name))
		if err != nil {
			t.Fatalf("read migration %s: %v", name, err)
		}
		if _, err := db.ExecContext(ctx, string(raw)); err != nil {
			t.Fatalf("apply migration %s: %v", name, err)
		}
	}

	apply("000001_init_schema.up.sql")
	apply("000002_create_messages.up.sql")
	apply("000003_create_contacts_and_chats.up.sql")
	apply("000004_create_account_daily_stats.up.sql")
	apply("000005_create_alerts.up.sql")
	apply("000006_create_chat_summaries_and_fks.up.sql")
	apply("000008_account_constraints_and_assignments.up.sql")
	apply("000010_chat_search_trgm_indexes.up.sql")
	// Verify idempotency of migration 000010.
	apply("000010_chat_search_trgm_indexes.up.sql")

	now := time.Date(2026, 11, 28, 12, 0, 0, 0, time.UTC)
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, query, args...); err != nil {
			t.Fatalf("seed test data: %v", err)
		}
	}
	exec(`INSERT INTO companies (id, name) VALUES (1, 'Acme'), (2, 'Other')`)
	exec(`INSERT INTO employees (id, company_id, name, position, is_active) VALUES
		(1, 1, 'Alice', 'manager', true),
		(2, 1, 'Bob', 'manager', true),
		(3, 2, 'Eve', 'manager', true)`)
	exec(`INSERT INTO accounts (id, company_id, employee_id, phone, username, telegram_user_id, status) VALUES
		(1, 1, 1, '+1001', 'acc_alice', 1001, 'active'),
		(2, 1, 2, '+1002', 'acc_bob', 1002, 'active'),
		(3, 2, 3, '+2001', 'acc_eve', 2001, 'active')`)
	exec(`INSERT INTO contacts (id, company_id, telegram_user_id, name, username) VALUES
		(1, 1, 5001, 'Ivan Petrov', 'ivan_p'),
		(2, 1, 5002, 'Maria Sidorova', 'maria_vip'),
		(3, 2, 5003, 'Ivan Secret', 'ivan_other')`)
	exec(`INSERT INTO chats (id, account_id, contact_id, status, status_source, priority, last_message_at, awaiting_reply_since, unread_count) VALUES
		(1, 1, 1, 'new_lead', 'auto', 'high', $1, $2, 2),
		(2, 2, 2, 'negotiation', 'manual', 'normal', $3, NULL, 0),
		(3, 3, 3, 'new_lead', 'auto', 'high', $4, $5, 1)`,
		now.Add(-2*time.Hour), now.Add(-3*time.Hour),
		now.Add(-1*time.Hour),
		now, now.Add(-10*time.Minute),
	)
	exec(`INSERT INTO messages (chat_id, telegram_message_id, direction, text, sent_at) VALUES
		(1, 10, 'in', 'Discount 50%_special offer?', $1),
		(2, 20, 'out', 'Sent invoice PDF', $2),
		(3, 30, 'in', 'Discount 50%_special other company', $3)`,
		now.Add(-2*time.Hour), now.Add(-1*time.Hour), now,
	)

	storage := postgres.NewChatStorage(db)

	t.Run("company isolation and default sort", func(t *testing.T) {
		chats, err := storage.ListByFilter(ctx, domain.ChatFilter{CompanyID: 1, Limit: 50})
		if err != nil {
			t.Fatalf("ListByFilter: %v", err)
		}
		if len(chats) != 2 || chats[0].ID != 2 || chats[1].ID != 1 {
			t.Fatalf("expected [2, 1], got %+v", chats)
		}
	})

	t.Run("search q escapes literal percent and underscore across message text", func(t *testing.T) {
		chats, err := storage.ListByFilter(ctx, domain.ChatFilter{
			CompanyID: 1,
			Query:     "50%_SPECIAL",
			Limit:     50,
		})
		if err != nil {
			t.Fatalf("ListByFilter: %v", err)
		}
		if len(chats) != 1 || chats[0].ID != 1 {
			t.Fatalf("expected only chat 1, got %+v", chats)
		}

		// Ensure % is NOT treated as wildcard matching "invoice".
		noMatch, err := storage.ListByFilter(ctx, domain.ChatFilter{
			CompanyID: 1,
			Query:     "%",
			Limit:     50,
		})
		if err != nil {
			t.Fatalf("ListByFilter literal %%: %v", err)
		}
		if len(noMatch) != 1 || noMatch[0].ID != 1 {
			t.Fatalf("expected only chat 1 containing literal %%, got %+v", noMatch)
		}
	})

	t.Run("unanswered, priority, account_id and awaiting_reply sort", func(t *testing.T) {
		accID := int64(1)
		prio := domain.ChatPriorityHigh
		chats, err := storage.ListByFilter(ctx, domain.ChatFilter{
			CompanyID:      1,
			AccountID:      &accID,
			Priority:       &prio,
			UnansweredOnly: true,
			SortBy:         domain.ChatSortAwaitingReply,
			Limit:          10,
			Offset:         0,
		})
		if err != nil {
			t.Fatalf("ListByFilter: %v", err)
		}
		if len(chats) != 1 || chats[0].ID != 1 {
			t.Fatalf("expected [1], got %+v", chats)
		}
	})

	t.Run("foreign account_id returns empty non-nil slice", func(t *testing.T) {
		foreignAcc := int64(3)
		chats, err := storage.ListByFilter(ctx, domain.ChatFilter{
			CompanyID: 1,
			AccountID: &foreignAcc,
			Limit:     10,
		})
		if err != nil {
			t.Fatalf("ListByFilter: %v", err)
		}
		if chats == nil || len(chats) != 0 {
			t.Fatalf("expected non-nil empty slice, got %#v", chats)
		}
	})

	// Verify down migration cleans up cleanly.
	apply("000010_chat_search_trgm_indexes.down.sql")
}
