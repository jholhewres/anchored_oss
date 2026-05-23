package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jholhewres/anchored_oss/internal/config"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	cfg := config.DefaultConfig()
	logger := slog.Default()
	srv := New(cfg, nil, logger)
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

// TestSPARoutes covers the embedded dashboard's serving behavior: root
// returns HTML, /v1/* falls through to the API 404 JSON, and unknown
// non-API paths fall back to index.html.
func TestSPARoutes(t *testing.T) {
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
