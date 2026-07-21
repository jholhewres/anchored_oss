package embeddings

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAIEmbedderSendsSupportedDimensionsAndValidatesResponse(t *testing.T) {
	var request openAIRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"embedding": []float32{3, 4}},
			},
		})
	}))
	defer server.Close()

	embedder := NewOpenAIEmbedder(server.URL, "text-embedding-3-small", "", 2)
	vectors, err := embedder.Embed(context.Background(), []string{"memory"})
	if err != nil {
		t.Fatalf("embed: %v", err)
	}
	if request.Dimensions != 2 {
		t.Fatalf("request dimensions = %d, want 2", request.Dimensions)
	}
	if len(vectors) != 1 || len(vectors[0]) != 2 {
		t.Fatalf("vectors = %+v", vectors)
	}
}

func TestOpenAIEmbedderRejectsProviderDimensionMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"embedding": []float32{1, 2, 3}},
			},
		})
	}))
	defer server.Close()

	embedder := NewOpenAIEmbedder(server.URL, "text-embedding-3-small", "", 2)
	_, err := embedder.Embed(context.Background(), []string{"memory"})
	if err == nil || !strings.Contains(err.Error(), "vector 0 has 3 dimensions, want 2") {
		t.Fatalf("dimension mismatch error = %v", err)
	}
}

func TestOpenAIEmbedderOmitsDimensionsForLegacyModel(t *testing.T) {
	var request map[string]json.RawMessage
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"embedding": []float32{1, 0}},
			},
		})
	}))
	defer server.Close()

	embedder := NewOpenAIEmbedder(server.URL, "text-embedding-ada-002", "", 2)
	if _, err := embedder.Embed(context.Background(), []string{"memory"}); err != nil {
		t.Fatalf("embed legacy model: %v", err)
	}
	if _, ok := request["dimensions"]; ok {
		t.Fatal("legacy model request unexpectedly included dimensions")
	}
}

func TestOpenAICompatibleEndpointsHaveDistinctSemanticProviders(t *testing.T) {
	first := NewOpenAIEmbedder(
		"https://EXAMPLE.com/v1/",
		"shared-model",
		"",
		384,
	)
	canonicalEquivalent := NewOpenAIEmbedder(
		"https://example.com/v1",
		"shared-model",
		"",
		384,
	)
	otherEndpoint := NewOpenAIEmbedder(
		"https://other.example/v1",
		"shared-model",
		"",
		384,
	)

	firstSpace := SemanticSpace(first)
	if firstSpace.ID() != SemanticSpace(canonicalEquivalent).ID() {
		t.Fatal("equivalent endpoint spelling changed semantic-space identity")
	}
	if firstSpace.ID() == SemanticSpace(otherEndpoint).ID() {
		t.Fatal("different OpenAI-compatible endpoints shared a semantic-space identity")
	}
	if first.Name() != "openai" {
		t.Fatalf("legacy provider name = %q, want openai", first.Name())
	}
}
