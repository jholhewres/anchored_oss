//go:build onnx

package embeddings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestONNXArtifactRevisionUsesCachedArtifactBytes(t *testing.T) {
	dir := t.TempDir()
	paths := &onnxPaths{
		ModelFile:     filepath.Join(dir, "model.onnx"),
		TokenizerFile: filepath.Join(dir, "tokenizer.json"),
		VocabFile:     filepath.Join(dir, "vocab.txt"),
	}
	for path, content := range map[string]string{
		paths.ModelFile: "model-a", paths.TokenizerFile: "tokenizer-a",
		paths.VocabFile: "vocab-a",
	} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	first, err := onnxArtifactRevision(paths)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.ModelFile, []byte("model-b"), 0o600); err != nil {
		t.Fatal(err)
	}
	second, err := onnxArtifactRevision(paths)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatalf("cached model bytes did not change semantic identity: %q", first)
	}
	otherPipeline, err := onnxArtifactRevisionForPipeline(paths, "different-pipeline")
	if err != nil {
		t.Fatal(err)
	}
	if second == otherPipeline {
		t.Fatalf("pipeline contract did not change semantic identity: %q", second)
	}
	if !strings.HasPrefix(second, "sha256:") {
		t.Fatalf("artifact revision = %q, want sha256 identity", second)
	}
}

func TestONNXAssetURLsUsePinnedModelFamily(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		model, tokenizer, vocab := onnxAssetURLs(legacy)
		for _, url := range []string{model, tokenizer, vocab} {
			if strings.Contains(url, "/resolve/main/") {
				t.Fatalf("unpinned ONNX asset URL: %s", url)
			}
		}
	}
}
