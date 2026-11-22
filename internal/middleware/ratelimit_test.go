package middleware

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimiter_AllowBurstThenBlock(t *testing.T) {
	// 60 rpm => 1 token/sec, burst 3. First 3 immediate requests pass, 4th is
	// blocked until the bucket refills.
	rl := NewRateLimiter(context.Background(), 60, 3)
	now := time.Unix(1000, 0)

	for i := 0; i < 3; i++ {
		if !rl.allow("client-a", now) {
			t.Fatalf("request %d should be allowed within burst", i+1)
		}
	}
	if rl.allow("client-a", now) {
		t.Fatal("4th request should be blocked once burst is exhausted")
	}

	// After 1 second one token refills.
	if !rl.allow("client-a", now.Add(time.Second)) {
		t.Fatal("request should be allowed after 1s refill")
	}
	if rl.allow("client-a", now.Add(time.Second)) {
		t.Fatal("only one token should have refilled")
	}
}

func TestRateLimiter_PerClientIsolation(t *testing.T) {
	rl := NewRateLimiter(context.Background(), 60, 1)
	now := time.Unix(2000, 0)

	if !rl.allow("a", now) {
		t.Fatal("client a first request should pass")
	}
	if rl.allow("a", now) {
		t.Fatal("client a second request should be blocked")
	}
	if !rl.allow("b", now) {
		t.Fatal("client b should have its own bucket")
	}
}

func TestRateLimiter_DisabledPassesThrough(t *testing.T) {
	rl := NewRateLimiter(context.Background(), 0, 0) // disabled
	called := 0
	h := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++
		w.WriteHeader(http.StatusOK)
	}))
	for i := 0; i < 50; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("disabled limiter should always pass, got %d", rec.Code)
		}
	}
	if called != 50 {
		t.Fatalf("expected 50 passthrough calls, got %d", called)
	}
}

func TestRateLimiter_Middleware429(t *testing.T) {
	rl := NewRateLimiter(context.Background(), 60, 1)
	h := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	first := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/login", nil)
	req.RemoteAddr = "10.0.0.1:5555"
	h.ServeHTTP(first, req)
	if first.Code != http.StatusOK {
		t.Fatalf("first request should pass, got %d", first.Code)
	}

	second := httptest.NewRecorder()
	h.ServeHTTP(second, req)
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second request should be 429, got %d", second.Code)
	}
	if second.Header().Get("Retry-After") == "" {
		t.Error("429 response should set Retry-After")
	}
}

func TestClientKey_ForwardedFor(t *testing.T) {
	rl := NewRateLimiter(context.Background(), 0, 0)
	rl.TrustProxies(nets(t, "10.0.0.0/8", "192.168.0.0/16", "127.0.0.1/32"))
	for _, tc := range []struct {
		name, remote, xff, want string
	}{
		{"proxy appends the client", "10.0.0.9:1234", "203.0.113.7, 10.0.0.9", "203.0.113.7"},
		{"a client's own hops sit left of the real one", "10.0.0.9:1234", "198.51.100.1, 203.0.113.7", "203.0.113.7"},
		{"no header from a proxy", "192.168.1.5:9999", "", "192.168.1.5"},
		{"untrusted connection ignores the header", "203.0.113.50:4000", "198.51.100.1", "203.0.113.50"},
		{"malformed hop stops the walk", "127.0.0.1:80", "not-an-ip", "127.0.0.1"},
		{"all hops trusted", "127.0.0.1:80", "192.168.1.7, 10.0.0.2", "192.168.1.7"},
		{"IPv6 clients share their /64", "[2001:db8:1:2:aaaa::1]:443", "", "2001:db8:1:2::/64"},
	} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = tc.remote
		if tc.xff != "" {
			req.Header.Set("X-Forwarded-For", tc.xff)
		}
		if got := rl.clientKey(req); got != tc.want {
			t.Errorf("%s: clientKey = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestRateLimit_SpoofedForwardedForDoesNotEscape: a client that is not a
// trusted proxy gets one bucket, whatever X-Forwarded-For it sends.
func TestRateLimit_SpoofedForwardedForDoesNotEscape(t *testing.T) {
	rl := NewRateLimiter(context.Background(), 60, 2)
	rl.TrustProxies(nets(t, "10.0.0.0/8"))
	h := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	limited := false
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodPost, "/v1/onboarding/complete", nil)
		req.RemoteAddr = "203.0.113.50:4000"
		req.Header.Set("X-Forwarded-For", fmt.Sprintf("198.51.100.%d", i))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code == http.StatusTooManyRequests {
			limited = true
			break
		}
	}
	if !limited {
		t.Fatal("a rotating X-Forwarded-For from an untrusted client escaped the limit")
	}
}

// nets parses CIDRs for the tests.
func nets(t *testing.T, cidrs ...string) []*net.IPNet {
	t.Helper()
	out := make([]*net.IPNet, 0, len(cidrs))
	for _, c := range cidrs {
		_, n, err := net.ParseCIDR(c)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, n)
	}
	return out
}

// TestRateLimit_WarnsOnceAboutAnUntrustedProxy: forwarded requests from a
// private address that is not trusted are keyed on that address, and the
// operator is told once.
func TestRateLimit_WarnsOnceAboutAnUntrustedProxy(t *testing.T) {
	rl := NewRateLimiter(context.Background(), 0, 0)
	var warned []string
	rl.WarnUntrustedProxy(func(remote string) { warned = append(warned, remote) })
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "172.18.0.5:5000"
		req.Header.Set("X-Forwarded-For", "203.0.113.7")
		if got := rl.clientKey(req); got != "172.18.0.5" {
			t.Fatalf("clientKey = %q, want the untrusted peer 172.18.0.5", got)
		}
	}
	public := httptest.NewRequest(http.MethodGet, "/", nil)
	public.RemoteAddr = "203.0.113.50:5000"
	public.Header.Set("X-Forwarded-For", "198.51.100.1")
	rl.clientKey(public)
	if len(warned) != 1 || warned[0] != "172.18.0.5" {
		t.Fatalf("warnings = %v, want one about 172.18.0.5", warned)
	}
}
