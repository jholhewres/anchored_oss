package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jholhewres/anchored_oss/internal/model"
)

func (s *PostgresStore) CreateTeam(ctx context.Context, orgID, name, slug string) (*model.Team, error) {
	var t model.Team
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO teams (org_id, name, slug) VALUES ($1, $2, $3)
		 RETURNING id, org_id, name, slug, created_at`,
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

// GetTeamDetail returns the team plus its members and the projects it grants
// access to. Returns ErrNotFound when no team matches.
func (s *PostgresStore) GetTeamDetail(ctx context.Context, teamID string) (*model.TeamDetail, error) {
	var d model.TeamDetail
	err := s.db.QueryRowContext(ctx,
		`SELECT id, org_id, name, slug, created_at FROM teams WHERE id = $1`,
		teamID,
	).Scan(&d.ID, &d.OrgID, &d.Name, &d.Slug, &d.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get team: %w", err)
	}

	memberRows, err := s.db.QueryContext(ctx,
		`SELECT a.id, a.email, a.display_name, tm.created_at
		 FROM team_members tm
		 JOIN accounts a ON a.id = tm.account_id
		 WHERE tm.team_id = $1
		 ORDER BY a.display_name`,
		teamID,
	)
	if err != nil {
		return nil, fmt.Errorf("list team members: %w", err)
	}
	defer memberRows.Close()

	d.Members = make([]model.TeamMember, 0)
	for memberRows.Next() {
		var m model.TeamMember
		if err := memberRows.Scan(&m.AccountID, &m.Email, &m.DisplayName, &m.AddedAt); err != nil {
			return nil, fmt.Errorf("scan team member: %w", err)
		}
		d.Members = append(d.Members, m)
	}
	if err := memberRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate team members: %w", err)
	}

	grantRows, err := s.db.QueryContext(ctx,
		`SELECT p.id, p.name, p.slug, tpa.role
		 FROM team_project_access tpa
		 JOIN projects p ON p.id = tpa.project_id
		 WHERE tpa.team_id = $1 AND p.deleted_at IS NULL
		 ORDER BY p.name`,
		teamID,
	)
	if err != nil {
		return nil, fmt.Errorf("list project grants: %w", err)
	}
	defer grantRows.Close()

	d.ProjectGrants = make([]model.ProjectGrant, 0)
	for grantRows.Next() {
		var g model.ProjectGrant
		if err := grantRows.Scan(&g.ProjectID, &g.ProjectName, &g.ProjectSlug, &g.Role); err != nil {
			return nil, fmt.Errorf("scan project grant: %w", err)
		}
		d.ProjectGrants = append(d.ProjectGrants, g)
	}
	if err := grantRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate project grants: %w", err)
	}

	return &d, nil
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
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM team_members WHERE team_id = $1 AND account_id = $2`,
		teamID, accountID,
	)
	if err != nil {
		return fmt.Errorf("remove team member: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("remove team member rows affected: %w", err)
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}
