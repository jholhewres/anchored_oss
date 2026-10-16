package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

// TestMigration025RevokesLegacySessionKeys covers servers upgraded from a
// release where every dashboard login minted a never-expiring admin key named
// "session". Those keys must stop working so the next login gets a session
// scoped to the account's org role; other keys are untouched.
func TestMigration025RevokesLegacySessionKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy-sessions.db")
	st, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	ctx := context.Background()
	orgID, acctID := projectTestOrg(t, st)
	if _, err := st.CreateAPIKey(ctx, orgID, acctID, "session", "anc_live_ses", "legacy-session-hash", "admin", nil); err != nil {
		t.Fatalf("seed legacy session key: %v", err)
	}
	expires := time.Now().UTC().Add(24 * time.Hour)
	if _, err := st.CreateAPIKey(ctx, orgID, acctID, "laptop", "anc_live_lap", "personal-key-hash", "sync", &expires); err != nil {
		t.Fatalf("seed personal key: %v", err)
	}
	if _, err := st.db.Exec(`DELETE FROM schema_version WHERE version >= 25`); err != nil {
		t.Fatalf("rewind schema version: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	upgraded, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("upgrade store: %v", err)
	}
	defer upgraded.Close()

	if _, err := upgraded.GetAPIKeyByHash(ctx, "legacy-session-hash"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("legacy session key after upgrade: err = %v, want ErrNotFound (revoked)", err)
	}
	if _, err := upgraded.GetAPIKeyByHash(ctx, "personal-key-hash"); err != nil {
		t.Fatalf("personal key after upgrade: %v, want still active", err)
	}
}
