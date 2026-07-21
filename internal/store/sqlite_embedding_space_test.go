package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jholhewres/anchored_oss/internal/model"
	"github.com/jholhewres/anchored_oss/internal/semanticspace"
)

func TestSQLiteSemanticSpaceFiltersModelAndDimensions(t *testing.T) {
	st := newSQLiteTestStore(t)
	ctx := context.Background()
	orgID, actorID := projectTestOrg(t, st)
	project, err := st.CreateProject(
		ctx,
		orgID,
		"Embedding Space",
		"embedding-space",
		"embedding-space",
		"",
		"",
		actorID,
		"other",
	)
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	now := time.Date(2026, time.July, 20, 12, 0, 0, 0, time.UTC)
	for _, id := range []string{"active", "old-model", "wrong-dims", "missing"} {
		if err := st.UpsertMemory(ctx, &model.Memory{
			ID:          id,
			ProjectID:   project.ID,
			Category:    "fact",
			Content:     "embedding " + id,
			ContentHash: "sha256:" + id,
			AuthorID:    actorID,
			AuthorName:  "Test",
			CreatedAt:   now,
			UpdatedAt:   now,
		}); err != nil {
			t.Fatalf("upsert %s: %v", id, err)
		}
	}
	if err := st.UpdateMemoryEmbedding(ctx, "active", []float32{1, 0, 0}, "active-model"); err != nil {
		t.Fatalf("embed active: %v", err)
	}
	if err := st.UpdateMemoryEmbedding(ctx, "old-model", []float32{1, 0, 0}, "old-model"); err != nil {
		t.Fatalf("embed old model: %v", err)
	}
	if err := st.UpdateMemoryEmbedding(ctx, "wrong-dims", []float32{1, 0}, "active-model"); err != nil {
		t.Fatalf("embed wrong dimensions: %v", err)
	}

	results, err := st.SearchMemoriesByVectorSpace(
		ctx,
		project.ID,
		[]float32{1, 0, 0},
		"active-model",
		3,
		10,
	)
	if err != nil {
		t.Fatalf("semantic-space search: %v", err)
	}
	if len(results) != 1 || results[0].ID != "active" {
		t.Fatalf("semantic-space results = %+v, want only active", memoryIDs(results))
	}

	stale, err := st.MemoriesStaleEmbeddingSpace(ctx, "active-model", 3, "", 10)
	if err != nil {
		t.Fatalf("stale semantic space: %v", err)
	}
	gotStale := make(map[string]bool)
	for _, memory := range stale {
		gotStale[memory.ID] = true
	}
	for _, id := range []string{"old-model", "wrong-dims", "missing"} {
		if !gotStale[id] {
			t.Errorf("%s must be stale, got %v", id, gotStale)
		}
	}
	if gotStale["active"] {
		t.Errorf("active vector incorrectly stale: %v", gotStale)
	}
}

func TestSQLiteSemanticSpaceRejectsInvalidQueryIdentity(t *testing.T) {
	st := newSQLiteTestStore(t)
	ctx := context.Background()

	tests := []struct {
		name  string
		model string
		dims  int
		vec   []float32
	}{
		{name: "missing model", dims: 3, vec: []float32{1, 0, 0}},
		{name: "missing dimensions", model: "active", vec: []float32{1}},
		{name: "dimension mismatch", model: "active", dims: 3, vec: []float32{1, 0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := st.SearchMemoriesByVectorSpace(ctx, "project", tt.vec, tt.model, tt.dims, 10)
			if err == nil {
				t.Fatal("invalid semantic space unexpectedly accepted")
			}
		})
	}
	if err := st.UpdateMemoryEmbedding(ctx, "memory", nil, "active"); err == nil {
		t.Fatal("empty embedding unexpectedly accepted")
	}
	if _, err := st.MemoriesStaleEmbeddingSpace(ctx, "active", 0, "", 10); err == nil {
		t.Fatal("zero-width stale query unexpectedly accepted")
	}
}

func TestSQLiteCompleteSemanticSpaceSeparatesProviderAndRevision(t *testing.T) {
	st := newSQLiteTestStore(t)
	ctx := context.Background()
	orgID, actorID := projectTestOrg(t, st)
	project, err := st.CreateProject(
		ctx,
		orgID,
		"Complete Embedding Space",
		"complete-embedding-space",
		"complete-embedding-space",
		"",
		"",
		actorID,
		"other",
	)
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	now := time.Date(2026, time.July, 20, 12, 0, 0, 0, time.UTC)
	for _, id := range []string{"active", "other-provider", "other-revision", "legacy"} {
		if err := st.UpsertMemory(ctx, &model.Memory{
			ID:          id,
			ProjectID:   project.ID,
			Category:    "fact",
			Content:     "same model and width " + id,
			ContentHash: "sha256:" + id,
			AuthorID:    actorID,
			AuthorName:  "Test",
			CreatedAt:   now,
			UpdatedAt:   now,
		}); err != nil {
			t.Fatalf("upsert %s: %v", id, err)
		}
	}
	for _, item := range []struct {
		id     string
		status string
	}{
		{id: "low-signal", status: "low_signal"},
		{id: "near-duplicate", status: "near_duplicate"},
	} {
		if err := st.UpsertMemory(ctx, &model.Memory{
			ID: item.id, ProjectID: project.ID, Category: "fact",
			Content:     "intentionally unembedded " + item.id,
			ContentHash: "sha256:" + item.id,
			AuthorID:    actorID, AuthorName: "Test",
			CreatedAt: now, UpdatedAt: now,
			Metadata: map[string]any{"curation_status": item.status},
		}); err != nil {
			t.Fatalf("upsert %s: %v", item.id, err)
		}
	}

	active := semanticspace.New("openai", "shared-model", "revision-a", 3, semanticspace.L2Normalization)
	otherProvider := semanticspace.New("compatible-api", "shared-model", "revision-a", 3, semanticspace.L2Normalization)
	otherRevision := semanticspace.New("openai", "shared-model", "revision-b", 3, semanticspace.L2Normalization)
	if err := st.UpdateMemoryEmbeddingInSpace(ctx, "active", []float32{1, 0, 0}, active); err != nil {
		t.Fatalf("embed active: %v", err)
	}
	if err := st.UpdateMemoryEmbeddingInSpace(ctx, "other-provider", []float32{1, 0, 0}, otherProvider); err != nil {
		t.Fatalf("embed other provider: %v", err)
	}
	if err := st.UpdateMemoryEmbeddingInSpace(ctx, "other-revision", []float32{1, 0, 0}, otherRevision); err != nil {
		t.Fatalf("embed other revision: %v", err)
	}
	if err := st.UpdateMemoryEmbedding(ctx, "legacy", []float32{1, 0, 0}, active.Model); err != nil {
		t.Fatalf("embed legacy row: %v", err)
	}

	results, err := st.SearchMemoriesByCompleteSemanticSpace(
		ctx,
		project.ID,
		[]float32{1, 0, 0},
		active,
		10,
	)
	if err != nil {
		t.Fatalf("complete semantic-space search: %v", err)
	}
	if len(results) != 1 || results[0].ID != "active" {
		t.Fatalf("complete semantic-space results = %v, want only active", memoryIDs(results))
	}

	stale, err := st.MemoriesStaleInCompleteSemanticSpace(ctx, active, "", 10)
	if err != nil {
		t.Fatalf("complete semantic-space stale scan: %v", err)
	}
	gotStale := make(map[string]bool, len(stale))
	for _, memory := range stale {
		gotStale[memory.ID] = true
	}
	for _, id := range []string{"other-provider", "other-revision", "legacy"} {
		if !gotStale[id] {
			t.Errorf("%s must be stale, got %v", id, gotStale)
		}
	}
	if gotStale["active"] {
		t.Errorf("active complete-space vector incorrectly stale: %v", gotStale)
	}
	for _, id := range []string{"low-signal", "near-duplicate"} {
		if gotStale[id] {
			t.Errorf("%s should remain intentionally unembedded: %v", id, gotStale)
		}
	}
}

func TestSQLiteSemanticCoverageIgnoresFilteredRows(t *testing.T) {
	st := newSQLiteTestStore(t)
	ctx := context.Background()
	orgID, actorID := projectTestOrg(t, st)
	project, err := st.CreateProject(
		ctx,
		orgID,
		"Coverage",
		"coverage",
		"coverage",
		"",
		"",
		actorID,
		"other",
	)
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	now := time.Date(2026, time.July, 20, 12, 0, 0, 0, time.UTC)
	visible := &model.Memory{
		ID:          "visible",
		ProjectID:   project.ID,
		Category:    "fact",
		Content:     "visible memory",
		ContentHash: "sha256:visible",
		AuthorID:    actorID,
		AuthorName:  "Test",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	hidden := &model.Memory{
		ID:          "hidden",
		ProjectID:   project.ID,
		Category:    "fact",
		Content:     "filtered low quality memory",
		ContentHash: "sha256:hidden",
		AuthorID:    actorID,
		AuthorName:  "Test",
		CreatedAt:   now,
		UpdatedAt:   now,
		Metadata:    map[string]any{"quality_score": 0.1},
	}
	failedEmbedding := &model.Memory{
		ID:          "failed-embedding",
		ProjectID:   project.ID,
		Category:    "fact",
		Content:     "eligible memory whose embedding failed",
		ContentHash: "sha256:failed-embedding",
		AuthorID:    actorID,
		AuthorName:  "Test",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := st.UpsertMemories(ctx, []*model.Memory{visible, hidden, failedEmbedding}); err != nil {
		t.Fatalf("seed memories: %v", err)
	}
	space := semanticspace.New("local", "local-hash-v1", "revision-a", 3, semanticspace.L2Normalization)
	stale, err := st.ProjectHasStaleSemanticSpace(ctx, project.ID, space)
	if err != nil || !stale {
		t.Fatalf("coverage before visible embedding = stale:%v err:%v", stale, err)
	}
	if err := st.UpdateMemoryEmbeddingInSpace(ctx, visible.ID, []float32{1, 0, 0}, space); err != nil {
		t.Fatalf("embed visible memory: %v", err)
	}
	setSQLiteCurationStatus(t, st, visible.ID, "done")
	setSQLiteCurationStatus(t, st, hidden.ID, "done")
	setSQLiteCurationStatus(t, st, failedEmbedding.ID, "failed")
	stale, err = st.ProjectHasStaleSemanticSpace(ctx, project.ID, space)
	if err != nil {
		t.Fatalf("coverage after visible embedding: %v", err)
	}
	if !stale {
		t.Fatal("eligible row with a failed embedding was hidden from semantic readiness")
	}
	if err := st.UpdateMemoryMetadata(
		ctx,
		failedEmbedding.ID,
		map[string]any{"curation_status": "low_signal"},
	); err != nil {
		t.Fatalf("mark failed embedding intentionally unembedded: %v", err)
	}
	stale, err = st.ProjectHasStaleSemanticSpace(ctx, project.ID, space)
	if err != nil {
		t.Fatalf("coverage after excluding intentionally unembedded rows: %v", err)
	}
	if stale {
		t.Fatal("intentionally unembedded rows blocked semantic readiness")
	}
	if err := st.UpdateMemoryEmbedding(ctx, visible.ID, []float32{1, 0, 0}, space.Model); err != nil {
		t.Fatalf("write legacy vector: %v", err)
	}
	stale, err = st.ProjectHasStaleSemanticSpace(ctx, project.ID, space)
	if err != nil || !stale {
		t.Fatalf("legacy vector readiness = stale:%v err:%v, want stale", stale, err)
	}
}

func TestSQLiteContentChangesInvalidateEmbeddings(t *testing.T) {
	st := newSQLiteTestStore(t)
	ctx := context.Background()
	orgID, actorID := projectTestOrg(t, st)
	project, err := st.CreateProject(
		ctx,
		orgID,
		"Embedding Invalidation",
		"embedding-invalidation",
		"embedding-invalidation",
		"",
		"",
		actorID,
		"other",
	)
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	t0 := time.Date(2026, time.July, 20, 12, 0, 0, 0, time.UTC)
	memory := &model.Memory{
		ID:          "content-change",
		ProjectID:   project.ID,
		Category:    "fact",
		Content:     "original content",
		ContentHash: "sha256:original",
		AuthorID:    actorID,
		AuthorName:  "Test",
		CreatedAt:   t0,
		UpdatedAt:   t0,
	}
	if err := st.UpsertMemory(ctx, memory); err != nil {
		t.Fatalf("insert memory: %v", err)
	}
	space := semanticspace.New("local", "local-hash-v1", "revision-a", 3, semanticspace.L2Normalization)
	if err := st.UpdateMemoryEmbeddingInSpace(ctx, memory.ID, []float32{1, 0, 0}, space); err != nil {
		t.Fatalf("embed memory: %v", err)
	}
	setSQLiteCurationStatus(t, st, memory.ID, "done")

	// A non-content update must retain a still-valid embedding.
	metadataOnly := *memory
	metadataOnly.Category = "decision"
	metadataOnly.UpdatedAt = t0.Add(time.Minute)
	if err := st.UpsertMemory(ctx, &metadataOnly); err != nil {
		t.Fatalf("metadata-only update: %v", err)
	}
	assertSQLiteEmbeddingPresence(t, st, memory.ID, true)
	setSQLiteCurationStatus(t, st, memory.ID, "done")

	contentChange := metadataOnly
	contentChange.Content = "replacement content"
	contentChange.ContentHash = "sha256:replacement"
	contentChange.UpdatedAt = t0.Add(2 * time.Minute)
	if err := st.UpsertMemory(ctx, &contentChange); err != nil {
		t.Fatalf("content update: %v", err)
	}
	assertSQLiteEmbeddingPresence(t, st, memory.ID, false)
	assertSQLiteCurationStatus(t, st, memory.ID, "pending")
	metadataUpdated, err := st.UpdateMemoryMetadataIfContent(
		ctx,
		memory.ID,
		memory.ContentHash,
		map[string]any{"curation_status": "stale-worker-result"},
	)
	if err != nil {
		t.Fatalf("conditionally store stale metadata: %v", err)
	}
	if metadataUpdated {
		t.Fatal("curation metadata from old content was attached to replacement content")
	}
	currentMemory, err := st.GetMemoryByID(ctx, memory.ID)
	if err != nil {
		t.Fatalf("read current memory: %v", err)
	}
	if metadata, ok := currentMemory.Metadata.(map[string]any); ok &&
		metadata["curation_status"] == "stale-worker-result" {
		t.Fatal("stale curation metadata leaked onto replacement content")
	}

	updated, err := st.UpdateMemoryEmbeddingInSpaceIfContent(
		ctx,
		memory.ID,
		memory.ContentHash,
		[]float32{1, 0, 0},
		space,
	)
	if err != nil {
		t.Fatalf("conditionally store stale embedding: %v", err)
	}
	if updated {
		t.Fatal("embedding computed for old content was attached to replacement content")
	}
	assertSQLiteEmbeddingPresence(t, st, memory.ID, false)

	updated, err = st.UpdateMemoryEmbeddingInSpaceIfContent(
		ctx,
		memory.ID,
		contentChange.ContentHash,
		[]float32{0, 1, 0},
		space,
	)
	if err != nil || !updated {
		t.Fatalf("re-embed current content: updated=%v err=%v", updated, err)
	}
	setSQLiteCurationStatus(t, st, memory.ID, "done")
	batchChange := contentChange
	batchChange.ContentHash = "sha256:replacement-v2"
	batchChange.UpdatedAt = t0.Add(3 * time.Minute)
	if err := st.UpsertMemories(ctx, []*model.Memory{&batchChange}); err != nil {
		t.Fatalf("batch content update: %v", err)
	}
	assertSQLiteEmbeddingPresence(t, st, memory.ID, false)
	assertSQLiteCurationStatus(t, st, memory.ID, "pending")

	if err := st.UpdateMemoryEmbeddingInSpace(ctx, memory.ID, []float32{0, 0, 1}, space); err != nil {
		t.Fatalf("re-embed after batch: %v", err)
	}
	staleWrite := batchChange
	staleWrite.Content = "stale incoming content"
	staleWrite.ContentHash = "sha256:stale"
	staleWrite.UpdatedAt = t0
	if err := st.UpsertMemory(ctx, &staleWrite); err != nil {
		t.Fatalf("stale upsert: %v", err)
	}
	assertSQLiteEmbeddingPresence(t, st, memory.ID, true)
}

func assertSQLiteEmbeddingPresence(t *testing.T, st *SQLiteStore, memoryID string, want bool) {
	t.Helper()
	var embedding, model, spaceID sql.NullString
	var dimensions sql.NullInt64
	if err := st.db.QueryRow(
		`SELECT embedding, embed_model, embed_dims, semantic_space_id
		 FROM memories WHERE id = ?`,
		memoryID,
	).Scan(&embedding, &model, &dimensions, &spaceID); err != nil {
		t.Fatalf("read embedding state: %v", err)
	}
	got := embedding.Valid && model.Valid && dimensions.Valid && spaceID.Valid
	if got != want {
		t.Fatalf(
			"embedding presence = %v, want %v (embedding=%v model=%v dims=%v space=%v)",
			got,
			want,
			embedding.Valid,
			model.Valid,
			dimensions.Valid,
			spaceID.Valid,
		)
	}
	if !want && (embedding.Valid || model.Valid || dimensions.Valid || spaceID.Valid) {
		t.Fatal("content changes must clear the vector and every identity column")
	}
}

func assertSQLiteCurationStatus(t *testing.T, st *SQLiteStore, memoryID, want string) {
	t.Helper()
	var status string
	if err := st.db.QueryRow(
		`SELECT status FROM curation_queue WHERE memory_id = ?`,
		memoryID,
	).Scan(&status); err != nil {
		t.Fatalf("read curation status: %v", err)
	}
	if status != want {
		t.Fatalf("curation status = %q, want %q", status, want)
	}
}

func setSQLiteCurationStatus(t *testing.T, st *SQLiteStore, memoryID, status string) {
	t.Helper()
	if _, err := st.db.Exec(
		`UPDATE curation_queue SET status = ? WHERE memory_id = ?`,
		status,
		memoryID,
	); err != nil {
		t.Fatalf("set curation status %q: %v", status, err)
	}
}

func TestSQLiteCurationRequeueWinsOverOlderWorkerCompletion(t *testing.T) {
	st := newSQLiteTestStore(t)
	ctx := context.Background()
	orgID, actorID := projectTestOrg(t, st)
	project, err := st.CreateProject(
		ctx,
		orgID,
		"Curation Requeue",
		"curation-requeue",
		"curation-requeue",
		"",
		"",
		actorID,
		"other",
	)
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	now := time.Date(2026, time.July, 20, 12, 0, 0, 0, time.UTC)
	memory := &model.Memory{
		ID:          "requeue",
		ProjectID:   project.ID,
		Category:    "fact",
		Content:     "first content",
		ContentHash: "sha256:first",
		AuthorID:    actorID,
		AuthorName:  "Test",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := st.UpsertMemory(ctx, memory); err != nil {
		t.Fatalf("insert memory: %v", err)
	}
	claimed, err := st.ClaimCurationBatch(ctx, 1)
	if err != nil || len(claimed) != 1 || claimed[0] != memory.ID {
		t.Fatalf("claim = %v, err=%v", claimed, err)
	}

	edited := *memory
	edited.Content = "second content"
	edited.ContentHash = "sha256:second"
	edited.UpdatedAt = now.Add(time.Minute)
	if err := st.UpsertMemory(ctx, &edited); err != nil {
		t.Fatalf("edit while old worker is active: %v", err)
	}
	assertSQLiteCurationStatus(t, st, memory.ID, "processing_dirty")
	secondClaim, err := st.ClaimCurationBatch(ctx, 1)
	if err != nil {
		t.Fatalf("second worker claim: %v", err)
	}
	if len(secondClaim) != 0 {
		t.Fatalf("second worker claimed dirty in-flight row: %v", secondClaim)
	}

	if err := st.SetCurationDone(ctx, memory.ID); err != nil {
		t.Fatalf("old worker completion: %v", err)
	}
	assertSQLiteCurationStatus(t, st, memory.ID, "pending")
	secondClaim, err = st.ClaimCurationBatch(ctx, 1)
	if err != nil || len(secondClaim) != 1 || secondClaim[0] != memory.ID {
		t.Fatalf("replacement claim = %v, err=%v", secondClaim, err)
	}
}

func TestPostgresEmbeddingWidthRejectedBeforeDatabaseCall(t *testing.T) {
	st := &PostgresStore{}
	err := st.UpdateMemoryEmbedding(context.Background(), "memory", make([]float32, 1536), "large-model")
	if err == nil {
		t.Fatal("1536-dimensional vector unexpectedly accepted for vector(384)")
	}
	if got, fixed := st.EmbeddingDimensionConstraint(); !fixed || got != PostgresEmbeddingDimensions {
		t.Fatalf("Postgres constraint = %d fixed=%v", got, fixed)
	}
	if got, fixed := (&SQLiteStore{}).EmbeddingDimensionConstraint(); fixed || got != 0 {
		t.Fatalf("SQLite constraint = %d fixed=%v, want unconstrained", got, fixed)
	}
}

func TestEmbeddingMigrationsPreservePublishedOrdering(t *testing.T) {
	if schemaVersion != 20 || sqliteSchemaVersion != 20 {
		t.Fatalf(
			"schema versions = postgres:%d sqlite:%d, want 20",
			schemaVersion,
			sqliteSchemaVersion,
		)
	}
	if !strings.Contains(migrations[19], "memory_write_idempotency") ||
		!strings.Contains(sqliteMigrations[19], "memory_write_idempotency") {
		t.Fatal("migration 019 must remain the idempotency migration in both adapters")
	}
	if !strings.Contains(migrations[20], "semantic_space_id") {
		t.Fatal("Postgres migration 020 must add semantic_space_id")
	}
	if sqliteMigrations[20] != "" {
		t.Fatal("SQLite migration 020 must stay in its guarded ALTER TABLE branch")
	}
}

func TestSQLiteMigration020UpgradesVersion019WithoutDataLoss(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy-v19.db")
	st, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("create current store: %v", err)
	}
	ctx := context.Background()
	orgID, actorID := projectTestOrg(t, st)
	project, err := st.CreateProject(
		ctx,
		orgID,
		"Migration 020",
		"migration-020",
		"migration-020",
		"",
		"",
		actorID,
		"other",
	)
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	memory := &model.Memory{
		ID:          "legacy-v19-memory",
		ProjectID:   project.ID,
		Category:    "fact",
		Content:     "preserved across migration 020",
		ContentHash: "sha256:legacy-v19-memory",
		AuthorID:    actorID,
		AuthorName:  "Test",
		CreatedAt:   time.Date(2026, time.July, 20, 12, 0, 0, 0, time.UTC),
		UpdatedAt:   time.Date(2026, time.July, 20, 12, 0, 0, 0, time.UTC),
	}
	if err := st.UpsertMemory(ctx, memory); err != nil {
		t.Fatalf("seed memory: %v", err)
	}
	if err := st.UpdateMemoryEmbedding(ctx, memory.ID, []float32{1, 0, 0}, "legacy-model"); err != nil {
		t.Fatalf("seed legacy embedding: %v", err)
	}
	if _, err := st.db.Exec(`DROP INDEX idx_memories_semantic_space`); err != nil {
		t.Fatalf("drop migration 020 index: %v", err)
	}
	if _, err := st.db.Exec(`ALTER TABLE memories DROP COLUMN semantic_space_id`); err != nil {
		t.Fatalf("remove migration 020 column: %v", err)
	}
	if _, err := st.db.Exec(`DELETE FROM schema_version WHERE version >= 20`); err != nil {
		t.Fatalf("rewind schema version: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("close legacy store: %v", err)
	}

	upgraded, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("upgrade legacy store: %v", err)
	}
	defer upgraded.Close()
	if !columnExists(upgraded.db, "memories", "semantic_space_id") {
		t.Fatal("migration 020 did not add semantic_space_id")
	}
	if _, err := upgraded.GetMemoryByID(ctx, memory.ID); err != nil {
		t.Fatalf("legacy memory after migration: %v", err)
	}
	var embedding, modelName sql.NullString
	var dimensions sql.NullInt64
	if err := upgraded.db.QueryRow(
		`SELECT embedding, embed_model, embed_dims
		 FROM memories WHERE id = ?`,
		memory.ID,
	).Scan(&embedding, &modelName, &dimensions); err != nil {
		t.Fatalf("read legacy embedding after migration: %v", err)
	}
	if !embedding.Valid || !modelName.Valid || !dimensions.Valid {
		t.Fatalf(
			"legacy embedding lost during migration (embedding=%v model=%v dims=%v)",
			embedding.Valid,
			modelName.Valid,
			dimensions.Valid,
		)
	}
}

func memoryIDs(memories []*model.Memory) []string {
	ids := make([]string, len(memories))
	for i, memory := range memories {
		ids[i] = memory.ID
	}
	return ids
}
