package store

import "errors"

// ErrNotFound is returned when a query finds no matching row.
// Callers should map this to a domain-specific error (e.g. 404) instead
// of treating it as an unexpected database failure.
var ErrNotFound = errors.New("not found")
