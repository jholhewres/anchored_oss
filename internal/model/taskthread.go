package model

import "time"

// AccountTaskThread is one card of the personal kanban: a ticket-keyed,
// cross-project unit of work synced from the owner's client. Threads are
// PRIVATE to the owning account — there is deliberately no org-wide listing
// and no admin override; the dashboard only ever shows the caller's own.
type AccountTaskThread struct {
	AccountID   string         `json:"-"`
	TaskKey     string         `json:"task_key"`
	ExternalRef string         `json:"external_ref,omitempty"`
	Status      string         `json:"status"`
	Projects    []string       `json:"projects,omitempty"`
	Journal     []string       `json:"journal,omitempty"`
	Details     map[string]any `json:"details,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}
