//go:build integration

package store

import (
	"context"
	"os"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jholhewres/anchored_oss/internal/ai/embeddings"
	"github.com/jholhewres/anchored_oss/internal/model"
)

var vecSeq uint64

// uniq yields a per-run-unique suffix so repeated integration runs against the
// same persistent database don't collide on unique slugs/emails.
func uniq() string {
	return strconv.FormatInt(time.Now().UnixNano(), 36) + "-" + strconv.FormatUint(atomic.AddUint64(&vecSeq, 1), 10)
}

// TestVectorSearch_Postgres exercises the pgvector path end-to-end: migration
// 011 (extension + columns + HNSW index), UpdateMemoryEmbedding, and cosine
// KNN ranking. Requires a pgvector-enabled Postgres; set ANCHORED_TEST_DSN,
// e.g. ANCHORED_TEST_DSN=postgres://anchored:anchored@localhost:55433/anchored_oss?sslmode=disable
func TestVectorSearch_Postgres(t *testing.T) {
	dsn := os.Getenv("ANCHORED_TEST_DSN")
	if dsn == "" {
		t.Skip("ANCHORED_TEST_DSN not set; skipping pgvector integration test")
	}

	st, err := NewPostgresStore(PoolConfig{DSN: dsn})
	if err != nil {
		t.Fatalf("open postgres store (migrations include pgvector 011): %v", err)
	}
	defer st.Close()

	ctx := context.Background()
	u := uniq()
	org, err := st.CreateOrganization(ctx, "VecOrg", "vecorg-"+u)
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	acc, err := st.CreateAccount(ctx, "vec-"+u+"@example.com", "Vec", "x")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	proj, err := st.CreateProject(ctx, org.ID, "VecProj", "vecproj-"+u, "vec-remote-"+u, acc.ID, "service")
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	emb := embeddings.NewLocalEmbedder(384)
	now := time.Now().UTC()
	seed := []struct {
		id, content string
	}{
		{"m-db", "database migrations are applied automatically on server startup"},
		{"m-fe", "the dashboard frontend uses a custom design system without shadcn"},
		{"m-rl", "rate limiting uses a per-client token bucket guarding auth endpoints"},
	}
	for _, s := range seed {
		m := &model.Memory{
			ID: s.id, ProjectID: proj.ID, Category: "fact",
			Content: s.content, ContentHash: s.id, AuthorID: acc.ID,
			AuthorName: "Vec", CreatedAt: now, UpdatedAt: now,
		}
		if err := st.UpsertMemory(ctx, m); err != nil {
			t.Fatalf("upsert %s: %v", s.id, err)
		}
		vec, err := embeddings.EmbedOne(ctx, emb, s.content)
		if err != nil {
			t.Fatalf("embed %s: %v", s.id, err)
		}
		if err := st.UpdateMemoryEmbedding(ctx, s.id, vec, emb.Model()); err != nil {
			t.Fatalf("store embedding %s: %v", s.id, err)
		}
	}

	qvec, err := embeddings.EmbedOne(ctx, emb, "how are schema migrations handled when the server boots")
	if err != nil {
		t.Fatalf("embed query: %v", err)
	}
	results, err := st.SearchMemoriesByVector(ctx, proj.ID, qvec, 3)
	if err != nil {
		t.Fatalf("vector search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected results, got none")
	}
	if results[0].ID != "m-db" {
		t.Fatalf("expected migrations memory (m-db) ranked first, got %s", results[0].ID)
	}
}

// TestAppendAuditsBatch_Postgres is a regression test for the audit batch bug
// (SQLSTATE 42P18 "could not determine data type of parameter $8"): a wrong
// per-row column stride left orphan placeholders. A multi-row batch must now
// insert cleanly.
func TestAppendAuditsBatch_Postgres(t *testing.T) {
	dsn := os.Getenv("ANCHORED_TEST_DSN")
	if dsn == "" {
		t.Skip("ANCHORED_TEST_DSN not set")
	}
	st, err := NewPostgresStore(PoolConfig{DSN: dsn})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	ctx := context.Background()
	org, err := st.CreateOrganization(ctx, "AuditOrg", "auditorg-"+uniq())
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	entries := []*model.AuditEntry{
		{OrgID: org.ID, Action: "a.one"},
		{OrgID: org.ID, Action: "a.two", Metadata: map[string]any{"k": "v"}},
		{OrgID: org.ID, Action: "a.three"},
	}
	if err := st.AppendAudits(ctx, entries); err != nil {
		t.Fatalf("append audits batch (regression: $8 stride bug): %v", err)
	}
	_, total, err := st.ListAuditEntries(ctx, org.ID, model.AuditFilters{})
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	if total < 3 {
		t.Fatalf("expected >=3 audit rows, got %d", total)
	}
}

// TestReindexBackfill_Postgres verifies MemoriesMissingEmbedding pages rows
// lacking a vector and that storing one removes it from the missing set —
// the contract the -reindex command relies on.
func TestReindexBackfill_Postgres(t *testing.T) {
	dsn := os.Getenv("ANCHORED_TEST_DSN")
	if dsn == "" {
		t.Skip("ANCHORED_TEST_DSN not set")
	}
	st, err := NewPostgresStore(PoolConfig{DSN: dsn})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	ctx := context.Background()
	u := uniq()
	org, _ := st.CreateOrganization(ctx, "RixOrg", "rixorg-"+u)
	acc, _ := st.CreateAccount(ctx, "rix-"+u+"@example.com", "Rix", "x")
	proj, err := st.CreateProject(ctx, org.ID, "RixProj", "rixproj-"+u, "rix-"+u, acc.ID, "service")
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	now := time.Now().UTC()
	id := "rix-" + u
	if err := st.UpsertMemory(ctx, &model.Memory{
		ID: id, ProjectID: proj.ID, Category: "fact",
		Content: "reindex backfill target memory", ContentHash: id,
		AuthorID: acc.ID, AuthorName: "Rix", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	missing, err := st.MemoriesMissingEmbedding(ctx, "", 1000)
	if err != nil {
		t.Fatalf("missing list: %v", err)
	}
	found := false
	for _, m := range missing {
		if m.ID == id {
			found = true
		}
	}
	if !found {
		t.Fatal("freshly inserted memory should be missing an embedding")
	}

	emb := embeddings.NewLocalEmbedder(384)
	vec, _ := embeddings.EmbedOne(ctx, emb, "reindex backfill target memory")
	if err := st.UpdateMemoryEmbedding(ctx, id, vec, emb.Model()); err != nil {
		t.Fatalf("store embedding: %v", err)
	}

	missing2, _ := st.MemoriesMissingEmbedding(ctx, "", 1000)
	for _, m := range missing2 {
		if m.ID == id {
			t.Fatal("memory should no longer be missing an embedding after backfill")
		}
	}
}

// TestStaleEmbedding_Postgres verifies the model-aware reindex contract: a
// memory embedded by one model is "stale" relative to a different model (so a
// provider switch re-embeds it) but not relative to its own model. This is what
// makes `-reindex` migrate an existing lexical-hash corpus to ONNX vectors.
func TestStaleEmbedding_Postgres(t *testing.T) {
	dsn := os.Getenv("ANCHORED_TEST_DSN")
	if dsn == "" {
		t.Skip("ANCHORED_TEST_DSN not set")
	}
	st, err := NewPostgresStore(PoolConfig{DSN: dsn})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	ctx := context.Background()
	u := uniq()
	org, _ := st.CreateOrganization(ctx, "StaleOrg", "staleorg-"+u)
	acc, _ := st.CreateAccount(ctx, "stale-"+u+"@example.com", "Stale", "x")
	proj, err := st.CreateProject(ctx, org.ID, "StaleProj", "staleproj-"+u, "stale-"+u, acc.ID, "service")
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	now := time.Now().UTC()
	id := "stale-" + u
	if err := st.UpsertMemory(ctx, &model.Memory{
		ID: id, ProjectID: proj.ID, Category: "fact",
		Content: "embedded with the old model", ContentHash: id,
		AuthorID: acc.ID, AuthorName: "Stale", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	emb := embeddings.NewLocalEmbedder(384)
	vec, _ := embeddings.EmbedOne(ctx, emb, "embedded with the old model")
	if err := st.UpdateMemoryEmbedding(ctx, id, vec, "old-model-v1"); err != nil {
		t.Fatalf("store embedding: %v", err)
	}

	// Relative to a DIFFERENT model => stale (must be re-embedded).
	contains := func(list []*model.Memory, want string) bool {
		for _, m := range list {
			if m.ID == want {
				return true
			}
		}
		return false
	}
	staleVsNew, err := st.MemoriesStaleEmbedding(ctx, "onnx-multilingual-v2", "", 1000)
	if err != nil {
		t.Fatalf("stale vs new: %v", err)
	}
	if !contains(staleVsNew, id) {
		t.Fatal("memory embedded by old-model-v1 must be stale relative to a different model")
	}

	// Relative to its OWN model => not stale.
	staleVsSame, err := st.MemoriesStaleEmbedding(ctx, "old-model-v1", "", 1000)
	if err != nil {
		t.Fatalf("stale vs same: %v", err)
	}
	if contains(staleVsSame, id) {
		t.Fatal("memory must NOT be stale relative to the same model that produced it")
	}
}

// TestOrgPolicy_Postgres verifies guardrail overrides: a fresh org returns
// defaults, and an upsert round-trips the custom values.
func TestOrgPolicy_Postgres(t *testing.T) {
	dsn := os.Getenv("ANCHORED_TEST_DSN")
	if dsn == "" {
		t.Skip("ANCHORED_TEST_DSN not set")
	}
	st, err := NewPostgresStore(PoolConfig{DSN: dsn})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	ctx := context.Background()
	org, err := st.CreateOrganization(ctx, "PolOrg", "polorg-"+uniq())
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	def, err := st.GetOrgPolicy(ctx, org.ID)
	if err != nil {
		t.Fatalf("get default policy: %v", err)
	}
	if def.QualityThreshold != 0.55 || def.NearDupThreshold != 0.85 {
		t.Fatalf("expected default thresholds 0.55/0.85, got %v/%v", def.QualityThreshold, def.NearDupThreshold)
	}

	if err := st.UpsertOrgPolicy(ctx, &model.OrgPolicy{
		OrgID:             org.ID,
		BlockedCategories: []string{"fact", "event"},
		QualityThreshold:  0.7,
		NearDupThreshold:  0.9,
	}); err != nil {
		t.Fatalf("upsert policy: %v", err)
	}

	got, err := st.GetOrgPolicy(ctx, org.ID)
	if err != nil {
		t.Fatalf("get policy: %v", err)
	}
	if got.QualityThreshold != 0.7 || got.NearDupThreshold != 0.9 || len(got.BlockedCategories) != 2 {
		t.Fatalf("policy did not round-trip: %+v", got)
	}
}
