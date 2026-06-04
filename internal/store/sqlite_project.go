package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jholhewres/anchored_oss/internal/model"
	projectpkg "github.com/jholhewres/anchored_oss/internal/project"
)

// projectColumns is the canonical SELECT column list for a project row. The
// scan order in scanProjectRow / scanProjectRows must match this exactly.
const projectColumns = `id, org_id, name, slug, category, remote_key, remote_key_v1, repo_url, created_by, created_at`

// scanProjectRow scans one project row using the projectColumns order. The
// nullable remote_key_v1 / repo_url columns are scanned through NullString so a
// pre-backfill NULL reads as "". created_at goes through scanTime (modernc
// returns DATETIME as a string).
func scanProjectRow(row interface{ Scan(...any) error }, p *model.Project) error {
	var keyV1, repoURL sql.NullString
	if err := row.Scan(&p.ID, &p.OrgID, &p.Name, &p.Slug, &p.Category, &p.RemoteKey, &keyV1, &repoURL, &p.CreatedBy, scanTime(&p.CreatedAt)); err != nil {
		return err
	}
	p.RemoteKeyV1 = keyV1.String
	p.RepoURL = repoURL.String
	return nil
}

func (s *SQLiteStore) CreateProject(ctx context.Context, orgID, name, slug, remoteKey, remoteKeyV1, repoURL, createdBy, category string) (*model.Project, error) {
	id := newUUID()
	category = model.NormalizeCategory(category)
	var p model.Project
	err := scanProjectRow(s.db.QueryRowContext(ctx,
		`INSERT INTO projects (id, org_id, name, slug, remote_key, remote_key_v1, repo_url, created_by, category) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		 RETURNING `+projectColumns,
		id, orgID, name, slug, remoteKey, nullIfEmpty(remoteKeyV1), nullIfEmpty(repoURL), createdBy, category,
	), &p)
	if err != nil {
		return nil, fmt.Errorf("create project: %w", err)
	}
	return &p, nil
}

func (s *SQLiteStore) GetProjectByID(ctx context.Context, id string) (*model.Project, error) {
	var p model.Project
	err := scanProjectRow(s.db.QueryRowContext(ctx,
		`SELECT `+projectColumns+` FROM projects WHERE id = ?`,
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

func (s *SQLiteStore) GetActiveProjectByID(ctx context.Context, id string) (*model.Project, error) {
	var p model.Project
	err := scanProjectRow(s.db.QueryRowContext(ctx,
		`SELECT `+projectColumns+` FROM projects WHERE id = ? AND deleted_at IS NULL`,
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

// GetProjectByRemoteKey resolves an active project by EITHER the canonical
// remote_key or the legacy remote_key_v1, so a push stamped with either key
// lands in the same project after the v2 normalization change.
func (s *SQLiteStore) GetProjectByRemoteKey(ctx context.Context, orgID, remoteKey string) (*model.Project, error) {
	var p model.Project
	err := scanProjectRow(s.db.QueryRowContext(ctx,
		`SELECT `+projectColumns+` FROM projects WHERE org_id = ? AND (remote_key = ? OR remote_key_v1 = ?) AND deleted_at IS NULL`,
		orgID, remoteKey, remoteKey,
	), &p)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get project by remote key: %w", err)
	}
	return &p, nil
}

// UpdateProject applies a partial update within an org. RepoURL recomputes both
// remote keys: non-empty derives canonical + legacy keys and stores the URL;
// empty clears repo_url and both keys (unlinking the repo). Returns ErrNotFound
// when the project is absent or owned by another org.
func (s *SQLiteStore) UpdateProject(ctx context.Context, orgID, id string, upd model.ProjectUpdate) (*model.Project, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var cur model.Project
	err = scanProjectRow(tx.QueryRowContext(ctx,
		`SELECT `+projectColumns+` FROM projects WHERE id = ? AND org_id = ? AND deleted_at IS NULL`,
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
		`UPDATE projects SET name = ?, slug = ?, category = ?, remote_key = ?, remote_key_v1 = ?, repo_url = ?
		 WHERE id = ? AND org_id = ?`,
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

func (s *SQLiteStore) ListProjectsByTeamAccess(ctx context.Context, accountID string) ([]*model.Project, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT DISTINCT p.id, p.org_id, p.name, p.slug, p.category, p.remote_key, p.remote_key_v1, p.repo_url, p.created_by, p.created_at
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
		if err := scanProjectRow(rows, &p); err != nil {
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

// SoftDeleteProject marks a project deleted and mangles its identity fields so
// the slug and both remote keys are freed for reuse: a new project can take the
// same slug/repo immediately. The original values survive only in the audit
// log. Mangling values are computed in Go (slug + "-deleted-" + id[:8]), not in
// SQL, to keep the statement backend-agnostic.
func (s *SQLiteStore) SoftDeleteProject(ctx context.Context, id string) error {
	var slug string
	err := s.db.QueryRowContext(ctx,
		`SELECT slug FROM projects WHERE id = ? AND deleted_at IS NULL`, id,
	).Scan(&slug)
	if errors.Is(err, sql.ErrNoRows) {
		// Either never existed or already deleted: distinguish for ErrNotFound.
		var exists bool
		if qerr := s.db.QueryRowContext(ctx,
			`SELECT EXISTS (SELECT 1 FROM projects WHERE id = ?)`, id,
		).Scan(&exists); qerr != nil {
			return fmt.Errorf("verify project existence: %w", qerr)
		}
		if !exists {
			return ErrNotFound
		}
		return nil // already deleted: idempotent no-op
	}
	if err != nil {
		return fmt.Errorf("load project for soft delete: %w", err)
	}

	mangledSlug := mangleDeletedSlug(slug, id)
	if _, err := s.db.ExecContext(ctx,
		`UPDATE projects SET deleted_at = datetime('now'), slug = ?, remote_key = ?, remote_key_v1 = NULL, repo_url = NULL
		 WHERE id = ? AND deleted_at IS NULL`,
		mangledSlug, deletedRemoteKey(id), id,
	); err != nil {
		return fmt.Errorf("soft delete project: %w", err)
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
