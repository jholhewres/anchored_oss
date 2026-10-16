//go:build integration

package store

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

// TestMigration025RevokesLegacySessionKeys_Postgres is the Postgres twin of
// TestMigration025RevokesLegacySessionKeys.
func TestMigration025RevokesLegacySessionKeys_Postgres(t *testing.T) {
	dsn := os.Getenv("ANCHORED_TEST_DSN")
	if dsn == "" {
		t.Skip("ANCHORED_TEST_DSN not set; skipping Postgres migration test")
	}
	st, err := NewPostgresStore(PoolConfig{DSN: dsn})
	if err != nil {
		t.Fatalf("open postgres store: %v", err)
	}
	ctx := context.Background()
	u := uniq()
	org, err := st.CreateOrganization(ctx, "SessionOrg", "session-org-"+u)
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	acct, err := st.CreateAccount(ctx, "session-"+u+"@example.test", "S", "hash")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	sessionHash, personalHash := "legacy-session-"+u, "personal-"+u
	if _, err := st.CreateAPIKey(ctx, org.ID, acct.ID, "session", "anc_live_ses", sessionHash, "admin", nil); err != nil {
		t.Fatalf("seed legacy session key: %v", err)
	}
	expires := time.Now().UTC().Add(24 * time.Hour)
	if _, err := st.CreateAPIKey(ctx, org.ID, acct.ID, "laptop", "anc_live_lap", personalHash, "sync", &expires); err != nil {
		t.Fatalf("seed personal key: %v", err)
	}
	if _, err := st.db.ExecContext(ctx, `DELETE FROM schema_version WHERE version >= 25`); err != nil {
		t.Fatalf("rewind schema version: %v", err)
	}
	st.Close()

	upgraded, err := NewPostgresStore(PoolConfig{DSN: dsn})
	if err != nil {
		t.Fatalf("upgrade store: %v", err)
	}
	defer upgraded.Close()

	if _, err := upgraded.GetAPIKeyByHash(ctx, sessionHash); !errors.Is(err, ErrNotFound) {
		t.Fatalf("legacy session key after upgrade: err = %v, want ErrNotFound (revoked)", err)
	}
	if _, err := upgraded.GetAPIKeyByHash(ctx, personalHash); err != nil {
		t.Fatalf("personal key after upgrade: %v, want still active", err)
	}
}
