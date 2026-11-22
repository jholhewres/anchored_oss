//go:build integration

package store

import (
	"os"
	"testing"
)

// TestProjectAccess_ScopedToOrg_Postgres is the Postgres twin of
// TestProjectAccess_ScopedToOrg.
func TestProjectAccess_ScopedToOrg_Postgres(t *testing.T) {
	dsn := os.Getenv("ANCHORED_TEST_DSN")
	if dsn == "" {
		t.Skip("ANCHORED_TEST_DSN not set; skipping Postgres project access test")
	}
	st, err := NewPostgresStore(PoolConfig{DSN: dsn})
	if err != nil {
		t.Fatalf("open postgres store: %v", err)
	}
	defer st.Close()
	checkProjectAccessScopedToOrg(t, st, "-"+uniq())
}

// TestTeams_ScopedToOrg_Postgres is the Postgres twin of TestTeams_ScopedToOrg.
func TestTeams_ScopedToOrg_Postgres(t *testing.T) {
	dsn := os.Getenv("ANCHORED_TEST_DSN")
	if dsn == "" {
		t.Skip("ANCHORED_TEST_DSN not set; skipping Postgres team scope test")
	}
	st, err := NewPostgresStore(PoolConfig{DSN: dsn})
	if err != nil {
		t.Fatalf("open postgres store: %v", err)
	}
	defer st.Close()
	checkTeamsScopedToOrg(t, st, "-"+uniq())
}
