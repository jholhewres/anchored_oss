package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jholhewres/anchored_oss/internal/model"
)

// IncrementRejectionStat upserts the per-day rejection counter for
// (org, project, rule). Callers treat errors as best-effort.
func (s *SQLiteStore) IncrementRejectionStat(ctx context.Context, orgID, projectID, rule string, delta int64) error {
	if delta <= 0 {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sync_rejection_stats (org_id, project_id, rule, day, count)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (org_id, project_id, rule, day)
		DO UPDATE SET count = count + excluded.count
	`, orgID, projectID, rule, rejectionDay(time.Now()), delta)
	if err != nil {
		return fmt.Errorf("increment rejection stat: %w", err)
	}
	return nil
}

// ListRejectionStats returns rejection counters since the given UTC day
// (inclusive). Empty projectID aggregates across the whole org.
func (s *SQLiteStore) ListRejectionStats(ctx context.Context, orgID, projectID, sinceDay string) ([]*model.RejectionStat, error) {
	query := `
		SELECT project_id, rule, day, count
		FROM sync_rejection_stats
		WHERE org_id = ? AND day >= ?
	`
	args := []any{orgID, sinceDay}
	if projectID != "" {
		query += ` AND project_id = ?`
		args = append(args, projectID)
	}
	query += ` ORDER BY day DESC, count DESC`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list rejection stats: %w", err)
	}
	defer rows.Close()

	var out []*model.RejectionStat
	for rows.Next() {
		var r model.RejectionStat
		if err := rows.Scan(&r.ProjectID, &r.Rule, &r.Day, &r.Count); err != nil {
			return nil, fmt.Errorf("scan rejection stat: %w", err)
		}
		out = append(out, &r)
	}
	return out, rows.Err()
}

// PurgeRejectionStatsOlderThan deletes counters for days before the cutoff day.
func (s *SQLiteStore) PurgeRejectionStatsOlderThan(ctx context.Context, beforeDay string) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM sync_rejection_stats WHERE day < ?`, beforeDay)
	if err != nil {
		return 0, fmt.Errorf("purge rejection stats: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}
