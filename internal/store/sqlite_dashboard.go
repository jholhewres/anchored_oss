package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jholhewres/anchored_oss/internal/model"
)

func (s *SQLiteStore) GetDashboardStats(ctx context.Context, orgID string) (*model.DashboardStats, error) {
	stats := &model.DashboardStats{
		RecentPushes:  []model.PushActivity{},
		Organizations: 1,
	}

	err := s.db.QueryRowContext(ctx, `
		WITH
		  acc AS (SELECT COUNT(*) AS n FROM org_members WHERE org_id = ?),
		  tm  AS (SELECT COUNT(*) AS n FROM teams WHERE org_id = ?),
		  prj AS (SELECT COUNT(*) AS n FROM projects WHERE org_id = ? AND deleted_at IS NULL),
		  mem AS (
		    SELECT COUNT(*) AS n
		    FROM memories m
		    JOIN projects p ON p.id = m.project_id
		    WHERE p.org_id = ? AND m.deleted_at IS NULL AND p.deleted_at IS NULL
		  ),
		  keys AS (
		    SELECT COUNT(*) AS n FROM api_keys
		    WHERE org_id = ? AND revoked_at IS NULL
		      AND (expires_at IS NULL OR expires_at > datetime('now'))
		  ),
		  aud AS (
		    SELECT COUNT(*) AS n FROM audit_log
		    WHERE org_id = ? AND created_at >= datetime('now', '-24 hours')
		  )
		SELECT acc.n, tm.n, prj.n, mem.n, keys.n, aud.n
		FROM acc, tm, prj, mem, keys, aud
	`, orgID, orgID, orgID, orgID, orgID, orgID).Scan(
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
		SELECT p.id, p.name, COUNT(*) AS push_count, MAX(a.created_at) AS last_push
		FROM audit_log a
		JOIN projects p ON p.id = a.project_id
		WHERE a.org_id = ?
		  AND a.action = 'sync.push.accepted'
		  AND a.created_at >= datetime('now', '-24 hours')
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
		if err := rows.Scan(&a.ProjectID, &a.ProjectName, &a.Count, scanTime(&a.LastPush)); err != nil {
			return nil, fmt.Errorf("scan recent push: %w", err)
		}
		stats.RecentPushes = append(stats.RecentPushes, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate recent pushes: %w", err)
	}
	return stats, nil
}

func (s *SQLiteStore) GetOrgStorageBytes(ctx context.Context, orgID string) (int64, error) {
	var bytes int64
	err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(LENGTH(m.content)), 0)
		FROM memories m
		JOIN projects p ON p.id = m.project_id
		WHERE p.org_id = ? AND m.deleted_at IS NULL AND p.deleted_at IS NULL
	`, orgID).Scan(&bytes)
	if err != nil {
		return 0, fmt.Errorf("get org storage bytes: %w", err)
	}
	return bytes, nil
}
