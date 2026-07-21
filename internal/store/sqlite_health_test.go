package store

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/jholhewres/anchored_oss/internal/model"
)

func seedHealthProject(t *testing.T, st *SQLiteStore) (orgID, projectID string) {
	t.Helper()
	ctx := context.Background()
	org, err := st.CreateOrganization(ctx, "Acme", "acme")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	acct, err := st.CreateAccount(ctx, "dev@example.com", "Dev", "x")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	proj, err := st.CreateProject(ctx, org.ID, "Repo", "repo", "hk1", "hk1",
		"https://github.example.com/org/repo.git", acct.ID, "service")
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	return org.ID, proj.ID
}

func seedMemory(t *testing.T, st *SQLiteStore, projectID, id, source string, age time.Duration, metadata any) {
	t.Helper()
	ts := time.Now().UTC().Add(-age)
	m := &model.Memory{
		ID: id, ProjectID: projectID, Category: "fact",
		Content: "content " + id, ContentHash: "h" + id,
		AuthorName: "dev", Source: source,
		CreatedAt: ts, UpdatedAt: ts, Metadata: metadata,
	}
	if err := st.UpsertMemory(context.Background(), m); err != nil {
		t.Fatalf("seed memory %s: %v", id, err)
	}
}

// TestGetProjectMemoryHealth_GoldenCounts seeds a known mix and asserts exact
// counters, score and recommendations.
func TestGetProjectMemoryHealth_GoldenCounts(t *testing.T) {
	st, err := NewSQLiteStore(filepath.Join(t.TempDir(), "h.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()
	ctx := context.Background()
	_, projID := seedHealthProject(t, st)

	// 6 live memories: 1 low_signal, 1 near_duplicate, 1 stale (200d old,
	// unpinned), 1 old-but-pinned (NOT stale), 2 plain fresh.
	seedMemory(t, st, projID, "m1", "mcp", time.Hour, map[string]any{"curation_status": "low_signal"})
	seedMemory(t, st, projID, "m2", "mcp", time.Hour, map[string]any{"curation_status": "near_duplicate"})
	seedMemory(t, st, projID, "m3", "sync", 200*24*time.Hour, nil)
	seedMemory(t, st, projID, "m4", "sync", 200*24*time.Hour, map[string]any{"pinned": true})
	seedMemory(t, st, projID, "m5", "mcp", time.Hour, nil)
	seedMemory(t, st, projID, "m6", "import", 40*24*time.Hour, nil)

	h, err := st.GetProjectMemoryHealth(ctx, projID)
	if err != nil {
		t.Fatalf("health: %v", err)
	}

	want := model.HealthCounts{
		Live: 6, LowSignal: 1, NearDuplicate: 1, Stale: 1,
		Contradictions: 0, MissingEmbeddings: 6, // none embedded in this seed
	}
	if h.Counts != want {
		t.Fatalf("counts mismatch:\n got %+v\nwant %+v", h.Counts, want)
	}

	// score = 1 - (low_signal + near_duplicate + contradictions)/live
	//       = 1 - (1 + 1 + 0)/6 = 0.67. Age-based stale is not in the score.
	if h.Score != 0.67 {
		t.Fatalf("score: got %v want 0.67", h.Score)
	}
	if len(h.BySource) == 0 || h.BySource[0].Name != "mcp" || h.BySource[0].Count != 3 {
		t.Fatalf("by_source top: %+v", h.BySource)
	}
	if len(h.Anomalies) != 0 {
		t.Fatalf("no anomaly expected on small seed, got %+v", h.Anomalies)
	}
	// Reindex recommendation counts only embeddable memories missing an
	// embedding: 6 missing - 1 low_signal - 1 near_duplicate = 4 eligible.
	foundReindex := false
	for _, r := range h.Recommendations {
		if r == "Run reindex: 4 memories missing embeddings" {
			foundReindex = true
		}
	}
	if !foundReindex {
		t.Fatalf("missing reindex recommendation, got %+v", h.Recommendations)
	}
}

// TestGetProjectMemoryHealth_VolumeSpike reproduces the 7k-dump incident: one
// source pushing 7000 memories in a day against a ~40/day baseline must flag a
// volume_spike anomaly with baseline and count populated.
func TestGetProjectMemoryHealth_VolumeSpike(t *testing.T) {
	st, err := NewSQLiteStore(filepath.Join(t.TempDir(), "spike.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()
	ctx := context.Background()
	_, projID := seedHealthProject(t, st)

	// Baseline: ~40/day for the prior 29 days (batched for speed).
	baseline := make([]*model.Memory, 0, 29*40)
	for d := 1; d <= 29; d++ {
		for i := 0; i < 40; i++ {
			// Extra -1h keeps the closest baseline row safely outside the 24h
			// window (timestamps truncate to seconds; the boundary is >=).
			ts := time.Now().UTC().Add(-time.Duration(d)*24*time.Hour - time.Hour - time.Duration(i)*time.Minute)
			id := fmt.Sprintf("b-%d-%d", d, i)
			baseline = append(baseline, &model.Memory{
				ID: id, ProjectID: projID, Category: "fact",
				Content: "baseline " + id, ContentHash: "hb" + id,
				AuthorName: "dev", Source: "sync",
				CreatedAt: ts, UpdatedAt: ts,
			})
		}
	}
	// Spike: 7000 in the last 24h from the same source.
	spike := make([]*model.Memory, 0, 7000)
	for i := 0; i < 7000; i++ {
		ts := time.Now().UTC().Add(-time.Duration(i) * time.Second)
		id := fmt.Sprintf("s-%d", i)
		spike = append(spike, &model.Memory{
			ID: id, ProjectID: projID, Category: "fact",
			Content: "spike " + id, ContentHash: "hs" + id,
			AuthorName: "dev", Source: "sync",
			CreatedAt: ts, UpdatedAt: ts,
		})
	}
	if err := st.UpsertMemories(ctx, baseline); err != nil {
		t.Fatalf("seed baseline: %v", err)
	}
	if err := st.UpsertMemories(ctx, spike); err != nil {
		t.Fatalf("seed spike: %v", err)
	}

	h, err := st.GetProjectMemoryHealth(ctx, projID)
	if err != nil {
		t.Fatalf("health: %v", err)
	}
	if len(h.Anomalies) != 1 {
		t.Fatalf("want exactly 1 anomaly, got %+v", h.Anomalies)
	}
	a := h.Anomalies[0]
	if a.Type != "volume_spike" || a.Source != "sync" || a.Window != "24h" {
		t.Fatalf("anomaly shape: %+v", a)
	}
	if a.Count != 7000 {
		t.Fatalf("anomaly count: got %d want 7000", a.Count)
	}
	if a.Baseline < 39 || a.Baseline > 41 {
		t.Fatalf("anomaly baseline: got %v want ~40", a.Baseline)
	}
	// The spike must surface as the first recommendation (highest count).
	if len(h.Recommendations) == 0 {
		t.Fatal("expected recommendations")
	}
}

// TestGetOrgMemoryHealth aggregates across the org's projects.
func TestGetOrgMemoryHealth(t *testing.T) {
	st, err := NewSQLiteStore(filepath.Join(t.TempDir(), "org.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()
	ctx := context.Background()
	orgID, projID := seedHealthProject(t, st)

	seedMemory(t, st, projID, "m1", "mcp", time.Hour, nil)
	seedMemory(t, st, projID, "m2", "mcp", time.Hour, nil)

	h, err := st.GetOrgMemoryHealth(ctx, orgID)
	if err != nil {
		t.Fatalf("org health: %v", err)
	}
	if h.Counts.Live != 2 {
		t.Fatalf("org live: got %d want 2", h.Counts.Live)
	}
}
