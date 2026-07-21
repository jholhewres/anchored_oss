package store

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"

	"github.com/jholhewres/anchored_oss/internal/model"
	"github.com/jholhewres/anchored_oss/internal/semanticspace"
)

// UpdateMemoryEmbedding stores the vector as a JSON float array (SQLite has no
// pgvector) plus the producing model.
func (s *SQLiteStore) UpdateMemoryEmbedding(ctx context.Context, memoryID string, vec []float32, model string) error {
	if len(vec) == 0 {
		return fmt.Errorf("update memory embedding: vector must not be empty")
	}
	blob, err := json.Marshal(vec)
	if err != nil {
		return fmt.Errorf("marshal embedding: %w", err)
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE memories
		 SET embedding = ?, embed_model = ?, embed_dims = ?,
		     semantic_space_id = NULL
		 WHERE id = ?`,
		string(blob), model, len(vec), memoryID,
	); err != nil {
		return fmt.Errorf("update memory embedding: %w", err)
	}
	return nil
}

func (s *SQLiteStore) UpdateMemoryEmbeddingInSpace(
	ctx context.Context,
	memoryID string,
	vec []float32,
	space semanticspace.Identity,
) error {
	if err := space.Validate(); err != nil {
		return fmt.Errorf("update memory embedding: %w", err)
	}
	if len(vec) != space.Dimensions {
		return fmt.Errorf(
			"update memory embedding: vector has %d dimensions, semantic space declares %d",
			len(vec),
			space.Dimensions,
		)
	}
	blob, err := json.Marshal(vec)
	if err != nil {
		return fmt.Errorf("marshal embedding: %w", err)
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE memories
		 SET embedding = ?, embed_model = ?, embed_dims = ?,
		     semantic_space_id = ?
		 WHERE id = ?`,
		string(blob), space.Model, space.Dimensions, space.ID(), memoryID,
	); err != nil {
		return fmt.Errorf("update memory embedding: %w", err)
	}
	return nil
}

func (s *SQLiteStore) UpdateMemoryEmbeddingInSpaceIfContent(
	ctx context.Context,
	memoryID string,
	expectedContentHash string,
	vec []float32,
	space semanticspace.Identity,
) (bool, error) {
	if expectedContentHash == "" {
		return false, fmt.Errorf("update memory embedding: expected content hash is required")
	}
	if err := space.Validate(); err != nil {
		return false, fmt.Errorf("update memory embedding: %w", err)
	}
	if len(vec) != space.Dimensions {
		return false, fmt.Errorf(
			"update memory embedding: vector has %d dimensions, semantic space declares %d",
			len(vec),
			space.Dimensions,
		)
	}
	blob, err := json.Marshal(vec)
	if err != nil {
		return false, fmt.Errorf("marshal embedding: %w", err)
	}
	result, err := s.db.ExecContext(ctx,
		`UPDATE memories
		 SET embedding = ?, embed_model = ?, embed_dims = ?,
		     semantic_space_id = ?
		 WHERE id = ? AND content_hash = ?`,
		string(blob),
		space.Model,
		space.Dimensions,
		space.ID(),
		memoryID,
		expectedContentHash,
	)
	if err != nil {
		return false, fmt.Errorf("update memory embedding: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("inspect memory embedding update: %w", err)
	}
	return affected == 1, nil
}

func (s *SQLiteStore) EmbeddingDimensionConstraint() (int, bool) {
	return 0, false
}

// MemoriesMissingEmbedding returns a page of non-deleted memories without an
// embedding, ordered by id and starting after afterID. Only id, content, and
// content hash are set.
func (s *SQLiteStore) MemoriesMissingEmbedding(ctx context.Context, afterID string, limit int) ([]*model.Memory, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, content, content_hash FROM memories
		 WHERE embedding IS NULL AND deleted_at IS NULL AND id > ?
		 ORDER BY id LIMIT ?`,
		afterID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list memories missing embedding: %w", err)
	}
	defer rows.Close()
	var out []*model.Memory
	for rows.Next() {
		var m model.Memory
		if err := rows.Scan(&m.ID, &m.Content, &m.ContentHash); err != nil {
			return nil, err
		}
		out = append(out, &m)
	}
	return out, rows.Err()
}

// MemoriesStaleEmbedding pages non-deleted memories whose embedding is NULL or
// was produced by a different model, so a provider/model switch re-embeds the
// corpus. (Avoids IS DISTINCT FROM for broad SQLite compatibility.)
func (s *SQLiteStore) MemoriesStaleEmbedding(ctx context.Context, embedModel, afterID string, limit int) ([]*model.Memory, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, content, content_hash FROM memories
		 WHERE deleted_at IS NULL AND id > ?
		   AND (embedding IS NULL OR embed_model IS NULL OR embed_model <> ?)
		 ORDER BY id LIMIT ?`,
		afterID, embedModel, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list memories stale embedding: %w", err)
	}
	defer rows.Close()
	var out []*model.Memory
	for rows.Next() {
		var m model.Memory
		if err := rows.Scan(&m.ID, &m.Content, &m.ContentHash); err != nil {
			return nil, err
		}
		out = append(out, &m)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) MemoriesStaleEmbeddingSpace(ctx context.Context, embedModel string, embedDims int, afterID string, limit int) ([]*model.Memory, error) {
	if embedDims <= 0 {
		return nil, fmt.Errorf("list memories stale embedding: dimensions must be > 0")
	}
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, content, content_hash FROM memories
		 WHERE deleted_at IS NULL AND id > ?
		   AND (
		     embedding IS NULL
		     OR embed_model IS NULL
		     OR embed_model <> ?
		     OR embed_dims IS NULL
		     OR embed_dims <> ?
		   )
		 ORDER BY id LIMIT ?`,
		afterID, embedModel, embedDims, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list memories stale embedding space: %w", err)
	}
	defer rows.Close()
	var out []*model.Memory
	for rows.Next() {
		var m model.Memory
		if err := rows.Scan(&m.ID, &m.Content, &m.ContentHash); err != nil {
			return nil, err
		}
		out = append(out, &m)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) MemoriesStaleInCompleteSemanticSpace(
	ctx context.Context,
	space semanticspace.Identity,
	afterID string,
	limit int,
) ([]*model.Memory, error) {
	if err := space.Validate(); err != nil {
		return nil, fmt.Errorf("list memories stale embedding space: %w", err)
	}
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, content, content_hash FROM memories
		 WHERE deleted_at IS NULL AND id > ?
		   AND (
		     (
		       embedding IS NULL
		       AND COALESCE(json_extract(metadata, '$.curation_status'), '')
		         NOT IN ('low_signal', 'near_duplicate')
		     )
		     OR (
		       embedding IS NOT NULL
		       AND (
		         semantic_space_id IS NULL
		         OR semantic_space_id <> ?
		       )
		     )
		   )
		 ORDER BY id LIMIT ?`,
		afterID, space.ID(), limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list memories stale embedding space: %w", err)
	}
	defer rows.Close()
	var out []*model.Memory
	for rows.Next() {
		var memory model.Memory
		if err := rows.Scan(&memory.ID, &memory.Content, &memory.ContentHash); err != nil {
			return nil, err
		}
		out = append(out, &memory)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) ProjectHasStaleSemanticSpace(
	ctx context.Context,
	projectID string,
	space semanticspace.Identity,
) (bool, error) {
	if err := space.Validate(); err != nil {
		return false, fmt.Errorf("check semantic space coverage: %w", err)
	}
	var stale bool
	err := s.db.QueryRowContext(ctx,
		`SELECT EXISTS(
		   SELECT 1 FROM memories
		   WHERE project_id = ?
		     AND deleted_at IS NULL
		     AND (
		       (
		         embedding IS NOT NULL
		         AND (
		           semantic_space_id IS NULL
		           OR semantic_space_id <> ?
		         )
		       )
		       OR (
		         embedding IS NULL
		         AND COALESCE(json_extract(metadata, '$.curation_status'), '')
		           NOT IN ('low_signal', 'near_duplicate')
		       )
		     )`+sqliteQualityFilterSQL+`
		 )`,
		projectID,
		space.ID(),
	).Scan(&stale)
	if err != nil {
		return false, fmt.Errorf("check semantic space coverage: %w", err)
	}
	return stale, nil
}

// SearchMemoriesByVector brute-forces cosine similarity in Go (acceptable for
// the single-node SQLite dev path) and returns up to k nearest memories.
func (s *SQLiteStore) SearchMemoriesByVector(ctx context.Context, projectID string, vec []float32, k int) ([]*model.Memory, error) {
	if k <= 0 {
		k = 20
	}
	if k > 100 {
		k = 100
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, embedding FROM memories
		 WHERE project_id = ? AND deleted_at IS NULL AND embedding IS NOT NULL`+sqliteQualityFilterSQL,
		projectID,
	)
	if err != nil {
		return nil, fmt.Errorf("vector search scan: %w", err)
	}
	defer rows.Close()

	type scored struct {
		id    string
		score float64
	}
	var ranked []scored
	for rows.Next() {
		var id, blob string
		if err := rows.Scan(&id, &blob); err != nil {
			return nil, fmt.Errorf("scan embedding row: %w", err)
		}
		var emb []float32
		if err := json.Unmarshal([]byte(blob), &emb); err != nil {
			continue // skip malformed vectors rather than fail the whole search
		}
		ranked = append(ranked, scored{id: id, score: cosine(vec, emb)})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	sort.Slice(ranked, func(i, j int) bool { return ranked[i].score > ranked[j].score })
	if len(ranked) > k {
		ranked = ranked[:k]
	}
	if len(ranked) == 0 {
		return nil, nil
	}

	order := make(map[string]int, len(ranked))
	ids := make([]any, len(ranked))
	placeholders := ""
	for i, r := range ranked {
		order[r.id] = i
		ids[i] = r.id
		if i > 0 {
			placeholders += ","
		}
		placeholders += "?"
	}

	q := `SELECT id, project_id, category, content, content_hash, keywords, source, author_id, author_name, created_at, updated_at, deleted_at, metadata
		 FROM memories WHERE id IN (` + placeholders + `)`
	memRows, err := s.db.QueryContext(ctx, q, ids...)
	if err != nil {
		return nil, fmt.Errorf("fetch ranked memories: %w", err)
	}
	defer memRows.Close()
	mems, err := sqliteScanMemories(memRows)
	if err != nil {
		return nil, err
	}
	// Restore cosine ranking lost by the IN() fetch.
	sort.Slice(mems, func(i, j int) bool { return order[mems[i].ID] < order[mems[j].ID] })
	return mems, nil
}

// SearchMemoriesByVectorSpace is the safe SQLite semantic path. Filtering
// happens in SQL before cosine scoring so incompatible models and dimensions
// never enter the candidate set.
func (s *SQLiteStore) SearchMemoriesByVectorSpace(ctx context.Context, projectID string, vec []float32, embedModel string, embedDims, k int) ([]*model.Memory, error) {
	if embedModel == "" {
		return nil, fmt.Errorf("vector search memories: embedding model is required")
	}
	if embedDims <= 0 || len(vec) != embedDims {
		return nil, fmt.Errorf("vector search memories: query dimensions=%d, declared dimensions=%d", len(vec), embedDims)
	}
	if k <= 0 {
		k = 20
	}
	if k > 100 {
		k = 100
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, embedding FROM memories
		 WHERE project_id = ?
		   AND deleted_at IS NULL
		   AND embedding IS NOT NULL
		   AND embed_model = ?
		   AND embed_dims = ?`+sqliteQualityFilterSQL,
		projectID, embedModel, embedDims,
	)
	if err != nil {
		return nil, fmt.Errorf("vector search scan: %w", err)
	}
	defer rows.Close()

	type scored struct {
		id    string
		score float64
	}
	var ranked []scored
	for rows.Next() {
		var id, blob string
		if err := rows.Scan(&id, &blob); err != nil {
			return nil, fmt.Errorf("scan embedding row: %w", err)
		}
		var emb []float32
		if err := json.Unmarshal([]byte(blob), &emb); err != nil || len(emb) != embedDims {
			continue
		}
		ranked = append(ranked, scored{id: id, score: cosine(vec, emb)})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	sort.Slice(ranked, func(i, j int) bool { return ranked[i].score > ranked[j].score })
	if len(ranked) > k {
		ranked = ranked[:k]
	}
	if len(ranked) == 0 {
		return nil, nil
	}

	order := make(map[string]int, len(ranked))
	ids := make([]any, len(ranked))
	placeholders := ""
	for i, result := range ranked {
		order[result.id] = i
		ids[i] = result.id
		if i > 0 {
			placeholders += ","
		}
		placeholders += "?"
	}

	query := `SELECT id, project_id, category, content, content_hash, keywords, source, author_id, author_name, created_at, updated_at, deleted_at, metadata
		 FROM memories WHERE id IN (` + placeholders + `)`
	memoryRows, err := s.db.QueryContext(ctx, query, ids...)
	if err != nil {
		return nil, fmt.Errorf("fetch ranked memories: %w", err)
	}
	defer memoryRows.Close()
	memories, err := sqliteScanMemories(memoryRows)
	if err != nil {
		return nil, err
	}
	sort.Slice(memories, func(i, j int) bool {
		return order[memories[i].ID] < order[memories[j].ID]
	})
	return memories, nil
}

func (s *SQLiteStore) SearchMemoriesByCompleteSemanticSpace(
	ctx context.Context,
	projectID string,
	vec []float32,
	space semanticspace.Identity,
	k int,
) ([]*model.Memory, error) {
	if err := space.Validate(); err != nil {
		return nil, fmt.Errorf("vector search memories: %w", err)
	}
	if len(vec) != space.Dimensions {
		return nil, fmt.Errorf(
			"vector search memories: query dimensions=%d, declared dimensions=%d",
			len(vec),
			space.Dimensions,
		)
	}
	if k <= 0 {
		k = 20
	}
	if k > 100 {
		k = 100
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, embedding FROM memories
		 WHERE project_id = ?
		   AND deleted_at IS NULL
		   AND embedding IS NOT NULL
		   AND semantic_space_id = ?`+sqliteQualityFilterSQL,
		projectID, space.ID(),
	)
	if err != nil {
		return nil, fmt.Errorf("vector search scan: %w", err)
	}
	defer rows.Close()

	type scored struct {
		id    string
		score float64
	}
	var ranked []scored
	for rows.Next() {
		var id, blob string
		if err := rows.Scan(&id, &blob); err != nil {
			return nil, fmt.Errorf("scan embedding row: %w", err)
		}
		var embedding []float32
		if err := json.Unmarshal([]byte(blob), &embedding); err != nil ||
			len(embedding) != space.Dimensions {
			continue
		}
		ranked = append(ranked, scored{id: id, score: cosine(vec, embedding)})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	sort.Slice(ranked, func(i, j int) bool { return ranked[i].score > ranked[j].score })
	if len(ranked) > k {
		ranked = ranked[:k]
	}
	if len(ranked) == 0 {
		return nil, nil
	}

	order := make(map[string]int, len(ranked))
	ids := make([]any, len(ranked))
	placeholders := ""
	for i, result := range ranked {
		order[result.id] = i
		ids[i] = result.id
		if i > 0 {
			placeholders += ","
		}
		placeholders += "?"
	}
	query := `SELECT id, project_id, category, content, content_hash, keywords, source, author_id, author_name, created_at, updated_at, deleted_at, metadata
		 FROM memories WHERE id IN (` + placeholders + `)`
	memoryRows, err := s.db.QueryContext(ctx, query, ids...)
	if err != nil {
		return nil, fmt.Errorf("fetch ranked memories: %w", err)
	}
	defer memoryRows.Close()
	memories, err := sqliteScanMemories(memoryRows)
	if err != nil {
		return nil, err
	}
	sort.Slice(memories, func(i, j int) bool {
		return order[memories[i].ID] < order[memories[j].ID]
	})
	return memories, nil
}

// cosine returns the cosine similarity of two equal-length vectors, or 0 when
// they differ in length or either is zero.
func cosine(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}
