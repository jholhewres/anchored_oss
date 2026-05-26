package store

import (
	"context"
	"fmt"
	"strings"
)

func (s *SQLiteStore) EnqueueCuration(ctx context.Context, memoryIDs []string) error {
	if len(memoryIDs) == 0 {
		return nil
	}
	placeholders := make([]string, len(memoryIDs))
	args := make([]any, len(memoryIDs))
	for i, id := range memoryIDs {
		placeholders[i] = "(?)"
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

// ClaimCurationBatch atomically selects pending rows and marks them processing.
// SQLite doesn't support FOR UPDATE SKIP LOCKED; use a transaction + two statements.
func (s *SQLiteStore) ClaimCurationBatch(ctx context.Context, batchSize int) ([]string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx,
		`SELECT memory_id FROM curation_queue WHERE status = 'pending' ORDER BY scheduled_at LIMIT ?`,
		batchSize,
	)
	if err != nil {
		return nil, fmt.Errorf("select curation batch: %w", err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan memory_id: %w", err)
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate curation batch: %w", err)
	}

	for _, id := range ids {
		if _, err := tx.ExecContext(ctx,
			`UPDATE curation_queue SET status = 'processing', updated_at = datetime('now') WHERE memory_id = ?`,
			id,
		); err != nil {
			return nil, fmt.Errorf("mark processing %s: %w", id, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit claim batch: %w", err)
	}
	return ids, nil
}

func (s *SQLiteStore) SetCurationDone(ctx context.Context, memoryID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE curation_queue SET status = 'done', updated_at = datetime('now') WHERE memory_id = ?`,
		memoryID,
	)
	if err != nil {
		return fmt.Errorf("set curation done: %w", err)
	}
	return nil
}

func (s *SQLiteStore) SetCurationFailed(ctx context.Context, memoryID, errMsg string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE curation_queue
		 SET attempts = attempts + 1,
		     last_error = ?,
		     status = CASE WHEN attempts + 1 >= 5 THEN 'failed' ELSE 'pending' END,
		     updated_at = datetime('now')
		 WHERE memory_id = ?`,
		errMsg, memoryID,
	)
	if err != nil {
		return fmt.Errorf("set curation failed: %w", err)
	}
	return nil
}
