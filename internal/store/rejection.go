package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jholhewres/anchored_oss/internal/model"
)

// rejectionDay returns the UTC day bucket used by the rejection counters.
func rejectionDay(now time.Time) string {
	return now.UTC().Format("2006-01-02")
}

// IncrementRejectionStat upserts the per-day rejection counter for
// (org, project, rule). Callers treat errors as best-effort.
func (s *PostgresStore) IncrementRejectionStat(ctx context.Context, orgID, projectID, rule string, delta int64) error {
	if delta <= 0 {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sync_rejection_stats (org_id, project_id, rule, day, count)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (org_id, project_id, rule, day)
		DO UPDATE SET count = sync_rejection_stats.count + EXCLUDED.count
	`, orgID, projectID, rule, rejectionDay(time.Now()), delta)
	if err != nil {
		return fmt.Errorf("increment rejection stat: %w", err)
	}
	return nil
}

// ListRejectionStats returns rejection counters since the given UTC day
// (inclusive). Empty projectID aggregates across the whole org.
func (s *PostgresStore) ListRejectionStats(ctx context.Context, orgID, projectID, sinceDay string) ([]*model.RejectionStat, error) {
	query := `
		SELECT project_id, rule, day, count
		FROM sync_rejection_stats
		WHERE org_id = $1 AND day >= $2
	`
	args := []any{orgID, sinceDay}
	if projectID != "" {
		query += ` AND project_id = $3`
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
func (s *PostgresStore) PurgeRejectionStatsOlderThan(ctx context.Context, beforeDay string) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM sync_rejection_stats WHERE day < $1`, beforeDay)
	if err != nil {
		return 0, fmt.Errorf("purge rejection stats: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}
