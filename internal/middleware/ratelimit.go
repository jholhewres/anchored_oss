package middleware

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// RateLimiter is a dependency-free, per-client token-bucket limiter. Each
// client key (resolved from X-Forwarded-For or RemoteAddr) gets its own bucket
// that refills at ratePerSec up to burst. Stale buckets are reaped by a
// background sweeper so memory stays bounded under churn.
type RateLimiter struct {
	ratePerSec float64
	burst      float64

	mu      sync.Mutex
	buckets map[string]*bucket
}

type bucket struct {
	tokens float64
	last   time.Time
}

// NewRateLimiter builds a limiter allowing perMinute requests with the given
// burst. A perMinute <= 0 disables limiting (Middleware becomes a pass-through).
// The sweeper goroutine stops when ctx is cancelled, so it doesn't leak across
// server re-creation (e.g. tests) or graceful shutdown.
func NewRateLimiter(ctx context.Context, perMinute, burst int) *RateLimiter {
	rl := &RateLimiter{
		ratePerSec: float64(perMinute) / 60.0,
		burst:      float64(burst),
		buckets:    make(map[string]*bucket),
	}
	if perMinute > 0 {
		go rl.sweep(ctx)
	}
	return rl
}

func (rl *RateLimiter) enabled() bool { return rl.ratePerSec > 0 }

// allow consumes one token for key, returning false when the bucket is empty.
func (rl *RateLimiter) allow(key string, now time.Time) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	b, ok := rl.buckets[key]
	if !ok {
		rl.buckets[key] = &bucket{tokens: rl.burst - 1, last: now}
		return true
	}
	elapsed := now.Sub(b.last).Seconds()
	if elapsed > 0 {
		b.tokens += elapsed * rl.ratePerSec
		if b.tokens > rl.burst {
			b.tokens = rl.burst
		}
		b.last = now
	}
	if b.tokens >= 1 {
		b.tokens--
		return true
	}
	return false
}

// sweep evicts buckets idle long enough to have fully refilled, bounding the
// map under high client churn.
func (rl *RateLimiter) sweep(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cutoff := time.Now().Add(-10 * time.Minute)
			rl.mu.Lock()
			for k, b := range rl.buckets {
				if b.last.Before(cutoff) {
					delete(rl.buckets, k)
				}
			}
			rl.mu.Unlock()
		}
	}
}

// Middleware enforces the limit, returning 429 with a Retry-After hint when a
// client exceeds it. When disabled it passes through untouched.
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	if !rl.enabled() {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !rl.allow(clientKey(r), time.Now()) {
			w.Header().Set("Retry-After", "1")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "RATE_LIMITED"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// clientKey resolves the throttling identity. Behind a reverse proxy (Caddy),
// the first X-Forwarded-For hop is the real client; otherwise fall back to the
// connection's remote host.
func clientKey(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if idx := strings.IndexByte(xff, ','); idx >= 0 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
