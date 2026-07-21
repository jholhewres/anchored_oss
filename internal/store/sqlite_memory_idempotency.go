package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jholhewres/anchored_oss/internal/model"
)

// UpsertMemoryIdempotent is the SQLite equivalent of the Postgres transaction.
// SQLiteStore serializes these short write transactions in-process; SQLite's
// own writer lock still protects separate processes.
func (s *SQLiteStore) UpsertMemoryIdempotent(
	ctx context.Context,
	orgID, actorID, operationID, payloadHash string,
	m *model.Memory,
) (*model.Memory, bool, error) {
	result, replayed, _, err := s.UpsertMemoryIdempotentWithStatus(
		ctx,
		orgID,
		actorID,
		operationID,
		payloadHash,
		m,
	)
	return result, replayed, err
}

func (s *SQLiteStore) UpsertMemoryIdempotentWithStatus(
	ctx context.Context,
	orgID, actorID, operationID, payloadHash string,
	m *model.Memory,
) (*model.Memory, bool, bool, error) {
	responseJSON, err := encodeIdempotentMemoryRecord(m, false)
	if err != nil {
		return nil, false, false, fmt.Errorf("marshal idempotent memory response: %w", err)
	}
	s.idempotencyMu.Lock()
	defer s.idempotencyMu.Unlock()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, false, fmt.Errorf("begin idempotent memory write: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	result, err := tx.ExecContext(ctx,
		`INSERT OR IGNORE INTO memory_write_idempotency
		   (org_scope, actor_scope, operation_id, payload_hash, memory_id, response_json)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		orgID, actorID, operationID, payloadHash, m.ID, responseJSON,
	)
	if err != nil {
		return nil, false, false, fmt.Errorf("reserve memory idempotency key: %w", err)
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return nil, false, false, fmt.Errorf("inspect memory idempotency reservation: %w", err)
	}
	if inserted == 0 {
		replayed, created, err := sqliteIdempotentMemoryStatus(ctx, tx, orgID, actorID, operationID, payloadHash)
		if err != nil {
			return nil, false, false, err
		}
		if err := tx.Commit(); err != nil {
			return nil, false, false, fmt.Errorf("commit idempotent memory replay: %w", err)
		}
		return replayed, true, created, nil
	}

	created, err := sqliteUpsertMemoryStatusResult(ctx, tx, m)
	if err != nil {
		return nil, false, false, fmt.Errorf("upsert idempotent memory: %w", err)
	}
	authoritative, err := sqliteMemoryByID(ctx, tx, m.ID)
	if err != nil {
		return nil, false, false, fmt.Errorf("load authoritative idempotent memory: %w", err)
	}
	responseJSON, err = encodeIdempotentMemoryRecord(authoritative, created)
	if err != nil {
		return nil, false, false, fmt.Errorf("marshal authoritative idempotent memory: %w", err)
	}
	result, err = tx.ExecContext(ctx,
		`UPDATE memory_write_idempotency
		 SET memory_id = ?, response_json = ?
		 WHERE org_scope = ? AND actor_scope = ? AND operation_id = ?`,
		authoritative.ID, responseJSON, orgID, actorID, operationID,
	)
	if err != nil {
		return nil, false, false, fmt.Errorf("snapshot authoritative idempotent memory: %w", err)
	}
	if changed, rowsErr := result.RowsAffected(); rowsErr != nil {
		return nil, false, false, fmt.Errorf("inspect authoritative idempotency snapshot: %w", rowsErr)
	} else if changed != 1 {
		return nil, false, false, fmt.Errorf(
			"snapshot authoritative idempotent memory affected %d rows, want 1",
			changed,
		)
	}

	if err := tx.Commit(); err != nil {
		return nil, false, false, fmt.Errorf("commit idempotent memory write: %w", err)
	}
	if err := s.EnqueueCuration(ctx, []string{authoritative.ID}); err != nil {
		slog.Default().Warn("enqueue curation failed", "memory_id", authoritative.ID, "error", err)
	}
	return authoritative, false, created, nil
}

// GetMemoryIdempotency returns a committed SQLite operation response. It does
// not take idempotencyMu: a concurrent uncommitted first write is intentionally
// treated as not found and is resolved by UpsertMemoryIdempotent's transaction.
func (s *SQLiteStore) GetMemoryIdempotency(
	ctx context.Context,
	orgID, actorID, operationID, payloadHash string,
) (*model.Memory, bool, error) {
	memory, _, found, err := s.GetMemoryIdempotencyWithStatus(
		ctx,
		orgID,
		actorID,
		operationID,
		payloadHash,
	)
	return memory, found, err
}

func (s *SQLiteStore) GetMemoryIdempotencyWithStatus(
	ctx context.Context,
	orgID, actorID, operationID, payloadHash string,
) (*model.Memory, bool, bool, error) {
	var existingHash string
	var responseJSON []byte
	err := s.db.QueryRowContext(ctx,
		`SELECT payload_hash, response_json
		 FROM memory_write_idempotency
		 WHERE org_scope = ? AND actor_scope = ? AND operation_id = ?`,
		orgID, actorID, operationID,
	).Scan(&existingHash, &responseJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, false, nil
	}
	if err != nil {
		return nil, false, false, fmt.Errorf("load memory idempotency record: %w", err)
	}
	memory, created, err := decodeIdempotentMemoryRecord(existingHash, payloadHash, responseJSON)
	if err != nil {
		return nil, false, false, err
	}
	return memory, created, true, nil
}

func (s *SQLiteStore) PurgeMemoryIdempotencyOlderThan(
	ctx context.Context,
	before time.Time,
) (int64, error) {
	result, err := s.db.ExecContext(
		ctx,
		`DELETE FROM memory_write_idempotency WHERE created_at < ?`,
		before,
	)
	if err != nil {
		return 0, fmt.Errorf("purge memory idempotency records: %w", err)
	}
	removed, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("inspect memory idempotency purge: %w", err)
	}
	return removed, nil
}

func sqliteIdempotentMemoryStatus(
	ctx context.Context,
	tx *sql.Tx,
	orgID, actorID, operationID, payloadHash string,
) (*model.Memory, bool, error) {
	var existingHash string
	var responseJSON []byte
	err := tx.QueryRowContext(ctx,
		`SELECT payload_hash, response_json
		 FROM memory_write_idempotency
		 WHERE org_scope = ? AND actor_scope = ? AND operation_id = ?`,
		orgID, actorID, operationID,
	).Scan(&existingHash, &responseJSON)
	if err != nil {
		return nil, false, fmt.Errorf("load memory idempotency record: %w", err)
	}
	return decodeIdempotentMemoryRecord(existingHash, payloadHash, responseJSON)
}

func sqliteMemoryByID(
	ctx context.Context,
	querier queryRower,
	id string,
) (*model.Memory, error) {
	var memory model.Memory
	var keywordsJSON, metadataJSON []byte
	err := querier.QueryRowContext(ctx,
		`SELECT id, project_id, category, content, content_hash, keywords,
		 source, author_id, author_name, created_at, updated_at, deleted_at, metadata
		 FROM memories WHERE id = ?`,
		id,
	).Scan(
		&memory.ID,
		&memory.ProjectID,
		&memory.Category,
		&memory.Content,
		&memory.ContentHash,
		&keywordsJSON,
		&memory.Source,
		scanNullString(&memory.AuthorID),
		&memory.AuthorName,
		scanTime(&memory.CreatedAt),
		scanTime(&memory.UpdatedAt),
		scanNullTime(&memory.DeletedAt),
		&metadataJSON,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if len(keywordsJSON) > 0 {
		if err := json.Unmarshal(keywordsJSON, &memory.Keywords); err != nil {
			return nil, fmt.Errorf("decode authoritative memory keywords: %w", err)
		}
	}
	if len(metadataJSON) > 0 {
		if err := json.Unmarshal(metadataJSON, &memory.Metadata); err != nil {
			return nil, fmt.Errorf("decode authoritative memory metadata: %w", err)
		}
	}
	return &memory, nil
}
