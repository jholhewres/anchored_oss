//go:build e2e

// Package e2e drives the REAL client binary against an in-process server:
// temp SQLite, programmatic org/keys, isolated $HOME per scenario, and a
// throwaway git repo with a generic origin. Build the client from the sibling
// checkout (../../../anchored) or point ANCHORED_CLIENT_BIN at a binary.
package e2e

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jholhewres/anchored_oss/internal/auth"
	"github.com/jholhewres/anchored_oss/internal/config"
	"github.com/jholhewres/anchored_oss/internal/server"
	"github.com/jholhewres/anchored_oss/internal/store"
)

const testOrigin = "git@github.example.com:org/api-service.git"

// env is the harness for one scenario: a running server, its store handle,
// minted keys, and an isolated client HOME.
type env struct {
	store    *store.SQLiteStore
	baseURL  string
	adminKey string
	syncKey  string
	orgID    string
	acctID   string
	home     string
	bin      string
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("free port: %v", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// startServer boots the real HTTP server on a random port with a temp SQLite
// store and waits for /v1/health.
func startServer(t *testing.T) (*store.SQLiteStore, string) {
	t.Helper()
	st, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "e2e.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	cfg := config.DefaultConfig()
	port := freePort(t)
	cfg.Server.Address = fmt.Sprintf("127.0.0.1:%d", port)
	cfg.RateLimit.Enabled = false
	cfg.Curation.WorkerEnabled = false

	ctx, cancel := context.WithCancel(context.Background())
	srv := server.New(ctx, cfg, st, nil, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})))
	go func() { _ = srv.Start() }()
	t.Cleanup(func() {
		shCtx, shCancel := context.WithTimeout(context.Background(), 3*time.Second)
		_ = srv.Shutdown(shCtx)
		shCancel()
		cancel()
	})

	base := "http://" + cfg.Server.Address
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", cfg.Server.Address, 200*time.Millisecond)
		if err == nil {
			conn.Close()
			return st, base
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("server did not start")
	return nil, ""
}

// seedOrg creates the org, an account with default-team membership, and two
// API keys (admin + sync) the way the dashboard would.
func seedOrg(t *testing.T, st *store.SQLiteStore) (orgID, acctID, adminKey, syncKey string) {
	t.Helper()
	ctx := context.Background()
	org, err := st.CreateOrganization(ctx, "E2E Org", "e2e-org")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	acct, err := st.CreateAccount(ctx, "dev@e2e.example.com", "E2E Dev", "x")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	if err := st.AddOrgMember(ctx, org.ID, acct.ID, "admin"); err != nil {
		t.Fatalf("add org member: %v", err)
	}
	if err := st.EnsureDefaultTeamMembership(ctx, org.ID, acct.ID); err != nil {
		t.Fatalf("default team: %v", err)
	}

	mint := func(scope string) string {
		full, prefix, hash, err := auth.GenerateAPIKey()
		if err != nil {
			t.Fatalf("generate key: %v", err)
		}
		if _, err := st.CreateAPIKey(ctx, org.ID, acct.ID, "e2e-"+scope, prefix, hash, scope, nil); err != nil {
			t.Fatalf("create key: %v", err)
		}
		return full
	}
	return org.ID, acct.ID, mint("admin"), mint("sync")
}

var (
	buildOnce sync.Once
	builtBin  string
	buildErr  error
)

// clientBin returns the anchored client binary: $ANCHORED_CLIENT_BIN if set,
// else a one-time CGO build of the sibling checkout.
func clientBin(t *testing.T) string {
	t.Helper()
	if env := os.Getenv("ANCHORED_CLIENT_BIN"); env != "" {
		return env
	}
	buildOnce.Do(func() {
		src, err := filepath.Abs(filepath.Join("..", "..", "..", "anchored"))
		if err != nil {
			buildErr = err
			return
		}
		if _, err := os.Stat(filepath.Join(src, "go.mod")); err != nil {
			buildErr = fmt.Errorf("client checkout not found at %s (set ANCHORED_CLIENT_BIN)", src)
			return
		}
		out := filepath.Join(os.TempDir(), "anchored-e2e-client")
		cmd := exec.Command("go", "build", "-o", out, "./cmd/anchored/")
		cmd.Dir = src
		cmd.Env = append(os.Environ(),
			"CGO_ENABLED=1", "CGO_CFLAGS=-DSQLITE_ENABLE_FTS5", "CGO_LDFLAGS=-lm")
		if outB, err := cmd.CombinedOutput(); err != nil {
			buildErr = fmt.Errorf("build client: %v\n%s", err, outB)
			return
		}
		builtBin = out
	})
	if buildErr != nil {
		t.Fatalf("client binary: %v", buildErr)
	}
	return builtBin
}

// newEnv assembles a full scenario environment. The client HOME starts with
// only the embedding opt-out (no ONNX downloads in tests); remote config is
// added by each scenario via the real `anchored remote configure`.
func newEnv(t *testing.T) *env {
	t.Helper()
	st, base := startServer(t)
	orgID, acctID, adminKey, syncKey := seedOrg(t, st)

	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".anchored"), 0o755); err != nil {
		t.Fatalf("mkdir home: %v", err)
	}
	cfgYAML := "embedding:\n  provider: none\n"
	if err := os.WriteFile(filepath.Join(home, ".anchored", "config.yaml"), []byte(cfgYAML), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	return &env{
		store: st, baseURL: base,
		orgID: orgID, acctID: acctID,
		adminKey: adminKey, syncKey: syncKey,
		home: home, bin: clientBin(t),
	}
}

// run executes the client binary with the scenario HOME, returning combined
// output and exit code.
func (e *env) run(t *testing.T, dir string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(e.bin, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = append(os.Environ(), "HOME="+e.home)
	out, err := cmd.CombinedOutput()
	code := 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		code = exitErr.ExitCode()
	} else if err != nil {
		t.Fatalf("run %v: %v\n%s", args, err, out)
	}
	return string(out), code
}

// configureRemote runs the real onboarding command against the test server.
func (e *env) configureRemote(t *testing.T, key string) {
	t.Helper()
	out, code := e.run(t, "",
		"remote", "configure", "--server", e.baseURL, "--key", key, "--name", "e2e")
	if code != 0 {
		t.Fatalf("remote configure failed (%d):\n%s", code, out)
	}
}

// newRepo creates a git repo with the given origin and one commit-ready file.
func newRepo(t *testing.T, origin string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	repo := filepath.Join(t.TempDir(), "repo")
	for _, args := range [][]string{
		{"init", "-q", repo},
		{"-C", repo, "remote", "add", "origin", origin},
	} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v (%s)", args, err, out)
		}
	}
	return repo
}

// countProjectMemories asks the store how many live memories a project has.
func countProjectMemories(t *testing.T, st *store.SQLiteStore, projectID string) int {
	t.Helper()
	_, total, err := st.ListMemoriesPaginated(context.Background(), projectID, 1, 0, "")
	if err != nil {
		t.Fatalf("list memories: %v", err)
	}
	return total
}

// allMemoryCount counts live memories across every project in the org.
func allMemoryCount(t *testing.T, e *env) int {
	t.Helper()
	projects, err := e.store.ListProjectsByTeamAccess(context.Background(), e.acctID)
	if err != nil {
		t.Fatalf("list projects: %v", err)
	}
	total := 0
	for _, p := range projects {
		total += countProjectMemories(t, e.store, p.ID)
	}
	return total
}

func readFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	return string(b), err
}

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}

func mustContain(t *testing.T, out string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(out, w) {
			t.Errorf("output missing %q\n--- output ---\n%s", w, out)
		}
	}
}
