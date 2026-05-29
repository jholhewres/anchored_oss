//go:build onnx

// Package embeddings ONNX integration test. Builds only under `-tags onnx`
// (same tag as the embedder itself) and needs the real model + runtime on disk;
// it skips when neither is present. Point it at an existing model directory
// (default ~/.anchored/data/onnx, matching the anchored CLI's layout) to avoid
// a ~470MB download:
//
//	go test -tags onnx ./internal/ai/embeddings/ -run ONNX -v
//
// Override with ANCHORED_ONNX_MODEL_DIR if the model lives elsewhere.
package embeddings

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func onnxModelDir(t *testing.T) string {
	t.Helper()
	if d := os.Getenv("ANCHORED_ONNX_MODEL_DIR"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("no home dir: %v", err)
	}
	dir := filepath.Join(home, ".anchored", "data", "onnx")
	if _, err := os.Stat(filepath.Join(dir, onnxModelName, "model.onnx")); err != nil {
		t.Skipf("model not found at %s (set ANCHORED_ONNX_MODEL_DIR): %v", dir, err)
	}
	return dir
}

func cos(a, b []float32) float64 {
	var s float64
	for i := range a {
		s += float64(a[i]) * float64(b[i])
	}
	return s // vectors are L2-normalized => dot == cosine
}

func TestONNXEmbedder_RealModel(t *testing.T) {
	dir := onnxModelDir(t)
	e, err := NewONNXEmbedder(dir, nil)
	if err != nil {
		t.Fatalf("NewONNXEmbedder: %v", err)
	}
	defer e.Close()

	if e.Dimensions() != 384 {
		t.Fatalf("expected 384 dims, got %d", e.Dimensions())
	}
	if e.Name() != "onnx" {
		t.Fatalf("expected name onnx, got %q", e.Name())
	}

	ctx := context.Background()

	// Shape + L2 normalization.
	vecs, err := e.Embed(ctx, []string{"deploy do servidor com pm2", ""})
	if err != nil {
		t.Fatalf("embed: %v", err)
	}
	for i, v := range vecs {
		if len(v) != 384 {
			t.Fatalf("vector %d wrong width %d", i, len(v))
		}
	}
	if n := math.Sqrt(cos(vecs[0], vecs[0])); math.Abs(n-1) > 1e-3 {
		t.Fatalf("not L2-normalized: norm=%f", n)
	}

	// Determinism.
	a, _ := EmbedOne(ctx, e, "anchored compartilha memória do time")
	b, _ := EmbedOne(ctx, e, "anchored compartilha memória do time")
	if cos(a, b) < 0.9999 {
		t.Fatalf("same input should be identical, cos=%f", cos(a, b))
	}

	// Parity lock: the same four sentences embedded by the anchored CLI's
	// memory.ONNXEmbedder (identical model + ported tokenizer/pooling code)
	// produce these exact pairwise cosines. Asserting them here guards against
	// any drift between the two implementations — if a future change to the
	// tokenizer or pooling diverges from the CLI, the shared pgvector space
	// would silently corrupt and this catches it. Reference values captured via
	// `go test -tags onnx_parity ./pkg/memory -run TestParityDump` in anchored.
	s0, _ := EmbedOne(ctx, e, "como fazemos deploy do servidor em produção")
	s1, _ := EmbedOne(ctx, e, "o servidor é publicado com pm2 e systemd")
	s2, _ := EmbedOne(ctx, e, "o front-end usa um design system próprio")
	s3, _ := EmbedOne(ctx, e, "how is the server deployed in production")

	parity := []struct {
		name string
		got  float64
		want float64
	}{
		{"cos(deploy_pt, pm2_pt)", cos(s0, s1), 0.923621},
		{"cos(deploy_pt, design_pt)", cos(s0, s2), 0.920595},
		{"cos(deploy_en, pm2_pt)", cos(s3, s1), 0.922049},
		{"cos(deploy_en, design_pt)", cos(s3, s2), 0.933139},
	}
	for _, p := range parity {
		if math.Abs(p.got-p.want) > 1e-3 {
			t.Errorf("parity drift %s: got %.6f want %.6f (Δ=%.6f)", p.name, p.got, p.want, p.got-p.want)
		} else {
			t.Logf("parity ok %s: %.6f", p.name, p.got)
		}
	}
}
