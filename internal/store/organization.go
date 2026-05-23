package store

import (
	"context"
	"fmt"

	"github.com/jholhewres/anchored_oss/internal/model"
)

func (s *PostgresStore) CreateOrganization(ctx context.Context, name, slug string) (*model.Organization, error) {
	var o model.Organization
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO organizations (name, slug) VALUES ($1, $2) RETURNING id, name, slug, created_at`,
		name, slug,
	).Scan(&o.ID, &o.Name, &o.Slug, &o.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create organization: %w", err)
	}
	return &o, nil
}

func (s *PostgresStore) AddOrgMember(ctx context.Context, orgID, accountID, role string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO org_members (org_id, account_id, role) VALUES ($1, $2, $3)
		 ON CONFLICT (org_id, account_id) DO UPDATE SET role = EXCLUDED.role`,
		orgID, accountID, role,
	)
	if err != nil {
		return fmt.Errorf("add org member: %w", err)
	}
	return nil
}
