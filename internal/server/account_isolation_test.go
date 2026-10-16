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

// TestAccountAdminActionsStayInCallerOrg: the org role decides the login
// session's key scope, so an admin of one org must not read or change an
// account of another org. Self-registration hands out admin of a new org, so
// "admin somewhere" is not a trust boundary.
func TestAccountAdminActionsStayInCallerOrg(t *testing.T) {
	st, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "account-isolation.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	hash, err := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	member := func(orgID, email, role string) string {
		t.Helper()
		acc, err := st.CreateAccount(ctx, email, email, string(hash))
		if err != nil {
			t.Fatal(err)
		}
		if err := st.AddOrgMember(ctx, orgID, acc.ID, role); err != nil {
			t.Fatal(err)
		}
		return acc.ID
	}
	orgA, err := st.CreateOrganization(ctx, "Victim", "victim")
	if err != nil {
		t.Fatal(err)
	}
	orgB, err := st.CreateOrganization(ctx, "Other", "other")
	if err != nil {
		t.Fatal(err)
	}
	member(orgA.ID, "admin-a@example.test", "admin")
	victimID := member(orgA.ID, "victim@example.test", "readonly")
	member(orgB.ID, "admin-b@example.test", "admin")

	srv := New(ctx, config.DefaultConfig(), st, nil, slog.Default())
	ts := httptest.NewServer(srv.http.Handler)
	defer ts.Close()
	do := func(method, path, key, body string) int {
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
		resp.Body.Close()
		return resp.StatusCode
	}
	login := func(email string) string {
		t.Helper()
		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/auth/login",
			strings.NewReader(`{"email":"`+email+`","password":"password123"}`))
		req.Header.Set("Content-Type", "application/json")
		resp, err := ts.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var body struct {
			APIKey string `json:"api_key"`
		}
		if resp.StatusCode != http.StatusOK || json.NewDecoder(resp.Body).Decode(&body) != nil {
			t.Fatalf("login %s = %d", email, resp.StatusCode)
		}
		return body.APIKey
	}

	victimKey := login("victim@example.test")
	otherAdmin := login("admin-b@example.test")

	if code := do(http.MethodPatch, "/v1/accounts/"+victimID, otherAdmin, `{"role":"admin"}`); code != http.StatusNotFound {
		t.Fatalf("foreign admin PATCH role = %d, want 404", code)
	}
	if code := do(http.MethodGet, "/v1/accounts/"+victimID+"/projects", otherAdmin, ""); code != http.StatusNotFound {
		t.Fatalf("foreign admin list projects = %d, want 404", code)
	}
	if code := do(http.MethodDelete, "/v1/accounts/"+victimID, otherAdmin, ""); code != http.StatusNotFound {
		t.Fatalf("foreign admin DELETE = %d, want 404", code)
	}
	if _, role, err := st.GetAccountOrgMembership(ctx, victimID); err != nil || role != "readonly" {
		t.Fatalf("victim role after foreign PATCH = %q (%v), want readonly", role, err)
	}
	if code := do(http.MethodGet, "/v1/me", victimKey, ""); code != http.StatusOK {
		t.Fatalf("victim session after foreign DELETE = %d, want 200", code)
	}

	ownAdmin := login("admin-a@example.test")
	if code := do(http.MethodPatch, "/v1/accounts/"+victimID, ownAdmin, `{"role":"superuser"}`); code != http.StatusBadRequest {
		t.Fatalf("PATCH unknown role = %d, want 400", code)
	}
	if code := do(http.MethodPatch, "/v1/accounts/"+victimID, ownAdmin, `{"role":"sync"}`); code != http.StatusOK {
		t.Fatalf("own admin PATCH role = %d, want 200", code)
	}
	if _, role, _ := st.GetAccountOrgMembership(ctx, victimID); role != "sync" {
		t.Fatalf("victim role after own admin PATCH = %q, want sync", role)
	}
}
