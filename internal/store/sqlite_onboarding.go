package store

import (
	"context"
	"fmt"

	"github.com/jholhewres/anchored_oss/internal/model"
)

// BootstrapOnboarding atomically creates a first-run organization and its
// owner account, access grants, API key, and initial remote-backed projects.
func (s *SQLiteStore) BootstrapOnboarding(ctx context.Context, input OnboardingBootstrapInput) (*OnboardingBootstrapResult, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin onboarding bootstrap: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var organizationCount int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM organizations`).Scan(&organizationCount); err != nil {
		return nil, fmt.Errorf("count organizations for onboarding: %w", err)
	}
	if organizationCount > 0 {
		return nil, ErrConflict
	}

	var org model.Organization
	if err := tx.QueryRowContext(ctx,
		`INSERT INTO organizations (id, name, slug) VALUES (?, ?, ?) RETURNING id, name, slug, created_at`,
		newUUID(), input.OrgName, input.OrgSlug,
	).Scan(&org.ID, &org.Name, &org.Slug, scanTime(&org.CreatedAt)); err != nil {
		if isUniqueViolation(err) {
			return nil, ErrConflict
		}
		return nil, fmt.Errorf("create onboarding organization: %w", err)
	}
	if err := seedDefaultGuardrailsSQLite(ctx, tx, org.ID); err != nil {
		return nil, err
	}

	var admin model.Account
	if err := tx.QueryRowContext(ctx,
		`INSERT INTO accounts (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)
		 RETURNING id, email, display_name, password_hash, created_at`,
		newUUID(), input.AdminEmail, input.AdminDisplayName, input.AdminPasswordHash,
	).Scan(&admin.ID, &admin.Email, &admin.DisplayName, &admin.PasswordHash, scanTime(&admin.CreatedAt)); err != nil {
		if isUniqueViolation(err) {
			return nil, ErrConflict
		}
		return nil, fmt.Errorf("create onboarding account: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO org_members (id, org_id, account_id, role) VALUES (?, ?, ?, 'admin')`, newUUID(), org.ID, admin.ID,
	); err != nil {
		return nil, fmt.Errorf("add onboarding org member: %w", err)
	}

	teamID, err := sqliteEnsureDefaultTeamTx(ctx, tx, org.ID)
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO team_members (team_id, account_id) VALUES (?, ?)`, teamID, admin.ID,
	); err != nil {
		return nil, fmt.Errorf("add onboarding team member: %w", err)
	}

	var apiKey model.APIKey
	if err := tx.QueryRowContext(ctx,
		`INSERT INTO api_keys (id, org_id, account_id, name, key_prefix, key_hash, scope)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 RETURNING id, org_id, account_id, name, key_prefix, key_hash, scope, expires_at, created_at, revoked_at`,
		newUUID(), org.ID, admin.ID, input.APIKeyName, input.APIKeyPrefix, input.APIKeyHash, input.APIKeyScope,
	).Scan(&apiKey.ID, &apiKey.OrgID, &apiKey.AccountID, &apiKey.Name, &apiKey.KeyPrefix, &apiKey.KeyHash, &apiKey.Scope, scanNullTime(&apiKey.ExpiresAt), scanTime(&apiKey.CreatedAt), scanNullTime(&apiKey.RevokedAt)); err != nil {
		if isUniqueViolation(err) {
			return nil, ErrConflict
		}
		return nil, fmt.Errorf("create onboarding api key: %w", err)
	}

	projects := make([]*model.Project, 0, len(input.Projects))
	for _, inputProject := range input.Projects {
		var project model.Project
		err := scanProjectRow(tx.QueryRowContext(ctx,
			`INSERT INTO projects (id, org_id, name, slug, remote_key, remote_key_v1, repo_url, created_by, category)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING `+projectColumns,
			newUUID(), org.ID, inputProject.Name, inputProject.Slug, inputProject.RemoteKey, nullIfEmpty(inputProject.RemoteKeyV1), nullIfEmpty(inputProject.RepoURL), admin.ID, model.NormalizeCategory(inputProject.Category),
		), &project)
		if err != nil {
			if isUniqueViolation(err) {
				return nil, ErrConflict
			}
			return nil, fmt.Errorf("create onboarding project: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO team_project_access (team_id, project_id, role) VALUES (?, ?, 'writer')`, teamID, project.ID,
		); err != nil {
			return nil, fmt.Errorf("grant onboarding project access: %w", err)
		}
		projects = append(projects, &project)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit onboarding bootstrap: %w", err)
	}
	return &OnboardingBootstrapResult{Organization: &org, Admin: &admin, APIKey: &apiKey, Projects: projects}, nil
}
