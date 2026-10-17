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
