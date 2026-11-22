package store

import (
	"context"
	"fmt"

	"github.com/jholhewres/anchored_oss/internal/auth"

	"github.com/jholhewres/anchored_oss/internal/model"
)

// UpdateAccount updates display_name and/or role for the given account.
// An empty string for either field means "no change", and so does a role equal
// to the current one: only a real role change revokes the login session and
// the keys above the new role.
func (s *PostgresStore) UpdateAccount(ctx context.Context, orgID, id, displayName, role string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin update account: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op after Commit
	if displayName != "" {
		if _, err := tx.ExecContext(ctx,
			`UPDATE accounts SET display_name = $1 WHERE id = $2`,
			displayName, id,
		); err != nil {
			return fmt.Errorf("update account display_name: %w", err)
		}
	}
	if role != "" {
		// Only a real change revokes anything: a PATCH that resends the
		// current role (a rename from the dashboard does) updates no row and
		// keeps every key. One conditional statement, so a concurrent role
		// change cannot slip between a read and the write.
		res, err := tx.ExecContext(ctx,
			`UPDATE org_members SET role = $1 WHERE account_id = $2 AND org_id = $3 AND role <> $1`,
			role, id, orgID,
		)
		if err != nil {
			return fmt.Errorf("update account role: %w", err)
		}
		changed, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("update account role rows affected: %w", err)
		}
		if changed > 0 {
			// The new role takes effect now: the login session (re-minted with the
			// new scope at the next login) and every key above the role go.
			above := auth.ScopesAboveRole(role)
			if _, err := tx.ExecContext(ctx,
				`UPDATE api_keys SET revoked_at = now()
				 WHERE account_id = $1 AND org_id = $2 AND revoked_at IS NULL
				   AND (name = 'session' OR scope = $3 OR scope = $4)`,
				id, orgID, above[0], above[1],
			); err != nil {
				return fmt.Errorf("revoke keys above the new role: %w", err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit update account: %w", err)
	}
	return nil
}

// SoftDeleteAccount removes the account from orgID: its keys there are
// revoked, it leaves the org's teams and its membership is deleted, so its
// password no longer opens a session in that org.
func (s *PostgresStore) SoftDeleteAccount(ctx context.Context, orgID, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin delete account: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op after Commit
	if _, err := tx.ExecContext(ctx,
		`UPDATE api_keys SET revoked_at = now() WHERE account_id = $1 AND org_id = $2 AND revoked_at IS NULL`,
		id, orgID,
	); err != nil {
		return fmt.Errorf("revoke account keys: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM team_members WHERE account_id = $1 AND team_id IN (SELECT id FROM teams WHERE org_id = $2)`,
		id, orgID,
	); err != nil {
		return fmt.Errorf("remove team memberships: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM org_members WHERE account_id = $1 AND org_id = $2`,
		id, orgID,
	); err != nil {
		return fmt.Errorf("remove org membership: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit delete account: %w", err)
	}
	return nil
}

// ListAccountProjects returns all non-deleted projects accessible to accountID.
func (s *PostgresStore) ListAccountProjects(ctx context.Context, orgID, accountID string) ([]*model.Project, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT DISTINCT p.id, p.org_id, p.name, p.slug, p.category, p.remote_key, p.created_by, p.created_at
		 FROM projects p
		 JOIN team_project_access tpa ON tpa.project_id = p.id
		 JOIN team_members tm ON tm.team_id = tpa.team_id
		 WHERE tm.account_id = $1 AND p.org_id = $2 AND p.deleted_at IS NULL
		 ORDER BY p.name`,
		accountID, orgID,
	)
	if err != nil {
		return nil, fmt.Errorf("list account projects: %w", err)
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

// SetAccountProjects sets the account's project access via the org's default
// team. Adds grants for new projects and removes grants no longer in the list.
// Restricts to projects belonging to orgID.
func (s *PostgresStore) SetAccountProjects(ctx context.Context, orgID, accountID string, projectIDs []string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	teamID, err := ensureDefaultTeamTx(ctx, tx, orgID)
	if err != nil {
		return err
	}

	// Ensure the account is in the default team.
	// Only a member of orgID joins its default team (ErrNotFound otherwise).
	var member bool
	if err := tx.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM org_members WHERE org_id = $1 AND account_id = $2)`,
		orgID, accountID,
	).Scan(&member); err != nil {
		return fmt.Errorf("check org membership: %w", err)
	}
	if !member {
		return ErrNotFound
	}
	if err := pgJoinTeamIfOrgMember(ctx, tx, orgID, teamID, accountID); err != nil {
		return err
	}

	// Build a set of desired project IDs (validated against org ownership).
	desired := make(map[string]bool, len(projectIDs))
	for _, pid := range projectIDs {
		var belongs bool
		if err := tx.QueryRowContext(ctx,
			`SELECT EXISTS(SELECT 1 FROM projects WHERE id = $1 AND org_id = $2 AND deleted_at IS NULL)`,
			pid, orgID,
		).Scan(&belongs); err != nil {
			return fmt.Errorf("validate project %s: %w", pid, err)
		}
		if belongs {
			desired[pid] = true
		}
	}

	// Current grants for this team.
	rows, err := tx.QueryContext(ctx,
		`SELECT project_id FROM team_project_access WHERE team_id = $1`,
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

	// Add missing grants.
	for pid := range desired {
		if !current[pid] {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO team_project_access (team_id, project_id, role) VALUES ($1, $2, 'writer')
				 ON CONFLICT (team_id, project_id) DO NOTHING`,
				teamID, pid,
			); err != nil {
				return fmt.Errorf("grant project %s: %w", pid, err)
			}
		}
	}

	// Remove stale grants (only for projects in this org).
	for pid := range current {
		if !desired[pid] {
			var inOrg bool
			if err := tx.QueryRowContext(ctx,
				`SELECT EXISTS(SELECT 1 FROM projects WHERE id = $1 AND org_id = $2)`,
				pid, orgID,
			).Scan(&inOrg); err != nil {
				return fmt.Errorf("check project org: %w", err)
			}
			if inOrg {
				if _, err := tx.ExecContext(ctx,
					`DELETE FROM team_project_access WHERE team_id = $1 AND project_id = $2`,
					teamID, pid,
				); err != nil {
					return fmt.Errorf("remove grant %s: %w", pid, err)
				}
			}
		}
	}

	return tx.Commit()
}
