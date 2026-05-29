//go:build !onnx

package embeddings

import (
	"fmt"
	"log/slog"
)

// newONNXEmbedder is the stub linked into the default (pure-Go, static) build.
// The real onnx provider depends on github.com/yalue/onnxruntime_go, which uses
// cgo — so it is gated behind `-tags onnx` (and CGO_ENABLED=1). Selecting
// provider: onnx on a binary built without that tag returns this clear error
// rather than silently falling back.
func newONNXEmbedder(_ string, _ *slog.Logger) (Embedder, error) {
	return nil, fmt.Errorf("embeddings: provider \"onnx\" is not compiled into this build; rebuild with `-tags onnx` (requires CGO), or use provider local/openai")
}
