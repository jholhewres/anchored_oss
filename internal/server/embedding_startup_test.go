package server

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/jholhewres/anchored_oss/internal/ai/embeddings"
	"github.com/jholhewres/anchored_oss/internal/config"
	"github.com/jholhewres/anchored_oss/internal/store"
)

type constrainedStartupStore struct {
	store.Store
}

func (*constrainedStartupStore) EmbeddingDimensionConstraint() (int, bool) {
	return store.PostgresEmbeddingDimensions, true
}

type startupEmbedder struct {
	dims int
}

func (e startupEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	return make([][]float32, len(texts)), nil
}
func (e startupEmbedder) Dimensions() int { return e.dims }
func (startupEmbedder) Model() string     { return "startup-test" }
func (startupEmbedder) Name() string      { return "test" }

var _ embeddings.Embedder = startupEmbedder{}

func TestStartRejectsEmbeddingWidthBeforeListening(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Server.Address = "127.0.0.1:0"
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := New(
		context.Background(),
		cfg,
		&constrainedStartupStore{},
		startupEmbedder{dims: 1536},
		logger,
	)

	err := server.Start()
	if err == nil || !strings.Contains(err.Error(), "vector(384)") {
		t.Fatalf("Start error = %v, want vector(384) configuration error before listen", err)
	}
}
