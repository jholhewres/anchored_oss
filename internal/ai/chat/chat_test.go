package chat

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAIProvider_Complete(t *testing.T) {
	var gotPath, gotAuth, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"hi from openai"}}]}`))
	}))
	defer srv.Close()

	p := NewOpenAIProvider(srv.URL, "gpt-x", "sk-test", 256)
	out, err := p.Complete(context.Background(), "be brief", []Message{{Role: RoleUser, Content: "hello"}})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if out != "hi from openai" {
		t.Fatalf("unexpected answer: %q", out)
	}
	if gotPath != "/chat/completions" {
		t.Fatalf("wrong path: %s", gotPath)
	}
	if gotAuth != "Bearer sk-test" {
		t.Fatalf("wrong auth: %s", gotAuth)
	}
	// System prompt must be the first message.
	var payload struct {
		Messages []Message `json:"messages"`
	}
	_ = json.Unmarshal([]byte(gotBody), &payload)
	if len(payload.Messages) != 2 || payload.Messages[0].Role != RoleSystem {
		t.Fatalf("system prompt not prepended: %+v", payload.Messages)
	}
}

func TestOpenAIProvider_ErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"rate limited"}}`))
	}))
	defer srv.Close()
	p := NewOpenAIProvider(srv.URL, "m", "", 0)
	if _, err := p.Complete(context.Background(), "", []Message{{Role: RoleUser, Content: "x"}}); err == nil || !strings.Contains(err.Error(), "rate limited") {
		t.Fatalf("expected provider error surfaced, got %v", err)
	}
}

func TestAnthropicProvider_Complete(t *testing.T) {
	var gotPath, gotVer, gotKey string
	var hasSystem bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotVer = r.Header.Get("anthropic-version")
		gotKey = r.Header.Get("x-api-key")
		b, _ := io.ReadAll(r.Body)
		var payload map[string]any
		_ = json.Unmarshal(b, &payload)
		_, hasSystem = payload["system"]
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"hi from claude"}]}`))
	}))
	defer srv.Close()

	p := NewAnthropicProvider(srv.URL, "claude-x", "ak-test", 256)
	out, err := p.Complete(context.Background(), "be brief", []Message{{Role: RoleUser, Content: "hello"}})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if out != "hi from claude" {
		t.Fatalf("unexpected answer: %q", out)
	}
	if gotPath != "/messages" || gotVer == "" || gotKey != "ak-test" {
		t.Fatalf("anthropic request shape wrong: path=%s ver=%s key=%s", gotPath, gotVer, gotKey)
	}
	if !hasSystem {
		t.Fatal("system should be a top-level field for anthropic")
	}
}

func TestFactory(t *testing.T) {
	if p, err := New(Config{Enabled: false}); err != nil || p != nil {
		t.Fatalf("disabled should yield (nil,nil): %v %v", p, err)
	}
	if _, err := New(Config{Enabled: true}); err == nil {
		t.Fatal("enabled without model should error")
	}
	if p, err := New(Config{Enabled: true, Provider: "openai", Model: "m"}); err != nil || p.Name() != "openai" {
		t.Fatalf("openai: %v %v", p, err)
	}
	if p, err := New(Config{Enabled: true, Provider: "anthropic", Model: "m"}); err != nil || p.Name() != "anthropic" {
		t.Fatalf("anthropic: %v %v", p, err)
	}
	if _, err := New(Config{Enabled: true, Provider: "martian", Model: "m"}); err == nil {
		t.Fatal("unknown provider should error")
	}
}
