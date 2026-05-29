package store

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/jholhewres/anchored_oss/internal/model"
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
	_, err := s.db.ExecContext(ctx,
		`UPDATE memories SET embedding = $1::vector, embed_model = $2, embed_dims = $3 WHERE id = $4`,
		vectorLiteral(vec), model, len(vec), memoryID,
	)
	if err != nil {
		return fmt.Errorf("update memory embedding: %w", err)
	}
	return nil
}

// MemoriesMissingEmbedding returns a page of non-deleted memories that have no
// embedding yet, ordered by id and starting after afterID (pass "" for the
// first page). Used by the reindex/backfill command to embed an existing
// corpus. Only id and content are populated.
func (s *PostgresStore) MemoriesMissingEmbedding(ctx context.Context, afterID string, limit int) ([]*model.Memory, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, content FROM memories
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
		if err := rows.Scan(&m.ID, &m.Content); err != nil {
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
		`SELECT id, content FROM memories
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
		if err := rows.Scan(&m.ID, &m.Content); err != nil {
			return nil, err
		}
		out = append(out, &m)
	}
	return out, rows.Err()
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
