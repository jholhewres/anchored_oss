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

func (s *PostgresStore) GetOrganizationBySlug(ctx context.Context, slug string) (*model.Organization, error) {
	var o model.Organization
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, slug, created_at FROM organizations WHERE slug = $1`,
		slug,
	).Scan(&o.ID, &o.Name, &o.Slug, &o.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("get organization by slug: %w", err)
	}
	return &o, nil
}

func (s *PostgresStore) ListOrganizationsByAccount(ctx context.Context, accountID string) ([]*model.Organization, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT o.id, o.name, o.slug, o.created_at
		 FROM organizations o
		 JOIN org_members om ON om.org_id = o.id
		 WHERE om.account_id = $1
		 ORDER BY o.name`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("list organizations by account: %w", err)
	}
	defer rows.Close()

	var orgs []*model.Organization
	for rows.Next() {
		var o model.Organization
		if err := rows.Scan(&o.ID, &o.Name, &o.Slug, &o.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan organization: %w", err)
		}
		orgs = append(orgs, &o)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate organizations: %w", err)
	}
	return orgs, nil
}

func (s *PostgresStore) CreateTeam(ctx context.Context, orgID, name, slug string) (*model.Team, error) {
	var t model.Team
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO teams (org_id, name, slug) VALUES ($1, $2, $3) RETURNING id, org_id, name, slug, created_at`,
		orgID, name, slug,
	).Scan(&t.ID, &t.OrgID, &t.Name, &t.Slug, &t.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create team: %w", err)
	}
	return &t, nil
}

func (s *PostgresStore) ListTeamsByOrg(ctx context.Context, orgID string) ([]*model.Team, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, org_id, name, slug, created_at FROM teams WHERE org_id = $1 ORDER BY name`,
		orgID,
	)
	if err != nil {
		return nil, fmt.Errorf("list teams by org: %w", err)
	}
	defer rows.Close()

	var teams []*model.Team
	for rows.Next() {
		var t model.Team
		if err := rows.Scan(&t.ID, &t.OrgID, &t.Name, &t.Slug, &t.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan team: %w", err)
		}
		teams = append(teams, &t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate teams: %w", err)
	}
	return teams, nil
}

func (s *PostgresStore) AddOrgMember(ctx context.Context, orgID, accountID, role string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO org_members (org_id, account_id, role) VALUES ($1, $2, $3) ON CONFLICT (org_id, account_id) DO UPDATE SET role = EXCLUDED.role`,
		orgID, accountID, role,
	)
	if err != nil {
		return fmt.Errorf("add org member: %w", err)
	}
	return nil
}

func (s *PostgresStore) AddTeamMember(ctx context.Context, teamID, accountID string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO team_members (team_id, account_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		teamID, accountID,
	)
	if err != nil {
		return fmt.Errorf("add team member: %w", err)
	}
	return nil
}

func (s *PostgresStore) RemoveTeamMember(ctx context.Context, teamID, accountID string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM team_members WHERE team_id = $1 AND account_id = $2`,
		teamID, accountID,
	)
	if err != nil {
		return fmt.Errorf("remove team member: %w", err)
	}
	return nil
}
