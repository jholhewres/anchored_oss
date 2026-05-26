package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jholhewres/anchored_oss/internal/model"
	"github.com/jholhewres/anchored_oss/internal/policy"
)

// ---------------------------------------------------------------------------
// Memory methods for SQLiteStore
// ---------------------------------------------------------------------------

// sqliteQualityFilterSQL hides low-signal / mis-categorized rows from read
// paths. Mirrors qualityFilterSQL in memory.go (Postgres) using SQLite's
// json_extract for metadata access.
var sqliteQualityFilterSQL = fmt.Sprintf(`
		   AND (metadata IS NULL OR json_extract(metadata, '$.curation_status') IS NOT 'low_signal')
		   AND (
		     metadata IS NULL
		     OR json_extract(metadata, '$.quality_score') IS NULL
		     OR CAST(json_extract(metadata, '$.quality_score') AS REAL) >= %f
		     OR json_extract(metadata, '$.pinned') = 1
		   )
		   AND (metadata IS NULL OR json_extract(metadata, '$.scope') IS NOT 'user')
		   AND (
		     metadata IS NULL
		     OR json_extract(metadata, '$.memory_type') IS NOT 'operational'
		     OR json_extract(metadata, '$.kind') = 'handoff'
		   )
		   AND (metadata IS NULL OR json_extract(metadata, '$.origin') IS NOT 'precompact')
		   AND (metadata IS NULL OR json_extract(metadata, '$.origin') IS NOT 'handoff')`,
	policy.RemoteQualityThreshold,
)

// SearchMemories does LIKE pattern matching on content and keywords.
func (s *SQLiteStore) SearchMemories(ctx context.Context, projectID string, query string, limit int) ([]*model.Memory, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	escaped := strings.ReplaceAll(query, "%", "\\%")
	escaped = strings.ReplaceAll(escaped, "_", "\\_")
	pattern := "%" + escaped + "%"
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, project_id, category, content, content_hash, keywords, source, author_id, author_name, created_at, updated_at, deleted_at, metadata
		 FROM memories
		 WHERE project_id = ? AND deleted_at IS NULL`+sqliteQualityFilterSQL+`
		   AND (content LIKE ? ESCAPE '\' OR keywords LIKE ? ESCAPE '\')
		 ORDER BY updated_at DESC
		 LIMIT ?`,
		projectID, pattern, pattern, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("search memories: %w", err)
	}
	defer rows.Close()

	return sqliteScanMemories(rows)
}

// UpsertMemory inserts or updates a single memory (last-write-wins by id).
func (s *SQLiteStore) UpsertMemory(ctx context.Context, m *model.Memory) error {
	var metadataBytes []byte
	if m.Metadata != nil {
		var err error
		metadataBytes, err = json.Marshal(m.Metadata)
		if err != nil {
			return fmt.Errorf("marshal memory metadata: %w", err)
		}
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO memories (id, project_id, category, content, content_hash, keywords, source, author_id, author_name, created_at, updated_at, metadata)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT (id) DO UPDATE SET
		   project_id = excluded.project_id,
		   category = excluded.category,
		   content = excluded.content,
		   content_hash = excluded.content_hash,
		   keywords = excluded.keywords,
		   source = excluded.source,
		   author_id = excluded.author_id,
		   author_name = excluded.author_name,
		   updated_at = excluded.updated_at,
		   metadata = excluded.metadata,
		   deleted_at = NULL
		 WHERE memories.updated_at <= excluded.updated_at`,
		m.ID, m.ProjectID, m.Category, m.Content, m.ContentHash,
		jsonMarshalKeywords(m.Keywords), m.Source, nilIfEmpty(m.AuthorID), m.AuthorName,
		m.CreatedAt, m.UpdatedAt, metadataBytes,
	)
	if err != nil {
		return fmt.Errorf("upsert memory: %w", err)
	}
	return nil
}

// UpsertMemories runs a single batched INSERT ... ON CONFLICT (id) DO UPDATE
// for every memory in ms, chunked by memoryBatchSize.
func (s *SQLiteStore) UpsertMemories(ctx context.Context, ms []*model.Memory) error {
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

func (s *SQLiteStore) upsertMemoriesChunk(ctx context.Context, ms []*model.Memory) error {
	args := make([]any, 0, memoryUpsertCols*len(ms))

	rowPlaceholder := "(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)"
	placeholders := make([]string, 0, len(ms))

	for _, m := range ms {
		var metadataBytes []byte
		if m.Metadata != nil {
			b, err := json.Marshal(m.Metadata)
			if err != nil {
				return fmt.Errorf("marshal memory metadata: %w", err)
			}
			metadataBytes = b
		}
		placeholders = append(placeholders, rowPlaceholder)
		args = append(args,
			m.ID, m.ProjectID, m.Category, m.Content, m.ContentHash,
			jsonMarshalKeywords(m.Keywords), m.Source, nilIfEmpty(m.AuthorID), m.AuthorName,
			m.CreatedAt, m.UpdatedAt, metadataBytes,
		)
	}

	query := `INSERT INTO memories (id, project_id, category, content, content_hash, keywords, source, author_id, author_name, created_at, updated_at, metadata) VALUES ` +
		strings.Join(placeholders, ",") +
		` ON CONFLICT (id) DO UPDATE SET
		   category = excluded.category,
		   content = excluded.content,
		   content_hash = excluded.content_hash,
		   keywords = excluded.keywords,
		   source = excluded.source,
		   author_id = excluded.author_id,
		   author_name = excluded.author_name,
		   updated_at = excluded.updated_at,
		   metadata = excluded.metadata,
		   deleted_at = NULL
		 WHERE memories.updated_at <= excluded.updated_at`

	if _, err := s.db.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("upsert memories batch: %w", err)
	}
	return nil
}

// GetMemoriesUpdatedSince returns all non-deleted memories updated after since.
func (s *SQLiteStore) GetMemoriesUpdatedSince(ctx context.Context, projectID string, since time.Time) ([]*model.Memory, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, project_id, category, content, content_hash, keywords, source, author_id, author_name, created_at, updated_at, deleted_at, metadata
		 FROM memories
		 WHERE project_id = ? AND updated_at > ? AND deleted_at IS NULL`+sqliteQualityFilterSQL+`
		 ORDER BY updated_at`,
		projectID, since,
	)
	if err != nil {
		return nil, fmt.Errorf("get memories updated since: %w", err)
	}
	defer rows.Close()

	return sqliteScanMemories(rows)
}

// ListMemoriesPaginated returns a page of memories for a non-deleted project.
func (s *SQLiteStore) ListMemoriesPaginated(ctx context.Context, projectID string, limit, offset int) ([]*model.Memory, int, error) {
	{
		var exists bool
		err := s.db.QueryRowContext(ctx,
			`SELECT EXISTS(SELECT 1 FROM projects WHERE id = ? AND deleted_at IS NULL)`,
			projectID,
		).Scan(&exists)
		if err != nil {
			return nil, 0, fmt.Errorf("check project: %w", err)
		}
		if !exists {
			return nil, 0, ErrNotFound
		}
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
		 WHERE project_id = ? AND deleted_at IS NULL`+sqliteQualityFilterSQL+`
		 ORDER BY updated_at DESC
		 LIMIT ? OFFSET ?`,
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
		var kwBytes []byte
		if err := rows.Scan(
			&m.ID, &m.ProjectID, &m.Category, &m.Content, &m.ContentHash,
			&kwBytes, &m.Source, &m.AuthorID, &m.AuthorName,
			scanTime(&m.CreatedAt), scanTime(&m.UpdatedAt), scanNullTime(&m.DeletedAt), &metadataBytes, &total,
		); err != nil {
			return nil, 0, fmt.Errorf("scan memory: %w", err)
		}
		if len(kwBytes) > 0 {
			if err := json.Unmarshal(kwBytes, &m.Keywords); err != nil {
				return nil, 0, fmt.Errorf("unmarshal memory keywords: %w", err)
			}
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

// SoftDeleteMemory sets deleted_at on a memory.
func (s *SQLiteStore) SoftDeleteMemory(ctx context.Context, id, projectID string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE memories SET deleted_at = datetime('now'), updated_at = datetime('now') WHERE id = ? AND project_id = ? AND deleted_at IS NULL`,
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

// GetTombstonesSince returns IDs of memories soft-deleted since the given time.
func (s *SQLiteStore) GetTombstonesSince(ctx context.Context, projectID string, since time.Time) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id FROM memories WHERE project_id = ? AND deleted_at IS NOT NULL AND updated_at > ?`,
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

// sqliteScanMemories scans rows into a slice of Memory, using JSON for keywords.
func sqliteScanMemories(rows *sql.Rows) ([]*model.Memory, error) {
	var memories []*model.Memory
	for rows.Next() {
		var m model.Memory
		var metadataBytes []byte
		var kwBytes []byte
		if err := rows.Scan(
			&m.ID, &m.ProjectID, &m.Category, &m.Content, &m.ContentHash,
			&kwBytes, &m.Source, &m.AuthorID, &m.AuthorName,
			scanTime(&m.CreatedAt), scanTime(&m.UpdatedAt), scanNullTime(&m.DeletedAt), &metadataBytes,
		); err != nil {
			return nil, fmt.Errorf("scan memory: %w", err)
		}
		if len(kwBytes) > 0 {
			if err := json.Unmarshal(kwBytes, &m.Keywords); err != nil {
				return nil, fmt.Errorf("unmarshal memory keywords: %w", err)
			}
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
