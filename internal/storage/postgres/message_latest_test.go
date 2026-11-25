package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
	"github.com/kraconst/GO_server/internal/domain"
	"github.com/kraconst/GO_server/internal/storage/postgres"
)

// TestListLatestByChatValidation checks that invalid arguments are rejected
// before the database is touched (the storage has no DB handle here).
func TestListLatestByChatValidation(t *testing.T) {
	repo := postgres.NewMessageStorage(nil)
	cases := []struct {
		name      string
		companyID int64
		chatID    int64
		limit     int
	}{
		{"zero limit", 1, 1, 0},
		{"negative limit", 1, 1, -5},
		{"zero company", 0, 1, 10},
		{"negative company", -1, 1, 10},
		{"zero chat", 1, 0, 10},
		{"negative chat", 1, -1, 10},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			msgs, err := repo.ListLatestByChat(context.Background(), tc.companyID, tc.chatID, tc.limit)
			if !errors.Is(err, domain.ErrValidation) {
				t.Fatalf("expected ErrValidation, got %v", err)
			}
			if msgs != nil {
				t.Fatalf("expected nil slice on validation error, got %+v", msgs)
			}
		})
	}
}

// Set TEST_POSTGRES_DSN to run this check against an actual PostgreSQL server.
// All tables live in a unique schema which is removed at the end of the test.
func TestListLatestByChatPostgres(t *testing.T) {
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
	schema := fmt.Sprintf("message_latest_test_%d", time.Now().UnixNano())
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

	latestMsgApplyMigrations(ctx, t, db)

	insertID := func(query string, args ...any) int64 {
		t.Helper()
		var id int64
		if err := db.GetContext(ctx, &id, query, args...); err != nil {
			t.Fatalf("fixture %q: %v", query, err)
		}
		return id
	}
	companyA := insertID("INSERT INTO companies (name) VALUES ('Latest A') RETURNING id")
	companyB := insertID("INSERT INTO companies (name) VALUES ('Latest B') RETURNING id")
	employeeA := insertID("INSERT INTO employees (company_id, name) VALUES ($1, 'Employee A') RETURNING id", companyA)
	employeeB := insertID("INSERT INTO employees (company_id, name) VALUES ($1, 'Employee B') RETURNING id", companyB)
	accountA := insertID("INSERT INTO accounts (company_id, employee_id, phone, status) VALUES ($1, $2, '+70000000001', 'active') RETURNING id", companyA, employeeA)
	accountB := insertID("INSERT INTO accounts (company_id, employee_id, phone, status) VALUES ($1, $2, '+70000000002', 'active') RETURNING id", companyB, employeeB)
	contactA1 := insertID("INSERT INTO contacts (company_id, telegram_user_id, name) VALUES ($1, 501, 'Contact A1') RETURNING id", companyA)
	contactA2 := insertID("INSERT INTO contacts (company_id, telegram_user_id, name) VALUES ($1, 502, 'Contact A2') RETURNING id", companyA)
	contactB := insertID("INSERT INTO contacts (company_id, telegram_user_id, name) VALUES ($1, 601, 'Contact B') RETURNING id", companyB)
	chatA := insertID("INSERT INTO chats (account_id, contact_id) VALUES ($1, $2) RETURNING id", accountA, contactA1)
	emptyChatA := insertID("INSERT INTO chats (account_id, contact_id) VALUES ($1, $2) RETURNING id", accountA, contactA2)
	chatB := insertID("INSERT INTO chats (account_id, contact_id) VALUES ($1, $2) RETURNING id", accountB, contactB)

	base := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	insertMsg := func(chatID, tgID int64, direction domain.MessageDirection, text string, sentAt time.Time) int64 {
		t.Helper()
		return insertID(
			"INSERT INTO messages (chat_id, telegram_message_id, direction, text, sent_at) VALUES ($1, $2, $3, $4, $5) RETURNING id",
			chatID, tgID, direction, text, sentAt,
		)
	}
	// The newest message is inserted first, so it has the smallest id: ordering
	// must be driven by sent_at, with id used only as a tie-break.
	newest := insertMsg(chatA, 100, domain.MessageDirectionIn, "newest", base.Add(5*time.Minute))
	m1 := insertMsg(chatA, 101, domain.MessageDirectionIn, "m1", base.Add(1*time.Minute))
	m2 := insertMsg(chatA, 102, domain.MessageDirectionOut, "m2", base.Add(2*time.Minute))
	m3 := insertMsg(chatA, 103, domain.MessageDirectionIn, "m3", base.Add(3*time.Minute))
	// Two messages with the same sent_at; tieHigh has the larger id.
	tieLow := insertMsg(chatA, 104, domain.MessageDirectionOut, "tie-low", base.Add(4*time.Minute))
	tieHigh := insertMsg(chatA, 105, domain.MessageDirectionIn, "tie-high", base.Add(4*time.Minute))
	if tieHigh <= tieLow {
		t.Fatalf("fixture precondition: expected increasing ids, got %d <= %d", tieHigh, tieLow)
	}
	insertMsg(chatB, 200, domain.MessageDirectionIn, "other company", base.Add(10*time.Minute))

	repo := postgres.NewMessageStorage(db)

	assertIDs := func(label string, got []domain.Message, want ...int64) {
		t.Helper()
		if got == nil {
			t.Fatalf("%s: expected non-nil slice", label)
		}
		if len(got) != len(want) {
			t.Fatalf("%s: expected %d messages, got %d: %+v", label, len(want), len(got), got)
		}
		for i := range want {
			if got[i].ID != want[i] {
				t.Fatalf("%s: position %d: expected id %d, got %d (%+v)", label, i, want[i], got[i].ID, got)
			}
			if got[i].ChatID != chatA {
				t.Fatalf("%s: message from foreign chat: %+v", label, got[i])
			}
		}
		latestMsgAssertChronological(t, label, got)
	}

	// limit=3: latest by (sent_at DESC, id DESC) are newest, tieHigh, tieLow; returned ASC.
	got, err := repo.ListLatestByChat(ctx, companyA, chatA, 3)
	if err != nil {
		t.Fatalf("list latest (limit 3): %v", err)
	}
	assertIDs("limit 3", got, tieLow, tieHigh, newest)
	if got[len(got)-1].Text != "newest" || !got[len(got)-1].SentAt.Equal(base.Add(5*time.Minute)) {
		t.Fatalf("last element must be the newest message, got %+v", got[len(got)-1])
	}
	if got[0].Direction != domain.MessageDirectionOut || got[0].TelegramMessageID != 104 || got[0].MediaType != nil {
		t.Fatalf("unexpected column mapping: %+v", got[0])
	}

	// limit=2 cuts between two messages with equal sent_at: the id tie-break must keep tieHigh.
	got, err = repo.ListLatestByChat(ctx, companyA, chatA, 2)
	if err != nil {
		t.Fatalf("list latest (limit 2): %v", err)
	}
	assertIDs("limit 2 tie-break", got, tieHigh, newest)

	// limit larger than the chat history returns everything in ASC order.
	got, err = repo.ListLatestByChat(ctx, companyA, chatA, 100)
	if err != nil {
		t.Fatalf("list latest (limit 100): %v", err)
	}
	assertIDs("limit 100", got, m1, m2, m3, tieLow, tieHigh, newest)

	// A chat of company A requested in the scope of company B is invisible.
	got, err = repo.ListLatestByChat(ctx, companyB, chatA, 10)
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("cross-company chat must yield empty non-nil slice, got %+v, %v", got, err)
	}
	// And vice versa: company B's chat in company A's scope.
	got, err = repo.ListLatestByChat(ctx, companyA, chatB, 10)
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("foreign chat must yield empty non-nil slice, got %+v, %v", got, err)
	}
	// A chat without messages.
	got, err = repo.ListLatestByChat(ctx, companyA, emptyChatA, 10)
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("empty chat must yield empty non-nil slice, got %+v, %v", got, err)
	}
	// A missing chat.
	got, err = repo.ListLatestByChat(ctx, companyA, 999999, 10)
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("missing chat must yield empty non-nil slice, got %+v, %v", got, err)
	}
	// limit=0 is rejected with a validation error.
	if _, err := repo.ListLatestByChat(ctx, companyA, chatA, 0); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("limit 0: expected ErrValidation, got %v", err)
	}
}

// latestMsgApplyMigrations applies every *.up.sql migration in lexical order.
func latestMsgApplyMigrations(ctx context.Context, t *testing.T, db *sqlx.DB) {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("..", "..", "..", "migrations", "*.up.sql"))
	if err != nil {
		t.Fatalf("list migrations: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no up migrations found")
	}
	sort.Strings(files)
	for _, file := range files {
		sqlText, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read migration %s: %v", file, err)
		}
		if _, err := db.ExecContext(ctx, string(sqlText)); err != nil {
			t.Fatalf("apply %s: %v", filepath.Base(file), err)
		}
	}
}

// latestMsgAssertChronological checks the (sent_at ASC, id ASC) ordering.
func latestMsgAssertChronological(t *testing.T, label string, msgs []domain.Message) {
	t.Helper()
	for i := 1; i < len(msgs); i++ {
		prev, cur := msgs[i-1], msgs[i]
		if cur.SentAt.Before(prev.SentAt) || (cur.SentAt.Equal(prev.SentAt) && cur.ID <= prev.ID) {
			t.Fatalf("%s: messages are not in (sent_at, id) ASC order at %d: %+v", label, i, msgs)
		}
	}
}
