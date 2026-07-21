package store

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/jholhewres/anchored_oss/internal/model"
	"github.com/jholhewres/anchored_oss/internal/semanticspace"
)

// vectorLiteral encodes a float slice as a pgvector text literal: "[a,b,c]".
// Used with an explicit ::vector cast so it works over the database/sql pgx
// stdlib driver without a pgvector-specific type.
func vectorLiteral(vec []float32) string {
	var b strings.Builder
	b.WriteByte('[')
	for i, v := range vec {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatFloat(float64(v), 'f', -1, 32))
	}
	b.WriteByte(']')
	return b.String()
}

// UpdateMemoryEmbedding stores (or replaces) a memory's vector and the model
// that produced it. A model change is detectable via embed_model for reindex.
func (s *PostgresStore) UpdateMemoryEmbedding(ctx context.Context, memoryID string, vec []float32, model string) error {
	if len(vec) != PostgresEmbeddingDimensions {
		return fmt.Errorf(
			"update memory embedding: vector has %d dimensions, Postgres schema requires %d",
			len(vec),
			PostgresEmbeddingDimensions,
		)
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE memories
		 SET embedding = $1::vector, embed_model = $2, embed_dims = $3,
		     semantic_space_id = NULL
		 WHERE id = $4`,
		vectorLiteral(vec), model, len(vec), memoryID,
	)
	if err != nil {
		return fmt.Errorf("update memory embedding: %w", err)
	}
	return nil
}

// UpdateMemoryEmbeddingInSpace stores a vector under its complete semantic
// identity. The legacy UpdateMemoryEmbedding method remains available and
// intentionally writes a NULL semantic_space_id because its caller did not
// provide enough information to prove compatibility.
func (s *PostgresStore) UpdateMemoryEmbeddingInSpace(
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
	if len(vec) != PostgresEmbeddingDimensions {
		return fmt.Errorf(
			"update memory embedding: vector has %d dimensions, Postgres schema requires %d",
			len(vec),
			PostgresEmbeddingDimensions,
		)
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE memories
		 SET embedding = $1::vector, embed_model = $2, embed_dims = $3,
		     semantic_space_id = $4
		 WHERE id = $5`,
		vectorLiteral(vec), space.Model, space.Dimensions, space.ID(), memoryID,
	)
	if err != nil {
		return fmt.Errorf("update memory embedding: %w", err)
	}
	return nil
}

func (s *PostgresStore) UpdateMemoryEmbeddingInSpaceIfContent(
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
	if len(vec) != PostgresEmbeddingDimensions {
		return false, fmt.Errorf(
			"update memory embedding: vector has %d dimensions, Postgres schema requires %d",
			len(vec),
			PostgresEmbeddingDimensions,
		)
	}
	result, err := s.db.ExecContext(ctx,
		`UPDATE memories
		 SET embedding = $1::vector, embed_model = $2, embed_dims = $3,
		     semantic_space_id = $4
		 WHERE id = $5 AND content_hash = $6`,
		vectorLiteral(vec),
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

func (s *PostgresStore) EmbeddingDimensionConstraint() (int, bool) {
	return PostgresEmbeddingDimensions, true
}

// MemoriesMissingEmbedding returns a page of non-deleted memories that have no
// embedding yet, ordered by id and starting after afterID (pass "" for the
// first page). Used by the reindex/backfill command to embed an existing
// corpus. Only id, content, and content hash are populated.
func (s *PostgresStore) MemoriesMissingEmbedding(ctx context.Context, afterID string, limit int) ([]*model.Memory, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, content, content_hash FROM memories
		 WHERE embedding IS NULL AND deleted_at IS NULL AND id > $1
		 ORDER BY id LIMIT $2`,
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
// whole corpus. `IS DISTINCT FROM` treats NULL embed_model as stale too.
func (s *PostgresStore) MemoriesStaleEmbedding(ctx context.Context, embedModel, afterID string, limit int) ([]*model.Memory, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, content, content_hash FROM memories
		 WHERE deleted_at IS NULL AND id > $1
		   AND (embedding IS NULL OR embed_model IS DISTINCT FROM $2)
		 ORDER BY id LIMIT $3`,
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

// MemoriesStaleEmbeddingSpace includes dimension identity in the generation
// check so a same-name model configured at another width is fully reindexed.
func (s *PostgresStore) MemoriesStaleEmbeddingSpace(ctx context.Context, embedModel string, embedDims int, afterID string, limit int) ([]*model.Memory, error) {
	if embedDims <= 0 {
		return nil, fmt.Errorf("list memories stale embedding: dimensions must be > 0")
	}
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, content, content_hash FROM memories
		 WHERE deleted_at IS NULL AND id > $1
		   AND (
		     embedding IS NULL
		     OR embed_model IS DISTINCT FROM $2
		     OR embed_dims IS DISTINCT FROM $3
		   )
		 ORDER BY id LIMIT $4`,
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

func (s *PostgresStore) MemoriesStaleInCompleteSemanticSpace(
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
		 WHERE deleted_at IS NULL AND id > $1
		   AND (
		     (
		       embedding IS NULL
		       AND (
		         metadata IS NULL
		         OR COALESCE(metadata->>'curation_status', '') NOT IN ('low_signal', 'near_duplicate')
		       )
		     )
		     OR (
		       embedding IS NOT NULL
		       AND semantic_space_id IS DISTINCT FROM $2
		     )
		   )
		 ORDER BY id LIMIT $3`,
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

func (s *PostgresStore) ProjectHasStaleSemanticSpace(
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
		   WHERE project_id = $1
		     AND deleted_at IS NULL
		     AND (
		       (
		         embedding IS NOT NULL
		         AND semantic_space_id IS DISTINCT FROM $2
		       )
		       OR (
		         embedding IS NULL
		         AND (
		           metadata IS NULL
		           OR COALESCE(metadata->>'curation_status', '')
		             NOT IN ('low_signal', 'near_duplicate')
		         )
		       )
		     )`+qualityFilterSQL+`
		 )`,
		projectID,
		space.ID(),
	).Scan(&stale)
	if err != nil {
		return false, fmt.Errorf("check semantic space coverage: %w", err)
	}
	return stale, nil
}

// SearchMemoriesByVector returns up to k project memories ranked by cosine
// distance to vec (nearest first), skipping rows without an embedding and
// applying the same quality filter as text search.
func (s *PostgresStore) SearchMemoriesByVector(ctx context.Context, projectID string, vec []float32, k int) ([]*model.Memory, error) {
	if k <= 0 {
		k = 20
	}
	if k > 100 {
		k = 100
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, project_id, category, content, content_hash, keywords, source, author_id, author_name, created_at, updated_at, deleted_at, metadata
		 FROM memories
		 WHERE project_id = $1 AND deleted_at IS NULL AND embedding IS NOT NULL`+qualityFilterSQL+`
		 ORDER BY embedding <=> $2::vector
		 LIMIT $3`,
		projectID, vectorLiteral(vec), k,
	)
	if err != nil {
		return nil, fmt.Errorf("vector search memories: %w", err)
	}
	defer rows.Close()
	return scanMemories(rows)
}

// SearchMemoriesByVectorSpace compares only vectors produced by the active
// model at the active width. Raw scores from another semantic space are never
// eligible, even when dimensions happen to match.
func (s *PostgresStore) SearchMemoriesByVectorSpace(ctx context.Context, projectID string, vec []float32, embedModel string, embedDims, k int) ([]*model.Memory, error) {
	if embedModel == "" {
		return nil, fmt.Errorf("vector search memories: embedding model is required")
	}
	if embedDims <= 0 || len(vec) != embedDims {
		return nil, fmt.Errorf("vector search memories: query dimensions=%d, declared dimensions=%d", len(vec), embedDims)
	}
	if embedDims != PostgresEmbeddingDimensions {
		return nil, fmt.Errorf(
			"vector search memories: dimensions=%d, Postgres schema requires %d",
			embedDims,
			PostgresEmbeddingDimensions,
		)
	}
	if k <= 0 {
		k = 20
	}
	if k > 100 {
		k = 100
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, project_id, category, content, content_hash, keywords, source, author_id, author_name, created_at, updated_at, deleted_at, metadata
		 FROM memories
		 WHERE project_id = $1
		   AND deleted_at IS NULL
		   AND embedding IS NOT NULL
		   AND embed_model = $3
		   AND embed_dims = $4`+qualityFilterSQL+`
		 ORDER BY embedding <=> $2::vector
		 LIMIT $5`,
		projectID, vectorLiteral(vec), embedModel, embedDims, k,
	)
	if err != nil {
		return nil, fmt.Errorf("vector search memories in semantic space: %w", err)
	}
	defer rows.Close()
	return scanMemories(rows)
}

func (s *PostgresStore) SearchMemoriesByCompleteSemanticSpace(
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
	if space.Dimensions != PostgresEmbeddingDimensions {
		return nil, fmt.Errorf(
			"vector search memories: dimensions=%d, Postgres schema requires %d",
			space.Dimensions,
			PostgresEmbeddingDimensions,
		)
	}
	if k <= 0 {
		k = 20
	}
	if k > 100 {
		k = 100
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, project_id, category, content, content_hash, keywords, source, author_id, author_name, created_at, updated_at, deleted_at, metadata
		 FROM memories
		 WHERE project_id = $1
		   AND deleted_at IS NULL
		   AND embedding IS NOT NULL
		   AND semantic_space_id = $3`+qualityFilterSQL+`
		 ORDER BY embedding <=> $2::vector
		 LIMIT $4`,
		projectID, vectorLiteral(vec), space.ID(), k,
	)
	if err != nil {
		return nil, fmt.Errorf("vector search memories in semantic space: %w", err)
	}
	defer rows.Close()
	return scanMemories(rows)
}
