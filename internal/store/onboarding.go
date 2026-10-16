package store

import (
	"context"
	"fmt"

	"github.com/jholhewres/anchored_oss/internal/model"
)

// onboardingBootstrapLockKey serializes the first-run check in PostgreSQL so
// two simultaneous onboarding calls cannot both observe an empty database.
const onboardingBootstrapLockKey int64 = 0x416E63684F6E6264 // "AnchOnbd"

// OnboardingProject is a validated initial project for an organization
// bootstrap. Its remote keys are derived by the handler before persistence.
type OnboardingProject struct {
	Name        string
	Slug        string
	Category    string
	RepoURL     string
	RemoteKey   string
	RemoteKeyV1 string
}

// OnboardingBootstrapInput contains every persisted value for first-run
// onboarding. Secrets are already hashed by the handler.
type OnboardingBootstrapInput struct {
	OrgName           string
	OrgSlug           string
	AdminEmail        string
	AdminDisplayName  string
	AdminPasswordHash string
	APIKeyName        string
	APIKeyPrefix      string
	APIKeyHash        string
	APIKeyScope       string
	Projects          []OnboardingProject
}

type OnboardingBootstrapResult struct {
	Organization *model.Organization
	Admin        *model.Account
	APIKey       *model.APIKey
	Projects     []*model.Project
}

// BootstrapOnboarding atomically creates a first-run organization and its
// owner account, access grants, API key, and initial remote-backed projects.
func (s *PostgresStore) BootstrapOnboarding(ctx context.Context, input OnboardingBootstrapInput) (*OnboardingBootstrapResult, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin onboarding bootstrap: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, onboardingBootstrapLockKey); err != nil {
		return nil, fmt.Errorf("lock onboarding bootstrap: %w", err)
	}

	var organizationCount int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM organizations`).Scan(&organizationCount); err != nil {
		return nil, fmt.Errorf("count organizations for onboarding: %w", err)
	}
	if organizationCount > 0 {
		return nil, ErrConflict
	}

	var org model.Organization
	if err := tx.QueryRowContext(ctx,
		`INSERT INTO organizations (name, slug) VALUES ($1, $2) RETURNING id, name, slug, created_at`,
		input.OrgName, input.OrgSlug,
	).Scan(&org.ID, &org.Name, &org.Slug, &org.CreatedAt); err != nil {
		if isUniqueViolation(err) {
			return nil, ErrConflict
		}
		return nil, fmt.Errorf("create onboarding organization: %w", err)
	}
	if err := seedDefaultGuardrailsPG(ctx, tx, org.ID); err != nil {
		return nil, err
	}

	var admin model.Account
	if err := tx.QueryRowContext(ctx,
		`INSERT INTO accounts (email, display_name, password_hash) VALUES ($1, $2, $3)
		 RETURNING id, email, display_name, password_hash, created_at`,
		input.AdminEmail, input.AdminDisplayName, input.AdminPasswordHash,
	).Scan(&admin.ID, &admin.Email, &admin.DisplayName, &admin.PasswordHash, &admin.CreatedAt); err != nil {
		if isUniqueViolation(err) {
			return nil, ErrConflict
		}
		return nil, fmt.Errorf("create onboarding account: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO org_members (org_id, account_id, role) VALUES ($1, $2, 'admin')`,
		org.ID, admin.ID,
	); err != nil {
		return nil, fmt.Errorf("add onboarding org member: %w", err)
	}

	teamID, err := ensureDefaultTeamTx(ctx, tx, org.ID)
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO team_members (team_id, account_id) VALUES ($1, $2)`, teamID, admin.ID,
	); err != nil {
		return nil, fmt.Errorf("add onboarding team member: %w", err)
	}

	var apiKey model.APIKey
	if err := tx.QueryRowContext(ctx,
		`INSERT INTO api_keys (org_id, account_id, name, key_prefix, key_hash, scope)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, org_id, account_id, name, key_prefix, key_hash, scope, expires_at, created_at, revoked_at`,
		org.ID, admin.ID, input.APIKeyName, input.APIKeyPrefix, input.APIKeyHash, input.APIKeyScope,
	).Scan(&apiKey.ID, &apiKey.OrgID, &apiKey.AccountID, &apiKey.Name, &apiKey.KeyPrefix, &apiKey.KeyHash, &apiKey.Scope, &apiKey.ExpiresAt, &apiKey.CreatedAt, &apiKey.RevokedAt); err != nil {
		if isUniqueViolation(err) {
			return nil, ErrConflict
		}
		return nil, fmt.Errorf("create onboarding api key: %w", err)
	}

	projects := make([]*model.Project, 0, len(input.Projects))
	for _, inputProject := range input.Projects {
		var project model.Project
		err := scanPGProjectRow(tx.QueryRowContext(ctx,
			`INSERT INTO projects (org_id, name, slug, remote_key, remote_key_v1, repo_url, created_by, category)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING `+pgProjectColumns,
			org.ID, inputProject.Name, inputProject.Slug, inputProject.RemoteKey, nullIfEmpty(inputProject.RemoteKeyV1), nullIfEmpty(inputProject.RepoURL), admin.ID, model.NormalizeCategory(inputProject.Category),
		), &project)
		if err != nil {
			if isUniqueViolation(err) {
				return nil, ErrConflict
			}
			return nil, fmt.Errorf("create onboarding project: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO team_project_access (team_id, project_id, role) VALUES ($1, $2, 'writer')`, teamID, project.ID,
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
