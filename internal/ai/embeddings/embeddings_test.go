package embeddings

import (
	"context"
	"math"
	"testing"
)

func dot(a, b []float32) float64 {
	var s float64
	for i := range a {
		s += float64(a[i]) * float64(b[i])
	}
	return s
}

func norm(v []float32) float64 {
	var s float64
	for _, x := range v {
		s += float64(x) * float64(x)
	}
	return math.Sqrt(s)
}

func TestLocalEmbedder_ShapeAndNormalization(t *testing.T) {
	e := NewLocalEmbedder(384)
	if e.Dimensions() != 384 {
		t.Fatalf("expected 384 dims, got %d", e.Dimensions())
	}
	vecs, err := e.Embed(context.Background(), []string{"hello world", ""})
	if err != nil {
		t.Fatalf("embed: %v", err)
	}
	if len(vecs) != 2 {
		t.Fatalf("expected 2 vectors, got %d", len(vecs))
	}
	for i, v := range vecs {
		if len(v) != 384 {
			t.Fatalf("vector %d wrong width: %d", i, len(v))
		}
		if n := norm(v); math.Abs(n-1) > 1e-4 {
			t.Fatalf("vector %d not L2-normalized: norm=%f", i, n)
		}
	}
}

func TestLocalEmbedder_Deterministic(t *testing.T) {
	e := NewLocalEmbedder(256)
	a, _ := EmbedOne(context.Background(), e, "anchored shares team memory")
	b, _ := EmbedOne(context.Background(), e, "anchored shares team memory")
	if dot(a, b) < 0.9999 {
		t.Fatalf("same input should yield identical vectors, cos=%f", dot(a, b))
	}
}

func TestLocalEmbedder_SimilarityOrdering(t *testing.T) {
	e := NewLocalEmbedder(384)
	ctx := context.Background()
	query, _ := EmbedOne(ctx, e, "how do we handle database migrations")
	near, _ := EmbedOne(ctx, e, "database migrations are applied on startup")
	far, _ := EmbedOne(ctx, e, "the frontend uses a custom design system")

	simNear := dot(query, near) // normalized => cosine
	simFar := dot(query, far)
	if simNear <= simFar {
		t.Fatalf("expected related text to score higher: near=%f far=%f", simNear, simFar)
	}
}

func TestFactory(t *testing.T) {
	if e, err := New(Config{Provider: "local", Dimensions: 384}, nil); err != nil || e == nil || e.Name() != "local" {
		t.Fatalf("local provider: e=%v err=%v", e, err)
	}
	if e, err := New(Config{Provider: "none"}, nil); err != nil || e != nil {
		t.Fatalf("none provider should yield (nil,nil): e=%v err=%v", e, err)
	}
	if _, err := New(Config{Provider: "martian"}, nil); err == nil {
		t.Fatal("unknown provider should error")
	}
	if _, err := New(Config{Provider: "openai"}, nil); err == nil {
		t.Fatal("openai without model should error")
	}
	if e, err := New(Config{Provider: "openai", Model: "text-embedding-3-small", Dimensions: 1536}, nil); err != nil || e.Name() != "openai" {
		t.Fatalf("openai provider: e=%v err=%v", e, err)
	}
}
