package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jholhewres/anchored_oss/internal/model"
)

// GetDashboardStats returns aggregate counts for the org's admin overview
// plus the ten most active projects in the last 24 hours. Empty results are
// returned as zero counts and an empty slice, never as null/nil.
func (s *PostgresStore) GetDashboardStats(ctx context.Context, orgID string) (*model.DashboardStats, error) {
	stats := &model.DashboardStats{
		RecentPushes:  []model.PushActivity{},
		Organizations: 1,
	}

	err := s.db.QueryRowContext(ctx, `
		WITH
		  acc AS (SELECT COUNT(*)::int AS n FROM org_members WHERE org_id = $1),
		  tm  AS (SELECT COUNT(*)::int AS n FROM teams WHERE org_id = $1),
		  prj AS (SELECT COUNT(*)::int AS n FROM projects WHERE org_id = $1 AND deleted_at IS NULL),
		  mem AS (
		    SELECT COUNT(*)::int AS n
		    FROM memories m
		    JOIN projects p ON p.id = m.project_id
		    WHERE p.org_id = $1 AND m.deleted_at IS NULL AND p.deleted_at IS NULL
		  ),
		  keys AS (
		    SELECT COUNT(*)::int AS n FROM api_keys
		    WHERE org_id = $1 AND revoked_at IS NULL
		      AND (expires_at IS NULL OR expires_at > now())
		  ),
		  aud AS (
		    SELECT COUNT(*)::int AS n FROM audit_log
		    WHERE org_id = $1 AND created_at >= now() - INTERVAL '24 hours'
		  )
		SELECT acc.n, tm.n, prj.n, mem.n, keys.n, aud.n
		FROM acc, tm, prj, mem, keys, aud
	`, orgID).Scan(
		&stats.Accounts, &stats.Teams, &stats.Projects,
		&stats.MemoriesLive, &stats.KeysActive, &stats.AuditEntries24h,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return stats, nil
	}
	if err != nil {
		return nil, fmt.Errorf("dashboard stats: %w", err)
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT p.id, p.name, COUNT(*)::int AS push_count, MAX(a.created_at) AS last_push
		FROM audit_log a
		JOIN projects p ON p.id = a.project_id
		WHERE a.org_id = $1
		  AND a.action = 'sync.push.accepted'
		  AND a.created_at >= now() - INTERVAL '24 hours'
		  AND p.deleted_at IS NULL
		GROUP BY p.id, p.name
		ORDER BY push_count DESC, last_push DESC
		LIMIT 10
	`, orgID)
	if err != nil {
		return nil, fmt.Errorf("dashboard recent pushes: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var a model.PushActivity
		if err := rows.Scan(&a.ProjectID, &a.ProjectName, &a.Count, &a.LastPush); err != nil {
			return nil, fmt.Errorf("scan recent push: %w", err)
		}
		stats.RecentPushes = append(stats.RecentPushes, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate recent pushes: %w", err)
	}
	return stats, nil
}
