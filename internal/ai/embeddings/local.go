package embeddings

import (
	"context"
	"hash/fnv"
	"math"
	"strings"
	"unicode"
)

// LocalEmbedder is a dependency-free, deterministic embedder. It hashes word
// unigrams and character trigrams into a fixed-dimension bag and L2-normalizes
// the result. It is multilingual-agnostic (operates on raw runes, no stemming)
// and stable across runs, which makes it ideal as a zero-config default and for
// tests. It is NOT a semantic model — swap in ONNX or OpenAI for production
// retrieval quality.
type LocalEmbedder struct {
	dims int
}

// NewLocalEmbedder builds a local embedder of the given dimension (default 384,
// matching the planned ONNX model width).
func NewLocalEmbedder(dims int) *LocalEmbedder {
	if dims <= 0 {
		dims = 384
	}
	return &LocalEmbedder{dims: dims}
}

func (e *LocalEmbedder) Dimensions() int { return e.dims }
func (e *LocalEmbedder) Model() string   { return "local-hash-v1" }
func (e *LocalEmbedder) Name() string    { return "local" }

func (e *LocalEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, t := range texts {
		out[i] = e.embedOne(t)
	}
	return out, nil
}

func (e *LocalEmbedder) embedOne(text string) []float32 {
	vec := make([]float32, e.dims)
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		// Avoid a zero vector (undefined cosine); seed a stable unit value.
		vec[0] = 1
		return vec
	}

	addFeature := func(token string, weight float32) {
		h := fnv.New32a()
		_, _ = h.Write([]byte(token))
		idx := int(h.Sum32()) % e.dims
		if idx < 0 {
			idx += e.dims
		}
		vec[idx] += weight
	}

	// Word unigrams.
	for _, w := range strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	}) {
		addFeature("w:"+w, 1)
		// Character trigrams capture morphology and tolerate small variations.
		runes := []rune(w)
		for j := 0; j+3 <= len(runes); j++ {
			addFeature("t:"+string(runes[j:j+3]), 0.5)
		}
	}

	return l2normalize(vec)
}

func l2normalize(v []float32) []float32 {
	var sum float64
	for _, x := range v {
		sum += float64(x) * float64(x)
	}
	if sum == 0 {
		v[0] = 1
		return v
	}
	inv := float32(1.0 / math.Sqrt(sum))
	for i := range v {
		v[i] *= inv
	}
	return v
}
