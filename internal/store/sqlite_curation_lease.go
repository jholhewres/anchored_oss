package store

import (
	"context"
	"fmt"
	"time"
)

// ClaimCurationBatchLeased selects pending rows and marks them processing,
// stamping the owner and a lease expiry. SQLite has no FOR UPDATE SKIP LOCKED,
// so it uses a transaction plus per-row updates (the same shape as the
// unleased ClaimCurationBatch).
func (s *SQLiteStore) ClaimCurationBatchLeased(ctx context.Context, batchSize int, owner string, leaseTTL time.Duration) ([]string, error) {
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

	leaseMod := fmt.Sprintf("+%d seconds", leaseSeconds(leaseTTL))
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx,
			`UPDATE curation_queue
			 SET status = 'processing', owner_id = ?, lease_expires_at = datetime('now', ?), updated_at = datetime('now')
			 WHERE memory_id = ?`,
			owner, leaseMod, id,
		); err != nil {
			return nil, fmt.Errorf("mark processing %s: %w", id, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit claim batch: %w", err)
	}
	return ids, nil
}

// SetCurationDoneLeased completes a row only while owner still holds the lease.
// A processing_dirty row (edited mid-flight) returns to pending for re-curation;
// either way the lease is cleared.
func (s *SQLiteStore) SetCurationDoneLeased(ctx context.Context, memoryID, owner string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE curation_queue
		 SET status = CASE
		       WHEN status = 'processing_dirty' THEN 'pending'
		       ELSE 'done'
		     END,
		     owner_id = NULL,
		     lease_expires_at = NULL,
		     updated_at = datetime('now')
		 WHERE memory_id = ? AND owner_id = ? AND status IN ('processing', 'processing_dirty')`,
		memoryID, owner,
	)
	if err != nil {
		return fmt.Errorf("set curation done: %w", err)
	}
	return nil
}

// SetCurationFailedLeased records a failure only while owner still holds the
// lease, mirroring SetCurationFailed's retry/backoff and clearing the lease.
func (s *SQLiteStore) SetCurationFailedLeased(ctx context.Context, memoryID, owner, errMsg string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE curation_queue
		 SET attempts = CASE
		       WHEN status = 'processing_dirty' THEN attempts
		       ELSE attempts + 1
		     END,
		     last_error = CASE
		       WHEN status = 'processing_dirty' THEN NULL
		       ELSE ?
		     END,
		     status = CASE
		       WHEN status = 'processing_dirty' THEN 'pending'
		       WHEN attempts + 1 >= 5 THEN 'failed'
		       ELSE 'pending'
		     END,
		     owner_id = NULL,
		     lease_expires_at = NULL,
		     updated_at = datetime('now')
		 WHERE memory_id = ? AND owner_id = ? AND status IN ('processing', 'processing_dirty')`,
		errMsg, memoryID, owner,
	)
	if err != nil {
		return fmt.Errorf("set curation failed: %w", err)
	}
	return nil
}

// ReclaimExpiredCuration returns rows whose lease has expired to pending so a
// crashed worker's in-flight batch is picked up again.
func (s *SQLiteStore) ReclaimExpiredCuration(ctx context.Context) (int, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE curation_queue
		 SET status = 'pending', owner_id = NULL, lease_expires_at = NULL, updated_at = datetime('now')
		 WHERE status IN ('processing', 'processing_dirty')
		   AND lease_expires_at IS NOT NULL
		   AND lease_expires_at < datetime('now')`,
	)
	if err != nil {
		return 0, fmt.Errorf("reclaim expired curation: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}
