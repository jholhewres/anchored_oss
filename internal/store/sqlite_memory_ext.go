package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jholhewres/anchored_oss/internal/model"
)

func (s *SQLiteStore) GetMemoryByID(ctx context.Context, id string) (*model.Memory, error) {
	var m model.Memory
	var metadataBytes []byte
	var kwBytes []byte
	err := s.db.QueryRowContext(ctx,
		`SELECT id, project_id, category, content, content_hash, keywords, source, author_id, author_name, created_at, updated_at, deleted_at, metadata
		 FROM memories WHERE id = ?`,
		id,
	).Scan(&m.ID, &m.ProjectID, &m.Category, &m.Content, &m.ContentHash,
		&kwBytes, &m.Source, scanNullString(&m.AuthorID), &m.AuthorName,
		scanTime(&m.CreatedAt), scanTime(&m.UpdatedAt), scanNullTime(&m.DeletedAt), &metadataBytes)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get memory by id: %w", err)
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
	return &m, nil
}

func (s *SQLiteStore) UpdateMemoryMetadata(ctx context.Context, id string, metadata any) error {
	b, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}
	// SQLite: read existing metadata, merge in Go, write back.
	var existing []byte
	err = s.db.QueryRowContext(ctx, `SELECT metadata FROM memories WHERE id = ?`, id).Scan(&existing)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("read existing metadata: %w", err)
	}

	merged := make(map[string]any)
	if len(existing) > 0 {
		if err := json.Unmarshal(existing, &merged); err != nil {
			return fmt.Errorf("unmarshal existing metadata: %w", err)
		}
		// A column holding the JSON literal `null` unmarshals to a nil map;
		// re-initialize so the merge below doesn't panic.
		if merged == nil {
			merged = make(map[string]any)
		}
	}
	var patch map[string]any
	if err := json.Unmarshal(b, &patch); err != nil {
		return fmt.Errorf("unmarshal patch metadata: %w", err)
	}
	for k, v := range patch {
		merged[k] = v
	}
	out, err := json.Marshal(merged)
	if err != nil {
		return fmt.Errorf("marshal merged metadata: %w", err)
	}

	if _, err := s.db.ExecContext(ctx,
		`UPDATE memories SET metadata = ? WHERE id = ?`,
		string(out), id,
	); err != nil {
		return fmt.Errorf("update memory metadata: %w", err)
	}
	return nil
}

func (s *SQLiteStore) UpdateMemoryMetadataIfContent(
	ctx context.Context,
	id string,
	expectedContentHash string,
	metadata any,
) (bool, error) {
	if expectedContentHash == "" {
		return false, fmt.Errorf("update memory metadata: expected content hash is required")
	}
	b, err := json.Marshal(metadata)
	if err != nil {
		return false, fmt.Errorf("marshal metadata: %w", err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin metadata update: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var existing []byte
	var currentContentHash string
	err = tx.QueryRowContext(
		ctx,
		`SELECT metadata, content_hash FROM memories WHERE id = ?`,
		id,
	).Scan(&existing, &currentContentHash)
	if errors.Is(err, sql.ErrNoRows) {
		return false, ErrNotFound
	}
	if err != nil {
		return false, fmt.Errorf("read existing metadata: %w", err)
	}
	if currentContentHash != expectedContentHash {
		return false, nil
	}

	merged := make(map[string]any)
	if len(existing) > 0 {
		if err := json.Unmarshal(existing, &merged); err != nil {
			return false, fmt.Errorf("unmarshal existing metadata: %w", err)
		}
		if merged == nil {
			merged = make(map[string]any)
		}
	}
	var patch map[string]any
	if err := json.Unmarshal(b, &patch); err != nil {
		return false, fmt.Errorf("unmarshal patch metadata: %w", err)
	}
	for key, value := range patch {
		merged[key] = value
	}
	out, err := json.Marshal(merged)
	if err != nil {
		return false, fmt.Errorf("marshal merged metadata: %w", err)
	}
	result, err := tx.ExecContext(
		ctx,
		`UPDATE memories SET metadata = ? WHERE id = ? AND content_hash = ?`,
		string(out),
		id,
		expectedContentHash,
	)
	if err != nil {
		return false, fmt.Errorf("update memory metadata: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("inspect memory metadata update: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit metadata update: %w", err)
	}
	return affected == 1, nil
}

func (s *SQLiteStore) ListProjectMemoriesSince(ctx context.Context, projectID string, since time.Time) ([]*model.Memory, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, project_id, category, content, content_hash, keywords, source, author_id, author_name, created_at, updated_at, deleted_at, metadata
		 FROM memories WHERE project_id = ? AND deleted_at IS NULL AND created_at >= ?
		 ORDER BY created_at`,
		projectID, since,
	)
	if err != nil {
		return nil, fmt.Errorf("list project memories since: %w", err)
	}
	defer rows.Close()
	return sqliteScanMemories(rows)
}

// ListMemoriesByCurationStatus pages live memories whose metadata
// curation_status equals status, newest first.
func (s *SQLiteStore) ListMemoriesByCurationStatus(ctx context.Context, projectID, status string, limit int) ([]*model.Memory, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, project_id, category, content, content_hash, keywords, source, author_id, author_name, created_at, updated_at, deleted_at, metadata
		 FROM memories
		 WHERE project_id = ? AND deleted_at IS NULL
		   AND json_extract(metadata, '$.curation_status') = ?
		 ORDER BY updated_at DESC
		 LIMIT ?`,
		projectID, status, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list memories by curation status: %w", err)
	}
	defer rows.Close()
	return sqliteScanMemories(rows)
}

// CountCanonicalMembers mirrors the Postgres implementation for SQLite.
func (s *SQLiteStore) CountCanonicalMembers(ctx context.Context, projectID, canonicalID string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM memories
		 WHERE project_id = ? AND deleted_at IS NULL
		   AND json_extract(metadata, '$.canonical_of') = ?`,
		projectID, canonicalID,
	).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count canonical members: %w", err)
	}
	return n, nil
}
