package handler

import (
	"errors"
	"testing"

	"github.com/jholhewres/anchored_oss/internal/model"
)

// TestHealthCache_ReusesWithinTTL: a second lookup for the same key inside the
// TTL is served from cache (compute runs once); a different key recomputes.
func TestHealthCache_ReusesWithinTTL(t *testing.T) {
	h := &MemoryHealthHandler{cache: make(map[string]healthCacheEntry)}

	calls := 0
	compute := func() (*model.MemoryHealth, error) {
		calls++
		return &model.MemoryHealth{Score: 0.9}, nil
	}

	for i := 0; i < 3; i++ {
		got, err := h.cached("p:proj-1", compute)
		if err != nil || got == nil || got.Score != 0.9 {
			t.Fatalf("call %d: got %+v err %v", i, got, err)
		}
	}
	if calls != 1 {
		t.Fatalf("compute should run once within TTL, ran %d times", calls)
	}

	if _, err := h.cached("p:proj-2", compute); err != nil {
		t.Fatalf("second key: %v", err)
	}
	if calls != 2 {
		t.Fatalf("a new key must recompute, calls=%d", calls)
	}
}

// TestHealthCache_DoesNotCacheErrors: a failed compute is not cached, so the
// next request retries instead of serving a poisoned error.
func TestHealthCache_DoesNotCacheErrors(t *testing.T) {
	h := &MemoryHealthHandler{cache: make(map[string]healthCacheEntry)}

	calls := 0
	compute := func() (*model.MemoryHealth, error) {
		calls++
		if calls == 1 {
			return nil, errors.New("transient")
		}
		return &model.MemoryHealth{Score: 1}, nil
	}

	if _, err := h.cached("p:x", compute); err == nil {
		t.Fatal("expected first call to surface the error")
	}
	got, err := h.cached("p:x", compute)
	if err != nil || got == nil {
		t.Fatalf("retry after error should recompute: got %+v err %v", got, err)
	}
	if calls != 2 {
		t.Fatalf("error must not be cached, calls=%d", calls)
	}
}
