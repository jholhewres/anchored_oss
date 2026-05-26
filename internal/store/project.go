package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jholhewres/anchored_oss/internal/model"
)

const defaultTeamSlug = "default"

func (s *PostgresStore) CreateProject(ctx context.Context, orgID, name, slug, remoteKey, createdBy, category string) (*model.Project, error) {
	var p model.Project
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO projects (org_id, name, slug, remote_key, created_by, category) VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, org_id, name, slug, category, remote_key, created_by, created_at`,
		orgID, name, slug, remoteKey, createdBy, category,
	).Scan(&p.ID, &p.OrgID, &p.Name, &p.Slug, &p.Category, &p.RemoteKey, &p.CreatedBy, &p.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create project: %w", err)
	}
	return &p, nil
}

func (s *PostgresStore) GetProjectByID(ctx context.Context, id string) (*model.Project, error) {
	var p model.Project
	err := s.db.QueryRowContext(ctx,
		`SELECT id, org_id, name, slug, category, remote_key, created_by, created_at FROM projects WHERE id = $1`,
		id,
	).Scan(&p.ID, &p.OrgID, &p.Name, &p.Slug, &p.Category, &p.RemoteKey, &p.CreatedBy, &p.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get project by id: %w", err)
	}
	return &p, nil
}

// GetActiveProjectByID returns the project only when it is not soft-deleted.
// Used by dashboard handlers to keep deleted projects out of the user-facing
// surface while leaving the sync engine's GetProjectByID lookup unchanged.
func (s *PostgresStore) GetActiveProjectByID(ctx context.Context, id string) (*model.Project, error) {
	var p model.Project
	err := s.db.QueryRowContext(ctx,
		`SELECT id, org_id, name, slug, category, remote_key, created_by, created_at
		 FROM projects WHERE id = $1 AND deleted_at IS NULL`,
		id,
	).Scan(&p.ID, &p.OrgID, &p.Name, &p.Slug, &p.Category, &p.RemoteKey, &p.CreatedBy, &p.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get active project by id: %w", err)
	}
	return &p, nil
}

// HasProjectAccess reports whether accountID has team-based access to
// projectID. Used to authorize project read endpoints for non-admin scopes.
func (s *PostgresStore) HasProjectAccess(ctx context.Context, accountID, projectID string) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx,
		`SELECT EXISTS (
		   SELECT 1
		   FROM team_project_access tpa
		   JOIN team_members tm ON tm.team_id = tpa.team_id
		   WHERE tm.account_id = $1 AND tpa.project_id = $2
		 )`,
		accountID, projectID,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check project access: %w", err)
	}
	return exists, nil
}

// SoftDeleteProject marks a project as deleted. Idempotent: returns
// ErrNotFound only when the project never existed; double-deletes are a
// no-op so the dashboard never surprises an admin who clicks twice.
func (s *PostgresStore) SoftDeleteProject(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE projects SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL`,
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
		// Distinguish "never existed" from "already deleted" with a follow-up read.
		var exists bool
		if err := s.db.QueryRowContext(ctx,
			`SELECT EXISTS (SELECT 1 FROM projects WHERE id = $1)`, id,
		).Scan(&exists); err != nil {
			return fmt.Errorf("verify project existence: %w", err)
		}
		if !exists {
			return ErrNotFound
		}
	}
	return nil
}

func (s *PostgresStore) GetProjectByRemoteKey(ctx context.Context, orgID, remoteKey string) (*model.Project, error) {
	var p model.Project
	err := s.db.QueryRowContext(ctx,
		`SELECT id, org_id, name, slug, category, remote_key, created_by, created_at FROM projects WHERE org_id = $1 AND remote_key = $2`,
		orgID, remoteKey,
	).Scan(&p.ID, &p.OrgID, &p.Name, &p.Slug, &p.Category, &p.RemoteKey, &p.CreatedBy, &p.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get project by remote key: %w", err)
	}
	return &p, nil
}

func (s *PostgresStore) ListProjectsByTeamAccess(ctx context.Context, accountID string) ([]*model.Project, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT DISTINCT p.id, p.org_id, p.name, p.slug, p.category, p.remote_key, p.created_by, p.created_at
		 FROM projects p
		 JOIN team_project_access tpa ON tpa.project_id = p.id
		 JOIN team_members tm ON tm.team_id = tpa.team_id
		 WHERE tm.account_id = $1 AND p.deleted_at IS NULL
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
		if err := rows.Scan(&p.ID, &p.OrgID, &p.Name, &p.Slug, &p.Category, &p.RemoteKey, &p.CreatedBy, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan project: %w", err)
		}
		projects = append(projects, &p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate projects: %w", err)
	}
	return projects, nil
}

// EnsureCreatorProjectAccess wires the creator of a project into the org's
// default team and grants that team write access to the project, atomically.
// Safe to call multiple times.
func (s *PostgresStore) EnsureCreatorProjectAccess(ctx context.Context, orgID, accountID, projectID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	teamID, err := ensureDefaultTeamTx(ctx, tx, orgID)
	if err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO team_members (team_id, account_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		teamID, accountID,
	); err != nil {
		return fmt.Errorf("add team member: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO team_project_access (team_id, project_id, role) VALUES ($1, $2, 'writer')
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

// EnsureDefaultTeamMembership creates the org's default team if needed and
// adds the account to it. Used by bootstrap so the admin can immediately
// receive grants when projects are created later.
func (s *PostgresStore) EnsureDefaultTeamMembership(ctx context.Context, orgID, accountID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	teamID, err := ensureDefaultTeamTx(ctx, tx, orgID)
	if err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO team_members (team_id, account_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		teamID, accountID,
	); err != nil {
		return fmt.Errorf("add team member: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

func ensureDefaultTeamTx(ctx context.Context, tx *sql.Tx, orgID string) (string, error) {
	var teamID string
	err := tx.QueryRowContext(ctx,
		`SELECT id FROM teams WHERE org_id = $1 AND slug = $2`,
		orgID, defaultTeamSlug,
	).Scan(&teamID)
	if err == nil {
		return teamID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("lookup default team: %w", err)
	}

	if err := tx.QueryRowContext(ctx,
		`INSERT INTO teams (org_id, name, slug) VALUES ($1, $2, $3)
		 ON CONFLICT (org_id, slug) DO UPDATE SET name = EXCLUDED.name
		 RETURNING id`,
		orgID, "Default", defaultTeamSlug,
	).Scan(&teamID); err != nil {
		return "", fmt.Errorf("create default team: %w", err)
	}
	return teamID, nil
}
