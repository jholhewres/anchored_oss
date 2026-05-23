package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jholhewres/anchored_oss/internal/config"
	"log/slog"
)

func TestHealthEndpoint(t *testing.T) {
	cfg := config.DefaultConfig()
	logger := slog.Default()

	srv := New(cfg, nil, logger)

	ts := httptest.NewServer(srv.http.Handler)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/v1/health")
	if err != nil {
		t.Fatalf("GET /v1/health: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if ct != "application/json" {
		t.Fatalf("expected Content-Type application/json, got %s", ct)
	}

	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if body["service"] != "anchored-oss" {
		t.Errorf("expected service=anchored-oss, got %s", body["service"])
	}
	if body["status"] != "ok" {
		t.Errorf("expected status=ok, got %s", body["status"])
	}
	if body["version"] != "dev" {
		t.Errorf("expected version=dev, got %s", body["version"])
	}
	if body["db_status"] != "unavailable" {
		t.Errorf("expected db_status=unavailable, got %s", body["db_status"])
	}
	if body["timestamp"] == "" {
		t.Error("expected non-empty timestamp")
	}
}
