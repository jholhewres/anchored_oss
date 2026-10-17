//go:build integration

package store

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

// TestAccountRoleChangeAndDelete_Postgres is the Postgres twin of the server
// tests TestDemotedAccountLosesKeysAboveItsRole and
// TestDeletedAccountCannotLogInAgain, at the store seam.
func TestAccountRoleChangeAndDelete_Postgres(t *testing.T) {
	dsn := os.Getenv("ANCHORED_TEST_DSN")
	if dsn == "" {
		t.Skip("ANCHORED_TEST_DSN not set; skipping Postgres account lifecycle test")
	}
	st, err := NewPostgresStore(PoolConfig{DSN: dsn})
	if err != nil {
		t.Fatalf("open postgres store: %v", err)
	}
	defer st.Close()
	ctx := context.Background()
	u := uniq()
	org, err := st.CreateOrganization(ctx, "Lifecycle", "lifecycle-"+u)
	if err != nil {
		t.Fatal(err)
	}
	acc, err := st.CreateAccount(ctx, "lifecycle-"+u+"@example.test", "L", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AddOrgMember(ctx, org.ID, acc.ID, "admin"); err != nil {
		t.Fatal(err)
	}
	expires := time.Now().UTC().Add(24 * time.Hour)
	keys := map[string]struct{ name, scope string }{
		"session-" + u:  {"session", "admin"},
		"admin-" + u:    {"ops", "admin"},
		"sync-" + u:     {"laptop", "sync"},
		"readonly-" + u: {"reader", "readonly"},
	}
	for hash, k := range keys {
		if _, err := st.CreateAPIKey(ctx, org.ID, acc.ID, k.name, "anc_live_tst", hash, k.scope, &expires); err != nil {
			t.Fatalf("seed %s key: %v", k.name, err)
		}
	}

	if err := st.UpdateAccount(ctx, org.ID, acc.ID, "", "readonly"); err != nil {
		t.Fatalf("demote: %v", err)
	}
	for hash, k := range keys {
		_, err := st.GetAPIKeyByHash(ctx, hash)
		if k.scope == "readonly" && k.name != "session" {
			if err != nil {
				t.Fatalf("%s key after demotion: %v, want active", k.name, err)
			}
			continue
		}
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("%s key after demotion: err = %v, want revoked", k.name, err)
		}
	}
	if _, role, err := st.GetAccountOrgMembership(ctx, acc.ID); err != nil || role != "readonly" {
		t.Fatalf("role after demotion = %q (%v), want readonly", role, err)
	}

	if err := st.SoftDeleteAccount(ctx, org.ID, acc.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, _, err := st.GetAccountOrgMembership(ctx, acc.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("membership after delete: err = %v, want ErrNotFound", err)
	}
	if _, err := st.GetAPIKeyByHash(ctx, "readonly-"+u); !errors.Is(err, ErrNotFound) {
		t.Fatalf("readonly key after delete: err = %v, want revoked", err)
	}
}
