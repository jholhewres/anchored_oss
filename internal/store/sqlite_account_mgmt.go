package store

import (
	"context"
	"fmt"

	"github.com/jholhewres/anchored_oss/internal/model"
)

func (s *SQLiteStore) UpdateAccount(ctx context.Context, id, displayName, role string) error {
	if displayName != "" {
		if _, err := s.db.ExecContext(ctx,
			`UPDATE accounts SET display_name = ? WHERE id = ?`,
			displayName, id,
		); err != nil {
			return fmt.Errorf("update account display_name: %w", err)
		}
	}
	if role != "" {
		if _, err := s.db.ExecContext(ctx,
			`UPDATE org_members SET role = ? WHERE account_id = ?`,
			role, id,
		); err != nil {
			return fmt.Errorf("update account role: %w", err)
		}
	}
	return nil
}

func (s *SQLiteStore) SoftDeleteAccount(ctx context.Context, id string) error {
	if _, err := s.db.ExecContext(ctx,
		`UPDATE api_keys SET revoked_at = datetime('now') WHERE account_id = ? AND revoked_at IS NULL`,
		id,
	); err != nil {
		return fmt.Errorf("revoke account keys: %w", err)
	}
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM team_members WHERE account_id = ?`,
		id,
	); err != nil {
		return fmt.Errorf("remove team memberships: %w", err)
	}
	return nil
}

func (s *SQLiteStore) ListAccountProjects(ctx context.Context, accountID string) ([]*model.Project, error) {
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
		return nil, fmt.Errorf("list account projects: %w", err)
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

func (s *SQLiteStore) SetAccountProjects(ctx context.Context, orgID, accountID string, projectIDs []string) error {
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
		return fmt.Errorf("ensure team member: %w", err)
	}

	desired := make(map[string]bool, len(projectIDs))
	for _, pid := range projectIDs {
		var belongs bool
		if err := tx.QueryRowContext(ctx,
			`SELECT EXISTS(SELECT 1 FROM projects WHERE id = ? AND org_id = ? AND deleted_at IS NULL)`,
			pid, orgID,
		).Scan(&belongs); err != nil {
			return fmt.Errorf("validate project %s: %w", pid, err)
		}
		if belongs {
			desired[pid] = true
		}
	}

	rows, err := tx.QueryContext(ctx,
		`SELECT project_id FROM team_project_access WHERE team_id = ?`,
		teamID,
	)
	if err != nil {
		return fmt.Errorf("list current grants: %w", err)
	}
	current := make(map[string]bool)
	for rows.Next() {
		var pid string
		if err := rows.Scan(&pid); err != nil {
			rows.Close()
			return fmt.Errorf("scan grant: %w", err)
		}
		current[pid] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate grants: %w", err)
	}

	for pid := range desired {
		if !current[pid] {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO team_project_access (team_id, project_id, role) VALUES (?, ?, 'writer')
				 ON CONFLICT (team_id, project_id) DO NOTHING`,
				teamID, pid,
			); err != nil {
				return fmt.Errorf("grant project %s: %w", pid, err)
			}
		}
	}

	for pid := range current {
		if !desired[pid] {
			var inOrg bool
			if err := tx.QueryRowContext(ctx,
				`SELECT EXISTS(SELECT 1 FROM projects WHERE id = ? AND org_id = ?)`,
				pid, orgID,
			).Scan(&inOrg); err != nil {
				return fmt.Errorf("check project org: %w", err)
			}
			if inOrg {
				if _, err := tx.ExecContext(ctx,
					`DELETE FROM team_project_access WHERE team_id = ? AND project_id = ?`,
					teamID, pid,
				); err != nil {
					return fmt.Errorf("remove grant %s: %w", pid, err)
				}
			}
		}
	}

	return tx.Commit()
}
