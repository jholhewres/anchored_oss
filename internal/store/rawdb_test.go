package store

import (
	"database/sql"
	"testing"
)

// rawDB returns the *sql.DB behind either backend, for test setup and checks
// that no store method offers.
func rawDB(t *testing.T, st Store) *sql.DB {
	t.Helper()
	switch s := st.(type) {
	case *SQLiteStore:
		return s.db
	case *PostgresStore:
		return s.db
	}
	t.Fatalf("rawDB: unsupported store %T", st)
	return nil
}

// isPostgres reports whether st is the Postgres backend, for the few test
// statements whose SQL differs.
func isPostgres(st Store) bool {
	_, ok := st.(*PostgresStore)
	return ok
}
