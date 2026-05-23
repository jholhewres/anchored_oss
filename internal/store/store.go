package store

import (
	"context"
	"time"

	"github.com/jholhewres/anchored_oss/internal/model"
)

type Store interface {
	Ping(ctx context.Context) error
	Close() error

	CreateAccount(ctx context.Context, email, displayName string) (*model.Account, error)
	GetAccountByID(ctx context.Context, id string) (*model.Account, error)
	GetAccountByEmail(ctx context.Context, email string) (*model.Account, error)

	CreateOrganization(ctx context.Context, name, slug string) (*model.Organization, error)
	GetOrganizationBySlug(ctx context.Context, slug string) (*model.Organization, error)
	ListOrganizationsByAccount(ctx context.Context, accountID string) ([]*model.Organization, error)

	CreateTeam(ctx context.Context, orgID, name, slug string) (*model.Team, error)
	ListTeamsByOrg(ctx context.Context, orgID string) ([]*model.Team, error)
	AddOrgMember(ctx context.Context, orgID, accountID, role string) error
	AddTeamMember(ctx context.Context, teamID, accountID string) error
	RemoveTeamMember(ctx context.Context, teamID, accountID string) error

	CreateProject(ctx context.Context, orgID, name, slug, remoteKey, createdBy string) (*model.Project, error)
	GetProjectByID(ctx context.Context, id string) (*model.Project, error)
	GetProjectByRemoteKey(ctx context.Context, orgID, remoteKey string) (*model.Project, error)
	ListProjectsByTeamAccess(ctx context.Context, accountID string) ([]*model.Project, error)
	GrantTeamAccess(ctx context.Context, teamID, projectID, role string) error

	UpsertMemory(ctx context.Context, m *model.Memory) error
	GetMemoriesByProject(ctx context.Context, projectID string, limit, offset int) ([]*model.Memory, error)
	GetMemoriesUpdatedSince(ctx context.Context, projectID string, since time.Time) ([]*model.Memory, error)
	SoftDeleteMemory(ctx context.Context, id, projectID string) error
	GetTombstonesSince(ctx context.Context, projectID string, since time.Time) ([]string, error)

	CreateAPIKey(ctx context.Context, orgID, accountID, name, keyPrefix, keyHash, scope string, expiresAt *time.Time) (*model.APIKey, error)
	GetAPIKeyByHash(ctx context.Context, keyHash string) (*model.APIKey, error)
	RevokeAPIKey(ctx context.Context, id string) error
	ListAPIKeys(ctx context.Context, accountID string) ([]*model.APIKey, error)

	AppendAudit(ctx context.Context, entry *model.AuditEntry) error
	QueryAuditLog(ctx context.Context, orgID string, filters model.AuditFilters) ([]*model.AuditEntry, error)
}
