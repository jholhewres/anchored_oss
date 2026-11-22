package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jholhewres/anchored_oss/internal/auth"
)

// TestUpdateAccount_SameRoleKeepsKeys: UpdateAccount revokes keys only on a
// real role change. Its Postgres twin is TestUpdateAccount_SameRoleKeepsKeys_Postgres.
func TestUpdateAccount_SameRoleKeepsKeys(t *testing.T) {
	checkSameRoleKeepsKeys(t, newSQLiteTestStore(t), "")
}

// checkSameRoleKeepsKeys runs on any backend: a rename and a PATCH resending
// the current role keep the session and every key; a real change still
// revokes the session. u keeps names unique on a shared database.
func checkSameRoleKeepsKeys(t *testing.T, st Store, u string) {
	t.Helper()
	ctx := context.Background()
	org, err := st.CreateOrganization(ctx, "Rename", "rename"+u)
	if err != nil {
		t.Fatal(err)
	}
	acc, err := st.CreateAccount(ctx, "rename"+u+"@example.test", "Before", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AddOrgMember(ctx, org.ID, acc.ID, "admin"); err != nil {
		t.Fatal(err)
	}
	expires := time.Now().UTC().Add(24 * time.Hour)
	hashes := map[string]struct{ name, scope string }{
		"rename-session" + u: {"session", "admin"},
		"rename-admin" + u:   {"ops", "admin"},
		"rename-sync" + u:    {"laptop", "sync"},
	}
	for hash, k := range hashes {
		if _, err := st.CreateAPIKey(ctx, org.ID, acc.ID, k.name, "anc_live_tst", hash, k.scope, &expires); err != nil {
			t.Fatalf("seed %s key: %v", k.name, err)
		}
	}
	assertActive := func(when string) {
		t.Helper()
		for hash, k := range hashes {
			// GetAPIKeyByHash only returns keys that are not revoked.
			if _, err := st.GetAPIKeyByHash(ctx, hash); err != nil {
				t.Fatalf("%s key after %s: %v, want active", k.name, when, err)
			}
		}
	}

	if err := st.UpdateAccount(ctx, org.ID, acc.ID, "After", ""); err != nil {
		t.Fatalf("rename: %v", err)
	}
	assertActive("a rename")
	if err := st.UpdateAccount(ctx, org.ID, acc.ID, "After again", "admin"); err != nil {
		t.Fatalf("rename with the same role: %v", err)
	}
	assertActive("a rename with the same role")

	if err := st.UpdateAccount(ctx, org.ID, acc.ID, "", "sync"); err != nil {
		t.Fatalf("demote: %v", err)
	}
	if _, err := st.GetAPIKeyByHash(ctx, "rename-session"+u); !errors.Is(err, ErrNotFound) {
		t.Fatalf("session key after a real role change = %v, want revoked (ErrNotFound)", err)
	}

	// Every role, not only admin: a non-admin account renamed with its own
	// role keeps its session, and a promotion is a real change.
	for _, role := range []string{"sync", "member", "readonly"} {
		other, err := st.CreateAccount(ctx, role+"-rename"+u+"@example.test", "Before", "hash")
		if err != nil {
			t.Fatal(err)
		}
		if err := st.AddOrgMember(ctx, org.ID, other.ID, role); err != nil {
			t.Fatal(err)
		}
		session := role + "-session" + u
		if _, err := st.CreateAPIKey(ctx, org.ID, other.ID, "session", "anc_live_tst", session, auth.SessionScopeForRole(role), &expires); err != nil {
			t.Fatalf("seed %s session: %v", role, err)
		}
		if err := st.UpdateAccount(ctx, org.ID, other.ID, "Renamed", role); err != nil {
			t.Fatalf("rename %s with its own role: %v", role, err)
		}
		if _, err := st.GetAPIKeyByHash(ctx, session); err != nil {
			t.Fatalf("%s session after a rename with its own role: %v, want active", role, err)
		}
		if err := st.UpdateAccount(ctx, org.ID, other.ID, "", "admin"); err != nil {
			t.Fatalf("promote %s: %v", role, err)
		}
		if got, err := st.GetOrgMemberRole(ctx, org.ID, other.ID); err != nil || got != "admin" {
			t.Fatalf("%s role after promotion = %q, %v; want admin", role, got, err)
		}
		if _, err := st.GetAPIKeyByHash(ctx, session); !errors.Is(err, ErrNotFound) {
			t.Fatalf("%s session after promotion = %v, want revoked (ErrNotFound)", role, err)
		}
	}
}

// TestGetOrgMemberRole_NotAMember: a role is only read inside its org.
func TestGetOrgMemberRole_NotAMember(t *testing.T) {
	st := newSQLiteTestStore(t)
	ctx := context.Background()
	org, err := st.CreateOrganization(ctx, "Roles", "roles")
	if err != nil {
		t.Fatal(err)
	}
	acc, err := st.CreateAccount(ctx, "nobody@example.test", "Nobody", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetOrgMemberRole(ctx, org.ID, acc.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetOrgMemberRole for a non-member = %v, want ErrNotFound", err)
	}
}
