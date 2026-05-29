// Package embeddings provides the server-side vector embedding abstraction.
//
// The interface mirrors the anchored CLI's EmbeddingProvider so the local
// ONNX model (paraphrase-multilingual-MiniLM-L12-v2, 384d) can be ported in as
// a drop-in provider, keeping the server's vector space compatible with the
// vectors the CLI already produces. Until that runtime is wired, two providers
// ship here:
//
//   - "local": a dependency-free, deterministic hashing embedder. It is a
//     zero-config default and the test/dev fallback. It captures lexical
//     overlap (not deep semantics) — production should use ONNX or a paid
//     provider for real semantic retrieval.
//   - "openai": any OpenAI-compatible /v1/embeddings endpoint (OpenAI, z.ai,
//     OpenRouter, ...). Dimensions come from the configured model.
//
// All providers emit L2-normalized vectors so cosine distance reduces to a dot
// product and pgvector's vector_cosine_ops behaves consistently.
package embeddings

import (
	"context"
	"fmt"
)

// Embedder turns text into fixed-dimension vectors. Implementations must return
// L2-normalized vectors of exactly Dimensions() length.
type Embedder interface {
	// Embed returns one vector per input text, in order.
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	// Dimensions is the fixed vector width this embedder produces.
	Dimensions() int
	// Model identifies the underlying model (stored alongside each vector so a
	// provider/model change can trigger a reindex).
	Model() string
	// Name is the provider name ("local", "openai", ...).
	Name() string
}

// EmbedOne is a convenience wrapper for single-text embedding.
func EmbedOne(ctx context.Context, e Embedder, text string) ([]float32, error) {
	vecs, err := e.Embed(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(vecs) != 1 {
		return nil, fmt.Errorf("embeddings: expected 1 vector, got %d", len(vecs))
	}
	return vecs[0], nil
}
