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
	UpdateAccount(ctx context.Context, id, displayName, role string) error
	SoftDeleteAccount(ctx context.Context, id string) error
	ListAccountProjects(ctx context.Context, accountID string) ([]*model.Project, error)
	SetAccountProjects(ctx context.Context, orgID, accountID string, projectIDs []string) error
	CreateOrganization(ctx context.Context, name, slug string) (*model.Organization, error)
	GetOrganizationByID(ctx context.Context, id string) (*model.Organization, error)
	AddOrgMember(ctx context.Context, orgID, accountID, role string) error
	CountOrganizations(ctx context.Context) (int, error)

	// Teams.
	CreateTeam(ctx context.Context, orgID, name, slug string) (*model.Team, error)
	ListTeamsByOrg(ctx context.Context, orgID string) ([]*model.Team, error)
	GetTeamDetail(ctx context.Context, teamID string) (*model.TeamDetail, error)
	AddTeamMember(ctx context.Context, teamID, accountID string) error
	RemoveTeamMember(ctx context.Context, teamID, accountID string) error

	// Projects + team access.
	CreateProject(ctx context.Context, orgID, name, slug, remoteKey, remoteKeyV1, repoURL, createdBy, category string) (*model.Project, error)
	GetProjectByID(ctx context.Context, id string) (*model.Project, error)
	GetActiveProjectByID(ctx context.Context, id string) (*model.Project, error)
	// GetProjectByRemoteKey matches the key against EITHER the canonical
	// remote_key or the legacy remote_key_v1, so repos keyed before the v2
	// normalization still resolve to their project.
	GetProjectByRemoteKey(ctx context.Context, orgID, remoteKey string) (*model.Project, error)
	// UpdateProject applies a partial update within an org, recomputing both
	// remote keys from RepoURL when it is set. Returns ErrNotFound when the
	// project is absent or belongs to another org.
	UpdateProject(ctx context.Context, orgID, id string, upd model.ProjectUpdate) (*model.Project, error)
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
	// SearchMemoriesByVector ranks project memories by cosine similarity to the
	// query vector (semantic search). Postgres uses pgvector; SQLite brute-forces.
	SearchMemoriesByVector(ctx context.Context, projectID string, vec []float32, k int) ([]*model.Memory, error)
	// UpdateMemoryEmbedding stores (or replaces) a memory's vector and model.
	UpdateMemoryEmbedding(ctx context.Context, memoryID string, vec []float32, model string) error
	// MemoriesMissingEmbedding pages non-deleted memories lacking a vector
	// (id > afterID, ordered by id) for the reindex/backfill command.
	MemoriesMissingEmbedding(ctx context.Context, afterID string, limit int) ([]*model.Memory, error)
	// MemoriesStaleEmbedding pages non-deleted memories whose embedding is
	// missing OR was produced by a different model than `model` (id > afterID,
	// ordered by id). Used by reindex to re-embed an existing corpus after an
	// embeddings provider/model change so the whole vector space stays
	// consistent. Only id and content are populated.
	MemoriesStaleEmbedding(ctx context.Context, model, afterID string, limit int) ([]*model.Memory, error)
	UpsertMemory(ctx context.Context, m *model.Memory) error
	// UpsertMemories upserts a batch of memories in a single statement.
	// Chunks larger than ~5000 should be split by the caller to stay
	// under Postgres' 65535-parameter limit.
	UpsertMemories(ctx context.Context, ms []*model.Memory) error
	GetMemoriesUpdatedSince(ctx context.Context, projectID string, since time.Time) ([]*model.Memory, error)
	ListMemoriesPaginated(ctx context.Context, projectID string, limit, offset int, category string) (memories []*model.Memory, total int, err error)
	SoftDeleteMemory(ctx context.Context, id, projectID string) error
	// SoftDeleteMemoriesByWindow tombstones every live memory of a project
	// created inside [since, until) — admin moderation for undoing a sync
	// batch that landed in the wrong project. Nil bounds are open-ended.
	SoftDeleteMemoriesByWindow(ctx context.Context, projectID string, since, until *time.Time) (int64, error)
	GetTombstonesSince(ctx context.Context, projectID string, since time.Time) ([]string, error)
	GetMemoryByID(ctx context.Context, id string) (*model.Memory, error)
	UpdateMemoryMetadata(ctx context.Context, id string, metadata any) error

	// CountCanonicalMembers counts live memories whose metadata canonical_of
	// equals canonicalID — the size of a near-duplicate cluster minus its
	// canonical. Used for advisory consolidation-candidate marking.
	CountCanonicalMembers(ctx context.Context, projectID, canonicalID string) (int, error)
	ListProjectMemoriesSince(ctx context.Context, projectID string, since time.Time) ([]*model.Memory, error)
	// ListMemoriesByCurationStatus pages live memories whose metadata
	// curation_status matches status (e.g. "stale", "contradiction_candidate").
	ListMemoriesByCurationStatus(ctx context.Context, projectID, status string, limit int) ([]*model.Memory, error)

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
	// PurgeAuditOlderThan deletes audit entries created before the cutoff,
	// returning the number removed. Used by the retention sweep.
	PurgeAuditOlderThan(ctx context.Context, before time.Time) (int64, error)

	// Sync rejection stats (memory health). IncrementRejectionStat upserts the
	// per-day counter for (org, project, rule); callers treat failures as
	// best-effort (log only). ListRejectionStats returns counters since the
	// given UTC day (inclusive, "YYYY-MM-DD"); empty projectID means org-wide.
	IncrementRejectionStat(ctx context.Context, orgID, projectID, rule string, delta int64) error
	ListRejectionStats(ctx context.Context, orgID, projectID, sinceDay string) ([]*model.RejectionStat, error)
	// PurgeRejectionStatsOlderThan deletes counters for days before the cutoff
	// day ("YYYY-MM-DD"), returning the number removed.
	PurgeRejectionStatsOlderThan(ctx context.Context, beforeDay string) (int64, error)

	// Memory health (anti context-poisoning view): lifecycle counts, noisy
	// sources, age spread, rejection pressure and volume anomalies.
	GetProjectMemoryHealth(ctx context.Context, projectID string) (*model.MemoryHealth, error)
	GetOrgMemoryHealth(ctx context.Context, orgID string) (*model.MemoryHealth, error)

	// Guardrail policy (per-org overrides).
	GetOrgPolicy(ctx context.Context, orgID string) (*model.OrgPolicy, error)
	UpsertOrgPolicy(ctx context.Context, p *model.OrgPolicy) error

	// Guardrail manager (per-org list of configurable sync-time rules).
	ListGuardrails(ctx context.Context, orgID string) ([]*model.Guardrail, error)
	GetGuardrail(ctx context.Context, orgID, id string) (*model.Guardrail, error)
	CreateGuardrail(ctx context.Context, g *model.Guardrail) error
	UpdateGuardrail(ctx context.Context, g *model.Guardrail) error
	DeleteGuardrail(ctx context.Context, orgID, id string) error

	// Dashboard.
	GetDashboardStats(ctx context.Context, orgID string) (*model.DashboardStats, error)

	// Knowledge graph.
	UpsertTriple(ctx context.Context, t *model.Triple) error
	ListTriplesByProject(ctx context.Context, projectID string, limit, offset int) ([]*model.Triple, int, error)
	CountTriplesByProject(ctx context.Context, projectID string) (int, error)

	// Quota.
	// GetOrgStorageBytes returns the total bytes used by non-deleted memories
	// in all active projects under the given org.
	GetOrgStorageBytes(ctx context.Context, orgID string) (int64, error)

	// Invites.
	CreateInvite(ctx context.Context, orgID, email, displayName, role, tokenHash string, expiresAt time.Time, createdBy string) (*model.Invite, error)
	GetInviteByTokenHash(ctx context.Context, tokenHash string) (*model.Invite, error)
	ListInvitesByOrg(ctx context.Context, orgID string) ([]*model.Invite, error)
	DeleteInvite(ctx context.Context, id string) error
	MarkInviteAccepted(ctx context.Context, id string) error

	// Curation queue.
	EnqueueCuration(ctx context.Context, memoryIDs []string) error
	ClaimCurationBatch(ctx context.Context, batchSize int) ([]string, error)
	SetCurationDone(ctx context.Context, memoryID string) error
	SetCurationFailed(ctx context.Context, memoryID, errMsg string) error
	// EnqueueRecuration (re)queues up to limit live memories whose
	// curation_version is below the current version (or unset), so curation v2
	// marks roll out to memories curated by an older worker. Returns the number
	// of memories enqueued. Resets matching rows back to 'pending'; the worker
	// only calls it when the live queue is drained (nothing 'processing'), so a
	// single worker never races a reset against an in-flight claim.
	EnqueueRecuration(ctx context.Context, limit int) (int, error)
}
