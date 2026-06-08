package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jholhewres/anchored_oss/internal/model"
)

// sqliteTS formats a cutoff for comparison against the TEXT timestamp columns
// ("2006-01-02 15:04:05", UTC) used across the SQLite schema.
func sqliteTS(t time.Time) string {
	return t.UTC().Format("2006-01-02 15:04:05")
}

// GetProjectMemoryHealth aggregates the memory-health view for one project.
func (s *SQLiteStore) GetProjectMemoryHealth(ctx context.Context, projectID string) (*model.MemoryHealth, error) {
	var orgID string
	if err := s.db.QueryRowContext(ctx,
		`SELECT org_id FROM projects WHERE id = ?`, projectID,
	).Scan(&orgID); err != nil {
		return nil, fmt.Errorf("resolve project org: %w", err)
	}
	return s.memoryHealth(ctx, orgID, projectID, `m.project_id = ?`, []any{projectID})
}

// GetOrgMemoryHealth aggregates the memory-health view across all live
// projects of an org.
func (s *SQLiteStore) GetOrgMemoryHealth(ctx context.Context, orgID string) (*model.MemoryHealth, error) {
	scope := `m.project_id IN (SELECT id FROM projects WHERE org_id = ? AND deleted_at IS NULL)`
	return s.memoryHealth(ctx, orgID, "", scope, []any{orgID})
}

func (s *SQLiteStore) memoryHealth(ctx context.Context, orgID, projectID, scope string, scopeArgs []any) (*model.MemoryHealth, error) {
	now := time.Now().UTC()
	agg := healthAggregates{
		Last24h:  map[string]int64{},
		Prior29d: map[string]int64{},
	}

	live := `FROM memories m WHERE ` + scope + ` AND m.deleted_at IS NULL`
	count := func(dst *int64, extra string, extraArgs ...any) error {
		args := append(append([]any{}, scopeArgs...), extraArgs...)
		return s.db.QueryRowContext(ctx, `SELECT COUNT(*) `+live+extra, args...).Scan(dst)
	}

	if err := count(&agg.Counts.Live, ``); err != nil {
		return nil, fmt.Errorf("health live count: %w", err)
	}
	if err := count(&agg.Counts.LowSignal,
		` AND json_extract(m.metadata, '$.curation_status') = 'low_signal'`); err != nil {
		return nil, fmt.Errorf("health low_signal count: %w", err)
	}
	if err := count(&agg.Counts.NearDuplicate,
		` AND json_extract(m.metadata, '$.curation_status') = 'near_duplicate'`); err != nil {
		return nil, fmt.Errorf("health near_duplicate count: %w", err)
	}
	// Stale is age-based (not curation_status='stale') on purpose: it must report
	// from day one, before the curation worker has run, so a fresh deployment's
	// dashboard isn't blank. The worker's curation_status='stale' mark is the
	// finer-grained, per-memory signal surfaced via ListMemoriesByCurationStatus.
	if err := count(&agg.Counts.Stale,
		` AND m.updated_at < ? AND COALESCE(json_extract(m.metadata, '$.pinned'), 0) != 1`,
		sqliteTS(now.AddDate(0, 0, -180))); err != nil {
		return nil, fmt.Errorf("health stale count: %w", err)
	}
	if err := count(&agg.Counts.MissingEmbeddings, ` AND m.embedding IS NULL`); err != nil {
		return nil, fmt.Errorf("health missing embeddings count: %w", err)
	}
	if err := count(&agg.Counts.Contradictions,
		` AND json_extract(m.metadata, '$.curation_status') = 'contradiction_candidate'`); err != nil {
		return nil, fmt.Errorf("health contradictions count: %w", err)
	}

	group := func(expr, extra string, extraArgs ...any) ([]model.NameCount, error) {
		args := append(append([]any{}, scopeArgs...), extraArgs...)
		rows, err := s.db.QueryContext(ctx,
			`SELECT `+expr+` AS k, COUNT(*) AS n `+live+extra+` GROUP BY k ORDER BY n DESC, k LIMIT 20`,
			args...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []model.NameCount
		for rows.Next() {
			var nc model.NameCount
			if err := rows.Scan(&nc.Name, &nc.Count); err != nil {
				return nil, err
			}
			out = append(out, nc)
		}
		return out, rows.Err()
	}

	var err error
	if agg.BySource, err = group(`COALESCE(NULLIF(m.source, ''), 'unknown')`, ``); err != nil {
		return nil, fmt.Errorf("health by_source: %w", err)
	}
	if agg.ByCategory, err = group(`m.category`, ``); err != nil {
		return nil, fmt.Errorf("health by_category: %w", err)
	}

	ageExpr := fmt.Sprintf(`CASE
		WHEN m.updated_at >= '%s' THEN '<=7d'
		WHEN m.updated_at >= '%s' THEN '8-30d'
		WHEN m.updated_at >= '%s' THEN '31-90d'
		WHEN m.updated_at >= '%s' THEN '91-180d'
		ELSE '>180d' END`,
		sqliteTS(now.AddDate(0, 0, -7)), sqliteTS(now.AddDate(0, 0, -30)),
		sqliteTS(now.AddDate(0, 0, -90)), sqliteTS(now.AddDate(0, 0, -180)))
	if agg.AgeHistogram, err = group(ageExpr, ``); err != nil {
		return nil, fmt.Errorf("health age histogram: %w", err)
	}

	window := func(dst map[string]int64, extra string, extraArgs ...any) error {
		args := append(append([]any{}, scopeArgs...), extraArgs...)
		rows, err := s.db.QueryContext(ctx,
			`SELECT COALESCE(NULLIF(m.source, ''), 'unknown') AS k, COUNT(*) `+live+extra+` GROUP BY k`,
			args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var k string
			var n int64
			if err := rows.Scan(&k, &n); err != nil {
				return err
			}
			dst[k] = n
		}
		return rows.Err()
	}
	if err := window(agg.Last24h, ` AND m.created_at >= ?`, sqliteTS(now.Add(-24*time.Hour))); err != nil {
		return nil, fmt.Errorf("health 24h window: %w", err)
	}
	if err := window(agg.Prior29d, ` AND m.created_at >= ? AND m.created_at < ?`,
		sqliteTS(now.AddDate(0, 0, -30)), sqliteTS(now.Add(-24*time.Hour))); err != nil {
		return nil, fmt.Errorf("health 29d window: %w", err)
	}

	stats, err := s.ListRejectionStats(ctx, orgID, projectID, now.AddDate(0, 0, -7).Format("2006-01-02"))
	if err != nil {
		return nil, fmt.Errorf("health rejections: %w", err)
	}
	agg.Rejections = aggregateRejectionsByRule(stats)

	return composeHealth(agg), nil
}
