package model

// MemoryHealth is the anti context-poisoning view of a project's (or org's)
// memory corpus: lifecycle counts, noisiest sources/categories, age spread,
// guardrail rejection pressure, volume anomalies and suggested actions.
type MemoryHealth struct {
	Score           float64         `json:"score"`
	Counts          HealthCounts    `json:"counts"`
	BySource        []NameCount     `json:"by_source"`
	ByCategory      []NameCount     `json:"by_category"`
	AgeHistogram    []NameCount     `json:"age_histogram"`
	SyncRejections  []RuleCount     `json:"sync_rejections"`
	Anomalies       []HealthAnomaly `json:"anomalies"`
	Recommendations []string        `json:"recommendations"`
}

// HealthCounts groups the lifecycle counters over live (non-deleted) memories.
type HealthCounts struct {
	Live              int64 `json:"live"`
	LowSignal         int64 `json:"low_signal"`
	NearDuplicate     int64 `json:"near_duplicate"`
	Stale             int64 `json:"stale"`
	Contradictions    int64 `json:"contradictions"`
	// ConsolidationCandidates counts near-duplicate cluster canonicals the
	// curation worker flagged for synthesis (advisory; additive field).
	ConsolidationCandidates int64 `json:"consolidation_candidates,omitempty"`
	MissingEmbeddings int64 `json:"missing_embeddings"`
}

// NameCount is a generic (label, count) pair for grouped breakdowns.
type NameCount struct {
	Name  string `json:"name"`
	Count int64  `json:"count"`
}

// RuleCount aggregates sync rejections by guardrail rule.
type RuleCount struct {
	Rule  string `json:"rule"`
	Count int64  `json:"count"`
}

// HealthAnomaly flags an abnormal ingestion pattern, e.g. a source pushing an
// order of magnitude above its 30-day baseline within 24h.
type HealthAnomaly struct {
	Type     string  `json:"type"` // "volume_spike"
	Source   string  `json:"source"`
	Window   string  `json:"window"` // "24h"
	Count    int64   `json:"count"`
	Baseline float64 `json:"baseline"` // average items/day over the prior 29 days
}
