package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jholhewres/anchored_oss/internal/model"
	"github.com/lib/pq"
)

// DefaultQualityThreshold / DefaultNearDupThreshold mirror the server defaults
// applied when an org has no policy row. Exported so handlers normalize a
// "0 means default" input to the same value the sync engine enforces.
const (
	DefaultQualityThreshold   = 0.55
	DefaultNearDupThreshold   = 0.85
	DefaultMaxMemoriesPerSync = 500
)

func defaultOrgPolicy(orgID string) *model.OrgPolicy {
	return &model.OrgPolicy{
		OrgID:              orgID,
		BlockedCategories:  nil, // nil => caller applies code defaults
		QualityThreshold:   DefaultQualityThreshold,
		NearDupThreshold:   DefaultNearDupThreshold,
		MaxMemoriesPerSync: DefaultMaxMemoriesPerSync,
	}
}

// effectiveMaxPerSync coalesces a stored 0 (forward-compat / unset) to the
// server default so the engine never enforces a zero cap.
func effectiveMaxPerSync(v int) int {
	if v <= 0 {
		return DefaultMaxMemoriesPerSync
	}
	return v
}

// GetOrgPolicy returns the org's guardrail overrides, or server defaults when no
// row exists (so callers never need a special-case).
func (s *PostgresStore) GetOrgPolicy(ctx context.Context, orgID string) (*model.OrgPolicy, error) {
	p := &model.OrgPolicy{OrgID: orgID}
	err := s.db.QueryRowContext(ctx,
		`SELECT blocked_categories, quality_threshold, near_dup_threshold, max_memories_per_sync, updated_at
		 FROM org_policies WHERE org_id = $1`, orgID,
	).Scan(pq.Array(&p.BlockedCategories), &p.QualityThreshold, &p.NearDupThreshold, &p.MaxMemoriesPerSync, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return defaultOrgPolicy(orgID), nil
	}
	if err != nil {
		return nil, fmt.Errorf("get org policy: %w", err)
	}
	p.MaxMemoriesPerSync = effectiveMaxPerSync(p.MaxMemoriesPerSync)
	return p, nil
}

// UpsertOrgPolicy stores (or replaces) an org's guardrail overrides.
func (s *PostgresStore) UpsertOrgPolicy(ctx context.Context, p *model.OrgPolicy) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO org_policies (org_id, blocked_categories, quality_threshold, near_dup_threshold, max_memories_per_sync, updated_at)
		 VALUES ($1, $2, $3, $4, $5, now())
		 ON CONFLICT (org_id) DO UPDATE SET
		   blocked_categories = EXCLUDED.blocked_categories,
		   quality_threshold = EXCLUDED.quality_threshold,
		   near_dup_threshold = EXCLUDED.near_dup_threshold,
		   max_memories_per_sync = EXCLUDED.max_memories_per_sync,
		   updated_at = now()`,
		p.OrgID, pq.Array(p.BlockedCategories), p.QualityThreshold, p.NearDupThreshold, effectiveMaxPerSync(p.MaxMemoriesPerSync),
	)
	if err != nil {
		return fmt.Errorf("upsert org policy: %w", err)
	}
	return nil
}
