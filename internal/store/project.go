package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jholhewres/anchored_oss/internal/model"
	projectpkg "github.com/jholhewres/anchored_oss/internal/project"
)

const defaultTeamSlug = "default"

// pgProjectColumns is the canonical SELECT column list for a project row on
// Postgres. The scan order in scanPGProjectRow must match it exactly.
const pgProjectColumns = `id, org_id, name, slug, category, remote_key, remote_key_v1, repo_url, created_by, created_at`

// scanPGProjectRow scans one project row using pgProjectColumns. The nullable
// remote_key_v1 / repo_url columns are scanned through NullString so a
// pre-backfill NULL reads as "".
func scanPGProjectRow(row interface{ Scan(...any) error }, p *model.Project) error {
	var keyV1, repoURL sql.NullString
	if err := row.Scan(&p.ID, &p.OrgID, &p.Name, &p.Slug, &p.Category, &p.RemoteKey, &keyV1, &repoURL, &p.CreatedBy, &p.CreatedAt); err != nil {
		return err
	}
	p.RemoteKeyV1 = keyV1.String
	p.RepoURL = repoURL.String
	return nil
}

func (s *PostgresStore) CreateProject(ctx context.Context, orgID, name, slug, remoteKey, remoteKeyV1, repoURL, createdBy, category string) (*model.Project, error) {
	var p model.Project
	err := scanPGProjectRow(s.db.QueryRowContext(ctx,
		`INSERT INTO projects (org_id, name, slug, remote_key, remote_key_v1, repo_url, created_by, category) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 RETURNING `+pgProjectColumns,
		orgID, name, slug, remoteKey, nullIfEmpty(remoteKeyV1), nullIfEmpty(repoURL), createdBy, category,
	), &p)
	if err != nil {
		return nil, fmt.Errorf("create project: %w", err)
	}
	return &p, nil
}

func (s *PostgresStore) GetProjectByID(ctx context.Context, id string) (*model.Project, error) {
	var p model.Project
	err := scanPGProjectRow(s.db.QueryRowContext(ctx,
		`SELECT `+pgProjectColumns+` FROM projects WHERE id = $1`,
		id,
	), &p)
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
	err := scanPGProjectRow(s.db.QueryRowContext(ctx,
		`SELECT `+pgProjectColumns+`
		 FROM projects WHERE id = $1 AND deleted_at IS NULL`,
		id,
	), &p)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get active project by id: %w", err)
	}
	return &p, nil
}

// UpdateProject applies a partial update within an org. RepoURL recomputes both
// remote keys: non-empty derives canonical + legacy keys and stores the URL;
// empty clears repo_url and both keys. Returns ErrNotFound when the project is
// absent or owned by another org.
func (s *PostgresStore) UpdateProject(ctx context.Context, orgID, id string, upd model.ProjectUpdate) (*model.Project, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var cur model.Project
	err = scanPGProjectRow(tx.QueryRowContext(ctx,
		`SELECT `+pgProjectColumns+` FROM projects WHERE id = $1 AND org_id = $2 AND deleted_at IS NULL FOR UPDATE`,
		id, orgID,
	), &cur)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load project for update: %w", err)
	}

	if upd.Name != nil {
		cur.Name = *upd.Name
	}
	if upd.Slug != nil {
		cur.Slug = *upd.Slug
	}
	if upd.Category != nil {
		cur.Category = model.NormalizeCategory(*upd.Category)
	}
	if upd.RepoURL != nil {
		if *upd.RepoURL == "" {
			// Clearing the repo frees the canonical key for reuse. Park it on a
			// per-row sentinel so concurrent clears in the same org don't collide
			// on UNIQUE(org_id, remote_key) (the column is NOT NULL).
			cur.RepoURL = ""
			cur.RemoteKey = noRepoRemoteKey(id)
			cur.RemoteKeyV1 = ""
		} else {
			cur.RepoURL = *upd.RepoURL
			cur.RemoteKey = projectpkg.DeriveRemoteKey(*upd.RepoURL)
			cur.RemoteKeyV1 = projectpkg.DeriveLegacyRemoteKey(*upd.RepoURL)
		}
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE projects SET name = $1, slug = $2, category = $3, remote_key = $4, remote_key_v1 = $5, repo_url = $6
		 WHERE id = $7 AND org_id = $8`,
		cur.Name, cur.Slug, cur.Category, cur.RemoteKey, nullIfEmpty(cur.RemoteKeyV1), nullIfEmpty(cur.RepoURL), id, orgID,
	); err != nil {
		if isUniqueViolation(err) {
			return nil, ErrConflict
		}
		return nil, fmt.Errorf("update project: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit update: %w", err)
	}
	return &cur, nil
}

// HasProjectAccess reports whether accountID has team-based access to
// projectID inside orgID: the project is active and both it and the granting
// team belong to orgID. Used to authorize project endpoints and sync for
// non-admin scopes.
func (s *PostgresStore) HasProjectAccess(ctx context.Context, orgID, accountID, projectID string) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx,
		`SELECT EXISTS (
		   SELECT 1
		   FROM team_project_access tpa
		   JOIN team_members tm ON tm.team_id = tpa.team_id
		   JOIN teams t ON t.id = tpa.team_id
		   JOIN projects p ON p.id = tpa.project_id
		   WHERE tm.account_id = $1 AND tpa.project_id = $2
		     AND t.org_id = $3 AND p.org_id = $3 AND p.deleted_at IS NULL
		 )`,
		accountID, projectID, orgID,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check project access: %w", err)
	}
	return exists, nil
}

// SoftDeleteProject marks a project as deleted and mangles its identity fields
// so the slug and both remote keys are freed for reuse (a new project can take
// the same slug/repo immediately). Mangling values are computed in Go, not SQL,
// so the statement stays backend-agnostic. Idempotent: returns ErrNotFound only
// when the project never existed in orgID (another org's project counts as
// absent); double-deletes are a no-op.
func (s *PostgresStore) SoftDeleteProject(ctx context.Context, orgID, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin soft delete project: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var slug string
	err = tx.QueryRowContext(ctx,
		`SELECT slug FROM projects WHERE id = $1 AND org_id = $2 AND deleted_at IS NULL`, id, orgID,
	).Scan(&slug)
	if errors.Is(err, sql.ErrNoRows) {
		var exists bool
		if qerr := tx.QueryRowContext(ctx,
			`SELECT EXISTS (SELECT 1 FROM projects WHERE id = $1 AND org_id = $2)`, id, orgID,
		).Scan(&exists); qerr != nil {
			return fmt.Errorf("verify project existence: %w", qerr)
		}
		if !exists {
			return ErrNotFound
		}
		if _, purgeErr := tx.ExecContext(ctx,
			`UPDATE memory_write_idempotency
			 SET response_json = jsonb_build_object(
			   'memory', jsonb_build_object('id', memory_id),
			   'created', false
			 )
			 WHERE memory_id IN (SELECT id FROM memories WHERE project_id = $1)`,
			id,
		); purgeErr != nil {
			return fmt.Errorf("redact project idempotency after soft delete: %w", purgeErr)
		}
		return tx.Commit()
	}
	if err != nil {
		return fmt.Errorf("load project for soft delete: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE memory_write_idempotency
		 SET response_json = jsonb_build_object(
		   'memory', jsonb_build_object('id', memory_id),
		   'created', false
		 )
		 WHERE memory_id IN (SELECT id FROM memories WHERE project_id = $1)`,
		id,
	); err != nil {
		return fmt.Errorf("redact project idempotency on soft delete: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE projects SET deleted_at = now(), slug = $1, remote_key = $2, remote_key_v1 = NULL, repo_url = NULL
		 WHERE id = $3 AND org_id = $4 AND deleted_at IS NULL`,
		mangleDeletedSlug(slug, id), deletedRemoteKey(id), id, orgID,
	); err != nil {
		return fmt.Errorf("soft delete project: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit soft delete project: %w", err)
	}
	return nil
}

// GetProjectByRemoteKey resolves an active project by EITHER the canonical
// remote_key or the legacy remote_key_v1, so a push stamped with either key
// lands in the same project after the v2 normalization change.
func (s *PostgresStore) GetProjectByRemoteKey(ctx context.Context, orgID, remoteKey string) (*model.Project, error) {
	var p model.Project
	err := scanPGProjectRow(s.db.QueryRowContext(ctx,
		`SELECT `+pgProjectColumns+` FROM projects WHERE org_id = $1 AND (remote_key = $2 OR remote_key_v1 = $2) AND deleted_at IS NULL`,
		orgID, remoteKey,
	), &p)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get project by remote key: %w", err)
	}
	return &p, nil
}

func (s *PostgresStore) ListProjectsByTeamAccess(ctx context.Context, orgID, accountID string) ([]*model.Project, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT DISTINCT p.id, p.org_id, p.name, p.slug, p.category, p.remote_key, p.remote_key_v1, p.repo_url, p.created_by, p.created_at
		 FROM projects p
		 JOIN team_project_access tpa ON tpa.project_id = p.id
		 JOIN team_members tm ON tm.team_id = tpa.team_id
		 JOIN teams t ON t.id = tpa.team_id
		 WHERE tm.account_id = $1 AND p.org_id = $2 AND t.org_id = $2 AND p.deleted_at IS NULL
		 ORDER BY p.name`,
		accountID, orgID,
	)
	if err != nil {
		return nil, fmt.Errorf("list projects by team access: %w", err)
	}
	defer rows.Close()

	var projects []*model.Project
	for rows.Next() {
		var p model.Project
		if err := scanPGProjectRow(rows, &p); err != nil {
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

	if err := pgJoinTeamIfOrgMember(ctx, tx, orgID, teamID, accountID); err != nil {
		return err
	}

	if err := pgGrantIfOrgProject(ctx, tx, orgID, teamID, projectID); err != nil {
		return err
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

	if err := pgJoinTeamIfOrgMember(ctx, tx, orgID, teamID, accountID); err != nil {
		return err
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

// pgJoinTeamIfOrgMember puts accountID on teamID only when it is a member of
// orgID: a team never takes an account of another org, whoever calls.
func pgJoinTeamIfOrgMember(ctx context.Context, tx *sql.Tx, orgID, teamID, accountID string) error {
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO team_members (team_id, account_id)
		 SELECT $1::uuid, $2::uuid
		 WHERE EXISTS (SELECT 1 FROM org_members WHERE org_id = $3 AND account_id = $2)
		 ON CONFLICT DO NOTHING`,
		teamID, accountID, orgID,
	); err != nil {
		return fmt.Errorf("add team member: %w", err)
	}
	return nil
}

// IsDefaultTeamMember reports whether accountID is on orgID's default team.
func (s *PostgresStore) IsDefaultTeamMember(ctx context.Context, orgID, accountID string) (bool, error) {
	var ok bool
	err := s.db.QueryRowContext(ctx,
		`SELECT EXISTS (
		   SELECT 1 FROM team_members tm JOIN teams t ON t.id = tm.team_id
		   WHERE t.org_id = $1 AND t.slug = $2 AND tm.account_id = $3
		 )`,
		orgID, defaultTeamSlug, accountID,
	).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("check default team membership: %w", err)
	}
	return ok, nil
}

// pgGrantIfOrgProject grants teamID write access to projectID only when the
// project belongs to orgID: a team never holds another org's project.
func pgGrantIfOrgProject(ctx context.Context, tx *sql.Tx, orgID, teamID, projectID string) error {
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO team_project_access (team_id, project_id, role)
		 SELECT $1::uuid, $2::uuid, 'writer'
		 WHERE EXISTS (SELECT 1 FROM projects WHERE id = $2 AND org_id = $3)
		 ON CONFLICT (team_id, project_id) DO NOTHING`,
		teamID, projectID, orgID,
	); err != nil {
		return fmt.Errorf("grant project access: %w", err)
	}
	return nil
}

// GrantProjectToDefaultTeam grants orgID's default team access to a project of
// orgID, creating the team if needed, without adding any member.
func (s *PostgresStore) GrantProjectToDefaultTeam(ctx context.Context, orgID, projectID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	teamID, err := ensureDefaultTeamTx(ctx, tx, orgID)
	if err != nil {
		return err
	}
	if err := pgGrantIfOrgProject(ctx, tx, orgID, teamID, projectID); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}
