package store

import (
	"fmt"
	"math"
	"sort"

	"github.com/jholhewres/anchored_oss/internal/model"
)

// Anomaly thresholds: a source is spiking when its last-24h volume exceeds
// both this absolute floor and spikeFactor times its prior-29-day daily
// baseline. The floor keeps tiny projects from flagging normal activity.
const (
	spikeMinCount = 100
	spikeFactor   = 10.0
)

// healthAggregates carries backend-agnostic raw numbers; composeHealth turns
// them into the wire-level MemoryHealth (score, anomalies, recommendations).
type healthAggregates struct {
	Counts       model.HealthCounts
	BySource     []model.NameCount
	ByCategory   []model.NameCount
	AgeHistogram []model.NameCount
	Last24h      map[string]int64 // created in the last 24h, by source
	Prior29d     map[string]int64 // created in [now-30d, now-24h), by source
	Rejections   []model.RuleCount
}

func composeHealth(agg healthAggregates) *model.MemoryHealth {
	h := &model.MemoryHealth{
		Counts:          agg.Counts,
		BySource:        agg.BySource,
		ByCategory:      agg.ByCategory,
		AgeHistogram:    agg.AgeHistogram,
		SyncRejections:  agg.Rejections,
		Anomalies:       detectVolumeSpikes(agg.Last24h, agg.Prior29d),
		Recommendations: []string{},
	}
	h.Score = healthScore(agg.Counts)
	h.Recommendations = buildRecommendations(h)
	return h
}

// healthScore measures how much of the live corpus the curation worker flagged
// as an actionable defect: low-signal, near-duplicate, and contradiction
// candidates, each costing its share of Live. Age-based staleness is
// deliberately NOT in the score — an old but clean memory is not a defect, and
// including raw age pinned the score in the warning band for any mature corpus
// even when nothing was wrong. Staleness is still surfaced as a count and the
// age histogram; it just no longer drags the score. Contradictions come from a
// lexical heuristic, so they weigh the same as the other marks rather than
// double.
func healthScore(c model.HealthCounts) float64 {
	if c.Live <= 0 {
		return 1.0
	}
	penalty := (float64(c.LowSignal) + float64(c.NearDuplicate) +
		float64(c.Contradictions)) / float64(c.Live)
	score := 1.0 - penalty
	if score < 0 {
		score = 0
	}
	return math.Round(score*100) / 100
}

func detectVolumeSpikes(last24h, prior29d map[string]int64) []model.HealthAnomaly {
	anomalies := []model.HealthAnomaly{}
	for source, count := range last24h {
		if count <= spikeMinCount {
			continue
		}
		baseline := float64(prior29d[source]) / 29.0
		if baseline > 0 && float64(count) <= spikeFactor*baseline {
			continue
		}
		anomalies = append(anomalies, model.HealthAnomaly{
			Type:     "volume_spike",
			Source:   source,
			Window:   "24h",
			Count:    count,
			Baseline: math.Round(baseline*10) / 10,
		})
	}
	sort.Slice(anomalies, func(i, j int) bool { return anomalies[i].Count > anomalies[j].Count })
	return anomalies
}

func buildRecommendations(h *model.MemoryHealth) []string {
	recs := []string{}
	for _, a := range h.Anomalies {
		if a.Baseline == 0 {
			recs = append(recs, fmt.Sprintf(
				"Investigate new source %q: %d memories in 24h with no prior history",
				a.Source, a.Count))
			continue
		}
		recs = append(recs, fmt.Sprintf(
			"Investigate volume spike from source %q: %d memories in 24h vs baseline %.1f/day",
			a.Source, a.Count, a.Baseline))
	}
	if h.Counts.Contradictions > 0 {
		recs = append(recs, fmt.Sprintf("Review %d contradiction candidates", h.Counts.Contradictions))
	}
	if h.Counts.NearDuplicate > 0 {
		recs = append(recs, fmt.Sprintf("Review %d near-duplicate memories", h.Counts.NearDuplicate))
	}
	// Only recommend a reindex for memories that are eligible for embeddings but
	// lack one. Low-signal and near-duplicate rows are intentionally never
	// embedded, so counting them made this recommendation permanent noise; a
	// gap among otherwise-clean memories is the real reindex signal (e.g. the
	// embedding model changed, or the worker fell behind).
	eligibleMissing := h.Counts.MissingEmbeddings - h.Counts.LowSignal - h.Counts.NearDuplicate
	if eligibleMissing > 0 {
		recs = append(recs, fmt.Sprintf("Run reindex: %d memories missing embeddings", eligibleMissing))
	}
	var topRule *model.RuleCount
	for i := range h.SyncRejections {
		if topRule == nil || h.SyncRejections[i].Count > topRule.Count {
			topRule = &h.SyncRejections[i]
		}
	}
	if topRule != nil && topRule.Count >= 20 {
		recs = append(recs, fmt.Sprintf(
			"Frequent sync rejections by rule %q (%d in 7 days) — check client configuration or guardrails",
			topRule.Rule, topRule.Count))
	}
	return recs
}

// aggregateRejectionsByRule folds per-day rejection rows into per-rule totals,
// ordered by count descending.
func aggregateRejectionsByRule(stats []*model.RejectionStat) []model.RuleCount {
	byRule := map[string]int64{}
	for _, s := range stats {
		byRule[s.Rule] += s.Count
	}
	out := make([]model.RuleCount, 0, len(byRule))
	for rule, n := range byRule {
		out = append(out, model.RuleCount{Rule: rule, Count: n})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Count > out[j].Count })
	return out
}
