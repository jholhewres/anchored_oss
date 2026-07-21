// Package curation implements the background worker that processes newly
// persisted memories: quality-scores them, detects near-duplicates, and
// updates metadata accordingly.
package curation

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"runtime/debug"
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
	leased   store.LeasedCurationStore // non-nil when the backend supports lease/owner claiming
	owner    string                    // this process's claim identity; empty when leasing is unavailable
	filter   *policy.ContentFilter
	embedder embeddings.Embedder // nil when embeddings are disabled
	logger   *slog.Logger
}

// NewWorker creates a Worker that is ready to Start. embedder is the
// process-shared embedder (injected, may be nil); embedding generation is
// folded into the curation pass (the worker already loads each memory's
// content), so it covers both the /v1/memories and sync push write paths.
func NewWorker(cfg *config.Config, st store.Store, embedder embeddings.Embedder, logger *slog.Logger) *Worker {
	w := &Worker{
		cfg:      cfg.Curation,
		store:    st,
		filter:   policy.NewContentFilter(),
		embedder: embedder,
		logger:   logger,
	}
	// Prefer the leased claim path when the backend supports it so a crashed
	// worker's in-flight batch is reclaimed instead of stranding in processing.
	if leased, ok := st.(store.LeasedCurationStore); ok {
		w.leased = leased
		w.owner = newOwnerID()
	}
	return w
}

// newOwnerID returns a stable-per-process claim identity (host/pid/random) so
// completions can be scoped to the owner that still holds the lease.
func newOwnerID() string {
	host, _ := os.Hostname()
	var b [6]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("%s/%d/%s", host, os.Getpid(), hex.EncodeToString(b[:]))
}

// leaseTTL is the configured claim lease, falling back to a safe default when
// unset so an in-flight batch always has time to finish before reclaim.
func (w *Worker) leaseTTL() time.Duration {
	if w.cfg.LeaseTTL > 0 {
		return w.cfg.LeaseTTL
	}
	return 5 * time.Minute
}

// claimBatch claims a batch via the leased path when available, else the
// unleased base path (older backends / mocks).
func (w *Worker) claimBatch(ctx context.Context) ([]string, error) {
	if w.leased != nil {
		return w.leased.ClaimCurationBatchLeased(ctx, w.cfg.BatchSize, w.owner, w.leaseTTL())
	}
	return w.store.ClaimCurationBatch(ctx, w.cfg.BatchSize)
}

// markDone completes a memory, owner-scoped when leasing is active.
func (w *Worker) markDone(ctx context.Context, memID string) error {
	if w.leased != nil {
		return w.leased.SetCurationDoneLeased(ctx, memID, w.owner)
	}
	return w.store.SetCurationDone(ctx, memID)
}

// reclaimExpired returns any lease-expired rows to pending. No-op when the
// backend does not support leasing.
func (w *Worker) reclaimExpired(ctx context.Context) {
	if w.leased == nil {
		return
	}
	n, err := w.leased.ReclaimExpiredCuration(ctx)
	if err != nil {
		w.logger.Warn("curation: reclaim expired leases failed", "error", err)
		return
	}
	if n > 0 {
		w.logger.Info("curation: reclaimed expired leases", "count", n)
	}
}

// Start runs the processing loop until ctx is cancelled. Intended to be
// called in a dedicated goroutine.
func (w *Worker) Start(ctx context.Context) {
	ticker := time.NewTicker(w.cfg.Interval)
	defer ticker.Stop()

	// Reclaim expired leases on a cadence well under the lease TTL so a crashed
	// worker's batch is picked up within roughly the TTL window.
	reclaimEvery := w.leaseTTL() / 2
	if reclaimEvery < 30*time.Second {
		reclaimEvery = 30 * time.Second
	}
	reclaimTicker := time.NewTicker(reclaimEvery)
	defer reclaimTicker.Stop()

	w.logger.Info("curation worker started",
		"interval", w.cfg.Interval,
		"batch_size", w.cfg.BatchSize,
		"lease_ttl", w.leaseTTL(),
		"leased", w.leased != nil,
	)

	// Startup recovery: return any already-expired leases to pending before the
	// first tick. (The base migration also clears the pre-lease backlog once.)
	w.reclaimExpired(ctx)

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("curation worker stopped")
			return
		case <-ticker.C:
			w.runBatchSafely(ctx)
		case <-reclaimTicker.C:
			w.reclaimExpired(ctx)
		}
	}
}

// runBatchSafely runs a single curation batch with panic isolation. A panic
// anywhere in the batch path is logged and the worker continues on the next
// tick; it never propagates to crash the HTTP server. Per-memory panics are
// already contained inside processBatch via processOneSafe; this outer guard
// covers anything outside that path (e.g. ClaimCurationBatch).
func (w *Worker) runBatchSafely(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			w.logger.Error("curation batch panicked; skipping batch",
				"panic", r, "stack", string(debug.Stack()))
		}
	}()
	if err := w.processBatch(ctx); err != nil {
		w.logger.Error("curation batch failed", "error", err)
	}
}

// RunOnce processes a single curation batch synchronously. The Start loop
// calls the same path on each tick; RunOnce exposes it for one-shot triggering
// and for tests that need a deterministic pass without spinning the ticker.
func (w *Worker) RunOnce(ctx context.Context) error { return w.processBatch(ctx) }

func (w *Worker) processBatch(ctx context.Context) error {
	ids, err := w.claimBatch(ctx)
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
		// processOneSafe isolates panics per memory: a single bad row is logged,
		// marked failed, and skipped so the rest of the batch still runs and a
		// panic never crashes the HTTP server.
		err, panicked := w.processOneSafe(ctx, memID, getPeers)
		switch {
		case panicked:
			w.logger.Error("curation: process memory panicked",
				"memory_id", memID, "panic", err, "stack", string(debug.Stack()))
			w.markCurationFailed(ctx, memID, "panic recovered")
		case err != nil:
			w.logger.Error("curation: process memory failed",
				"memory_id", memID, "error", err)
			w.markCurationFailed(ctx, memID, err.Error())
		default:
			if err := w.markDone(ctx, memID); err != nil {
				w.logger.Warn("curation: set done status error",
					"memory_id", memID, "error", err)
			}
		}
	}
	return nil
}

// maxCurationPeers bounds the per-project peer set the worker scans for
// near-duplicate and contradiction detection, capping the per-item O(peers)
// cost on very large projects. The newest peers are kept.
const maxCurationPeers = 2000

// markCurationFailed records a curation failure for a memory, logging (not
// returning) any store error so the batch loop stays linear.
func (w *Worker) markCurationFailed(ctx context.Context, memID, reason string) {
	var err error
	if w.leased != nil {
		err = w.leased.SetCurationFailedLeased(ctx, memID, w.owner, reason)
	} else {
		err = w.store.SetCurationFailed(ctx, memID, reason)
	}
	if err != nil {
		w.logger.Warn("curation: set failed status error",
			"memory_id", memID, "error", err)
	}
}

// processOneSafe runs processOne with panic isolation so a single bad memory
// never aborts the batch or crashes the server. On panic the recovered value
// is returned as the error and panicked=true; the caller logs it, marks the
// memory failed, and continues with the rest of the batch.
func (w *Worker) processOneSafe(ctx context.Context, memID string, getPeers func(projectID string) []*model.Memory) (err error, panicked bool) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
			panicked = true
		}
	}()
	return w.processOne(ctx, memID, getPeers), false
}

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
	// v3 advisory hygiene: a canonical whose near-dup cluster shrank below 2
	// members (memories deleted or rewritten) loses the candidate flag on its
	// own re-curation pass — the suggestion never outlives the cluster.
	if truthy(metaMap["consolidation_candidate"]) {
		if n, err := w.store.CountCanonicalMembers(ctx, mem.ProjectID, mem.ID); err == nil && n < 2 {
			patch["consolidation_candidate"] = false
			patch["consolidation_members"] = n
		}
	}

	patch["curation_version"] = 2
	patch["last_curated_at"] = time.Now().UTC().Format(time.RFC3339)

	metadataUpdated, err := store.UpdateMemoryMetadataForContent(
		ctx,
		w.store,
		memID,
		mem.ContentHash,
		patch,
	)
	if err != nil {
		return err
	}
	if !metadataUpdated {
		w.logger.Debug("curation: content changed while evaluating metadata; leaving memory queued",
			"memory_id", memID)
		return nil
	}

	// Curation v3 (advisory): when this memory joined a near-duplicate
	// cluster, check the cluster size and mark the canonical as a
	// consolidation candidate once it has >=2 members (3 memories total).
	// Marking only — the server never synthesizes or deletes; the dashboard
	// and the client surface the suggestion.
	if patch["curation_status"] == "near_duplicate" {
		if canonicalID, ok := patch["canonical_of"].(string); ok && canonicalID != "" {
			if n, err := w.store.CountCanonicalMembers(ctx, mem.ProjectID, canonicalID); err == nil && n >= 2 {
				if err := w.store.UpdateMemoryMetadata(ctx, canonicalID, map[string]any{
					"consolidation_candidate": true,
					"consolidation_members":   n + 1,
				}); err != nil {
					w.logger.Warn("curation: mark consolidation candidate failed",
						"canonical_id", canonicalID, "error", err)
				}
			}
		}
	}

	// Generate the semantic embedding for accepted, non-duplicate memories.
	// Best-effort: an embedding failure must not fail curation (the memory is
	// still searchable by text). Skip low-signal/duplicate rows to avoid
	// indexing noise.
	if w.embedder != nil && qualityOK {
		vec, err := embeddings.EmbedOne(ctx, w.embedder, mem.Content)
		if err != nil {
			return fmt.Errorf("curation: embed memory %s: %w", memID, err)
		}
		updated, err := store.UpdateMemoryEmbeddingForContentSpace(
			ctx,
			w.store,
			memID,
			mem.ContentHash,
			vec,
			embeddings.SemanticSpace(w.embedder),
		)
		if err != nil {
			return fmt.Errorf("curation: store embedding for memory %s: %w", memID, err)
		} else if !updated {
			w.logger.Debug("curation: content changed while embedding; leaving memory queued",
				"memory_id", memID)
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
