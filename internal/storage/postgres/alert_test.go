package postgres_test

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
	"github.com/kraconst/GO_server/internal/domain"
	"github.com/kraconst/GO_server/internal/storage/postgres"
	"github.com/kraconst/GO_server/internal/usecase"
)

// Set TEST_POSTGRES_DSN to run these checks against an actual PostgreSQL server.
// All tables live in a unique schema which is removed at the end of the test.
func TestAlertPostgresLifecycle(t *testing.T) {
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
	schema := fmt.Sprintf("alert_test_%d", time.Now().UnixNano())
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
		sql, err := os.ReadFile(filepath.Join("..", "..", "..", "migrations", name))
		if err != nil {
			t.Fatalf("read migration: %v", err)
		}
		if _, err := db.ExecContext(ctx, string(sql)); err != nil {
			t.Fatalf("apply %s: %v", name, err)
		}
	}
	apply("000001_init_schema.up.sql")
	apply("000005_create_alerts.up.sql")
	// Reapplying the up migration must also be safe for initialized databases.
	apply("000005_create_alerts.up.sql")
	if _, err := db.ExecContext(ctx, "INSERT INTO companies (id, name) VALUES (1, 'First'), (2, 'Second')"); err != nil {
		t.Fatal(err)
	}
	repo := postgres.NewAlertStorage(db)
	u := usecase.NewAlertUsecase(repo, nil, nil)
	createdAt := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	create := func(companyID int64, kind domain.AlertType) int64 {
		t.Helper()
		id, err := u.Create(ctx, domain.Alert{CompanyID: companyID, ChatID: 10, Type: kind, CreatedAt: createdAt})
		if err != nil {
			t.Fatalf("create alert: %v", err)
		}
		return id
	}
	first := create(1, domain.AlertTypeNoReply)
	second := create(1, domain.AlertTypeComplaint)
	otherCompany := create(2, domain.AlertTypeUrgent)
	alerts, err := u.ListActiveByCompany(ctx, 1)
	if err != nil || len(alerts) != 2 {
		t.Fatalf("list active alerts: %+v, %v", alerts, err)
	}
	if alerts[0].ID != second || alerts[1].ID != first || !alerts[0].CreatedAt.Equal(createdAt) || alerts[0].ResolvedAt != nil {
		t.Fatalf("unexpected mapping/order: %+v", alerts)
	}
	if err := u.Resolve(ctx, 1, otherCompany); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("cross-company resolution must be denied, got %v", err)
	}
	if err := u.Resolve(ctx, 1, 999999); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected not found for missing alert, got %v", err)
	}
	if err := u.Resolve(ctx, 1, first); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	var firstResolution time.Time
	if err := db.GetContext(ctx, &firstResolution, "SELECT resolved_at FROM alerts WHERE id = $1", first); err != nil {
		t.Fatal(err)
	}
	if err := u.Resolve(ctx, 1, first); err != nil {
		t.Fatalf("repeat resolve: %v", err)
	}
	var secondResolution time.Time
	if err := db.GetContext(ctx, &secondResolution, "SELECT resolved_at FROM alerts WHERE id = $1", first); err != nil || !firstResolution.Equal(secondResolution) {
		t.Fatalf("repeat resolve changed timestamp: %v -> %v (%v)", firstResolution, secondResolution, err)
	}
	alerts, err = u.ListActiveByCompany(ctx, 1)
	if err != nil || len(alerts) != 1 || alerts[0].ID != second {
		t.Fatalf("resolved alert remains active: %+v, %v", alerts, err)
	}
	if err := u.Resolve(ctx, 1, second); err != nil {
		t.Fatal(err)
	}
	alerts, err = u.ListActiveByCompany(ctx, 1)
	if err != nil || alerts == nil || len(alerts) != 0 {
		t.Fatalf("expected empty active feed: %+v, %v", alerts, err)
	}
	alerts, err = u.ListActiveByCompany(ctx, 2)
	if err != nil || len(alerts) != 1 || alerts[0].ID != otherCompany {
		t.Fatalf("another company's alert was modified: %+v, %v", alerts, err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO alerts (company_id, chat_id, type) VALUES (1, 10, 'unsupported')"); err == nil {
		t.Fatal("database accepted an unsupported alert type")
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO alerts (company_id, chat_id, type) VALUES (1, 0, 'urgent')"); err == nil {
		t.Fatal("database accepted a non-positive chat ID")
	}
	apply("000005_create_alerts.down.sql")
	var remainingTables int
	if err := db.GetContext(ctx, &remainingTables, "SELECT count(*) FROM information_schema.tables WHERE table_schema = $1 AND table_name = 'alerts'", schema); err != nil || remainingTables != 0 {
		t.Fatalf("down migration did not remove alerts: %d, %v", remainingTables, err)
	}
}

func TestAlertStorage_DeduplicationAndNoReplyQueries(t *testing.T) {
	db, mock := setupMockDB(t)
	defer db.Close()
	storage := postgres.NewAlertStorage(db)
	ctx := context.Background()

	t.Run("Create checks WHERE NOT EXISTS for active alert and returns 0 on duplicate", func(t *testing.T) {
		var capturedQuery string
		mock.queryHandler = func(query string, args []driver.Value) (driver.Rows, error) {
			capturedQuery = query
			return &mockRows{cols: []string{"id"}, vals: [][]driver.Value{}}, nil
		}

		id, err := storage.Create(ctx, domain.Alert{
			CompanyID: 1,
			ChatID:    10,
			Type:      domain.AlertTypeNoReply,
			CreatedAt: time.Now().UTC(),
		})
		if err != nil {
			t.Fatalf("expected nil error on duplicate skip, got %v", err)
		}
		if id != 0 {
			t.Fatalf("expected id=0 when active duplicate skipped, got %d", id)
		}

		cleanQ := strings.ToLower(strings.Join(strings.Fields(capturedQuery), " "))
		if !strings.Contains(cleanQ, "where chat_id = $2 and type = $3 and resolved_at is null") {
			t.Errorf("Create query missing active deduplication check: %s", capturedQuery)
		}
	})

	t.Run("Create returns 0 on unique constraint conflict 23505", func(t *testing.T) {
		mock.queryHandler = func(query string, args []driver.Value) (driver.Rows, error) {
			return nil, errors.New("ERROR: duplicate key value violates unique constraint (SQLSTATE 23505)")
		}

		id, err := storage.Create(ctx, domain.Alert{
			CompanyID: 1,
			ChatID:    10,
			Type:      domain.AlertTypeComplaint,
			CreatedAt: time.Now().UTC(),
		})
		if err != nil || id != 0 {
			t.Fatalf("expected (0, nil) on concurrent 23505 conflict, got (%d, %v)", id, err)
		}
	})

	t.Run("ExistsActive queries unresolved alert by company_id, chat_id and type", func(t *testing.T) {
		var capturedQuery string
		var capturedArgs []driver.Value
		mock.queryHandler = func(query string, args []driver.Value) (driver.Rows, error) {
			capturedQuery = query
			capturedArgs = args
			return &mockRows{cols: []string{"exists"}, vals: [][]driver.Value{{true}}}, nil
		}

		exists, err := storage.ExistsActive(ctx, 1, 15, domain.AlertTypeUrgent)
		if err != nil || !exists {
			t.Fatalf("expected exists=true, err=nil; got %v, %v", exists, err)
		}
		cleanQ := strings.ToLower(strings.Join(strings.Fields(capturedQuery), " "))
		if !strings.Contains(cleanQ, "where company_id = $1 and chat_id = $2 and type = $3 and resolved_at is null") {
			t.Errorf("ExistsActive query mismatch: %s", capturedQuery)
		}
		if len(capturedArgs) != 3 || capturedArgs[0] != int64(1) || capturedArgs[1] != int64(15) || capturedArgs[2] != string(domain.AlertTypeUrgent) {
			t.Errorf("unexpected ExistsActive args: %#v", capturedArgs)
		}
	})

	t.Run("FindExpiredNoReplyChats joins companies and excludes active no_reply alerts", func(t *testing.T) {
		var capturedQuery string
		var capturedArgs []driver.Value
		mock.queryHandler = func(query string, args []driver.Value) (driver.Rows, error) {
			capturedQuery = query
			capturedArgs = args
			return &mockRows{
				cols: []string{"company_id", "chat_id"},
				vals: [][]driver.Value{{int64(2), int64(44)}},
			}, nil
		}

		chats, err := storage.FindExpiredNoReplyChats(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(chats) != 1 || chats[0].CompanyID != 2 || chats[0].ChatID != 44 {
			t.Fatalf("unexpected result: %+v", chats)
		}
		cleanQ := strings.ToLower(strings.Join(strings.Fields(capturedQuery), " "))
		if !strings.Contains(cleanQ, "c.awaiting_reply_since is not null") ||
			!strings.Contains(cleanQ, "(comp.no_reply_threshold / 1000000000.0) * interval '1 second'") ||
			!strings.Contains(cleanQ, "al.chat_id = c.id and al.type = $1 and al.resolved_at is null") {
			t.Errorf("FindExpiredNoReplyChats query mismatch: %s", capturedQuery)
		}
		if len(capturedArgs) != 1 || capturedArgs[0] != string(domain.AlertTypeNoReply) {
			t.Errorf("unexpected args: %#v", capturedArgs)
		}
	})
}
