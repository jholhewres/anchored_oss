package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jholhewres/anchored_oss/internal/model"
	"github.com/lib/pq"
)

func (s *PostgresStore) UpsertMemory(ctx context.Context, m *model.Memory) error {
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
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		 ON CONFLICT (content_hash, project_id) WHERE deleted_at IS NULL
		 DO UPDATE SET content = EXCLUDED.content, updated_at = EXCLUDED.updated_at, keywords = EXCLUDED.keywords, source = EXCLUDED.source, author_id = EXCLUDED.author_id, author_name = EXCLUDED.author_name, metadata = EXCLUDED.metadata`,
		m.ID, m.ProjectID, m.Category, m.Content, m.ContentHash,
		pq.Array(m.Keywords), m.Source, m.AuthorID, m.AuthorName,
		m.CreatedAt, m.UpdatedAt, metadataBytes,
	)
	if err != nil {
		return fmt.Errorf("upsert memory: %w", err)
	}
	return nil
}

func (s *PostgresStore) GetMemoriesByProject(ctx context.Context, projectID string, limit, offset int) ([]*model.Memory, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, project_id, category, content, content_hash, keywords, source, author_id, author_name, created_at, updated_at, deleted_at, metadata
		 FROM memories
		 WHERE project_id = $1 AND deleted_at IS NULL
		 ORDER BY updated_at DESC
		 LIMIT $2 OFFSET $3`,
		projectID, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("get memories by project: %w", err)
	}
	defer rows.Close()

	return scanMemories(rows)
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

func (s *PostgresStore) SoftDeleteMemory(ctx context.Context, id, projectID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE memories SET deleted_at = now(), updated_at = now() WHERE id = $1 AND project_id = $2`,
		id, projectID,
	)
	if err != nil {
		return fmt.Errorf("soft delete memory: %w", err)
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
