package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jholhewres/anchored_oss/internal/model"
)

func (s *PostgresStore) CreateOrganization(ctx context.Context, name, slug string) (*model.Organization, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("create organization: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op after a successful commit

	var o model.Organization
	if err := tx.QueryRowContext(ctx,
		`INSERT INTO organizations (name, slug) VALUES ($1, $2) RETURNING id, name, slug, created_at`,
		name, slug,
	).Scan(&o.ID, &o.Name, &o.Slug, &o.CreatedAt); err != nil {
		return nil, fmt.Errorf("create organization: %w", err)
	}
	// Seed in the same transaction: an org must never commit with a partial
	// guardrail set (which would silently disable security rules at sync time).
	if err := seedDefaultGuardrailsPG(ctx, tx, o.ID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("create organization: %w", err)
	}
	return &o, nil
}

func (s *PostgresStore) GetOrganizationByID(ctx context.Context, id string) (*model.Organization, error) {
	var o model.Organization
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, slug, created_at FROM organizations WHERE id = $1`,
		id,
	).Scan(&o.ID, &o.Name, &o.Slug, &o.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get organization: %w", err)
	}
	return &o, nil
}

func (s *PostgresStore) CountOrganizations(ctx context.Context) (int, error) {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM organizations`).Scan(&n); err != nil {
		return 0, fmt.Errorf("count organizations: %w", err)
	}
	return n, nil
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
