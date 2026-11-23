package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jholhewres/anchored_oss/internal/auth"
	"github.com/jholhewres/anchored_oss/internal/config"
	"github.com/jholhewres/anchored_oss/internal/store"
	"github.com/jholhewres/anchored_oss/internal/web"
	"golang.org/x/crypto/bcrypt"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	cfg := config.DefaultConfig()
	logger := slog.Default()
	srv := New(context.Background(), cfg, nil, nil, logger)
	return httptest.NewServer(srv.http.Handler)
}

func TestHealthEndpoint(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/v1/health")
	if err != nil {
		t.Fatalf("GET /v1/health: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if ct != "application/json" {
		t.Fatalf("expected Content-Type application/json, got %s", ct)
	}

	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if body["service"] != "anchored-oss" {
		t.Errorf("expected service=anchored-oss, got %s", body["service"])
	}
	if body["status"] != "ok" {
		t.Errorf("expected status=ok, got %s", body["status"])
	}
	if body["version"] != "dev" {
		t.Errorf("expected version=dev, got %s", body["version"])
	}
	if body["db_status"] != "unavailable" {
		t.Errorf("expected db_status=unavailable, got %s", body["db_status"])
	}
	if body["timestamp"] == "" {
		t.Error("expected non-empty timestamp")
	}
}

// TestProtectedRoutesRequireAuth verifies every newly-registered route
// rejects unauthenticated requests with 401. The test relies on the route
// being wired through the auth middleware; if a route is accidentally
// exposed without `authMW`, this catches it.
func TestProtectedRoutesRequireAuth(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	cases := []struct {
		method string
		path   string
	}{
		{"GET", "/v1/me"},
		{"GET", "/v1/stats"},
		{"GET", "/v1/accounts"},
		{"POST", "/v1/accounts"},
		{"GET", "/v1/teams"},
		{"POST", "/v1/teams"},
		{"GET", "/v1/teams/00000000-0000-0000-0000-000000000000"},
		{"POST", "/v1/teams/00000000-0000-0000-0000-000000000000/members"},
		{"DELETE", "/v1/teams/00000000-0000-0000-0000-000000000000/members/00000000-0000-0000-0000-000000000000"},
		{"GET", "/v1/api-keys"},
		{"GET", "/v1/me/api-keys"},
		{"POST", "/v1/me/api-keys"},
		{"DELETE", "/v1/me/api-keys/00000000-0000-0000-0000-000000000000"},
		{"GET", "/v1/audit"},
		{"GET", "/v1/projects"},
		{"GET", "/v1/projects/00000000-0000-0000-0000-000000000000"},
		{"GET", "/v1/projects/00000000-0000-0000-0000-000000000000/memories"},
		{"POST", "/v1/projects"},
		{"DELETE", "/v1/projects/00000000-0000-0000-0000-000000000000"},
		{"POST", "/v1/sync"},
		{"POST", "/api/v1/sync/push"},
		{"POST", "/api/v1/sync/pull"},
	}

	for _, c := range cases {
		t.Run(c.method+" "+c.path, func(t *testing.T) {
			req, err := http.NewRequest(c.method, ts.URL+c.path, strings.NewReader("{}"))
			if err != nil {
				t.Fatalf("new request: %v", err)
			}
			req.Header.Set("Content-Type", "application/json")
			resp, err := ts.Client().Do(req)
			if err != nil {
				t.Fatalf("do: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusUnauthorized {
				t.Errorf("expected 401, got %d", resp.StatusCode)
			}
		})
	}
}

func TestPersonalAPIKeyRoutesRequireLoginSession(t *testing.T) {
	st, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "server-personal-keys.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	org, err := st.CreateOrganization(ctx, "Session test", "session-test")
	if err != nil {
		t.Fatal(err)
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte("correct horse battery staple"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	account, err := st.CreateAccount(ctx, "session@example.test", "Session", string(passwordHash))
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AddOrgMember(ctx, org.ID, account.ID, "admin"); err != nil {
		t.Fatal(err)
	}
	issue := func(name, scope string) (string, string) {
		t.Helper()
		raw, prefix, hash, err := auth.GenerateAPIKey()
		if err != nil {
			t.Fatal(err)
		}
		key, err := st.CreateAPIKey(ctx, org.ID, account.ID, name, prefix, hash, scope, nil)
		if err != nil {
			t.Fatal(err)
		}
		return raw, key.ID
	}
	readonlyKey, targetID := issue("persistent-readonly", "readonly")
	syncKey, _ := issue("persistent-sync", "sync")
	persistentAdminKey, _ := issue("persistent-admin", "admin")

	cfg := config.DefaultConfig()
	srv := New(ctx, cfg, st, nil, slog.Default())
	ts := httptest.NewServer(srv.http.Handler)
	defer ts.Close()
	request := func(method, path, key, body string) *http.Response {
		t.Helper()
		req, err := http.NewRequest(method, ts.URL+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+key)
		req.Header.Set("Content-Type", "application/json")
		resp, err := ts.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}
	login := request(http.MethodPost, "/v1/auth/login", "", `{"email":"session@example.test","password":"correct horse battery staple"}`)
	if login.StatusCode != http.StatusOK {
		login.Body.Close()
		t.Fatalf("login = %d", login.StatusCode)
	}
	var loginResponse struct {
		APIKey string `json:"api_key"`
	}
	if err := json.NewDecoder(login.Body).Decode(&loginResponse); err != nil {
		login.Body.Close()
		t.Fatal(err)
	}
	login.Body.Close()
	if loginResponse.APIKey == "" {
		t.Fatal("login did not return a session credential")
	}
	sessionKey := loginResponse.APIKey

	for label, key := range map[string]string{"readonly": readonlyKey, "sync": syncKey, "persistent admin": persistentAdminKey} {
		for _, tc := range []struct{ method, path, body string }{
			{http.MethodGet, "/v1/me/api-keys", ""},
			{http.MethodPost, "/v1/me/api-keys", `{"name":"forbidden","scope":"sync"}`},
			{http.MethodDelete, "/v1/me/api-keys/" + targetID, ""},
		} {
			resp := request(tc.method, tc.path, key, tc.body)
			if resp.StatusCode != http.StatusForbidden {
				resp.Body.Close()
				t.Fatalf("%s %s with %s key = %d, want 403", tc.method, tc.path, label, resp.StatusCode)
			}
			resp.Body.Close()
		}
	}
	if keys, err := st.ListAPIKeysByOwner(ctx, org.ID, account.ID); err != nil || len(keys) != 2 || keys[0].RevokedAt != nil || keys[1].RevokedAt != nil {
		t.Fatalf("non-session request changed personal keys: %#v, %v", keys, err)
	}

	resp := request(http.MethodGet, "/v1/me/api-keys", sessionKey, "")
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("session GET = %d", resp.StatusCode)
	}
	resp.Body.Close()
	resp = request(http.MethodPost, "/v1/me/api-keys", sessionKey, `{"name":"allowed","scope":"sync"}`)
	if resp.StatusCode != http.StatusCreated {
		resp.Body.Close()
		t.Fatalf("session POST = %d", resp.StatusCode)
	}
	resp.Body.Close()
	resp = request(http.MethodDelete, "/v1/me/api-keys/"+targetID, sessionKey, "")
	if resp.StatusCode != http.StatusNoContent {
		resp.Body.Close()
		t.Fatalf("session DELETE = %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestSessionRolesManagePersonalKeysWithoutAdminEscalation(t *testing.T) {
	st, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "session-roles.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	org, err := st.CreateOrganization(ctx, "Session roles", "session-roles")
	if err != nil {
		t.Fatal(err)
	}
	addMember := func(email, role string) {
		t.Helper()
		hash, err := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.MinCost)
		if err != nil {
			t.Fatal(err)
		}
		account, err := st.CreateAccount(ctx, email, role, string(hash))
		if err != nil {
			t.Fatal(err)
		}
		if err := st.AddOrgMember(ctx, org.ID, account.ID, role); err != nil {
			t.Fatal(err)
		}
	}
	addMember("admin@example.test", "admin")
	addMember("sync@example.test", "sync")
	addMember("readonly@example.test", "readonly")
	for _, invite := range []struct{ email, role, token string }{
		{"invite-sync@example.test", "sync", "invite-sync-session"},
		{"invite-readonly@example.test", "readonly", "invite-readonly-session"},
	} {
		tokenHash := sha256.Sum256([]byte(invite.token))
		if _, err := st.CreateInvite(ctx, org.ID, invite.email, invite.role, invite.role, hex.EncodeToString(tokenHash[:]), time.Now().UTC().Add(time.Hour), ""); err != nil {
			t.Fatal(err)
		}
	}

	srv := New(ctx, config.DefaultConfig(), st, nil, slog.Default())
	ts := httptest.NewServer(srv.http.Handler)
	defer ts.Close()
	request := func(method, path, key, body string) *http.Response {
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
	login := func(email string) string {
		t.Helper()
		resp := request(http.MethodPost, "/v1/auth/login", "", `{"email":"`+email+`","password":"password123"}`)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("login %s = %d", email, resp.StatusCode)
		}
		var body struct {
			APIKey string `json:"api_key"`
			Scope  string `json:"scope"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.APIKey == "" {
			t.Fatalf("login %s returned no API key", email)
		}
		return body.APIKey
	}
	acceptInvite := func(token string) string {
		t.Helper()
		resp := request(http.MethodPost, "/v1/invites/accept/"+token, "", `{"password":"password123"}`)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("accept %s = %d", token, resp.StatusCode)
		}
		var body struct {
			APIKey string `json:"api_key"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		return body.APIKey
	}

	firstSyncLogin := login("sync@example.test")
	secondSyncLogin := login("sync@example.test")
	firstReadonlyLogin := login("readonly@example.test")
	secondReadonlyLogin := login("readonly@example.test")
	for label, previous := range map[string]string{"sync": firstSyncLogin, "readonly": firstReadonlyLogin} {
		resp := request(http.MethodGet, "/v1/me/api-keys", previous, "")
		if resp.StatusCode != http.StatusUnauthorized {
			resp.Body.Close()
			t.Fatalf("previous %s session after relogin = %d, want 401", label, resp.StatusCode)
		}
		resp.Body.Close()
	}

	sessions := []struct {
		label string
		key   string
		scope string
		admin bool
	}{
		{"admin login", login("admin@example.test"), "admin", true},
		{"sync login", secondSyncLogin, "sync", false},
		{"readonly login", secondReadonlyLogin, "readonly", false},
		{"sync invite", acceptInvite("invite-sync-session"), "sync", false},
		{"readonly invite", acceptInvite("invite-readonly-session"), "readonly", false},
	}
	for _, session := range sessions {
		stored, err := st.GetAPIKeyByHash(ctx, auth.HashAPIKey(session.key))
		if err != nil {
			t.Fatal(err)
		}
		if stored.Name != "session" || stored.Scope != session.scope {
			t.Fatalf("%s credential = name=%q scope=%q, want session/%s", session.label, stored.Name, stored.Scope, session.scope)
		}
		resp := request(http.MethodGet, "/v1/me/api-keys", session.key, "")
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			t.Fatalf("%s personal GET = %d, want 200", session.label, resp.StatusCode)
		}
		resp.Body.Close()

		requestedScope := "sync"
		if session.scope == "readonly" {
			resp = request(http.MethodPost, "/v1/me/api-keys", session.key, `{"name":"forbidden-`+strings.ReplaceAll(session.label, " ", "-")+`","scope":"sync"}`)
			if resp.StatusCode != http.StatusForbidden {
				resp.Body.Close()
				t.Fatalf("%s personal POST sync = %d, want 403", session.label, resp.StatusCode)
			}
			resp.Body.Close()
			requestedScope = "readonly"
		}
		resp = request(http.MethodPost, "/v1/me/api-keys", session.key, `{"name":"`+strings.ReplaceAll(session.label, " ", "-")+`","scope":"`+requestedScope+`"}`)
		if resp.StatusCode != http.StatusCreated {
			resp.Body.Close()
			t.Fatalf("%s personal POST = %d, want 201", session.label, resp.StatusCode)
		}
		var created struct {
			ID  string `json:"id"`
			Key string `json:"key"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
			resp.Body.Close()
			t.Fatal(err)
		}
		resp.Body.Close()
		if session.scope == "readonly" {
			resp = request(http.MethodPost, "/v1/sync", created.Key, `{"project_id":"00000000-0000-0000-0000-000000000000","pushes":[{}]}`)
			if resp.StatusCode != http.StatusForbidden {
				resp.Body.Close()
				t.Fatalf("%s readonly persistent key sync push = %d, want 403", session.label, resp.StatusCode)
			}
			resp.Body.Close()
		}
		resp = request(http.MethodDelete, "/v1/me/api-keys/"+created.ID, session.key, "")
		if resp.StatusCode != http.StatusNoContent {
			resp.Body.Close()
			t.Fatalf("%s personal DELETE = %d, want 204", session.label, resp.StatusCode)
		}
		resp.Body.Close()

		resp = request(http.MethodGet, "/v1/api-keys", session.key, "")
		want := http.StatusForbidden
		if session.admin {
			want = http.StatusOK
		}
		if resp.StatusCode != want {
			resp.Body.Close()
			t.Fatalf("%s admin GET = %d, want %d", session.label, resp.StatusCode, want)
		}
		resp.Body.Close()
	}

	adminKey := sessions[0].key
	for _, tc := range []struct{ path, body string }{
		{"/v1/api-keys", `{"name":"session","scope":"sync","account_id":"ignored"}`},
		{"/v1/me/api-keys", `{"name":"session","scope":"sync"}`},
	} {
		resp := request(http.MethodPost, tc.path, adminKey, tc.body)
		if resp.StatusCode != http.StatusBadRequest {
			resp.Body.Close()
			t.Fatalf("session-name create %s = %d, want 400", tc.path, resp.StatusCode)
		}
		resp.Body.Close()
	}
}

// TestSPARoutes covers the embedded dashboard's serving behavior: root
// returns HTML, /v1/* falls through to the API 404 JSON, and unknown
// non-API paths fall back to index.html.
func TestSPARoutes(t *testing.T) {
	// A checkout without `make web-build` embeds only dist/.keep, and the
	// server then serves no dashboard. CI builds the bundle first.
	if _, err := fs.Stat(web.DistFS, "dist/index.html"); err != nil {
		t.Skip("dashboard bundle not built (make web-build)")
	}
	ts := newTestServer(t)
	defer ts.Close()

	t.Run("root returns HTML with CSP", func(t *testing.T) {
		resp, err := http.Get(ts.URL + "/")
		if err != nil {
			t.Fatalf("GET /: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
		if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
			t.Errorf("expected HTML, got %q", ct)
		}
		if resp.Header.Get("Content-Security-Policy") == "" {
			t.Error("expected CSP header on index.html")
		}
	})

	t.Run("unknown v1 path returns 404 JSON", func(t *testing.T) {
		resp, err := http.Get(ts.URL + "/v1/nonexistent")
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", resp.StatusCode)
		}
		if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
			t.Errorf("expected JSON, got %q", ct)
		}
	})

	t.Run("unknown SPA path falls back to index", func(t *testing.T) {
		resp, err := http.Get(ts.URL + "/teams/some-team")
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
		if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
			t.Errorf("expected HTML, got %q", ct)
		}
	})
}
