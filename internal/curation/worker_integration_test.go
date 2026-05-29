//go:build integration

package curation

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jholhewres/anchored_oss/internal/ai/embeddings"
	"github.com/jholhewres/anchored_oss/internal/config"
	"github.com/jholhewres/anchored_oss/internal/model"
	"github.com/jholhewres/anchored_oss/internal/store"
)

var testSeq uint64

// unique yields a per-run-unique suffix (time + counter) so repeated runs
// against the same persistent database don't collide on unique slugs/emails.
func unique() string {
	return strconv.FormatInt(time.Now().UnixNano(), 36) + strconv.FormatUint(atomic.AddUint64(&testSeq, 1), 10)
}

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// TestWorkerEmbedsAndSemanticSearch drives the full Fase 1 wiring against a
// real pgvector Postgres: UpsertMemory enqueues curation, the worker scores and
// embeds each memory, and a vector KNN search returns the most relevant one.
// Requires ANCHORED_TEST_DSN pointing at a pgvector-enabled database.
func TestWorkerEmbedsAndSemanticSearch(t *testing.T) {
	dsn := os.Getenv("ANCHORED_TEST_DSN")
	if dsn == "" {
		t.Skip("ANCHORED_TEST_DSN not set; skipping curation/embeddings integration test")
	}

	st, err := store.NewPostgresStore(store.PoolConfig{DSN: dsn})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	ctx := context.Background()
	org, err := st.CreateOrganization(ctx, "WorkOrg"+unique(), "workorg-"+unique())
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	acc, err := st.CreateAccount(ctx, "work-"+unique()+"@example.com", "Work", "x")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	proj, err := st.CreateProject(ctx, org.ID, "WorkProj", "workproj-"+unique(), "work-"+unique(), acc.ID, "service")
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	now := time.Now().UTC()
	deployContent := "we deploy the server with pm2 and a systemd-managed process"
	seed := []struct{ id, content string }{
		{"w-deploy-" + unique(), deployContent},
		{"w-cors-" + unique(), "cors allowed origins must list the dashboard domain explicitly"},
	}
	for _, s := range seed {
		m := &model.Memory{
			ID: s.id, ProjectID: proj.ID, Category: "decision",
			Content: s.content, ContentHash: s.id, AuthorID: acc.ID,
			AuthorName: "Work", CreatedAt: now, UpdatedAt: now,
			Metadata: map[string]any{"pinned": true},
		}
		if err := st.UpsertMemory(ctx, m); err != nil {
			t.Fatalf("upsert %s: %v", s.id, err)
		}
	}

	cfg := config.DefaultConfig()
	cfg.Embeddings = embeddings.DefaultConfig() // local, 384d
	emb0, err := embeddings.New(cfg.Embeddings, discardLogger())
	if err != nil {
		t.Fatalf("build embedder: %v", err)
	}
	w := NewWorker(cfg, st, emb0, discardLogger())
	if err := w.processBatch(ctx); err != nil {
		t.Fatalf("process batch: %v", err)
	}

	emb := embeddings.NewLocalEmbedder(384)
	qvec, err := embeddings.EmbedOne(ctx, emb, "how is the server deployed in production")
	if err != nil {
		t.Fatalf("embed query: %v", err)
	}
	results, err := st.SearchMemoriesByVector(ctx, proj.ID, qvec, 5)
	if err != nil {
		t.Fatalf("vector search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected embedded memories to be searchable, got none")
	}
	if results[0].Content != deployContent {
		t.Fatalf("expected deploy memory ranked first, got %q", results[0].Content)
	}
}
