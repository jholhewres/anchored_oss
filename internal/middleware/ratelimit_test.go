package middleware

import (
	"context"
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
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.9:1234"
	req.Header.Set("X-Forwarded-For", "203.0.113.7, 10.0.0.9")
	if got := clientKey(req); got != "203.0.113.7" {
		t.Fatalf("expected first XFF hop 203.0.113.7, got %q", got)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.RemoteAddr = "192.168.1.5:9999"
	if got := clientKey(req2); got != "192.168.1.5" {
		t.Fatalf("expected RemoteAddr host 192.168.1.5, got %q", got)
	}
}
