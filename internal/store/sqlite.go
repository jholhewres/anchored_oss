package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"

	_ "modernc.org/sqlite"
)

func containsParam(dsn, param string) bool {
	return strings.Contains(dsn, param+"=") || strings.Contains(dsn, param+"&")
}

// SQLiteStore implements Store backed by a SQLite database file.
type SQLiteStore struct {
	db            *sql.DB
	idempotencyMu sync.Mutex
}

// NewSQLiteStore opens (or creates) a SQLite database, sets pragmas, runs
// migrations, and returns a ready store. If dsn is empty it defaults to
// "./anchored.db".
func NewSQLiteStore(dsn string) (*SQLiteStore, error) {
	if dsn == "" {
		dsn = "./anchored.db"
	}

	if !containsParam(dsn, "_loc") {
		sep := "?"
		if strings.Contains(dsn, "?") {
			sep = "&"
		}
		dsn += sep + "_loc=UTC"
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// Safety pragmas for single-writer operation.
	db.Exec("PRAGMA journal_mode=WAL")
	db.Exec("PRAGMA foreign_keys=ON")
	db.Exec("PRAGMA busy_timeout=5000")

	if err := MigrateSQLite(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("run sqlite migrations: %w", err)
	}

	return &SQLiteStore{db: db}, nil
}

// Ping verifies the database connection.
func (s *SQLiteStore) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

// Close releases the database connection.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}
