// Package curation implements the background worker that processes newly
// persisted memories: quality-scores them, detects near-duplicates, and
// updates metadata accordingly.
package curation

import (
	"context"
	"log/slog"
	"strings"
	"time"
	"unicode"

	"github.com/jholhewres/anchored_oss/internal/ai/embeddings"
	"github.com/jholhewres/anchored_oss/internal/config"
	"github.com/jholhewres/anchored_oss/internal/model"
	"github.com/jholhewres/anchored_oss/internal/policy"
	"github.com/jholhewres/anchored_oss/internal/store"
)

// Worker processes the curation_queue on a ticker interval.
type Worker struct {
	cfg      config.CurationConfig
	store    store.Store
	filter   *policy.ContentFilter
	embedder embeddings.Embedder // nil when embeddings are disabled
	logger   *slog.Logger
}

// NewWorker creates a Worker that is ready to Start. embedder is the
// process-shared embedder (injected, may be nil); embedding generation is
// folded into the curation pass (the worker already loads each memory's
// content), so it covers both the /v1/memories and sync push write paths.
func NewWorker(cfg *config.Config, st store.Store, embedder embeddings.Embedder, logger *slog.Logger) *Worker {
	return &Worker{
		cfg:      cfg.Curation,
		store:    st,
		filter:   policy.NewContentFilter(),
		embedder: embedder,
		logger:   logger,
	}
}

// Start runs the processing loop until ctx is cancelled. Intended to be
// called in a dedicated goroutine.
func (w *Worker) Start(ctx context.Context) {
	ticker := time.NewTicker(w.cfg.Interval)
	defer ticker.Stop()

	w.logger.Info("curation worker started",
		"interval", w.cfg.Interval,
		"batch_size", w.cfg.BatchSize,
	)

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("curation worker stopped")
			return
		case <-ticker.C:
			if err := w.processBatch(ctx); err != nil {
				w.logger.Error("curation batch failed", "error", err)
			}
		}
	}
}

// RunOnce processes a single curation batch synchronously. The Start loop
// calls the same path on each tick; RunOnce exposes it for one-shot triggering
// and for tests that need a deterministic pass without spinning the ticker.
func (w *Worker) RunOnce(ctx context.Context) error { return w.processBatch(ctx) }

func (w *Worker) processBatch(ctx context.Context) error {
	ids, err := w.store.ClaimCurationBatch(ctx, w.cfg.BatchSize)
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		// Queue drained: pull a bounded batch of memories that predate curation
		// v2 (or were never curated) so staleness/contradiction marking rolls out
		// incrementally without a migration. They are processed on the next tick.
		if n, err := w.store.EnqueueRecuration(ctx, w.cfg.BatchSize); err != nil {
			w.logger.Warn("curation: enqueue re-curation failed", "error", err)
		} else if n > 0 {
			w.logger.Debug("curation: enqueued memories for v2 re-curation", "count", n)
		}
		return nil
	}

	// The near-duplicate / contradiction window is the same for every item in
	// the batch, so load each project's peer set once and reuse it across the
	// batch instead of re-querying per memory (the old O(batch) load became
	// O(distinct projects)). The set is capped to the newest maxCurationPeers to
	// bound the O(peers) similarity scan on very large projects.
	since := time.Now().UTC().Add(-w.cfg.NearDupWindow)
	peerCache := make(map[string][]*model.Memory)
	getPeers := func(projectID string) []*model.Memory {
		if p, ok := peerCache[projectID]; ok {
			return p
		}
		p, err := w.store.ListProjectMemoriesSince(ctx, projectID, since)
		if err != nil {
			w.logger.Warn("curation: list peers failed", "project_id", projectID, "error", err)
			p = nil
		}
		if len(p) > maxCurationPeers {
			p = p[len(p)-maxCurationPeers:] // ListProjectMemoriesSince is created_at asc; keep newest
		}
		peerCache[projectID] = p
		return p
	}

	for _, memID := range ids {
		if err := w.processOne(ctx, memID, getPeers); err != nil {
			w.logger.Error("curation: process memory failed",
				"memory_id", memID, "error", err)
			if failErr := w.store.SetCurationFailed(ctx, memID, err.Error()); failErr != nil {
				w.logger.Warn("curation: set failed status error",
					"memory_id", memID, "error", failErr)
			}
			continue
		}
		if err := w.store.SetCurationDone(ctx, memID); err != nil {
			w.logger.Warn("curation: set done status error",
				"memory_id", memID, "error", err)
		}
	}
	return nil
}

// maxCurationPeers bounds the per-project peer set the worker scans for
// near-duplicate and contradiction detection, capping the per-item O(peers)
// cost on very large projects. The newest peers are kept.
const maxCurationPeers = 2000

func (w *Worker) processOne(ctx context.Context, memID string, getPeers func(projectID string) []*model.Memory) error {
	mem, err := w.store.GetMemoryByID(ctx, memID)
	if err != nil {
		return err
	}

	// Run through the content filter to determine quality.
	var metaMap map[string]any
	if m, ok := mem.Metadata.(map[string]any); ok {
		metaMap = m
	}
	results := w.filter.Filter([]policy.Filterable{{
		ID:       mem.ID,
		Content:  mem.Content,
		Category: mem.Category,
		Metadata: metaMap,
	}})

	patch := make(map[string]any)
	if len(results) > 0 && !results[0].Accepted {
		patch["curation_status"] = "low_signal"
		patch["curation_rule"] = results[0].Rule
	} else {
		patch["curation_status"] = "ok"
	}

	// Near-duplicate detection within the project over the configured window,
	// using the batch-shared (loaded-once, capped) peer set.
	peers := getPeers(mem.ProjectID)

	normSelf := normalizeForNgram(mem.Content)
	for _, peer := range peers {
		if peer.ID == mem.ID {
			continue
		}
		normPeer := normalizeForNgram(peer.Content)
		sim := fivegramJaccard(normSelf, normPeer)
		if sim >= w.cfg.NearDupThreshold {
			// Mark this memory as near-duplicate of the older one.
			patch["curation_status"] = "near_duplicate"
			patch["canonical_of"] = peer.ID
			break
		}
	}

	// qualityOK captures whether the memory passed quality + dedup, before the
	// curation-v2 advisory marks (stale/contradiction) reclassify the status.
	// Embedding eligibility keys off this so a contradiction candidate (still a
	// genuine, high-signal memory) is not silently dropped from the vector index.
	qualityOK := patch["curation_status"] == "ok"

	// Curation v2 (advisory only; never deletes). Staleness and contradiction
	// only apply to otherwise-clean memories — low-signal and near-duplicate are
	// higher-priority signals that already explain the memory's state.
	if qualityOK {
		if isStale(mem.UpdatedAt, metaMap, time.Duration(w.cfg.StaleAfterDays)*24*time.Hour, time.Now().UTC()) {
			patch["curation_status"] = "stale"
			patch["curation_rule"] = "stale"
		} else if w.cfg.ContradictionDetection {
			if peerID, ok := findContradiction(mem, peers); ok {
				patch["curation_status"] = "contradiction_candidate"
				patch["curation_rule"] = "contradiction"
				patch["contradicts"] = peerID
			}
		}
	}
	patch["curation_version"] = 2
	patch["last_curated_at"] = time.Now().UTC().Format(time.RFC3339)

	if err := w.store.UpdateMemoryMetadata(ctx, memID, patch); err != nil {
		return err
	}

	// Generate the semantic embedding for accepted, non-duplicate memories.
	// Best-effort: an embedding failure must not fail curation (the memory is
	// still searchable by text). Skip low-signal/duplicate rows to avoid
	// indexing noise.
	if w.embedder != nil && qualityOK {
		vec, err := embeddings.EmbedOne(ctx, w.embedder, mem.Content)
		if err != nil {
			w.logger.Warn("curation: embed failed", "memory_id", memID, "error", err)
			return nil
		}
		if err := w.store.UpdateMemoryEmbedding(ctx, memID, vec, w.embedder.Model()); err != nil {
			w.logger.Warn("curation: store embedding failed", "memory_id", memID, "error", err)
		}
	}
	return nil
}

// normalizeForNgram lower-cases and strips non-letter/digit runes so that
// trivial formatting differences don't prevent duplicate detection.
func normalizeForNgram(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsSpace(r) {
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

// fivegramJaccard computes the Jaccard similarity of the 5-gram sets of a and b.
func fivegramJaccard(a, b string) float64 {
	setA := ngrams(a, 5)
	setB := ngrams(b, 5)
	if len(setA) == 0 && len(setB) == 0 {
		return 1.0
	}
	if len(setA) == 0 || len(setB) == 0 {
		return 0.0
	}

	intersection := 0
	for g := range setA {
		if setB[g] {
			intersection++
		}
	}
	union := len(setA) + len(setB) - intersection
	if union == 0 {
		return 0.0
	}
	return float64(intersection) / float64(union)
}

// ngrams returns the set of n-character substrings of s.
func ngrams(s string, n int) map[string]bool {
	runes := []rune(s)
	if len(runes) < n {
		if len(runes) == 0 {
			return map[string]bool{}
		}
		return map[string]bool{string(runes): true}
	}
	set := make(map[string]bool, len(runes)-n+1)
	for i := 0; i <= len(runes)-n; i++ {
		set[string(runes[i:i+n])] = true
	}
	return set
}
