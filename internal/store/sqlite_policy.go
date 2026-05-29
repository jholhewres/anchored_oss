package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jholhewres/anchored_oss/internal/model"
)

// GetOrgPolicy returns the org's guardrail overrides, or server defaults when no
// row exists. blocked_categories is stored as a JSON array.
func (s *SQLiteStore) GetOrgPolicy(ctx context.Context, orgID string) (*model.OrgPolicy, error) {
	p := &model.OrgPolicy{OrgID: orgID}
	var blockedJSON string
	err := s.db.QueryRowContext(ctx,
		`SELECT blocked_categories, quality_threshold, near_dup_threshold, updated_at
		 FROM org_policies WHERE org_id = ?`, orgID,
	).Scan(&blockedJSON, &p.QualityThreshold, &p.NearDupThreshold, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return defaultOrgPolicy(orgID), nil
	}
	if err != nil {
		return nil, fmt.Errorf("get org policy: %w", err)
	}
	if blockedJSON != "" {
		_ = json.Unmarshal([]byte(blockedJSON), &p.BlockedCategories)
	}
	return p, nil
}

// UpsertOrgPolicy stores (or replaces) an org's guardrail overrides.
func (s *SQLiteStore) UpsertOrgPolicy(ctx context.Context, p *model.OrgPolicy) error {
	blocked := p.BlockedCategories
	if blocked == nil {
		blocked = []string{}
	}
	blob, err := json.Marshal(blocked)
	if err != nil {
		return fmt.Errorf("marshal blocked categories: %w", err)
	}
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO org_policies (org_id, blocked_categories, quality_threshold, near_dup_threshold, updated_at)
		 VALUES (?, ?, ?, ?, datetime('now'))
		 ON CONFLICT (org_id) DO UPDATE SET
		   blocked_categories = excluded.blocked_categories,
		   quality_threshold = excluded.quality_threshold,
		   near_dup_threshold = excluded.near_dup_threshold,
		   updated_at = datetime('now')`,
		p.OrgID, string(blob), p.QualityThreshold, p.NearDupThreshold,
	); err != nil {
		return fmt.Errorf("upsert org policy: %w", err)
	}
	return nil
}
