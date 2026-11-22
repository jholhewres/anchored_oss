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
// client IP gets its own bucket that refills at ratePerSec up to burst. The IP
// is the connection's, or, when the connection comes from a trusted proxy, the
// nearest untrusted hop of X-Forwarded-For. Stale buckets are reaped by a
// background sweeper so memory stays bounded under churn.
type RateLimiter struct {
	ratePerSec float64
	burst      float64
	trusted    []*net.IPNet
	// warnUntrustedProxy logs, once, a forwarded request from a private
	// address that is not trusted: most likely a proxy the operator has
	// not listed, whose clients now share one bucket.
	warnUntrustedProxy func(remote string)
	warnOnce           sync.Once

	mu      sync.Mutex
	buckets map[string]*bucket
}

// WarnUntrustedProxy sets the function told, once, about the first request
// that carries X-Forwarded-For from a private or loopback address the limiter
// does not trust.
func (rl *RateLimiter) WarnUntrustedProxy(warn func(remote string)) {
	rl.warnUntrustedProxy = warn
}

// TrustProxies sets the networks whose X-Forwarded-For the limiter believes
// (config.RateLimitConfig.TrustedNets parses them). A new limiter, or no
// networks, trusts none: every client is its connection's IP.
func (rl *RateLimiter) TrustProxies(nets []*net.IPNet) {
	rl.trusted = nets
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
		if !rl.allow(rl.clientKey(r), time.Now()) {
			w.Header().Set("Retry-After", "1")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "RATE_LIMITED"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// clientKey resolves the throttling identity: the connection's IP, unless
// that is a trusted proxy. Then X-Forwarded-For is read from the right, past
// the trusted hops, and the first untrusted IP is the client: hops a client
// writes itself sit to the left of the ones its proxies append, so they are
// never reached. A malformed hop ends the walk. The key is always a parsed IP,
// so a header cannot grow it.
func (rl *RateLimiter) clientKey(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	remote := net.ParseIP(host)
	if remote == nil {
		return host
	}
	if !rl.isTrusted(remote) {
		if rl.warnUntrustedProxy != nil && (remote.IsPrivate() || remote.IsLoopback()) && r.Header.Get("X-Forwarded-For") != "" {
			rl.warnOnce.Do(func() { rl.warnUntrustedProxy(remote.String()) })
		}
		return bucketKey(remote)
	}
	client := remote
	// Every X-Forwarded-For line, in order: a proxy may add its own line
	// rather than append to the client's.
	if lines := r.Header.Values("X-Forwarded-For"); len(lines) > 0 {
		hops := strings.Split(strings.Join(lines, ","), ",")
		for i := len(hops) - 1; i >= 0; i-- {
			ip := net.ParseIP(strings.TrimSpace(hops[i]))
			if ip == nil {
				break
			}
			client = ip
			if !rl.isTrusted(ip) {
				break
			}
		}
	}
	return bucketKey(client)
}

// bucketKey is the identity a client's bucket hangs on: its IPv4 address, or
// the /64 of an IPv6 one, since a single host usually holds a whole /64 and
// could otherwise mint a fresh bucket per request.
func bucketKey(ip net.IP) string {
	if v4 := ip.To4(); v4 != nil {
		return v4.String()
	}
	return ip.Mask(net.CIDRMask(64, 128)).String() + "/64"
}

func (rl *RateLimiter) isTrusted(ip net.IP) bool {
	for _, n := range rl.trusted {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}
