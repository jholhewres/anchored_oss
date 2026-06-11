package store

import (
	"context"
	"fmt"

	"github.com/jholhewres/anchored_oss/internal/model"
)

// UpsertAccountTaskThread mirrors the Postgres implementation for SQLite.
func (s *SQLiteStore) UpsertAccountTaskThread(ctx context.Context, t *model.AccountTaskThread) error {
	projects, journal, details, err := marshalTaskThreadJSON(t)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO account_task_threads (account_id, task_key, external_ref, status, projects, journal, details, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT (account_id, task_key) DO UPDATE SET
			external_ref = excluded.external_ref,
			status       = excluded.status,
			projects     = excluded.projects,
			journal      = excluded.journal,
			details      = excluded.details,
			updated_at   = CURRENT_TIMESTAMP`,
		t.AccountID, t.TaskKey, t.ExternalRef, t.Status, projects, journal, details)
	if err != nil {
		return fmt.Errorf("upsert account task thread: %w", err)
	}
	return nil
}

// ListAccountTaskThreads mirrors the Postgres implementation for SQLite.
// The columns are declared TIMESTAMP (not DATETIME), for which modernc
// returns time.Time directly — scan straight into the struct (scanTime is
// for DATETIME-declared columns, which come back as strings).
func (s *SQLiteStore) ListAccountTaskThreads(ctx context.Context, accountID string) ([]*model.AccountTaskThread, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT account_id, task_key, external_ref, status, projects, journal, details, created_at, updated_at
		FROM account_task_threads WHERE account_id = ?
		ORDER BY updated_at DESC LIMIT 200`, accountID)
	if err != nil {
		return nil, fmt.Errorf("list account task threads: %w", err)
	}
	defer rows.Close()

	var out []*model.AccountTaskThread
	for rows.Next() {
		var t model.AccountTaskThread
		var projects, journal, details []byte
		if err := rows.Scan(&t.AccountID, &t.TaskKey, &t.ExternalRef, &t.Status,
			&projects, &journal, &details, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan account task thread: %w", err)
		}
		unmarshalTaskThreadJSON(&t, projects, journal, details)
		out = append(out, &t)
	}
	return out, rows.Err()
}
