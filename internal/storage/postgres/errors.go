package postgres

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/kraconst/GO_server/internal/domain"
)

// mapPgError converts PostgreSQL errors into domain errors:
// - 23505 (unique_violation) -> domain.ErrConflict
// - 23503 (foreign_key_violation) -> domain.ErrNotFound
func mapPgError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, domain.ErrNotFound) || errors.Is(err, domain.ErrConflict) ||
		errors.Is(err, domain.ErrForbidden) || errors.Is(err, domain.ErrValidation) {
		return err
	}
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			msg := pgErr.Detail
			if msg == "" {
				msg = pgErr.Message
			}
			return fmt.Errorf("%w: %s", domain.ErrConflict, msg)
		case "23503":
			msg := pgErr.Detail
			if msg == "" {
				msg = pgErr.Message
			}
			return fmt.Errorf("%w: %s", domain.ErrNotFound, msg)
		}
	}
	errStr := err.Error()
	if strings.Contains(errStr, "23505") || strings.Contains(errStr, "unique constraint") || strings.Contains(errStr, "duplicate key") {
		return fmt.Errorf("%w: %s", domain.ErrConflict, errStr)
	}
	if strings.Contains(errStr, "23503") || strings.Contains(errStr, "foreign key constraint") {
		return fmt.Errorf("%w: %s", domain.ErrNotFound, errStr)
	}
	return err
}
