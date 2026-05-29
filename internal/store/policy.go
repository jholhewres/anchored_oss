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
	DefaultQualityThreshold = 0.55
	DefaultNearDupThreshold = 0.85
)

func defaultOrgPolicy(orgID string) *model.OrgPolicy {
	return &model.OrgPolicy{
		OrgID:             orgID,
		BlockedCategories: nil, // nil => caller applies code defaults
		QualityThreshold:  DefaultQualityThreshold,
		NearDupThreshold:  DefaultNearDupThreshold,
	}
}

// GetOrgPolicy returns the org's guardrail overrides, or server defaults when no
// row exists (so callers never need a special-case).
func (s *PostgresStore) GetOrgPolicy(ctx context.Context, orgID string) (*model.OrgPolicy, error) {
	p := &model.OrgPolicy{OrgID: orgID}
	err := s.db.QueryRowContext(ctx,
		`SELECT blocked_categories, quality_threshold, near_dup_threshold, updated_at
		 FROM org_policies WHERE org_id = $1`, orgID,
	).Scan(pq.Array(&p.BlockedCategories), &p.QualityThreshold, &p.NearDupThreshold, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return defaultOrgPolicy(orgID), nil
	}
	if err != nil {
		return nil, fmt.Errorf("get org policy: %w", err)
	}
	return p, nil
}

// UpsertOrgPolicy stores (or replaces) an org's guardrail overrides.
func (s *PostgresStore) UpsertOrgPolicy(ctx context.Context, p *model.OrgPolicy) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO org_policies (org_id, blocked_categories, quality_threshold, near_dup_threshold, updated_at)
		 VALUES ($1, $2, $3, $4, now())
		 ON CONFLICT (org_id) DO UPDATE SET
		   blocked_categories = EXCLUDED.blocked_categories,
		   quality_threshold = EXCLUDED.quality_threshold,
		   near_dup_threshold = EXCLUDED.near_dup_threshold,
		   updated_at = now()`,
		p.OrgID, pq.Array(p.BlockedCategories), p.QualityThreshold, p.NearDupThreshold,
	)
	if err != nil {
		return fmt.Errorf("upsert org policy: %w", err)
	}
	return nil
}
