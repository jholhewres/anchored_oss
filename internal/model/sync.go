package model

import "time"

// SyncRequest is the client-to-server sync payload.
type SyncRequest struct {
	ProjectID    string        `json:"project_id"`
	ClientID     string        `json:"client_id"`
	Watermark    *time.Time    `json:"watermark,omitempty"`
	Pushes       []SyncMemory  `json:"pushes,omitempty"`
	Tombstones   []string      `json:"tombstones,omitempty"`
	ProjectClaim *ProjectClaim `json:"project_claim,omitempty"`
	// ClientCapabilities is sent by capability-aware clients to negotiate
	// optional protocol features. Its presence (non-nil) is the signal that
	// the client understands the Policy hints in the response, so the server
	// only emits Policy when this is set — capability-less clients see a
	// byte-identical response to the pre-negotiation protocol.
	ClientCapabilities *ClientCapabilities `json:"client_capabilities,omitempty"`
}

// ClientCapabilities advertises optional protocol features the client supports.
// All fields default false; unknown future fields are ignored by older servers.
type ClientCapabilities struct {
	PromotionQueue    bool `json:"promotion_queue,omitempty"`
	TeamCache         bool `json:"team_cache,omitempty"`
	ArtifactSummaries bool `json:"artifact_summaries,omitempty"`
}

// SyncMemory represents a single memory item pushed by the client.
type SyncMemory struct {
	ID          string    `json:"id"`
	Category    string    `json:"category"`
	Content     string    `json:"content"`
	ContentHash string    `json:"content_hash"`
	Keywords    []string  `json:"keywords,omitempty"`
	Source      string    `json:"source,omitempty"`
	AuthorName  string    `json:"author_name"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Metadata    any       `json:"metadata,omitempty"`
}

// ProjectClaim is used to resolve a project by remote key instead of ID.
type ProjectClaim struct {
	Name      string `json:"name"`
	RemoteKey string `json:"remote_key"`
	GitHost   string `json:"git_host,omitempty"`
	RepoSlug  string `json:"repo_slug,omitempty"`
}

// SyncResponse is the server-to-client sync result.
type SyncResponse struct {
	// ProjectID is the resolved remote project the batch landed in. Clients
	// that route by project_claim (git-origin remote_key) need it for
	// follow-up per-project calls such as knowledge-graph triple ingest.
	ProjectID        string       `json:"project_id,omitempty"`
	Pulls            []Memory     `json:"pulls,omitempty"`
	ServerTombstones []string     `json:"server_tombstones,omitempty"`
	Results          []SyncResult `json:"results"`
	Watermark        time.Time    `json:"watermark"`
	// Policy carries the server's effective sync policy so a capability-aware
	// client can warn before it pushes (blocked categories, quality bar, batch
	// cap). Only set when the request advertised ClientCapabilities — nil for
	// older clients to keep their response byte-identical.
	Policy *PolicyHints `json:"policy,omitempty"`
}

// PolicyHints is the advisory view of the server's effective sync policy.
type PolicyHints struct {
	QualityThreshold   float64  `json:"quality_threshold"`
	BlockedCategories  []string `json:"blocked_categories"`
	MaxMemoriesPerSync int      `json:"max_memories_per_sync"`
}

// SyncResult describes the outcome for a single pushed or tombstoned item.
type SyncResult struct {
	ID     string `json:"id"`
	Status string `json:"status"` // "accepted" or "rejected"
	Rule   string `json:"rule,omitempty"`
	Detail string `json:"detail,omitempty"`
}
