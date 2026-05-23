package store

import (
	"context"
	"fmt"

	"github.com/jholhewres/anchored_oss/internal/model"
)

func (s *PostgresStore) CreateProject(ctx context.Context, orgID, name, slug, remoteKey, createdBy string) (*model.Project, error) {
	var p model.Project
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO projects (org_id, name, slug, remote_key, created_by) VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, org_id, name, slug, remote_key, created_by, created_at`,
		orgID, name, slug, remoteKey, createdBy,
	).Scan(&p.ID, &p.OrgID, &p.Name, &p.Slug, &p.RemoteKey, &p.CreatedBy, &p.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create project: %w", err)
	}
	return &p, nil
}

func (s *PostgresStore) GetProjectByID(ctx context.Context, id string) (*model.Project, error) {
	var p model.Project
	err := s.db.QueryRowContext(ctx,
		`SELECT id, org_id, name, slug, remote_key, created_by, created_at FROM projects WHERE id = $1`,
		id,
	).Scan(&p.ID, &p.OrgID, &p.Name, &p.Slug, &p.RemoteKey, &p.CreatedBy, &p.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("get project by id: %w", err)
	}
	return &p, nil
}

func (s *PostgresStore) GetProjectByRemoteKey(ctx context.Context, orgID, remoteKey string) (*model.Project, error) {
	var p model.Project
	err := s.db.QueryRowContext(ctx,
		`SELECT id, org_id, name, slug, remote_key, created_by, created_at FROM projects WHERE org_id = $1 AND remote_key = $2`,
		orgID, remoteKey,
	).Scan(&p.ID, &p.OrgID, &p.Name, &p.Slug, &p.RemoteKey, &p.CreatedBy, &p.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("get project by remote key: %w", err)
	}
	return &p, nil
}

func (s *PostgresStore) ListProjectsByTeamAccess(ctx context.Context, accountID string) ([]*model.Project, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT DISTINCT p.id, p.org_id, p.name, p.slug, p.remote_key, p.created_by, p.created_at
		 FROM projects p
		 JOIN team_project_access tpa ON tpa.project_id = p.id
		 JOIN team_members tm ON tm.team_id = tpa.team_id
		 WHERE tm.account_id = $1
		 ORDER BY p.name`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("list projects by team access: %w", err)
	}
	defer rows.Close()

	var projects []*model.Project
	for rows.Next() {
		var p model.Project
		if err := rows.Scan(&p.ID, &p.OrgID, &p.Name, &p.Slug, &p.RemoteKey, &p.CreatedBy, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan project: %w", err)
		}
		projects = append(projects, &p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate projects: %w", err)
	}
	return projects, nil
}

func (s *PostgresStore) GrantTeamAccess(ctx context.Context, teamID, projectID, role string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO team_project_access (team_id, project_id, role) VALUES ($1, $2, $3) ON CONFLICT (team_id, project_id) DO UPDATE SET role = EXCLUDED.role`,
		teamID, projectID, role,
	)
	if err != nil {
		return fmt.Errorf("grant team access: %w", err)
	}
	return nil
}
