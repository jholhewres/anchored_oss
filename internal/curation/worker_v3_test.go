package curation

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/jholhewres/anchored_oss/internal/config"
	"github.com/jholhewres/anchored_oss/internal/model"
	"github.com/jholhewres/anchored_oss/internal/store"
)

// TestCurationV3_MarksConsolidationCandidate drives the v3 advisory marking:
// a near-duplicate cluster of 4 memories gets its canonical flagged as a
// consolidation candidate (consolidation_candidate + member count) — marking
// only, nothing deleted or synthesized server-side.
func TestCurationV3_MarksConsolidationCandidate(t *testing.T) {
	st, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "v3.db"))
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
	proj, err := st.CreateProject(ctx, org.ID, "Repo", "repo", "hk3", "hk3",
		"https://github.example.com/org/repo.git", acct.ID, "service")
	if err != nil {
		t.Fatalf("project: %v", err)
	}

	now := time.Now().UTC()
	// Near-identical contents so fivegramJaccard crosses the near-dup
	// threshold; the oldest becomes the cluster canonical.
	base := "the production deploy pipeline requires a semver tag plus an explicit reason before the manual approval gate runs"
	for i, id := range []string{"c-canon", "c-m1", "c-m2", "c-m3"} {
		ts := now.Add(-time.Duration(4-i) * time.Hour)
		m := &model.Memory{
			ID: id, ProjectID: proj.ID, Category: "decision",
			Content: base, ContentHash: "h" + id,
			Keywords: []string{"deploy", "pipeline"},
			AuthorID: acct.ID, AuthorName: "Dev", Source: "mcp",
			CreatedAt: ts, UpdatedAt: ts,
		}
		if err := st.UpsertMemory(ctx, m); err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
	}

	w := NewWorker(config.DefaultConfig(), st, nil, quietLogger())
	if err := w.processBatch(ctx); err != nil {
		t.Fatalf("process batch: %v", err)
	}

	_, meta := curationStatus(t, st, "c-canon")
	if meta["consolidation_candidate"] != true {
		t.Fatalf("canonical not flagged as consolidation candidate: %+v", meta)
	}
	if n, _ := meta["consolidation_members"].(float64); n < 3 {
		t.Fatalf("consolidation_members = %v, want >= 3", meta["consolidation_members"])
	}

	// Members point at the canonical and nothing was deleted.
	for _, id := range []string{"c-m1", "c-m2", "c-m3"} {
		s, m := curationStatus(t, st, id)
		if s != "near_duplicate" {
			t.Fatalf("%s status = %q, want near_duplicate (%+v)", id, s, m)
		}
	}

	health, err := st.GetProjectMemoryHealth(ctx, proj.ID)
	if err != nil {
		t.Fatalf("health: %v", err)
	}
	if health.Counts.ConsolidationCandidates != 1 {
		t.Fatalf("health consolidation_candidates = %d, want 1", health.Counts.ConsolidationCandidates)
	}
}
