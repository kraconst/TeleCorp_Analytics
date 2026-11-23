package postgres

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
)

// New establishes a connection to PostgreSQL and verifies it with a ping.
func New(dsn string) (*sqlx.DB, error) {
	db, err := sqlx.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres connection: %w", err)
	}

	// Настройки пула соединений
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(15 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	return db, nil
}

// RunMigrations applies all *.up.sql files from migrationsDir in alphabetical order.
func RunMigrations(db *sqlx.DB, migrationsDir string) error {
	ctx := context.Background()

	// Получаем выделенное TCP-соединение из пула, так как advisory_lock привязан к конкретной сессии
	conn, err := db.Connx(ctx)
	if err != nil {
		return fmt.Errorf("acquire dedicated connection for migrations: %w", err)
	}
	defer conn.Close()

	const migrationLockID = 847291038
	if _, err := conn.ExecContext(ctx, fmt.Sprintf("SELECT pg_advisory_lock(%d)", migrationLockID)); err != nil {
		return fmt.Errorf("acquire migration advisory lock: %w", err)
	}
	defer func() {
		_, _ = conn.ExecContext(ctx, fmt.Sprintf("SELECT pg_advisory_unlock(%d)", migrationLockID))
	}()

	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		return fmt.Errorf("read migrations directory: %w", err)
	}

	var upFiles []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".up.sql") {
			upFiles = append(upFiles, e.Name())
		}
	}
	sort.Strings(upFiles)

	// Создаем таблицу миграций, если её нет
	_, err = conn.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version VARCHAR(255) PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
	`)
	if err != nil {
		return fmt.Errorf("create schema_migrations table: %w", err)
	}

	for _, file := range upFiles {
		var count int
		err := conn.GetContext(ctx, &count, "SELECT COUNT(*) FROM schema_migrations WHERE version = $1", file)
		if err != nil {
			return fmt.Errorf("check migration version %s: %w", file, err)
		}
		if count > 0 {
			continue
		}

		content, err := os.ReadFile(filepath.Join(migrationsDir, file))
		if err != nil {
			return fmt.Errorf("read migration file %s: %w", file, err)
		}

		tx, err := conn.BeginTxx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin tx for migration %s: %w", file, err)
		}

		if _, err := tx.ExecContext(ctx, string(content)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("execute migration %s: %w", file, err)
		}

		if _, err := tx.ExecContext(ctx, "INSERT INTO schema_migrations (version) VALUES ($1)", file); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record migration %s: %w", file, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %s: %w", file, err)
		}
	}

	return nil
}
