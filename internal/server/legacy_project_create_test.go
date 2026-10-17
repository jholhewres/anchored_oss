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
	projectpkg "github.com/jholhewres/anchored_oss/internal/project"
	"github.com/jholhewres/anchored_oss/internal/store"
	"golang.org/x/crypto/bcrypt"
)

// TestLegacyClientCreatesProjectByRemoteKey replays what `anchored remote sync
// --all` (client v0.20.x) sends: POST /v1/projects with name, slug and
// remote_key, no repo_url. Released clients depend on it, so it must keep
// working and stay idempotent on remote_key, while the dashboard path that
// sends repo_url keeps its stricter rules.
func TestLegacyClientCreatesProjectByRemoteKey(t *testing.T) {
	st, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "legacy-create.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	org, err := st.CreateOrganization(ctx, "Legacy", "legacy")
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
	do := func(method, path, key, body string) (int, map[string]any, []map[string]any) {
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
		var raw json.RawMessage
		_ = json.NewDecoder(resp.Body).Decode(&raw)
		var obj map[string]any
		var list []map[string]any
		_ = json.Unmarshal(raw, &obj)
		_ = json.Unmarshal(raw, &list)
		return resp.StatusCode, obj, list
	}

	_, login, _ := do(http.MethodPost, "/v1/auth/login", "", `{"email":"admin@example.test","password":"password123"}`)
	key, _ := login["api_key"].(string)
	if key == "" {
		t.Fatalf("login returned no key: %v", login)
	}

	repoKey := projectpkg.DeriveRemoteKey("https://github.com/acme/anchored.git")
	body := `{"name":"anchored","slug":"anchored","remote_key":"` + repoKey + `"}`
	code, first, _ := do(http.MethodPost, "/v1/projects", key, body)
	if code != http.StatusCreated {
		t.Fatalf("legacy create by remote_key = %d %v, want 201", code, first)
	}
	if first["remote_key"] != repoKey {
		t.Fatalf("created remote_key = %v, want %s", first["remote_key"], repoKey)
	}

	code, again, _ := do(http.MethodPost, "/v1/projects", key, body)
	if code != http.StatusOK || again["id"] != first["id"] {
		t.Fatalf("repeated legacy create = %d %v, want 200 with id %v", code, again, first["id"])
	}

	code, local, _ := do(http.MethodPost, "/v1/projects", key, `{"name":"notes","slug":"notes","remote_key":"local:notes"}`)
	if code != http.StatusCreated || local["remote_key"] != "local:notes" {
		t.Fatalf("legacy create of a project without origin = %d %v, want 201 keyed local:notes", code, local)
	}

	// Clients before v0.20.2 keyed origin-less projects as <slug>-<unix time>.
	code, older, _ := do(http.MethodPost, "/v1/projects", key, `{"name":"scratch","slug":"scratch","remote_key":"scratch-1760000000"}`)
	if code != http.StatusCreated || older["remote_key"] != "scratch-1760000000" {
		t.Fatalf("create with a pre-0.20.2 key = %d %v, want 201", code, older)
	}

	_, _, projects := do(http.MethodGet, "/v1/projects", key, "")
	keys := map[any]bool{}
	for _, p := range projects {
		keys[p["remote_key"]] = true
	}
	if !keys[repoKey] || !keys["local:notes"] || !keys["scratch-1760000000"] {
		t.Fatalf("GET /v1/projects keys = %v, want every legacy project", keys)
	}

	for _, body := range []string{
		`{"name":"bare","slug":"bare"}`,
		`{"name":"bad","slug":"bad","remote_key":"has spaces"}`,
		`{"name":"manual","slug":"manual","remote_key":"manual:client"}`,
	} {
		if code, _, _ := do(http.MethodPost, "/v1/projects", key, body); code != http.StatusBadRequest {
			t.Fatalf("create %s = %d, want 400", body, code)
		}
	}
}
