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

// TestMemberLoginGetsSyncScope covers accounts added through POST
// /v1/accounts, which joins them with the org role "member". That role is not
// a key scope, so their login session must carry sync to be usable.
func TestMemberLoginGetsSyncScope(t *testing.T) {
	st, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "member-login.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	org, err := st.CreateOrganization(ctx, "Member login", "member-login")
	if err != nil {
		t.Fatal(err)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	admin, err := st.CreateAccount(ctx, "admin@example.test", "Admin", string(hash))
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AddOrgMember(ctx, org.ID, admin.ID, "admin"); err != nil {
		t.Fatal(err)
	}

	srv := New(ctx, config.DefaultConfig(), st, nil, slog.Default())
	ts := httptest.NewServer(srv.http.Handler)
	defer ts.Close()

	do := func(method, path, key, body string) (int, map[string]any) {
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
	login := func(email string) (key, scope string) {
		t.Helper()
		code, body := do(http.MethodPost, "/v1/auth/login", "", `{"email":"`+email+`","password":"password123"}`)
		if code != http.StatusOK {
			t.Fatalf("login %s = %d %v", email, code, body)
		}
		key, _ = body["api_key"].(string)
		scope, _ = body["scope"].(string)
		return key, scope
	}

	adminKey, adminScope := login("admin@example.test")
	if adminScope != "admin" {
		t.Fatalf("admin login scope = %q, want admin", adminScope)
	}
	code, body := do(http.MethodPost, "/v1/accounts", adminKey,
		`{"email":"member@example.test","display_name":"Member","password":"password123"}`)
	if code != http.StatusCreated {
		t.Fatalf("create member = %d %v", code, body)
	}

	memberKey, memberScope := login("member@example.test")
	if memberScope != "sync" {
		t.Fatalf("member login scope = %q, want sync", memberScope)
	}
	code, body = do(http.MethodPost, "/v1/me/api-keys", memberKey,
		`{"name":"laptop","scope":"sync","expires_in":"90d"}`)
	if code != http.StatusCreated {
		t.Fatalf("member creates personal sync key = %d %v, want 201", code, body)
	}
	if code, _ := do(http.MethodGet, "/v1/api-keys", memberKey, ""); code != http.StatusForbidden {
		t.Fatalf("member session on admin key list = %d, want 403", code)
	}
}

// TestPersonalKeysExcludeTheLoginKey: the login credential (name "session")
// is not a personal key, so My API keys neither lists nor revokes it, whatever
// scope the session carries.
func TestPersonalKeysExcludeTheLoginKey(t *testing.T) {
	st, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "personal-keys.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	org, err := st.CreateOrganization(ctx, "Personal keys", "personal-keys")
	if err != nil {
		t.Fatal(err)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	acc, err := st.CreateAccount(ctx, "sync@example.test", "Sync", string(hash))
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AddOrgMember(ctx, org.ID, acc.ID, "sync"); err != nil {
		t.Fatal(err)
	}

	srv := New(ctx, config.DefaultConfig(), st, nil, slog.Default())
	ts := httptest.NewServer(srv.http.Handler)
	defer ts.Close()
	send := func(method, path, key, body string) *http.Response {
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
		return resp
	}

	resp := send(http.MethodPost, "/v1/auth/login", "", `{"email":"sync@example.test","password":"password123"}`)
	var login struct {
		APIKey string `json:"api_key"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&login); err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("login = %d (%v)", resp.StatusCode, err)
	}
	resp.Body.Close()
	sessionKey, err := st.GetAPIKeyByHash(ctx, auth.HashAPIKey(login.APIKey))
	if err != nil {
		t.Fatal(err)
	}

	resp = send(http.MethodGet, "/v1/me/api-keys", login.APIKey, "")
	var keys []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&keys); err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("list personal keys = %d (%v)", resp.StatusCode, err)
	}
	resp.Body.Close()
	for _, k := range keys {
		if k.ID == sessionKey.ID || k.Name == "session" {
			t.Fatalf("personal key list includes the login key: %+v", keys)
		}
	}

	resp = send(http.MethodDelete, "/v1/me/api-keys/"+sessionKey.ID, login.APIKey, "")
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("revoke login key through personal keys = %d, want 404", resp.StatusCode)
	}
}
