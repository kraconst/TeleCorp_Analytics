package postgres_test

import (
	"bytes"
	"context"
	"database/sql/driver"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/kraconst/GO_server/internal/domain"
	"github.com/kraconst/GO_server/internal/storage/postgres"
)

func TestAccountStorageTelegramSessionScope(t *testing.T) {
	db, mock := setupMockDB(t)
	t.Cleanup(func() { db.Close() })
	previous := mock.queryHandler
	t.Cleanup(func() { mock.queryHandler = previous })
	storage := postgres.NewAccountStorage(db)
	ctx := context.Background()
	const companyID, accountID int64 = 7, 42
	var stored []byte

	mock.queryHandler = func(query string, args []driver.Value) (driver.Rows, error) {
		query = strings.ToLower(strings.Join(strings.Fields(query), " "))
		if !strings.Contains(query, "where company_id = $1 and id = $2") || len(args) < 2 || args[1] != accountID {
			t.Fatalf("session query lacks the account scope: %s, %v", query, args[:min(2, len(args))])
		}
		column := "id"
		if strings.HasPrefix(query, "select session from accounts") {
			column = "session"
		}
		rows := &mockRows{cols: []string{column}}
		if args[0] != companyID {
			return rows, nil
		}
		if column == "session" {
			rows.vals = [][]driver.Value{{stored}}
			return rows, nil
		}
		if !strings.Contains(query, "set session = $3") || len(args) != 3 {
			t.Fatalf("unexpected session update query: %s", query)
		}
		stored = bytes.Clone(args[2].([]byte))
		rows.vals = [][]driver.Value{{accountID}}
		return rows, nil
	}

	data, err := storage.LoadSession(ctx, companyID, accountID)
	if err != nil || len(data) != 0 {
		t.Fatalf("empty account session: len=%d, err=%v", len(data), err)
	}
	want := []byte{0, 1, 255}
	if err := storage.UpdateSession(ctx, companyID, accountID, want); err != nil {
		t.Fatalf("store session: %v", err)
	}
	if err := storage.UpdateSession(ctx, companyID+1, accountID, []byte{2}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("cross-company update: got %v, want ErrNotFound", err)
	}
	if _, err := storage.LoadSession(ctx, companyID+1, accountID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("cross-company load: got %v, want ErrNotFound", err)
	}
	data, err = storage.LoadSession(ctx, companyID, accountID)
	if err != nil || !bytes.Equal(data, want) {
		t.Fatalf("session round trip: equal=%v, err=%v", bytes.Equal(data, want), err)
	}
}

func TestAccountStorageTelegramActivityAndStatus(t *testing.T) {
	db, mock := setupMockDB(t)
	t.Cleanup(func() { db.Close() })
	previous := mock.queryHandler
	t.Cleanup(func() { mock.queryHandler = previous })
	storage := postgres.NewAccountStorage(db)
	ctx := context.Background()
	at := time.Date(2026, 11, 26, 12, 34, 56, 0, time.FixedZone("fixture", 3*60*60))
	const companyID, accountID int64 = 7, 42
	var calls int

	mock.queryHandler = func(query string, args []driver.Value) (driver.Rows, error) {
		calls++
		query = strings.ToLower(strings.Join(strings.Fields(query), " "))
		if !strings.Contains(query, "where company_id = $1 and id = $2") || len(args) != 3 || args[1] != accountID {
			t.Fatalf("account update lacks company/account scope: %s", query)
		}
		rows := &mockRows{cols: []string{"id"}}
		if args[0] != companyID {
			return rows, nil
		}
		if strings.Contains(query, "last_activity_at = greatest(last_activity_at, $3)") {
			if args[2] != at.UTC() {
				t.Fatalf("expected input timestamp normalized to UTC, got %v", args[2])
			}
		} else if !strings.Contains(query, "set status = $3") || args[2] != string(domain.StatusBanned) {
			t.Fatalf("unexpected account update: %s", query)
		}
		rows.vals = [][]driver.Value{{accountID}}
		return rows, nil
	}

	if err := storage.UpdateLastActivity(ctx, companyID, accountID, at); err != nil {
		t.Fatalf("update activity: %v", err)
	}
	if err := storage.UpdateSessionStatus(ctx, companyID, accountID, domain.StatusBanned); err != nil {
		t.Fatalf("update status: %v", err)
	}
	if err := storage.UpdateLastActivity(ctx, companyID+1, accountID, at); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("cross-company activity: got %v, want ErrNotFound", err)
	}
	if err := storage.UpdateSessionStatus(ctx, companyID+1, accountID, domain.StatusDisconnected); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("cross-company status: got %v, want ErrNotFound", err)
	}
	if err := storage.UpdateSessionStatus(ctx, companyID, accountID, domain.StatusActive); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("invalid session status: got %v, want ErrValidation", err)
	}
	if err := storage.UpdateLastActivity(ctx, companyID, accountID, time.Time{}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("zero activity time: got %v, want ErrValidation", err)
	}
	if _, err := storage.LoadSession(ctx, 0, accountID); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("invalid company scope: got %v, want ErrValidation", err)
	}
	if calls != 4 {
		t.Fatalf("invalid input reached PostgreSQL: got %d calls, want 4", calls)
	}

	mock.queryHandler = func(string, []driver.Value) (driver.Rows, error) {
		return nil, &pgconn.PgError{Code: "23505", Message: "fixture conflict"}
	}
	if err := storage.UpdateSessionStatus(ctx, companyID, accountID, domain.StatusDisconnected); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("postgres error mapping: got %v, want ErrConflict", err)
	}
}
