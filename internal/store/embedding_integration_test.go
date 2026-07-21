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
	proj, err := st.CreateProject(ctx, org.ID, "VecProj", "vecproj-"+u, "vec-remote-"+u, "", "", acc.ID, "service")
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
	oldSpaceID := "m-old-space-" + u
	oldSpaceContent := "how are schema migrations handled when the server boots"
	if err := st.UpsertMemory(ctx, &model.Memory{
		ID: oldSpaceID, ProjectID: proj.ID, Category: "fact",
		Content: oldSpaceContent, ContentHash: oldSpaceID, AuthorID: acc.ID,
		AuthorName: "Vec", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("upsert old semantic space: %v", err)
	}
	oldSpaceVector, _ := embeddings.EmbedOne(ctx, emb, oldSpaceContent)
	if err := st.UpdateMemoryEmbedding(ctx, oldSpaceID, oldSpaceVector, "old-model-same-dims"); err != nil {
		t.Fatalf("store old semantic space: %v", err)
	}

	qvec, err := embeddings.EmbedOne(ctx, emb, "how are schema migrations handled when the server boots")
	if err != nil {
		t.Fatalf("embed query: %v", err)
	}
	results, err := st.SearchMemoriesByVectorSpace(ctx, proj.ID, qvec, emb.Model(), emb.Dimensions(), 3)
	if err != nil {
		t.Fatalf("vector search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected results, got none")
	}
	if results[0].ID != "m-db" {
		t.Fatalf("expected migrations memory (m-db) ranked first, got %s", results[0].ID)
	}
	for _, result := range results {
		if result.ID == oldSpaceID {
			t.Fatal("same-width vector from another model leaked into active semantic-space search")
		}
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
	proj, err := st.CreateProject(ctx, org.ID, "RixProj", "rixproj-"+u, "rix-"+u, "", "", acc.ID, "service")
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
	proj, err := st.CreateProject(ctx, org.ID, "StaleProj", "staleproj-"+u, "stale-"+u, "", "", acc.ID, "service")
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
	staleVsNew, err := st.MemoriesStaleEmbeddingSpace(ctx, "onnx-multilingual-v2", 384, "", 1000)
	if err != nil {
		t.Fatalf("stale vs new: %v", err)
	}
	if !contains(staleVsNew, id) {
		t.Fatal("memory embedded by old-model-v1 must be stale relative to a different model")
	}

	// Relative to its OWN model => not stale.
	staleVsSame, err := st.MemoriesStaleEmbeddingSpace(ctx, "old-model-v1", 384, "", 1000)
	if err != nil {
		t.Fatalf("stale vs same: %v", err)
	}
	if contains(staleVsSame, id) {
		t.Fatal("memory must NOT be stale relative to the same model that produced it")
	}

	if _, err := st.db.ExecContext(ctx, `UPDATE memories SET embed_dims = 1536 WHERE id = $1`, id); err != nil {
		t.Fatalf("simulate stale dimensions: %v", err)
	}
	staleVsWrongDims, err := st.MemoriesStaleEmbeddingSpace(ctx, "old-model-v1", 384, "", 1000)
	if err != nil {
		t.Fatalf("stale vs wrong dimensions: %v", err)
	}
	if !contains(staleVsWrongDims, id) {
		t.Fatal("same-model vector with incompatible embed_dims must be stale")
	}
}

func TestContentChangeInvalidatesEmbedding_Postgres(t *testing.T) {
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
	org, _ := st.CreateOrganization(ctx, "InvalidateOrg", "invalidateorg-"+u)
	account, _ := st.CreateAccount(ctx, "invalidate-"+u+"@example.com", "Invalidate", "x")
	project, err := st.CreateProject(
		ctx,
		org.ID,
		"InvalidateProj",
		"invalidateproj-"+u,
		"invalidate-"+u,
		"",
		"",
		account.ID,
		"service",
	)
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	t0 := time.Now().UTC().Add(-time.Hour)
	memory := &model.Memory{
		ID:          "invalidate-" + u,
		ProjectID:   project.ID,
		Category:    "fact",
		Content:     "original content",
		ContentHash: "sha256:original-" + u,
		AuthorID:    account.ID,
		AuthorName:  "Invalidate",
		CreatedAt:   t0,
		UpdatedAt:   t0,
	}
	if err := st.UpsertMemory(ctx, memory); err != nil {
		t.Fatalf("insert memory: %v", err)
	}
	embedder := embeddings.NewLocalEmbedder(PostgresEmbeddingDimensions)
	vector, err := embeddings.EmbedOne(ctx, embedder, memory.Content)
	if err != nil {
		t.Fatalf("embed memory: %v", err)
	}
	if err := st.UpdateMemoryEmbeddingInSpace(
		ctx,
		memory.ID,
		vector,
		embeddings.SemanticSpace(embedder),
	); err != nil {
		t.Fatalf("store embedding: %v", err)
	}
	if _, err := st.db.ExecContext(
		ctx,
		`UPDATE curation_queue SET status = 'done' WHERE memory_id = $1`,
		memory.ID,
	); err != nil {
		t.Fatalf("mark initial curation done: %v", err)
	}

	edited := *memory
	edited.Content = "replacement content"
	edited.ContentHash = "sha256:replacement-" + u
	edited.UpdatedAt = t0.Add(time.Minute)
	if err := st.UpsertMemory(ctx, &edited); err != nil {
		t.Fatalf("content update: %v", err)
	}
	assertPostgresEmbeddingPresence(t, st, memory.ID, false)
	assertPostgresCurationStatus(t, st, memory.ID, "pending")

	replacementVector, err := embeddings.EmbedOne(ctx, embedder, edited.Content)
	if err != nil {
		t.Fatalf("embed replacement: %v", err)
	}
	updated, err := st.UpdateMemoryEmbeddingInSpaceIfContent(
		ctx,
		memory.ID,
		memory.ContentHash,
		vector,
		embeddings.SemanticSpace(embedder),
	)
	if err != nil {
		t.Fatalf("conditionally store stale embedding: %v", err)
	}
	if updated {
		t.Fatal("embedding computed for old content was attached to replacement content")
	}
	assertPostgresEmbeddingPresence(t, st, memory.ID, false)

	updated, err = st.UpdateMemoryEmbeddingInSpaceIfContent(
		ctx,
		memory.ID,
		edited.ContentHash,
		replacementVector,
		embeddings.SemanticSpace(embedder),
	)
	if err != nil || !updated {
		t.Fatalf("store replacement embedding: updated=%v err=%v", updated, err)
	}
	if _, err := st.db.ExecContext(
		ctx,
		`UPDATE curation_queue SET status = 'processing' WHERE memory_id = $1`,
		memory.ID,
	); err != nil {
		t.Fatalf("mark old worker processing: %v", err)
	}

	batchEdit := edited
	batchEdit.ContentHash = "sha256:replacement-v2-" + u
	batchEdit.UpdatedAt = t0.Add(2 * time.Minute)
	if err := st.UpsertMemories(ctx, []*model.Memory{&batchEdit}); err != nil {
		t.Fatalf("batch content update: %v", err)
	}
	assertPostgresEmbeddingPresence(t, st, memory.ID, false)
	assertPostgresCurationStatus(t, st, memory.ID, "processing_dirty")

	if err := st.SetCurationDone(ctx, memory.ID); err != nil {
		t.Fatalf("old worker completion: %v", err)
	}
	assertPostgresCurationStatus(t, st, memory.ID, "pending")
}

func assertPostgresEmbeddingPresence(t *testing.T, st *PostgresStore, memoryID string, want bool) {
	t.Helper()
	var embeddingPresent, modelPresent, dimensionsPresent, spacePresent bool
	if err := st.db.QueryRow(
		`SELECT
		   embedding IS NOT NULL,
		   embed_model IS NOT NULL,
		   embed_dims IS NOT NULL,
		   semantic_space_id IS NOT NULL
		 FROM memories WHERE id = $1`,
		memoryID,
	).Scan(&embeddingPresent, &modelPresent, &dimensionsPresent, &spacePresent); err != nil {
		t.Fatalf("read embedding state: %v", err)
	}
	if want {
		if !embeddingPresent || !modelPresent || !dimensionsPresent || !spacePresent {
			t.Fatalf(
				"complete embedding missing (embedding=%v model=%v dims=%v space=%v)",
				embeddingPresent,
				modelPresent,
				dimensionsPresent,
				spacePresent,
			)
		}
		return
	}
	if embeddingPresent || modelPresent || dimensionsPresent || spacePresent {
		t.Fatalf(
			"content change retained embedding identity (embedding=%v model=%v dims=%v space=%v)",
			embeddingPresent,
			modelPresent,
			dimensionsPresent,
			spacePresent,
		)
	}
}

func assertPostgresCurationStatus(t *testing.T, st *PostgresStore, memoryID, want string) {
	t.Helper()
	var status string
	if err := st.db.QueryRow(
		`SELECT status FROM curation_queue WHERE memory_id = $1`,
		memoryID,
	).Scan(&status); err != nil {
		t.Fatalf("read curation status: %v", err)
	}
	if status != want {
		t.Fatalf("curation status = %q, want %q", status, want)
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
