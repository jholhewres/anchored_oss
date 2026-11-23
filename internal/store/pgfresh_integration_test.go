//go:build integration

package store

import (
	"context"
	"database/sql"
	"net/url"
	"os"
	"strings"
	"testing"
)

// freshPostgresStore opens a migrated Postgres store on a database of its own,
// created from ANCHORED_TEST_DSN's server and dropped when the test ends. The
// twins of SQLite tests use it where a shared database would get in the way:
// a first-run bootstrap needs zero organizations, and fixed slugs and emails
// would collide across runs. It skips without ANCHORED_TEST_DSN.
func freshPostgresStore(t *testing.T) *PostgresStore {
	t.Helper()
	dsn := os.Getenv("ANCHORED_TEST_DSN")
	if dsn == "" {
		t.Skip("ANCHORED_TEST_DSN not set; skipping Postgres twin")
	}
	base, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("ANCHORED_TEST_DSN must be a postgres:// URL for the per-test databases: %v", err)
	}
	if base.Scheme != "postgres" && base.Scheme != "postgresql" {
		t.Fatalf("ANCHORED_TEST_DSN must be a postgres:// URL for the per-test databases, not %q", base.Scheme)
	}
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open admin connection: %v", err)
	}
	t.Cleanup(func() { _ = admin.Close() })

	name := "anchored_t_" + strings.NewReplacer("-", "_").Replace(uniq())
	ctx := context.Background()
	if _, err := admin.ExecContext(ctx, `CREATE DATABASE `+name); err != nil {
		t.Fatalf("create test database: %v", err)
	}
	fresh := *base
	fresh.Path = "/" + name
	st, err := NewPostgresStore(PoolConfig{DSN: fresh.String()})
	if err != nil {
		_, _ = admin.ExecContext(ctx, `DROP DATABASE IF EXISTS `+name+` WITH (FORCE)`)
		t.Fatalf("open fresh postgres store: %v", err)
	}
	t.Cleanup(func() {
		st.Close()
		if _, err := admin.ExecContext(context.Background(), `DROP DATABASE IF EXISTS `+name+` WITH (FORCE)`); err != nil {
			t.Logf("drop test database %s: %v", name, err)
		}
	})
	return st
}

// TestFreshPostgresStore checks the helper itself: an empty, migrated
// database per call.
func TestFreshPostgresStore(t *testing.T) {
	st := freshPostgresStore(t)
	n, err := st.CountOrganizations(context.Background())
	if err != nil || n != 0 {
		t.Fatalf("fresh database organizations = %d, %v; want 0", n, err)
	}
}

// TestAPIKeyOwnershipAndOrgScope_Postgres is the Postgres twin of
// TestAPIKeyOwnershipAndOrgScope.
func TestAPIKeyOwnershipAndOrgScope_Postgres(t *testing.T) {
	checkAPIKeyOwnershipAndOrgScope(t, freshPostgresStore(t))
}
