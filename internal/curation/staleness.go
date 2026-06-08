package curation

import "time"

// isStale reports whether a memory should be marked with curation_status
// "stale". A memory is stale when it has not been updated within staleAfter,
// is not pinned, and has not been superseded by a newer memory. Pinned and
// superseded memories are always exempt — pinning is an explicit "keep" signal
// and a superseded memory already points at its replacement.
//
// staleAfter <= 0 disables staleness marking entirely (returns false).
func isStale(updatedAt time.Time, meta map[string]any, staleAfter time.Duration, now time.Time) bool {
	if staleAfter <= 0 {
		return false
	}
	if truthy(meta["pinned"]) {
		return false
	}
	if s, _ := meta["superseded_by"].(string); s != "" {
		return false
	}
	return now.Sub(updatedAt) > staleAfter
}

// truthy interprets a metadata value that may be a bool or a string ("true")
// as a boolean. JSON round-trips and SQLite TEXT columns can yield either form.
func truthy(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return t == "true" || t == "1"
	case float64:
		return t != 0
	default:
		return false
	}
}
