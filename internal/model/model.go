package model

import "time"

type Account struct {
	ID          string    `json:"id"`
	Email       string    `json:"email"`
	DisplayName string    `json:"display_name"`
	CreatedAt   time.Time `json:"created_at"`
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
	ID         string    `json:"id"`
	OrgID      string    `json:"org_id"`
	Name       string    `json:"name"`
	Slug       string    `json:"slug"`
	RemoteKey  string    `json:"remote_key"`
	CreatedBy  string    `json:"created_by"`
	CreatedAt  time.Time `json:"created_at"`
}

type Memory struct {
	ID          string    `json:"id"`
	ProjectID   string    `json:"project_id"`
	Category    string    `json:"category"`
	Content     string    `json:"content"`
	ContentHash string    `json:"content_hash"`
	Keywords    []string  `json:"keywords"`
	Source      string    `json:"source"`
	AuthorID    string    `json:"author_id"`
	AuthorName  string    `json:"author_name"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	DeletedAt   *time.Time `json:"deleted_at,omitempty"`
	Metadata    any       `json:"metadata,omitempty"`
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
	ProjectID  string    `json:"project_id,omitempty"`
	ActorID    string    `json:"actor_id,omitempty"`
	Action     string    `json:"action,omitempty"`
	TargetType string    `json:"target_type,omitempty"`
	From       *time.Time `json:"from,omitempty"`
	To         *time.Time `json:"to,omitempty"`
	Limit      int       `json:"limit,omitempty"`
	Offset     int       `json:"offset,omitempty"`
}
