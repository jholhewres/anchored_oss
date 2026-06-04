package model

import "time"

type Account struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	DisplayName  string    `json:"display_name"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
}

type Organization struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	CreatedAt time.Time `json:"created_at"`
}

type Team struct {
	ID        string    `json:"id"`
	OrgID     string    `json:"org_id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	CreatedAt time.Time `json:"created_at"`
}

type Project struct {
	ID          string     `json:"id"`
	OrgID       string     `json:"org_id"`
	Name        string     `json:"name"`
	Slug        string     `json:"slug"`
	Category    string     `json:"category"`
	RemoteKey   string     `json:"remote_key"`
	RemoteKeyV1 string     `json:"remote_key_v1"`
	RepoURL     string     `json:"repo_url"`
	CreatedBy   string     `json:"created_by"`
	CreatedAt   time.Time  `json:"created_at"`
	DeletedAt   *time.Time `json:"deleted_at,omitempty"`
}

// ProjectUpdate holds the partial fields a PATCH /v1/projects/{id} may change.
// A nil pointer means "leave unchanged"; a non-nil pointer (including an empty
// string) means "set to this value". RepoURL drives remote-key recomputation:
// non-empty derives both keys, empty clears repo_url and both keys.
type ProjectUpdate struct {
	Name     *string `json:"name"`
	Slug     *string `json:"slug"`
	RepoURL  *string `json:"repo_url"`
	Category *string `json:"category"`
}

// ProjectCategories enumerates the canonical category values the dashboard
// groups projects by. Anything else is normalized to "other" by the handler.
var ProjectCategories = []string{"service", "library", "app", "infra", "experiment", "other"}

// NormalizeCategory returns a valid category value, falling back to "other".
func NormalizeCategory(c string) string {
	for _, v := range ProjectCategories {
		if v == c {
			return c
		}
	}
	return "other"
}

type Memory struct {
	ID          string     `json:"id"`
	ProjectID   string     `json:"project_id"`
	Category    string     `json:"category"`
	Content     string     `json:"content"`
	ContentHash string     `json:"content_hash"`
	Keywords    []string   `json:"keywords"`
	Source      string     `json:"source"`
	AuthorID    string     `json:"author_id"`
	AuthorName  string     `json:"author_name"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	DeletedAt   *time.Time `json:"deleted_at,omitempty"`
	Metadata    any        `json:"metadata,omitempty"`
}

type APIKey struct {
	ID        string     `json:"id"`
	OrgID     string     `json:"org_id"`
	AccountID string     `json:"account_id"`
	Name      string     `json:"name"`
	KeyPrefix string     `json:"key_prefix"`
	KeyHash   string     `json:"-"`
	Scope     string     `json:"scope"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
}

type AuditEntry struct {
	ID         string    `json:"id"`
	OrgID      string    `json:"org_id"`
	ProjectID  string    `json:"project_id,omitempty"`
	ActorID    string    `json:"actor_id,omitempty"`
	Action     string    `json:"action"`
	TargetType string    `json:"target_type,omitempty"`
	TargetID   string    `json:"target_id,omitempty"`
	Metadata   any       `json:"metadata,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

type AuditFilters struct {
	ProjectID  string     `json:"project_id,omitempty"`
	ActorID    string     `json:"actor_id,omitempty"`
	Action     string     `json:"action,omitempty"`
	TargetType string     `json:"target_type,omitempty"`
	From       *time.Time `json:"from,omitempty"`
	To         *time.Time `json:"to,omitempty"`
	Limit      int        `json:"limit,omitempty"`
	Offset     int        `json:"offset,omitempty"`
}

type AccountWithRole struct {
	Account
	Role string `json:"role"`
}

type TeamMember struct {
	AccountID   string    `json:"account_id"`
	Email       string    `json:"email"`
	DisplayName string    `json:"display_name"`
	AddedAt     time.Time `json:"added_at"`
}

type ProjectGrant struct {
	ProjectID   string `json:"project_id"`
	ProjectName string `json:"project_name"`
	ProjectSlug string `json:"project_slug"`
	Role        string `json:"role"`
}

type TeamDetail struct {
	Team
	Members       []TeamMember   `json:"members"`
	ProjectGrants []ProjectGrant `json:"project_grants"`
}

type PushActivity struct {
	ProjectID   string    `json:"project_id"`
	ProjectName string    `json:"project_name"`
	Count       int       `json:"count"`
	LastPush    time.Time `json:"last_push"`
}

type Triple struct {
	ID         string    `json:"id"`
	Subject    string    `json:"subject"`
	Predicate  string    `json:"predicate"`
	Object     string    `json:"object"`
	Confidence float64   `json:"confidence"`
	ProjectID  string    `json:"project_id"`
	CreatedAt  time.Time `json:"created_at"`
}

type Invite struct {
	ID          string     `json:"id"`
	OrgID       string     `json:"org_id"`
	Email       string     `json:"email"`
	DisplayName string     `json:"display_name"`
	Role        string     `json:"role"`
	TokenHash   string     `json:"-"`
	ExpiresAt   time.Time  `json:"expires_at"`
	AcceptedAt  *time.Time `json:"accepted_at,omitempty"`
	CreatedBy   string     `json:"created_by,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

type DashboardStats struct {
	Accounts        int            `json:"accounts"`
	Organizations   int            `json:"organizations"`
	Teams           int            `json:"teams"`
	Projects        int            `json:"projects"`
	MemoriesLive    int            `json:"memories_live"`
	KeysActive      int            `json:"keys_active"`
	AuditEntries24h int            `json:"audit_entries_24h"`
	RecentPushes    []PushActivity `json:"recent_pushes"`
}

// OrgPolicy holds an organization's customizable guardrails. Absent fields fall
// back to server defaults. Secret and local-path guardrails are NOT represented
// here — they are always-on and live in code.
type OrgPolicy struct {
	OrgID             string    `json:"org_id"`
	BlockedCategories []string  `json:"blocked_categories"`
	QualityThreshold  float64   `json:"quality_threshold"`
	NearDupThreshold  float64   `json:"near_dup_threshold"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// Guardrail kinds. The three security kinds are seeded as builtins (toggleable
// but not deletable); category/regex/keyword are admin-managed (full CRUD).
const (
	GuardrailSecretDetection   = "secret_detection"
	GuardrailLocalPathRedaction = "local_path_redaction"
	GuardrailUserScopeBlock    = "user_scope_block"
	GuardrailCategory          = "category" // value = category name to block at sync
	GuardrailRegex             = "regex"    // value = RE2 pattern; content match => reject
	GuardrailKeyword           = "keyword"  // value = literal substring (case-insensitive) => reject
)

// Guardrail is one configurable sync-time rule for an org. builtin rows are the
// seeded security rules: they can be disabled (enabled=false) but not deleted.
type Guardrail struct {
	ID          string    `json:"id"`
	OrgID       string    `json:"org_id"`
	Kind        string    `json:"kind"`
	Value       string    `json:"value"`
	Label       string    `json:"label"`
	Description string    `json:"description"`
	Enabled     bool      `json:"enabled"`
	Builtin     bool      `json:"builtin"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
