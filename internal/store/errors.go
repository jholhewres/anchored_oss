package store

import (
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

// ErrNotFound is returned when a query finds no matching row.
// Callers should map this to a domain-specific error (e.g. 404) instead
// of treating it as an unexpected database failure.
var ErrNotFound = errors.New("not found")

// ErrConflict is returned when a write violates a unique constraint that the
// caller can surface as a 409 (e.g. a slug already in use within an org).
var ErrConflict = errors.New("conflict")

// ErrIdempotencyConflict is returned when a scoped operation ID was already
// committed with a different payload.
var ErrIdempotencyConflict = errors.New("idempotency conflict")

// isUniqueViolation reports whether err is a unique-constraint violation from
// either backend: modernc sqlite surfaces it as a string ("UNIQUE constraint
// failed"), while pgx surfaces a *pgconn.PgError with SQLSTATE 23505.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return strings.Contains(err.Error(), "UNIQUE constraint failed")
}
