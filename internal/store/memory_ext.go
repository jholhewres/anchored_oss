package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jholhewres/anchored_oss/internal/model"
	"github.com/lib/pq"
)

// GetMemoryByID returns a single memory by its ID (including soft-deleted).
func (s *PostgresStore) GetMemoryByID(ctx context.Context, id string) (*model.Memory, error) {
	var m model.Memory
	var metadataBytes []byte
	err := s.db.QueryRowContext(ctx,
		`SELECT id, project_id, category, content, content_hash, keywords, source, author_id, author_name, created_at, updated_at, deleted_at, metadata
		 FROM memories WHERE id = $1`,
		id,
	).Scan(&m.ID, &m.ProjectID, &m.Category, &m.Content, &m.ContentHash,
		pq.Array(&m.Keywords), &m.Source, &m.AuthorID, &m.AuthorName,
		&m.CreatedAt, &m.UpdatedAt, &m.DeletedAt, &metadataBytes)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get memory by id: %w", err)
	}
	if metadataBytes != nil {
		if err := json.Unmarshal(metadataBytes, &m.Metadata); err != nil {
			return nil, fmt.Errorf("unmarshal memory metadata: %w", err)
		}
	}
	return &m, nil
}

// UpdateMemoryMetadata merges the provided metadata into the existing JSONB
// blob using jsonb_set-compatible concatenation (|| operator).
func (s *PostgresStore) UpdateMemoryMetadata(ctx context.Context, id string, metadata any) error {
	b, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}
	_, err = s.db.ExecContext(ctx,
		`UPDATE memories SET metadata = COALESCE(metadata, '{}'::jsonb) || $1::jsonb WHERE id = $2`,
		string(b), id,
	)
	if err != nil {
		return fmt.Errorf("update memory metadata: %w", err)
	}
	return nil
}

// ListProjectMemoriesSince returns all non-deleted memories in a project
// created or updated since the given time (used by curation for near-dup).
func (s *PostgresStore) ListProjectMemoriesSince(ctx context.Context, projectID string, since time.Time) ([]*model.Memory, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, project_id, category, content, content_hash, keywords, source, author_id, author_name, created_at, updated_at, deleted_at, metadata
		 FROM memories WHERE project_id = $1 AND deleted_at IS NULL AND created_at >= $2
		 ORDER BY created_at`,
		projectID, since,
	)
	if err != nil {
		return nil, fmt.Errorf("list project memories since: %w", err)
	}
	defer rows.Close()
	return scanMemories(rows)
}

// ListMemoriesByCurationStatus pages live memories whose metadata
// curation_status equals status, newest first.
func (s *PostgresStore) ListMemoriesByCurationStatus(ctx context.Context, projectID, status string, limit int) ([]*model.Memory, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, project_id, category, content, content_hash, keywords, source, author_id, author_name, created_at, updated_at, deleted_at, metadata
		 FROM memories
		 WHERE project_id = $1 AND deleted_at IS NULL AND metadata ->> 'curation_status' = $2
		 ORDER BY updated_at DESC
		 LIMIT $3`,
		projectID, status, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list memories by curation status: %w", err)
	}
	defer rows.Close()
	return scanMemories(rows)
}

// CountCanonicalMembers counts live memories marked as near-duplicates of
// canonicalID (metadata canonical_of). Drives the advisory consolidation
// marking: a canonical with several members is a synthesis candidate.
func (s *PostgresStore) CountCanonicalMembers(ctx context.Context, projectID, canonicalID string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM memories
		 WHERE project_id = $1 AND deleted_at IS NULL AND metadata ->> 'canonical_of' = $2`,
		projectID, canonicalID,
	).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count canonical members: %w", err)
	}
	return n, nil
}
