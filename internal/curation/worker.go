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

	"github.com/jholhewres/anchored_oss/internal/config"
	"github.com/jholhewres/anchored_oss/internal/policy"
	"github.com/jholhewres/anchored_oss/internal/store"
)

// Worker processes the curation_queue on a ticker interval.
type Worker struct {
	cfg    config.CurationConfig
	store  store.Store
	filter *policy.ContentFilter
	logger *slog.Logger
}

// NewWorker creates a Worker that is ready to Start.
func NewWorker(cfg *config.Config, st store.Store, logger *slog.Logger) *Worker {
	return &Worker{
		cfg:    cfg.Curation,
		store:  st,
		filter: policy.NewContentFilter(),
		logger: logger,
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

func (w *Worker) processBatch(ctx context.Context) error {
	ids, err := w.store.ClaimCurationBatch(ctx, w.cfg.BatchSize)
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}

	for _, memID := range ids {
		if err := w.processOne(ctx, memID); err != nil {
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

func (w *Worker) processOne(ctx context.Context, memID string) error {
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

	// Near-duplicate detection within the project over the configured window.
	since := time.Now().UTC().Add(-w.cfg.NearDupWindow)
	peers, err := w.store.ListProjectMemoriesSince(ctx, mem.ProjectID, since)
	if err != nil {
		w.logger.Warn("curation: list peers failed", "memory_id", memID, "error", err)
	}

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

	return w.store.UpdateMemoryMetadata(ctx, memID, patch)
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
