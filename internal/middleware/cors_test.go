package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCORSAllowsIdempotencyAndExposesResponseTelemetry(t *testing.T) {
	handler := CORS([]string{"https://dashboard.example"})(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
	)
	request := httptest.NewRequest(http.MethodOptions, "/v1/memories", nil)
	request.Header.Set("Origin", "https://dashboard.example")
	request.Header.Set("Access-Control-Request-Headers", "Idempotency-Key")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d, want %d", response.Code, http.StatusNoContent)
	}
	if vary := response.Header().Get("Vary"); !strings.Contains(vary, "Origin") {
		t.Fatalf("Vary = %q, want Origin", vary)
	}
	allowed := response.Header().Get("Access-Control-Allow-Headers")
	if !strings.Contains(allowed, "Idempotency-Key") {
		t.Fatalf("allowed headers = %q, want Idempotency-Key", allowed)
	}
	exposed := response.Header().Get("Access-Control-Expose-Headers")
	for _, header := range []string{"Idempotency-Replayed", "X-Anchored-Effective-Mode"} {
		if !strings.Contains(exposed, header) {
			t.Fatalf("exposed headers = %q, want %s", exposed, header)
		}
	}
}
