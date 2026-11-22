package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jholhewres/anchored_oss/internal/auth"
	"github.com/jholhewres/anchored_oss/internal/config"
	"github.com/jholhewres/anchored_oss/internal/store"
	"golang.org/x/crypto/bcrypt"
)

// accountLifecycleServer starts a server over a fresh SQLite store with one
// org and the given admin-role accounts (all with password "password123").
func accountLifecycleServer(t *testing.T, emails ...string) (*httptest.Server, store.Store, map[string]string) {
	t.Helper()
	st, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "lifecycle.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ctx := context.Background()
	org, err := st.CreateOrganization(ctx, "Lifecycle", "lifecycle")
	if err != nil {
		t.Fatal(err)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]string{}
	for _, email := range emails {
		acc, err := st.CreateAccount(ctx, email, email, string(hash))
		if err != nil {
			t.Fatal(err)
		}
		if err := st.AddOrgMember(ctx, org.ID, acc.ID, "admin"); err != nil {
			t.Fatal(err)
		}
		ids[email] = acc.ID
	}
	srv := New(ctx, config.DefaultConfig(), st, nil, slog.Default())
	ts := httptest.NewServer(srv.http.Handler)
	t.Cleanup(ts.Close)
	return ts, st, ids
}

func call(t *testing.T, ts *httptest.Server, method, path, key, body string) (int, map[string]any) {
	t.Helper()
	req, err := http.NewRequest(method, ts.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func loginKey(t *testing.T, ts *httptest.Server, email string) (int, string, string) {
	t.Helper()
	code, body := call(t, ts, http.MethodPost, "/v1/auth/login", "", `{"email":"`+email+`","password":"password123"}`)
	key, _ := body["api_key"].(string)
	scope, _ := body["scope"].(string)
	return code, key, scope
}

// TestDeletedAccountCannotLogInAgain: deleting an account removes it from the
// org, so its password no longer yields a session (admin or otherwise).
func TestDeletedAccountCannotLogInAgain(t *testing.T) {
	ts, _, ids := accountLifecycleServer(t, "owner@example.test", "removed@example.test")

	_, ownerKey, _ := loginKey(t, ts, "owner@example.test")
	_, removedKey, _ := loginKey(t, ts, "removed@example.test")

	if code, _ := call(t, ts, http.MethodDelete, "/v1/accounts/"+ids["removed@example.test"], ownerKey, ""); code != http.StatusNoContent {
		t.Fatalf("delete account = %d, want 204", code)
	}
	if code, _ := call(t, ts, http.MethodGet, "/v1/me", removedKey, ""); code != http.StatusUnauthorized {
		t.Fatalf("deleted account's old session = %d, want 401", code)
	}
	if code, _, scope := loginKey(t, ts, "removed@example.test"); code != http.StatusUnauthorized {
		t.Fatalf("deleted account logs in again = %d (scope %q), want 401", code, scope)
	}
}

// TestDemotedAccountLosesKeysAboveItsRole: a role change takes effect at once.
// Keys whose scope is above the new role, and the login session, are revoked;
// keys the new role still allows keep working.
func TestDemotedAccountLosesKeysAboveItsRole(t *testing.T) {
	ts, _, ids := accountLifecycleServer(t, "owner@example.test", "demoted@example.test")

	_, ownerKey, _ := loginKey(t, ts, "owner@example.test")
	_, sessionKey, scope := loginKey(t, ts, "demoted@example.test")
	if scope != "admin" {
		t.Fatalf("demoted account starts with scope %q, want admin", scope)
	}
	code, syncKey := call(t, ts, http.MethodPost, "/v1/me/api-keys", sessionKey, `{"name":"laptop","scope":"sync","expires_in":"30d"}`)
	if code != http.StatusCreated {
		t.Fatalf("create personal sync key = %d %v", code, syncKey)
	}
	code, readonlyKey := call(t, ts, http.MethodPost, "/v1/me/api-keys", sessionKey, `{"name":"reader","scope":"readonly","expires_in":"30d"}`)
	if code != http.StatusCreated {
		t.Fatalf("create personal readonly key = %d %v", code, readonlyKey)
	}
	code, adminKey := call(t, ts, http.MethodPost, "/v1/api-keys", ownerKey, `{"name":"ops","scope":"admin","account_id":"`+ids["demoted@example.test"]+`"}`)
	if code != http.StatusCreated {
		t.Fatalf("owner mints an admin key for the account = %d %v", code, adminKey)
	}

	if code, _ := call(t, ts, http.MethodPatch, "/v1/accounts/"+ids["demoted@example.test"], ownerKey, `{"role":"readonly"}`); code != http.StatusOK {
		t.Fatalf("demote to readonly = %d, want 200", code)
	}

	for label, key := range map[string]string{
		"login session":     sessionKey,
		"personal sync key": syncKey["key"].(string),
		"admin key":         adminKey["key"].(string),
	} {
		if code, _ := call(t, ts, http.MethodGet, "/v1/me", key, ""); code != http.StatusUnauthorized {
			t.Fatalf("%s after demotion to readonly = %d, want 401", label, code)
		}
	}
	if code, _ := call(t, ts, http.MethodGet, "/v1/me", readonlyKey["key"].(string), ""); code != http.StatusOK {
		t.Fatalf("personal readonly key after demotion = %d, want 200", code)
	}
	if code, _, scope := loginKey(t, ts, "demoted@example.test"); code != http.StatusOK || scope != "readonly" {
		t.Fatalf("login after demotion = %d scope %q, want 200 readonly", code, scope)
	}
}

// TestRenameKeepsTheAccountsKeys: only a real role change revokes keys. A
// rename, alone or with the role the account already has (the dashboard sends
// both), leaves the login session and every key working.
func TestRenameKeepsTheAccountsKeys(t *testing.T) {
	ts, _, ids := accountLifecycleServer(t, "owner@example.test", "renamed@example.test")
	target := ids["renamed@example.test"]

	_, ownerKey, _ := loginKey(t, ts, "owner@example.test")
	_, sessionKey, _ := loginKey(t, ts, "renamed@example.test")
	code, syncKey := call(t, ts, http.MethodPost, "/v1/me/api-keys", sessionKey, `{"name":"laptop","scope":"sync","expires_in":"30d"}`)
	if code != http.StatusCreated {
		t.Fatalf("create personal sync key = %d %v", code, syncKey)
	}
	code, adminKey := call(t, ts, http.MethodPost, "/v1/api-keys", ownerKey, `{"name":"ops","scope":"admin","account_id":"`+target+`"}`)
	if code != http.StatusCreated {
		t.Fatalf("owner mints an admin key for the account = %d %v", code, adminKey)
	}
	keys := map[string]string{
		"login session":     sessionKey,
		"personal sync key": syncKey["key"].(string),
		"admin key":         adminKey["key"].(string),
	}

	for _, body := range []string{
		`{"display_name":"Renamed"}`,
		`{"display_name":"Renamed again","role":"admin"}`,
		`{"role":"admin"}`,
	} {
		if code, out := call(t, ts, http.MethodPatch, "/v1/accounts/"+target, ownerKey, body); code != http.StatusOK {
			t.Fatalf("PATCH %s = %d %v, want 200", body, code, out)
		}
		for label, key := range keys {
			if code, _ := call(t, ts, http.MethodGet, "/v1/me", key, ""); code != http.StatusOK {
				t.Fatalf("%s after PATCH %s = %d, want 200", label, body, code)
			}
		}
	}
}

// TestKeysNeverOutrankTheirAccount (attack h1, run 20261122T202909Z): an admin
// cannot mint a key above the target account's role. Otherwise an admin key
// planted on a readonly account would survive every re-save, since a re-save
// with the same role revokes nothing.
func TestKeysNeverOutrankTheirAccount(t *testing.T) {
	ts, _, ids := accountLifecycleServer(t, "owner@example.test", "reader@example.test")
	target := ids["reader@example.test"]
	_, ownerKey, _ := loginKey(t, ts, "owner@example.test")

	if code, out := call(t, ts, http.MethodPatch, "/v1/accounts/"+target, ownerKey, `{"role":"readonly"}`); code != http.StatusOK {
		t.Fatalf("demote to readonly = %d %v, want 200", code, out)
	}
	for _, scope := range []string{"admin", "sync"} {
		if code, out := call(t, ts, http.MethodPost, "/v1/api-keys", ownerKey, `{"name":"ops","scope":"`+scope+`","account_id":"`+target+`"}`); code != http.StatusBadRequest {
			t.Fatalf("mint a %s key for a readonly account = %d %v, want 400", scope, code, out)
		}
	}
	if code, out := call(t, ts, http.MethodPost, "/v1/api-keys", ownerKey, `{"name":"reader","scope":"readonly","account_id":"`+target+`"}`); code != http.StatusCreated {
		t.Fatalf("mint a readonly key for a readonly account = %d %v, want 201", code, out)
	}

	if code, out := call(t, ts, http.MethodPatch, "/v1/accounts/"+target, ownerKey, `{"role":"sync"}`); code != http.StatusOK {
		t.Fatalf("promote to sync = %d %v, want 200", code, out)
	}
	if code, out := call(t, ts, http.MethodPost, "/v1/api-keys", ownerKey, `{"name":"ops","scope":"admin","account_id":"`+target+`"}`); code != http.StatusBadRequest {
		t.Fatalf("mint an admin key for a sync account = %d %v, want 400", code, out)
	}
	if code, out := call(t, ts, http.MethodPost, "/v1/api-keys", ownerKey, `{"name":"laptop","scope":"sync","account_id":"`+target+`"}`); code != http.StatusCreated {
		t.Fatalf("mint a sync key for a sync account = %d %v, want 201", code, out)
	}
}

// TestAuthRefusesAKeyAboveItsAccountsRole: a key that outranks its account (one
// minted in a race with a demotion, or kept from before the mint check) does
// not authenticate, and neither does a key whose account left the org.
func TestAuthRefusesAKeyAboveItsAccountsRole(t *testing.T) {
	ts, st, ids := accountLifecycleServer(t, "owner@example.test", "reader@example.test")
	ctx := context.Background()
	target := ids["reader@example.test"]
	orgID, _, err := st.GetAccountOrgMembership(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	_, ownerKey, _ := loginKey(t, ts, "owner@example.test")
	if code, out := call(t, ts, http.MethodPatch, "/v1/accounts/"+target, ownerKey, `{"role":"readonly"}`); code != http.StatusOK {
		t.Fatalf("demote to readonly = %d %v, want 200", code, out)
	}

	plant := func(scope string) string {
		t.Helper()
		full, prefix, hash, err := auth.GenerateAPIKey()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := st.CreateAPIKey(ctx, orgID, target, "legacy-"+scope, prefix, hash, scope, nil); err != nil {
			t.Fatal(err)
		}
		return full
	}
	planted := map[string]string{"admin": plant("admin"), "sync": plant("sync")}
	for scope, key := range planted {
		if code, _ := call(t, ts, http.MethodGet, "/v1/me", key, ""); code != http.StatusUnauthorized {
			t.Fatalf("%s key of a readonly account = %d, want 401", scope, code)
		}
	}
	// A refused key is revoked, so a promotion does not bring it back.
	if code, out := call(t, ts, http.MethodPatch, "/v1/accounts/"+target, ownerKey, `{"role":"admin"}`); code != http.StatusOK {
		t.Fatalf("promote to admin = %d %v, want 200", code, out)
	}
	for scope, key := range planted {
		if code, _ := call(t, ts, http.MethodGet, "/v1/me", key, ""); code != http.StatusUnauthorized {
			t.Fatalf("refused %s key after a promotion = %d, want 401", scope, code)
		}
	}
	readonly := plant("readonly")
	if code, _ := call(t, ts, http.MethodGet, "/v1/me", readonly, ""); code != http.StatusOK {
		t.Fatalf("readonly key of a readonly account = %d, want 200", code)
	}
}
