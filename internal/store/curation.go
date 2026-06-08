package store

import (
	"context"
	"fmt"
	"strings"
)

// EnqueueCuration inserts memory IDs into the curation_queue with status
// 'pending'. Silently ignores IDs already present (ON CONFLICT DO NOTHING).
func (s *PostgresStore) EnqueueCuration(ctx context.Context, memoryIDs []string) error {
	if len(memoryIDs) == 0 {
		return nil
	}
	placeholders := make([]string, len(memoryIDs))
	args := make([]any, len(memoryIDs))
	for i, id := range memoryIDs {
		placeholders[i] = fmt.Sprintf("($%d)", i+1)
		args[i] = id
	}
	query := `INSERT INTO curation_queue (memory_id) VALUES ` +
		strings.Join(placeholders, ",") +
		` ON CONFLICT (memory_id) DO NOTHING`
	if _, err := s.db.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("enqueue curation: %w", err)
	}
	return nil
}

// EnqueueRecuration (re)queues up to limit live memories whose curation_version
// is below 2 (or unset), resetting any existing queue row back to pending. Used
// to roll curation v2 marks out to memories last curated by an older worker.
func (s *PostgresStore) EnqueueRecuration(ctx context.Context, limit int) (int, error) {
	if limit <= 0 {
		return 0, nil
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id FROM memories
		 WHERE deleted_at IS NULL
		   AND COALESCE((metadata ->> 'curation_version')::int, 0) < 2
		 ORDER BY updated_at
		 LIMIT $1`,
		limit,
	)
	if err != nil {
		return 0, fmt.Errorf("select re-curation candidates: %w", err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scan re-curation id: %w", err)
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate re-curation candidates: %w", err)
	}
	if len(ids) == 0 {
		return 0, nil
	}

	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = fmt.Sprintf("($%d)", i+1)
		args[i] = id
	}
	query := `INSERT INTO curation_queue (memory_id) VALUES ` +
		strings.Join(placeholders, ",") +
		` ON CONFLICT (memory_id) DO UPDATE SET status = 'pending', attempts = 0, updated_at = now()`
	if _, err := s.db.ExecContext(ctx, query, args...); err != nil {
		return 0, fmt.Errorf("enqueue re-curation: %w", err)
	}
	return len(ids), nil
}

// ClaimCurationBatch atomically selects up to batchSize pending rows and
// marks them as 'processing'. Uses a CTE so the SELECT and UPDATE are atomic.
func (s *PostgresStore) ClaimCurationBatch(ctx context.Context, batchSize int) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`WITH claimed AS (
		   SELECT memory_id FROM curation_queue
		   WHERE status = 'pending'
		   ORDER BY scheduled_at
		   LIMIT $1
		   FOR UPDATE SKIP LOCKED
		 )
		 UPDATE curation_queue SET status = 'processing', updated_at = now()
		 FROM claimed
		 WHERE curation_queue.memory_id = claimed.memory_id
		 RETURNING curation_queue.memory_id`,
		batchSize,
	)
	if err != nil {
		return nil, fmt.Errorf("claim curation batch: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan memory_id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate curation batch: %w", err)
	}
	return ids, nil
}

func (s *PostgresStore) SetCurationDone(ctx context.Context, memoryID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE curation_queue SET status = 'done', updated_at = now() WHERE memory_id = $1`,
		memoryID,
	)
	if err != nil {
		return fmt.Errorf("set curation done: %w", err)
	}
	return nil
}

func (s *PostgresStore) SetCurationFailed(ctx context.Context, memoryID, errMsg string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE curation_queue
		 SET attempts = attempts + 1,
		     last_error = $1,
		     status = CASE WHEN attempts + 1 >= 5 THEN 'failed' ELSE 'pending' END,
		     updated_at = now()
		 WHERE memory_id = $2`,
		errMsg, memoryID,
	)
	if err != nil {
		return fmt.Errorf("set curation failed: %w", err)
	}
	return nil
}
