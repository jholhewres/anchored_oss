package curation

import (
	"testing"
	"time"
)

func TestIsStale(t *testing.T) {
	now := time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC)
	staleAfter := 180 * 24 * time.Hour

	cases := []struct {
		name      string
		updatedAt time.Time
		meta      map[string]any
		after     time.Duration
		want      bool
	}{
		{"old unpinned -> stale", now.Add(-200 * 24 * time.Hour), nil, staleAfter, true},
		{"recent -> not stale", now.Add(-10 * 24 * time.Hour), nil, staleAfter, false},
		{"old but pinned bool -> exempt", now.Add(-200 * 24 * time.Hour), map[string]any{"pinned": true}, staleAfter, false},
		{"old but pinned string -> exempt", now.Add(-200 * 24 * time.Hour), map[string]any{"pinned": "true"}, staleAfter, false},
		{"old but superseded -> exempt", now.Add(-200 * 24 * time.Hour), map[string]any{"superseded_by": "m-new"}, staleAfter, false},
		{"old, empty superseded -> stale", now.Add(-200 * 24 * time.Hour), map[string]any{"superseded_by": ""}, staleAfter, true},
		{"disabled (0) -> never stale", now.Add(-9999 * 24 * time.Hour), nil, 0, false},
		{"exactly at boundary -> not stale", now.Add(-staleAfter), nil, staleAfter, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isStale(tc.updatedAt, tc.meta, tc.after, now); got != tc.want {
				t.Fatalf("isStale = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestTruthy(t *testing.T) {
	for _, tc := range []struct {
		v    any
		want bool
	}{
		{true, true}, {false, false},
		{"true", true}, {"1", true}, {"false", false}, {"", false},
		{float64(1), true}, {float64(0), false},
		{nil, false}, {42, false},
	} {
		if got := truthy(tc.v); got != tc.want {
			t.Fatalf("truthy(%#v) = %v, want %v", tc.v, got, tc.want)
		}
	}
}
