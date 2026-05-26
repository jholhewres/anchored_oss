package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jholhewres/anchored_oss/internal/model"
)

func (s *SQLiteStore) CreateProject(ctx context.Context, orgID, name, slug, remoteKey, createdBy, category string) (*model.Project, error) {
	id := newUUID()
	category = model.NormalizeCategory(category)
	var p model.Project
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO projects (id, org_id, name, slug, remote_key, created_by, category) VALUES (?, ?, ?, ?, ?, ?, ?)
		 RETURNING id, org_id, name, slug, category, remote_key, created_by, created_at`,
		id, orgID, name, slug, remoteKey, createdBy, category,
	).Scan(&p.ID, &p.OrgID, &p.Name, &p.Slug, &p.Category, &p.RemoteKey, &p.CreatedBy, scanTime(&p.CreatedAt))
	if err != nil {
		return nil, fmt.Errorf("create project: %w", err)
	}
	return &p, nil
}

func (s *SQLiteStore) GetProjectByID(ctx context.Context, id string) (*model.Project, error) {
	var p model.Project
	err := s.db.QueryRowContext(ctx,
		`SELECT id, org_id, name, slug, category, remote_key, created_by, created_at FROM projects WHERE id = ?`,
		id,
	).Scan(&p.ID, &p.OrgID, &p.Name, &p.Slug, &p.Category, &p.RemoteKey, &p.CreatedBy, scanTime(&p.CreatedAt))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get project by id: %w", err)
	}
	return &p, nil
}

func (s *SQLiteStore) GetActiveProjectByID(ctx context.Context, id string) (*model.Project, error) {
	var p model.Project
	err := s.db.QueryRowContext(ctx,
		`SELECT id, org_id, name, slug, category, remote_key, created_by, created_at
		 FROM projects WHERE id = ? AND deleted_at IS NULL`,
		id,
	).Scan(&p.ID, &p.OrgID, &p.Name, &p.Slug, &p.Category, &p.RemoteKey, &p.CreatedBy, scanTime(&p.CreatedAt))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get active project by id: %w", err)
	}
	return &p, nil
}

func (s *SQLiteStore) GetProjectByRemoteKey(ctx context.Context, orgID, remoteKey string) (*model.Project, error) {
	var p model.Project
	err := s.db.QueryRowContext(ctx,
		`SELECT id, org_id, name, slug, category, remote_key, created_by, created_at FROM projects WHERE org_id = ? AND remote_key = ?`,
		orgID, remoteKey,
	).Scan(&p.ID, &p.OrgID, &p.Name, &p.Slug, &p.Category, &p.RemoteKey, &p.CreatedBy, scanTime(&p.CreatedAt))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get project by remote key: %w", err)
	}
	return &p, nil
}

func (s *SQLiteStore) ListProjectsByTeamAccess(ctx context.Context, accountID string) ([]*model.Project, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT DISTINCT p.id, p.org_id, p.name, p.slug, p.category, p.remote_key, p.created_by, p.created_at
		 FROM projects p
		 JOIN team_project_access tpa ON tpa.project_id = p.id
		 JOIN team_members tm ON tm.team_id = tpa.team_id
		 WHERE tm.account_id = ? AND p.deleted_at IS NULL
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
		if err := rows.Scan(&p.ID, &p.OrgID, &p.Name, &p.Slug, &p.Category, &p.RemoteKey, &p.CreatedBy, scanTime(&p.CreatedAt)); err != nil {
			return nil, fmt.Errorf("scan project: %w", err)
		}
		projects = append(projects, &p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate projects: %w", err)
	}
	return projects, nil
}

func (s *SQLiteStore) HasProjectAccess(ctx context.Context, accountID, projectID string) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx,
		`SELECT EXISTS (
		   SELECT 1
		   FROM team_project_access tpa
		   JOIN team_members tm ON tm.team_id = tpa.team_id
		   WHERE tm.account_id = ? AND tpa.project_id = ?
		 )`,
		accountID, projectID,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check project access: %w", err)
	}
	return exists, nil
}

func (s *SQLiteStore) SoftDeleteProject(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE projects SET deleted_at = datetime('now') WHERE id = ? AND deleted_at IS NULL`,
		id,
	)
	if err != nil {
		return fmt.Errorf("soft delete project: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("soft delete rows affected: %w", err)
	}
	if affected == 0 {
		var exists bool
		if err := s.db.QueryRowContext(ctx,
			`SELECT EXISTS (SELECT 1 FROM projects WHERE id = ?)`, id,
		).Scan(&exists); err != nil {
			return fmt.Errorf("verify project existence: %w", err)
		}
		if !exists {
			return ErrNotFound
		}
	}
	return nil
}

func (s *SQLiteStore) EnsureCreatorProjectAccess(ctx context.Context, orgID, accountID, projectID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	teamID, err := sqliteEnsureDefaultTeamTx(ctx, tx, orgID)
	if err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO team_members (team_id, account_id) VALUES (?, ?) ON CONFLICT DO NOTHING`,
		teamID, accountID,
	); err != nil {
		return fmt.Errorf("add team member: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO team_project_access (team_id, project_id, role) VALUES (?, ?, 'writer')
		 ON CONFLICT (team_id, project_id) DO NOTHING`,
		teamID, projectID,
	); err != nil {
		return fmt.Errorf("grant project access: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

func (s *SQLiteStore) EnsureDefaultTeamMembership(ctx context.Context, orgID, accountID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	teamID, err := sqliteEnsureDefaultTeamTx(ctx, tx, orgID)
	if err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO team_members (team_id, account_id) VALUES (?, ?) ON CONFLICT DO NOTHING`,
		teamID, accountID,
	); err != nil {
		return fmt.Errorf("add team member: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}
