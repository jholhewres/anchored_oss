package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jholhewres/anchored_oss/internal/model"
)

// UpsertAccountTaskThread inserts or updates one of the caller's task
// threads. The (account_id, task_key) pair is the identity; the client is
// the source of truth for everything else.
func (s *PostgresStore) UpsertAccountTaskThread(ctx context.Context, t *model.AccountTaskThread) error {
	projects, journal, details, err := marshalTaskThreadJSON(t)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO account_task_threads (account_id, task_key, external_ref, status, projects, journal, details, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5::jsonb, $6::jsonb, $7::jsonb, now(), now())
		ON CONFLICT (account_id, task_key) DO UPDATE SET
			external_ref = EXCLUDED.external_ref,
			status       = EXCLUDED.status,
			projects     = EXCLUDED.projects,
			journal      = EXCLUDED.journal,
			details      = EXCLUDED.details,
			updated_at   = now()`,
		t.AccountID, t.TaskKey, t.ExternalRef, t.Status, projects, journal, details)
	if err != nil {
		return fmt.Errorf("upsert account task thread: %w", err)
	}
	return nil
}

// ListAccountTaskThreads returns ONLY the given account's threads, most
// recently touched first.
func (s *PostgresStore) ListAccountTaskThreads(ctx context.Context, accountID string) ([]*model.AccountTaskThread, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT account_id, task_key, external_ref, status, projects, journal, details, created_at, updated_at
		FROM account_task_threads WHERE account_id = $1
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

func marshalTaskThreadJSON(t *model.AccountTaskThread) (projects, journal, details string, err error) {
	p, err := json.Marshal(orEmptySlice(t.Projects))
	if err != nil {
		return "", "", "", fmt.Errorf("marshal projects: %w", err)
	}
	j, err := json.Marshal(orEmptySlice(t.Journal))
	if err != nil {
		return "", "", "", fmt.Errorf("marshal journal: %w", err)
	}
	d := t.Details
	if d == nil {
		d = map[string]any{}
	}
	db, err := json.Marshal(d)
	if err != nil {
		return "", "", "", fmt.Errorf("marshal details: %w", err)
	}
	return string(p), string(j), string(db), nil
}

func unmarshalTaskThreadJSON(t *model.AccountTaskThread, projects, journal, details []byte) {
	_ = json.Unmarshal(projects, &t.Projects)
	_ = json.Unmarshal(journal, &t.Journal)
	_ = json.Unmarshal(details, &t.Details)
}

func orEmptySlice(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}
