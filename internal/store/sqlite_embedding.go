package store

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"

	"github.com/jholhewres/anchored_oss/internal/model"
)

// UpdateMemoryEmbedding stores the vector as a JSON float array (SQLite has no
// pgvector) plus the producing model.
func (s *SQLiteStore) UpdateMemoryEmbedding(ctx context.Context, memoryID string, vec []float32, model string) error {
	blob, err := json.Marshal(vec)
	if err != nil {
		return fmt.Errorf("marshal embedding: %w", err)
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE memories SET embedding = ?, embed_model = ?, embed_dims = ? WHERE id = ?`,
		string(blob), model, len(vec), memoryID,
	); err != nil {
		return fmt.Errorf("update memory embedding: %w", err)
	}
	return nil
}

// MemoriesMissingEmbedding returns a page of non-deleted memories without an
// embedding, ordered by id and starting after afterID. Only id/content are set.
func (s *SQLiteStore) MemoriesMissingEmbedding(ctx context.Context, afterID string, limit int) ([]*model.Memory, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, content FROM memories
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
		if err := rows.Scan(&m.ID, &m.Content); err != nil {
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
		`SELECT id, content FROM memories
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
		if err := rows.Scan(&m.ID, &m.Content); err != nil {
			return nil, err
		}
		out = append(out, &m)
	}
	return out, rows.Err()
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
