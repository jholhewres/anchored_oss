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
