package curation

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/jholhewres/anchored_oss/internal/config"
	"github.com/jholhewres/anchored_oss/internal/model"
	"github.com/jholhewres/anchored_oss/internal/store"
)

func quietLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func curationStatus(t *testing.T, st *store.SQLiteStore, id string) (string, map[string]any) {
	t.Helper()
	m, err := st.GetMemoryByID(context.Background(), id)
	if err != nil {
		t.Fatalf("get %s: %v", id, err)
	}
	meta, _ := m.Metadata.(map[string]any)
	status, _ := meta["curation_status"].(string)
	return status, meta
}

// TestCurationV2_MarksStaleContradictionAndRecurates drives the curation worker
// against a real SQLite store and asserts the v2 behaviour: staleness +
// contradiction marking (advisory, never deleting), the health contradictions
// count becoming real, and the curation_version<2 re-curation path.
func TestCurationV2_MarksStaleContradictionAndRecurates(t *testing.T) {
	st, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "cur.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	ctx := context.Background()

	org, err := st.CreateOrganization(ctx, "Acme", "acme")
	if err != nil {
		t.Fatalf("org: %v", err)
	}
	acct, err := st.CreateAccount(ctx, "dev@example.com", "Dev", "x")
	if err != nil {
		t.Fatalf("account: %v", err)
	}
	proj, err := st.CreateProject(ctx, org.ID, "Repo", "repo", "hk1", "hk1",
		"https://github.example.com/org/repo.git", acct.ID, "service")
	if err != nil {
		t.Fatalf("project: %v", err)
	}

	now := time.Now().UTC()
	seed := func(id, content string, kw []string, age time.Duration, meta map[string]any) {
		ts := now.Add(-age)
		m := &model.Memory{
			ID: id, ProjectID: proj.ID, Category: "decision",
			Content: content, ContentHash: "h" + id, Keywords: kw,
			AuthorID: acct.ID, AuthorName: "Dev", Source: "mcp",
			CreatedAt: ts, UpdatedAt: ts, Metadata: meta,
		}
		if err := st.UpsertMemory(ctx, m); err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
	}

	seed("m-stale", "we standardized error handling on wrapped sentinel errors across the sync engine",
		[]string{"errors", "sync"}, 200*24*time.Hour, nil)
	seed("m-pinned", "the production deploy uses pm2 with a systemd unit guarding restarts",
		[]string{"deploy", "pm2"}, 200*24*time.Hour, map[string]any{"pinned": true})
	seed("m-fresh", "the dashboard reads project health from the aggregated memory-health endpoint",
		[]string{"dashboard", "health"}, 1*time.Hour, nil)
	seed("m-pnpm", "we use pnpm for the monorepo tooling workspace and lockfile",
		[]string{"pnpm", "monorepo", "tooling"}, 20*24*time.Hour, nil)
	seed("m-npm", "we no longer use pnpm; switched the monorepo tooling to npm for ci",
		[]string{"pnpm", "monorepo", "tooling", "npm"}, 1*24*time.Hour, nil)

	liveBefore := len(mustList(t, st, proj.ID))

	cfg := config.DefaultConfig()
	w := NewWorker(cfg, st, nil, quietLogger())
	if err := w.processBatch(ctx); err != nil {
		t.Fatalf("process batch: %v", err)
	}

	if s, meta := curationStatus(t, st, "m-stale"); s != "stale" {
		t.Fatalf("m-stale status = %q (meta %+v), want stale", s, meta)
	}
	if s, _ := curationStatus(t, st, "m-pinned"); s != "ok" {
		t.Fatalf("m-pinned status = %q, want ok (pinned is exempt from staleness)", s)
	}
	if s, _ := curationStatus(t, st, "m-fresh"); s != "ok" {
		t.Fatalf("m-fresh status = %q, want ok", s)
	}
	if s, meta := curationStatus(t, st, "m-npm"); s != "contradiction_candidate" {
		t.Fatalf("m-npm status = %q (meta %+v), want contradiction_candidate", s, meta)
	} else if c, _ := meta["contradicts"].(string); c == "" {
		t.Fatalf("m-npm missing contradicts pointer: %+v", meta)
	}

	// Every processed memory is stamped with curation_version 2.
	if _, meta := curationStatus(t, st, "m-fresh"); meta["curation_version"] != float64(2) {
		t.Fatalf("m-fresh curation_version = %v, want 2", meta["curation_version"])
	}

	// Advisory only: nothing was deleted.
	if liveAfter := len(mustList(t, st, proj.ID)); liveAfter != liveBefore {
		t.Fatalf("curation deleted memories: before %d after %d", liveBefore, liveAfter)
	}

	// Health reflects real contradiction + stale counts.
	h, err := st.GetProjectMemoryHealth(ctx, proj.ID)
	if err != nil {
		t.Fatalf("health: %v", err)
	}
	if h.Counts.Contradictions < 1 {
		t.Fatalf("health contradictions = %d, want >= 1", h.Counts.Contradictions)
	}
	if h.Counts.Stale < 1 {
		t.Fatalf("health stale = %d, want >= 1", h.Counts.Stale)
	}

	// Re-curation: simulate a memory last curated by a v1 worker, then confirm
	// the drained-queue path re-enqueues and re-marks it.
	if err := st.UpdateMemoryMetadata(ctx, "m-fresh", map[string]any{"curation_version": 1}); err != nil {
		t.Fatalf("downgrade version: %v", err)
	}
	// Queue is fully drained (all done); this batch should trigger re-curation.
	if err := w.processBatch(ctx); err != nil {
		t.Fatalf("re-curation enqueue batch: %v", err)
	}
	// Next batch processes the re-enqueued memory back to v2.
	if err := w.processBatch(ctx); err != nil {
		t.Fatalf("re-curation process batch: %v", err)
	}
	if _, meta := curationStatus(t, st, "m-fresh"); meta["curation_version"] != float64(2) {
		t.Fatalf("after re-curation m-fresh version = %v, want 2", meta["curation_version"])
	}

	// ListMemoriesByCurationStatus surfaces the marked rows for the dashboard.
	stale, err := st.ListMemoriesByCurationStatus(ctx, proj.ID, "stale", 50)
	if err != nil {
		t.Fatalf("list stale: %v", err)
	}
	if len(stale) != 1 || stale[0].ID != "m-stale" {
		t.Fatalf("list stale = %+v, want [m-stale]", stale)
	}
}

func mustList(t *testing.T, st *store.SQLiteStore, projectID string) []*model.Memory {
	t.Helper()
	ms, err := st.ListProjectMemoriesSince(context.Background(), projectID, time.Time{})
	if err != nil {
		t.Fatalf("list memories: %v", err)
	}
	return ms
}
