package store

import (
	"context"
	"time"

	"github.com/jholhewres/anchored_oss/internal/model"
)

// Store is the persistence surface required by the server. It intentionally
// exposes only the operations actually called from handlers, middleware,
// the sync engine, and the bootstrap command. Add a method here only when
// a caller needs it.
type Store interface {
	Ping(ctx context.Context) error
	Close() error

	// Accounts + orgs (bootstrap + author lookup).
	CreateAccount(ctx context.Context, email, displayName, passwordHash string) (*model.Account, error)
	GetAccountByID(ctx context.Context, id string) (*model.Account, error)
	GetAccountByEmail(ctx context.Context, email string) (*model.Account, error)
	GetAccountOrgID(ctx context.Context, accountID string) (string, error)
	SetAccountPassword(ctx context.Context, accountID, passwordHash string) error
	GetOrCreateAccountByEmail(ctx context.Context, orgID, email, displayName string) (account *model.Account, created bool, err error)
	ListAccountsByOrg(ctx context.Context, orgID string) ([]*model.AccountWithRole, error)
	CreateOrganization(ctx context.Context, name, slug string) (*model.Organization, error)
	AddOrgMember(ctx context.Context, orgID, accountID, role string) error

	// Teams.
	CreateTeam(ctx context.Context, orgID, name, slug string) (*model.Team, error)
	ListTeamsByOrg(ctx context.Context, orgID string) ([]*model.Team, error)
	GetTeamDetail(ctx context.Context, teamID string) (*model.TeamDetail, error)
	AddTeamMember(ctx context.Context, teamID, accountID string) error
	RemoveTeamMember(ctx context.Context, teamID, accountID string) error

	// Projects + team access.
	CreateProject(ctx context.Context, orgID, name, slug, remoteKey, createdBy string) (*model.Project, error)
	GetProjectByID(ctx context.Context, id string) (*model.Project, error)
	GetActiveProjectByID(ctx context.Context, id string) (*model.Project, error)
	GetProjectByRemoteKey(ctx context.Context, orgID, remoteKey string) (*model.Project, error)
	ListProjectsByTeamAccess(ctx context.Context, accountID string) ([]*model.Project, error)
	HasProjectAccess(ctx context.Context, accountID, projectID string) (bool, error)
	SoftDeleteProject(ctx context.Context, id string) error
	// EnsureCreatorProjectAccess wires accountID into the org's default
	// team and grants that team write access to projectID. Idempotent.
	EnsureCreatorProjectAccess(ctx context.Context, orgID, accountID, projectID string) error
	// EnsureDefaultTeamMembership ensures accountID is a member of the
	// org's default team (creating the team if needed). Idempotent.
	EnsureDefaultTeamMembership(ctx context.Context, orgID, accountID string) error

	// Memories.
	SearchMemories(ctx context.Context, projectID string, query string, limit int) ([]*model.Memory, error)
	UpsertMemory(ctx context.Context, m *model.Memory) error
	// UpsertMemories upserts a batch of memories in a single statement.
	// Chunks larger than ~5000 should be split by the caller to stay
	// under Postgres' 65535-parameter limit.
	UpsertMemories(ctx context.Context, ms []*model.Memory) error
	GetMemoriesUpdatedSince(ctx context.Context, projectID string, since time.Time) ([]*model.Memory, error)
	ListMemoriesPaginated(ctx context.Context, projectID string, limit, offset int) (memories []*model.Memory, total int, err error)
	SoftDeleteMemory(ctx context.Context, id, projectID string) error
	GetTombstonesSince(ctx context.Context, projectID string, since time.Time) ([]string, error)

	// API keys.
	CreateAPIKey(ctx context.Context, orgID, accountID, name, keyPrefix, keyHash, scope string, expiresAt *time.Time) (*model.APIKey, error)
	GetAPIKeyByHash(ctx context.Context, keyHash string) (*model.APIKey, error)
	ListAPIKeysByOrg(ctx context.Context, orgID string) ([]*model.APIKey, error)
	RevokeAPIKey(ctx context.Context, id string) error
	RevokeSessionKeys(ctx context.Context, accountID string) error

	// Audit.
	AppendAudit(ctx context.Context, entry *model.AuditEntry) error
	// AppendAudits inserts a batch of audit entries in a single statement.
	AppendAudits(ctx context.Context, entries []*model.AuditEntry) error
	ListAuditEntries(ctx context.Context, orgID string, filters model.AuditFilters) (entries []*model.AuditEntry, total int, err error)

	// Dashboard.
	GetDashboardStats(ctx context.Context, orgID string) (*model.DashboardStats, error)

	// Quota.
	// GetOrgStorageBytes returns the total bytes used by non-deleted memories
	// in all active projects under the given org.
	GetOrgStorageBytes(ctx context.Context, orgID string) (int64, error)
}
