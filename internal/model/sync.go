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
	Pulls            []Memory     `json:"pulls,omitempty"`
	ServerTombstones []string     `json:"server_tombstones,omitempty"`
	Results          []SyncResult `json:"results"`
	Watermark        time.Time    `json:"watermark"`
}

// SyncResult describes the outcome for a single pushed or tombstoned item.
type SyncResult struct {
	ID     string `json:"id"`
	Status string `json:"status"` // "accepted" or "rejected"
	Rule   string `json:"rule,omitempty"`
	Detail string `json:"detail,omitempty"`
}
