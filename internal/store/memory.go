package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jholhewres/anchored_oss/internal/model"
	"github.com/lib/pq"
)

// memoryUpsertCols counts the columns inserted per row by UpsertMemories.
const memoryUpsertCols = 12

// memoryBatchSize splits an UpsertMemories call into chunks that stay
// below Postgres' 65535-parameter limit (12 cols * 5000 rows = 60000).
const memoryBatchSize = 1000

func (s *PostgresStore) UpsertMemory(ctx context.Context, m *model.Memory) error {
	var metadataBytes []byte
	if m.Metadata != nil {
		var err error
		metadataBytes, err = json.Marshal(m.Metadata)
		if err != nil {
			return fmt.Errorf("marshal memory metadata: %w", err)
		}
	}

	// Last-write-wins by (id): editing a memory keeps the same id but may
	// change content/hash. The partial unique index on
	// (content_hash, project_id) still blocks accidental content-level dupes
	// from a different id, surfacing as a unique-violation error.
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO memories (id, project_id, category, content, content_hash, keywords, source, author_id, author_name, created_at, updated_at, metadata)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		 ON CONFLICT (id) DO UPDATE SET
		   category = EXCLUDED.category,
		   content = EXCLUDED.content,
		   content_hash = EXCLUDED.content_hash,
		   keywords = EXCLUDED.keywords,
		   source = EXCLUDED.source,
		   author_id = EXCLUDED.author_id,
		   author_name = EXCLUDED.author_name,
		   updated_at = EXCLUDED.updated_at,
		   metadata = EXCLUDED.metadata,
		   deleted_at = NULL
		 WHERE memories.updated_at <= EXCLUDED.updated_at`,
		m.ID, m.ProjectID, m.Category, m.Content, m.ContentHash,
		pq.Array(m.Keywords), m.Source, m.AuthorID, m.AuthorName,
		m.CreatedAt, m.UpdatedAt, metadataBytes,
	)
	if err != nil {
		return fmt.Errorf("upsert memory: %w", err)
	}
	return nil
}

// UpsertMemories runs a single batched INSERT ... ON CONFLICT (id) DO UPDATE
// for every memory in ms. Internally chunks by memoryBatchSize so we stay
// under the Postgres parameter limit.
func (s *PostgresStore) UpsertMemories(ctx context.Context, ms []*model.Memory) error {
	if len(ms) == 0 {
		return nil
	}
	for start := 0; start < len(ms); start += memoryBatchSize {
		end := start + memoryBatchSize
		if end > len(ms) {
			end = len(ms)
		}
		if err := s.upsertMemoriesChunk(ctx, ms[start:end]); err != nil {
			return err
		}
	}
	return nil
}

func (s *PostgresStore) upsertMemoriesChunk(ctx context.Context, ms []*model.Memory) error {
	args := make([]any, 0, memoryUpsertCols*len(ms))
	placeholders := make([]string, 0, len(ms))
	for i, m := range ms {
		var metadataBytes []byte
		if m.Metadata != nil {
			b, err := json.Marshal(m.Metadata)
			if err != nil {
				return fmt.Errorf("marshal memory metadata: %w", err)
			}
			metadataBytes = b
		}
		base := i * memoryUpsertCols
		placeholders = append(placeholders, fmt.Sprintf(
			"($%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d)",
			base+1, base+2, base+3, base+4, base+5, base+6,
			base+7, base+8, base+9, base+10, base+11, base+12,
		))
		args = append(args,
			m.ID, m.ProjectID, m.Category, m.Content, m.ContentHash,
			pq.Array(m.Keywords), m.Source, nilIfEmpty(m.AuthorID), m.AuthorName,
			m.CreatedAt, m.UpdatedAt, metadataBytes,
		)
	}

	query := `INSERT INTO memories (id, project_id, category, content, content_hash, keywords, source, author_id, author_name, created_at, updated_at, metadata) VALUES ` +
		strings.Join(placeholders, ",") +
		` ON CONFLICT (id) DO UPDATE SET
		   category = EXCLUDED.category,
		   content = EXCLUDED.content,
		   content_hash = EXCLUDED.content_hash,
		   keywords = EXCLUDED.keywords,
		   source = EXCLUDED.source,
		   author_id = EXCLUDED.author_id,
		   author_name = EXCLUDED.author_name,
		   updated_at = EXCLUDED.updated_at,
		   metadata = EXCLUDED.metadata,
		   deleted_at = NULL
		 WHERE memories.updated_at <= EXCLUDED.updated_at`

	if _, err := s.db.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("upsert memories batch: %w", err)
	}
	return nil
}

func (s *PostgresStore) GetMemoriesUpdatedSince(ctx context.Context, projectID string, since time.Time) ([]*model.Memory, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, project_id, category, content, content_hash, keywords, source, author_id, author_name, created_at, updated_at, deleted_at, metadata
		 FROM memories
		 WHERE project_id = $1 AND updated_at > $2 AND deleted_at IS NULL
		 ORDER BY updated_at`,
		projectID, since,
	)
	if err != nil {
		return nil, fmt.Errorf("get memories updated since: %w", err)
	}
	defer rows.Close()

	return scanMemories(rows)
}

const (
	memoryDefaultLimit = 20
	memoryMaxLimit     = 200
)

// ListMemoriesPaginated returns a page of memories for a non-deleted project.
// Returns ErrNotFound when the project is missing or soft-deleted so handlers
// can map directly to 404.
func (s *PostgresStore) ListMemoriesPaginated(ctx context.Context, projectID string, limit, offset int) ([]*model.Memory, int, error) {
	// Guard: project must be live. We piggyback on GetActiveProjectByID so the
	// caller cannot peek at a soft-deleted project's memories.
	if _, err := s.GetActiveProjectByID(ctx, projectID); err != nil {
		return nil, 0, err
	}

	if limit <= 0 {
		limit = memoryDefaultLimit
	}
	if limit > memoryMaxLimit {
		limit = memoryMaxLimit
	}
	if offset < 0 {
		offset = 0
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, project_id, category, content, content_hash, keywords, source,
		        author_id, author_name, created_at, updated_at, deleted_at, metadata,
		        COUNT(*) OVER() AS total
		 FROM memories
		 WHERE project_id = $1 AND deleted_at IS NULL
		 ORDER BY updated_at DESC
		 LIMIT $2 OFFSET $3`,
		projectID, limit, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("list memories paginated: %w", err)
	}
	defer rows.Close()

	memories := make([]*model.Memory, 0, limit)
	total := 0
	for rows.Next() {
		var m model.Memory
		var metadataBytes []byte
		if err := rows.Scan(
			&m.ID, &m.ProjectID, &m.Category, &m.Content, &m.ContentHash,
			pq.Array(&m.Keywords), &m.Source, &m.AuthorID, &m.AuthorName,
			&m.CreatedAt, &m.UpdatedAt, &m.DeletedAt, &metadataBytes, &total,
		); err != nil {
			return nil, 0, fmt.Errorf("scan memory: %w", err)
		}
		if metadataBytes != nil {
			if err := json.Unmarshal(metadataBytes, &m.Metadata); err != nil {
				return nil, 0, fmt.Errorf("unmarshal memory metadata: %w", err)
			}
		}
		memories = append(memories, &m)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate memories: %w", err)
	}
	return memories, total, nil
}

func (s *PostgresStore) SoftDeleteMemory(ctx context.Context, id, projectID string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE memories SET deleted_at = now(), updated_at = now() WHERE id = $1 AND project_id = $2 AND deleted_at IS NULL`,
		id, projectID,
	)
	if err != nil {
		return fmt.Errorf("soft delete memory: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("soft delete rows affected: %w", err)
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PostgresStore) GetTombstonesSince(ctx context.Context, projectID string, since time.Time) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id FROM memories WHERE project_id = $1 AND deleted_at IS NOT NULL AND updated_at > $2`,
		projectID, since,
	)
	if err != nil {
		return nil, fmt.Errorf("get tombstones since: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan tombstone: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tombstones: %w", err)
	}
	return ids, nil
}

func scanMemories(rows *sql.Rows) ([]*model.Memory, error) {
	var memories []*model.Memory
	for rows.Next() {
		var m model.Memory
		var metadataBytes []byte
		if err := rows.Scan(
			&m.ID, &m.ProjectID, &m.Category, &m.Content, &m.ContentHash,
			pq.Array(&m.Keywords), &m.Source, &m.AuthorID, &m.AuthorName,
			&m.CreatedAt, &m.UpdatedAt, &m.DeletedAt, &metadataBytes,
		); err != nil {
			return nil, fmt.Errorf("scan memory: %w", err)
		}
		if metadataBytes != nil {
			if err := json.Unmarshal(metadataBytes, &m.Metadata); err != nil {
				return nil, fmt.Errorf("unmarshal memory metadata: %w", err)
			}
		}
		memories = append(memories, &m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate memories: %w", err)
	}
	return memories, nil
}
