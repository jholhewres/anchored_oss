package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/jholhewres/anchored_oss/internal/ai/embeddings"
	"github.com/jholhewres/anchored_oss/internal/config"
	"github.com/jholhewres/anchored_oss/internal/model"
	"github.com/jholhewres/anchored_oss/internal/store"
)

type fixedTestEmbedder struct {
	name  string
	model string
	dims  int
}

func (e fixedTestEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	vectors := make([][]float32, len(texts))
	for i := range texts {
		vectors[i] = make([]float32, e.dims)
		if e.dims > 0 {
			vectors[i][0] = 1
		}
	}
	return vectors, nil
}

func (e fixedTestEmbedder) Dimensions() int { return e.dims }
func (e fixedTestEmbedder) Model() string   { return e.model }
func (e fixedTestEmbedder) Name() string    { return e.name }

type reindexTestStore struct {
	store.Store
	requiredDims int
	fixed        bool
	staleCalls   int
	staleModel   string
	staleDims    int
	updates      []string
}

func (s *reindexTestStore) EmbeddingDimensionConstraint() (int, bool) {
	return s.requiredDims, s.fixed
}

func (s *reindexTestStore) MemoriesStaleEmbeddingSpace(
	_ context.Context,
	embedModel string,
	dims int,
	_ string,
	_ int,
) ([]*model.Memory, error) {
	s.staleCalls++
	s.staleModel = embedModel
	s.staleDims = dims
	if s.staleCalls == 1 {
		return []*model.Memory{{ID: "memory-1", Content: "content to reindex"}}, nil
	}
	return nil, nil
}

func (*reindexTestStore) SearchMemoriesByVectorSpace(context.Context, string, []float32, string, int, int) ([]*model.Memory, error) {
	return nil, nil
}

func (s *reindexTestStore) UpdateMemoryEmbedding(_ context.Context, memoryID string, vec []float32, model string) error {
	s.updates = append(s.updates, fmt.Sprintf("%s:%s:%d", memoryID, model, len(vec)))
	return nil
}

func TestValidateEmbeddingCompatibility(t *testing.T) {
	tests := []struct {
		name      string
		st        store.Store
		embedder  embeddings.Embedder
		wantError string
	}{
		{
			name: "postgres accepts vector 384",
			st:   &reindexTestStore{requiredDims: 384, fixed: true},
			embedder: fixedTestEmbedder{
				name: "test", model: "model-384", dims: 384,
			},
		},
		{
			name: "postgres rejects mismatched dimensions",
			st:   &reindexTestStore{requiredDims: 384, fixed: true},
			embedder: fixedTestEmbedder{
				name: "openai", model: "large-model", dims: 1536,
			},
			wantError: "Postgres vector(384)",
		},
		{
			name: "sqlite explicitly allows dynamic dimensions",
			st:   &reindexTestStore{fixed: false},
			embedder: fixedTestEmbedder{
				name: "test", model: "model-1536", dims: 1536,
			},
		},
		{
			name:     "provider none remains valid",
			st:       &reindexTestStore{requiredDims: 384, fixed: true},
			embedder: nil,
		},
		{
			name: "invalid provider dimensions fail",
			st:   &reindexTestStore{fixed: false},
			embedder: fixedTestEmbedder{
				name: "broken", model: "broken", dims: 0,
			},
			wantError: "dimensions must be > 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateEmbeddingCompatibility(tt.st, tt.embedder)
			if tt.wantError == "" {
				if err != nil {
					t.Fatalf("unexpected validation error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("validation error = %v, want substring %q", err, tt.wantError)
			}
		})
	}
}

func TestRunReindexWithUsesModelAndDimensions(t *testing.T) {
	st := &reindexTestStore{requiredDims: 384, fixed: true}
	embedder := fixedTestEmbedder{name: "test", model: "active-model", dims: 384}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	if err := runReindexWith(context.Background(), st, embedder, logger); err != nil {
		t.Fatalf("run reindex: %v", err)
	}
	if st.staleModel != "active-model" || st.staleDims != 384 {
		t.Fatalf("stale query space = model %q dims %d", st.staleModel, st.staleDims)
	}
	if len(st.updates) != 1 || st.updates[0] != "memory-1:active-model:384" {
		t.Fatalf("updates = %v", st.updates)
	}
}

func TestRunReindexWithRejectsMismatchBeforeScanning(t *testing.T) {
	st := &reindexTestStore{requiredDims: 384, fixed: true}
	embedder := fixedTestEmbedder{name: "openai", model: "large-model", dims: 1536}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	err := runReindexWith(context.Background(), st, embedder, logger)
	if err == nil || !strings.Contains(err.Error(), "vector(384)") {
		t.Fatalf("reindex error = %v, want actionable vector(384) mismatch", err)
	}
	if st.staleCalls != 0 || len(st.updates) != 0 {
		t.Fatalf("reindex touched store before validation: stale=%d updates=%v", st.staleCalls, st.updates)
	}
}

func TestRunReindexWithProviderNoneIsNoop(t *testing.T) {
	st := &reindexTestStore{requiredDims: 384, fixed: true}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	embedder, err := embeddings.New(embeddings.Config{Provider: "none"}, logger)
	if err != nil {
		t.Fatalf("build provider none: %v", err)
	}
	if err := runReindexWith(context.Background(), st, embedder, logger); err != nil {
		t.Fatalf("provider none reindex: %v", err)
	}
	if st.staleCalls != 0 {
		t.Fatalf("provider none queried stale vectors %d times", st.staleCalls)
	}
	if !embeddingProviderDisabled(embeddings.Config{Provider: " NONE "}) {
		t.Fatal("provider none was not recognized as explicitly disabled")
	}
}

func TestBuildConfiguredEmbedderFailsClosedUnlessExplicitlyDisabled(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	embedder, err := buildConfiguredEmbedder(
		embeddings.Config{Provider: "none"},
		logger,
	)
	if err != nil || embedder != nil {
		t.Fatalf("explicit none = (%v, %v), want (nil, nil)", embedder, err)
	}

	embedder, err = buildConfiguredEmbedder(
		embeddings.Config{Provider: "unknown-provider"},
		logger,
	)
	if err == nil || embedder != nil {
		t.Fatalf("invalid provider = (%v, %v), want (nil, error)", embedder, err)
	}
}

type purgeRetentionStore struct {
	store.Store
	auditBefore       time.Time
	idempotencyBefore time.Time
	done              chan struct{}
}

func (s *purgeRetentionStore) PurgeAuditOlderThan(_ context.Context, before time.Time) (int64, error) {
	s.auditBefore = before
	return 0, nil
}

func (s *purgeRetentionStore) PurgeMemoryIdempotencyOlderThan(_ context.Context, before time.Time) (int64, error) {
	s.idempotencyBefore = before
	return 0, nil
}

func (s *purgeRetentionStore) PurgeRejectionStatsOlderThan(_ context.Context, _ string) (int64, error) {
	close(s.done)
	return 0, nil
}

func TestRunAuditPurgeKeepsIndependentIdempotencyRetention(t *testing.T) {
	st := &purgeRetentionStore{done: make(chan struct{})}
	cfg := config.DefaultConfig()
	cfg.Audit.RetentionDays = 7
	cfg.Audit.PurgeInterval = time.Hour
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx, cancel := context.WithCancel(context.Background())

	go runAuditPurge(ctx, st, cfg, logger)
	select {
	case <-st.done:
		cancel()
	case <-time.After(time.Second):
		cancel()
		t.Fatal("initial purge did not complete")
	}

	if got, want := st.auditBefore.Sub(st.idempotencyBefore), 83*24*time.Hour; got != want {
		t.Fatalf("audit/idempotency cutoff distance = %s, want %s", got, want)
	}
}
