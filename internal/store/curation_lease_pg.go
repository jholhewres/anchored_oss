package store

import (
	"context"
	"fmt"
	"time"
)

// ClaimCurationBatchLeased claims pending rows for owner with a lease, using a
// single CTE + FOR UPDATE SKIP LOCKED so concurrent workers never claim the
// same row. make_interval takes the lease TTL in whole seconds.
func (s *PostgresStore) ClaimCurationBatchLeased(ctx context.Context, batchSize int, owner string, leaseTTL time.Duration) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`WITH claimed AS (
		   SELECT memory_id FROM curation_queue
		   WHERE status = 'pending'
		   ORDER BY scheduled_at
		   LIMIT $1
		   FOR UPDATE SKIP LOCKED
		 )
		 UPDATE curation_queue
		 SET status = 'processing',
		     owner_id = $2,
		     lease_expires_at = now() + make_interval(secs => $3),
		     updated_at = now()
		 FROM claimed
		 WHERE curation_queue.memory_id = claimed.memory_id
		 RETURNING curation_queue.memory_id`,
		batchSize, owner, leaseSeconds(leaseTTL),
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

// SetCurationDoneLeased completes a row only while owner still holds the lease.
func (s *PostgresStore) SetCurationDoneLeased(ctx context.Context, memoryID, owner string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE curation_queue
		 SET status = CASE
		       WHEN status = 'processing_dirty' THEN 'pending'
		       ELSE 'done'
		     END,
		     owner_id = NULL,
		     lease_expires_at = NULL,
		     updated_at = now()
		 WHERE memory_id = $1 AND owner_id = $2 AND status IN ('processing', 'processing_dirty')`,
		memoryID, owner,
	)
	if err != nil {
		return fmt.Errorf("set curation done: %w", err)
	}
	return nil
}

// SetCurationFailedLeased records a failure only while owner still holds the
// lease, mirroring SetCurationFailed's retry/backoff and clearing the lease.
func (s *PostgresStore) SetCurationFailedLeased(ctx context.Context, memoryID, owner, errMsg string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE curation_queue
		 SET attempts = CASE
		       WHEN status = 'processing_dirty' THEN attempts
		       ELSE attempts + 1
		     END,
		     last_error = CASE
		       WHEN status = 'processing_dirty' THEN NULL
		       ELSE $1
		     END,
		     status = CASE
		       WHEN status = 'processing_dirty' THEN 'pending'
		       WHEN attempts + 1 >= 5 THEN 'failed'
		       ELSE 'pending'
		     END,
		     owner_id = NULL,
		     lease_expires_at = NULL,
		     updated_at = now()
		 WHERE memory_id = $2 AND owner_id = $3 AND status IN ('processing', 'processing_dirty')`,
		errMsg, memoryID, owner,
	)
	if err != nil {
		return fmt.Errorf("set curation failed: %w", err)
	}
	return nil
}

// ReclaimExpiredCuration returns rows whose lease has expired to pending.
func (s *PostgresStore) ReclaimExpiredCuration(ctx context.Context) (int, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE curation_queue
		 SET status = 'pending', owner_id = NULL, lease_expires_at = NULL, updated_at = now()
		 WHERE status IN ('processing', 'processing_dirty')
		   AND lease_expires_at IS NOT NULL
		   AND lease_expires_at < now()`,
	)
	if err != nil {
		return 0, fmt.Errorf("reclaim expired curation: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}
