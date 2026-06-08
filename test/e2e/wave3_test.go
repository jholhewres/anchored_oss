//go:build e2e

package e2e

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jholhewres/anchored_oss/internal/config"
	"github.com/jholhewres/anchored_oss/internal/curation"
	"github.com/jholhewres/anchored_oss/internal/model"

	_ "modernc.org/sqlite"
)

// TestEvalGates_ExitZero runs the client's three eval gates as real processes
// and asserts each passes (exit 0). This is the cross-process half of Fase C:
// the embedded fixtures + production retrieval path hold up in the shipped
// binary, not just in `go test`.
func TestEvalGates_ExitZero(t *testing.T) {
	e := newEnv(t)
	for _, sub := range []string{"recall", "sync-safety", "identity"} {
		out, code := e.run(t, "", "eval", sub)
		if code != 0 {
			t.Fatalf("eval %s exited %d:\n%s", sub, code, out)
		}
		if !strings.Contains(out, "[PASS]") {
			t.Fatalf("eval %s did not report PASS:\n%s", sub, out)
		}
	}
}

// TestWorkingSetFeed_PersistsAcrossProcess drives the REAL posttooluse hook
// binary with a Write event and verifies the touched file is recorded in the
// client's working_sets table — the cross-process half of Fase B (the boost
// ranking itself is unit-tested in pkg/memory). The hook must also stay
// fail-safe (runStdin fails the test on any non-zero exit).
func TestWorkingSetFeed_PersistsAcrossProcess(t *testing.T) {
	e := newEnv(t)
	repo := newRepo(t, testOrigin)

	// Migrate the client store (the lightweight hook context doesn't run
	// migrations; a throwaway save is the cheapest way to create the schema,
	// including working_sets from migration 015).
	if out, code := e.run(t, repo, "save", "fact: initialize the local store for this scenario", "--category", "fact"); code != 0 {
		t.Fatalf("seed save: %d\n%s", code, out)
	}

	payload := `{"session_id":"ws-sess","cwd":"` + repo + `",` +
		`"tool_name":"Write","tool_input":{"file_path":"pkg/sync/client.go","content":"package sync"},` +
		`"tool_response":{"ok":true}}`
	if out := e.runStdin(t, repo, payload, "hook", "posttooluse"); !strings.Contains(out, `"recorded":true`) {
		t.Fatalf("posttooluse should record the event:\n%s", out)
	}

	// Inspect the client DB directly: the working set for the session must now
	// carry the written file.
	dbPath := filepath.Join(e.home, ".anchored", "data", "anchored.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open client db: %v", err)
	}
	defer db.Close()

	var files string
	err = db.QueryRow(`SELECT files FROM working_sets WHERE session_id = ?`, "ws-sess").Scan(&files)
	if err != nil {
		t.Fatalf("read working set (the hook should have created it): %v", err)
	}
	if !strings.Contains(files, "pkg/sync/client.go") {
		t.Fatalf("working set should contain the written file, got %q", files)
	}
}

// TestCurationToHealth_ContradictionReflected seeds a contradiction pair and a
// stale memory, runs one curation pass, then reads the HTTP memory-health
// endpoint and asserts the worker's marks surface as real counts — the
// worker → store → API path of Server Fase E.
func TestCurationToHealth_ContradictionReflected(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	proj := seedProjectForRepo(t, e, "Curated", "curated", "git@github.example.com:org/curated.git")

	now := time.Now().UTC()
	seed := func(id, content string, kw []string, age time.Duration) {
		ts := now.Add(-age)
		if err := e.store.UpsertMemory(ctx, &model.Memory{
			ID: id, ProjectID: proj.ID, Category: "decision",
			Content: content, ContentHash: "h" + id, Keywords: kw,
			AuthorID: e.acctID, AuthorName: "Dev", Source: "mcp",
			CreatedAt: ts, UpdatedAt: ts,
		}); err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
	}
	seed("c-stale", "we standardized error wrapping with sentinel errors across the sync engine long ago",
		[]string{"errors", "sync"}, 200*24*time.Hour)
	seed("c-pnpm", "we use pnpm for the monorepo tooling workspace and lockfile management",
		[]string{"pnpm", "monorepo", "tooling"}, 20*24*time.Hour)
	seed("c-npm", "we no longer use pnpm; switched the monorepo tooling to npm for ci builds",
		[]string{"pnpm", "monorepo", "tooling", "npm"}, 1*24*time.Hour)

	// One deterministic curation pass (no embedder needed for marking).
	w := curation.NewWorker(config.DefaultConfig(), e.store, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := w.RunOnce(ctx); err != nil {
		t.Fatalf("curation run: %v", err)
	}

	// Read the health endpoint as admin.
	req, _ := http.NewRequest("GET", e.baseURL+"/v1/projects/"+proj.ID+"/memory-health", nil)
	req.Header.Set("Authorization", "Bearer "+e.adminKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("health request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("health status %d: %s", resp.StatusCode, body)
	}
	var health struct {
		Counts struct {
			Stale          int64 `json:"stale"`
			Contradictions int64 `json:"contradictions"`
		} `json:"counts"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		t.Fatalf("decode health: %v", err)
	}
	if health.Counts.Contradictions < 1 {
		t.Fatalf("expected curation to mark >=1 contradiction candidate, health reports %d", health.Counts.Contradictions)
	}
	if health.Counts.Stale < 1 {
		t.Fatalf("expected >=1 stale memory in health, got %d", health.Counts.Stale)
	}
}
