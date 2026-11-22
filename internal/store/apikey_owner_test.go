package store

import (
	"context"
	"errors"
	"testing"
)

// TestAPIKeyOwnershipAndOrgScope: personal key routes act only on the owner's
// own sync/readonly keys in the given org, org key revocation only inside the
// org, and membership lookups answer per org. Its Postgres twin is
// TestAPIKeyOwnershipAndOrgScope_Postgres.
func TestAPIKeyOwnershipAndOrgScope(t *testing.T) {
	checkAPIKeyOwnershipAndOrgScope(t, newSQLiteTestStore(t))
}

func checkAPIKeyOwnershipAndOrgScope(t *testing.T, st Store) {
	t.Helper()
	ctx := context.Background()
	orgA, err := st.CreateOrganization(ctx, "Acme", "acme-keys")
	if err != nil {
		t.Fatal(err)
	}
	orgB, err := st.CreateOrganization(ctx, "Other", "other-keys")
	if err != nil {
		t.Fatal(err)
	}
	owner, err := st.CreateAccount(ctx, "owner@keys.test", "Owner", "hash")
	if err != nil {
		t.Fatal(err)
	}
	peer, err := st.CreateAccount(ctx, "peer@keys.test", "Peer", "hash")
	if err != nil {
		t.Fatal(err)
	}
	for _, acc := range []string{owner.ID, peer.ID} {
		if err := st.AddOrgMember(ctx, orgA.ID, acc, "member"); err != nil {
			t.Fatal(err)
		}
	}

	if ok, err := st.IsOrgMember(ctx, orgA.ID, owner.ID); err != nil || !ok {
		t.Fatalf("IsOrgMember(a, owner) = %t, %v; want true", ok, err)
	}
	if ok, err := st.IsOrgMember(ctx, orgB.ID, owner.ID); err != nil || ok {
		t.Fatalf("IsOrgMember(b, owner) = %t, %v; want false", ok, err)
	}
	if org, role, err := st.GetAccountOrgMembership(ctx, owner.ID); err != nil || org != orgA.ID || role != "member" {
		t.Fatalf("GetAccountOrgMembership(owner) = %q, %q, %v; want org a, member", org, role, err)
	}
	if _, _, err := st.GetAccountOrgMembership(ctx, "00000000-0000-0000-0000-000000000000"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetAccountOrgMembership(unknown) = %v, want ErrNotFound", err)
	}

	mine, err := st.CreateAPIKey(ctx, orgA.ID, owner.ID, "laptop", "anc_live_own", "owner-laptop", "sync", nil)
	if err != nil {
		t.Fatal(err)
	}
	session, err := st.CreateAPIKey(ctx, orgA.ID, owner.ID, "session", "anc_live_ses", "owner-session", "sync", nil)
	if err != nil {
		t.Fatal(err)
	}
	ops, err := st.CreateAPIKey(ctx, orgA.ID, owner.ID, "ops", "anc_live_ops", "owner-ops", "admin", nil)
	if err != nil {
		t.Fatal(err)
	}
	theirs, err := st.CreateAPIKey(ctx, orgA.ID, peer.ID, "laptop", "anc_live_peer", "peer-laptop", "sync", nil)
	if err != nil {
		t.Fatal(err)
	}

	listed, err := st.ListAPIKeysByOwner(ctx, orgA.ID, owner.ID)
	if err != nil || len(listed) != 1 || listed[0].ID != mine.ID {
		t.Fatalf("ListAPIKeysByOwner(a, owner) = %d keys, %v; want only the personal sync key", len(listed), err)
	}
	if listed, err := st.ListAPIKeysByOwner(ctx, orgB.ID, owner.ID); err != nil || len(listed) != 0 {
		t.Fatalf("ListAPIKeysByOwner(b, owner) = %d keys, %v; want none", len(listed), err)
	}

	if err := st.RevokeAPIKeyByOwner(ctx, orgA.ID, owner.ID, theirs.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("RevokeAPIKeyByOwner on another account's key = %v, want ErrNotFound", err)
	}
	if err := st.RevokeAPIKeyByOwner(ctx, orgB.ID, owner.ID, mine.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("RevokeAPIKeyByOwner from another org = %v, want ErrNotFound", err)
	}
	// The personal routes never touch the login session or an admin key,
	// even the owner's own.
	for name, id := range map[string]string{"session": session.ID, "admin": ops.ID} {
		if err := st.RevokeAPIKeyByOwner(ctx, orgA.ID, owner.ID, id); !errors.Is(err, ErrNotFound) {
			t.Fatalf("RevokeAPIKeyByOwner on the owner's %s key = %v, want ErrNotFound", name, err)
		}
	}
	if _, err := st.GetAPIKeyByHash(ctx, "owner-ops"); err != nil {
		t.Fatalf("owner's admin key after a personal revoke = %v, want active", err)
	}
	if err := st.RevokeAPIKeyByOwner(ctx, orgA.ID, owner.ID, mine.ID); err != nil {
		t.Fatalf("RevokeAPIKeyByOwner on its own key: %v", err)
	}
	if _, err := st.GetAPIKeyByHash(ctx, "owner-laptop"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("own key after revoke = %v, want revoked", err)
	}

	if err := st.RevokeAPIKey(ctx, orgB.ID, theirs.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("RevokeAPIKey from another org = %v, want ErrNotFound", err)
	}
	if _, err := st.GetAPIKeyByHash(ctx, "peer-laptop"); err != nil {
		t.Fatalf("peer key after a foreign revoke = %v, want active", err)
	}
	if err := st.RevokeAPIKey(ctx, orgA.ID, theirs.ID); err != nil {
		t.Fatalf("RevokeAPIKey in its org: %v", err)
	}
}
