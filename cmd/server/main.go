package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jholhewres/anchored_oss/internal/ai/embeddings"
	"github.com/jholhewres/anchored_oss/internal/auth"
	"github.com/jholhewres/anchored_oss/internal/config"
	"github.com/jholhewres/anchored_oss/internal/curation"
	"github.com/jholhewres/anchored_oss/internal/server"
	"github.com/jholhewres/anchored_oss/internal/setup"
	"github.com/jholhewres/anchored_oss/internal/store"
	"github.com/jholhewres/anchored_oss/internal/version"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	addr := flag.String("addr", "", "override server address")
	bootstrap := flag.Bool("bootstrap", false, "create default org, admin account, and API key, then exit")
	setupFlag := flag.Bool("setup", false, "run interactive setup wizard")
	reindex := flag.Bool("reindex", false, "backfill embeddings for all memories lacking a vector, then exit")
	flag.Parse()

	if *setupFlag {
		if err := setup.RunInteractive(); err != nil {
			slog.Error("setup failed", "error", err)
			os.Exit(1)
		}
		return
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("failed to load configuration", "error", err)
		os.Exit(1)
	}

	if *addr != "" {
		cfg.Server.Address = *addr
	}

	if cfg.Database.Driver != "sqlite" && cfg.Database.Driver != "postgres" {
		slog.Error("invalid database.driver: must be 'sqlite' or 'postgres'", "driver", cfg.Database.Driver)
		os.Exit(1)
	}

	var st store.Store
	if cfg.Database.Driver == "sqlite" {
		st, err = store.NewSQLiteStore(cfg.Database.DSN)
	} else {
		st, err = store.NewPostgresStore(store.PoolConfig{
			DSN:             cfg.Database.DSN,
			MaxOpenConns:    cfg.Database.MaxOpenConns,
			MaxIdleConns:    cfg.Database.MaxIdleConns,
			ConnMaxLifetime: cfg.Database.ConnMaxLifetime,
		})
	}
	if err != nil {
		slog.Error("failed to open database", "error", err)
		os.Exit(1)
	}
	defer st.Close()

	if *bootstrap {
		if err := runBootstrap(st); err != nil {
			slog.Error("bootstrap failed", "error", err)
			os.Exit(1)
		}
		return
	}

	if *reindex {
		if err := runReindex(context.Background(), st, cfg, logger); err != nil {
			slog.Error("reindex failed", "error", err)
			os.Exit(1)
		}
		return
	}

	slog.Info("starting anchored-oss",
		"version", version.Version,
		"address", cfg.Server.Address,
	)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Build the embedder once and share it across the server handlers and the
	// curation worker. The onnx provider keeps a ~470MB model resident, so a
	// single instance avoids multiplying memory use. A bad config degrades to
	// "no embeddings" rather than blocking startup.
	embedder, err := embeddings.New(cfg.Embeddings, logger)
	if err != nil {
		slog.Error("embeddings disabled (config error)", "error", err)
		embedder = nil
	}
	if embedder != nil {
		slog.Info("embeddings enabled", "provider", embedder.Name(), "model", embedder.Model(), "dims", embedder.Dimensions())
		if closer, ok := embedder.(interface{ Close() error }); ok {
			defer closer.Close()
		}
	}

	srv := server.New(ctx, cfg, st, embedder, logger)

	if cfg.Curation.WorkerEnabled {
		w := curation.NewWorker(cfg, st, embedder, logger)
		go w.Start(ctx)
	}

	if cfg.Audit.RetentionDays > 0 {
		go runAuditPurge(ctx, st, cfg, logger)
	}

	go func() {
		if err := srv.Start(); err != nil {
			logger.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown error", "error", err)
	}

	slog.Info("shutdown complete")
}

// runAuditPurge periodically deletes audit entries older than the configured
// retention window, bounding unbounded audit_log growth. Runs an initial sweep
// immediately, then every cfg.Audit.PurgeInterval until ctx is cancelled.
func runAuditPurge(ctx context.Context, st store.Store, cfg *config.Config, logger *slog.Logger) {
	interval := cfg.Audit.PurgeInterval
	if interval <= 0 {
		interval = 6 * time.Hour
	}
	purge := func() {
		cutoff := time.Now().UTC().AddDate(0, 0, -cfg.Audit.RetentionDays)
		n, err := st.PurgeAuditOlderThan(ctx, cutoff)
		if err != nil {
			logger.Error("audit purge failed", "error", err)
			return
		}
		if n > 0 {
			logger.Info("audit purge", "removed", n, "older_than", cutoff)
		}
		// Rejection counters back the memory-health view; 90 days is plenty.
		statsCutoff := time.Now().UTC().AddDate(0, 0, -90).Format("2006-01-02")
		if n, err := st.PurgeRejectionStatsOlderThan(ctx, statsCutoff); err != nil {
			logger.Error("rejection stats purge failed", "error", err)
		} else if n > 0 {
			logger.Info("rejection stats purge", "removed", n, "older_than_day", statsCutoff)
		}
	}
	purge()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			purge()
		}
	}
}

// runReindex backfills embeddings for every memory that lacks a vector. Unlike
// the curation worker (which only embeds freshly-enqueued "ok" memories), this
// covers an existing corpus — e.g. memories synced before embeddings shipped,
// whose curation_queue rows are already 'done'. Pages by id so a mid-run
// failure still makes forward progress.
func runReindex(ctx context.Context, st store.Store, cfg *config.Config, logger *slog.Logger) error {
	embedder, err := embeddings.New(cfg.Embeddings, logger)
	if err != nil {
		return fmt.Errorf("build embedder: %w", err)
	}
	if closer, ok := embedder.(interface{ Close() error }); ok {
		defer closer.Close()
	}
	if embedder == nil {
		logger.Info("reindex: embeddings disabled in config; nothing to do")
		return nil
	}
	logger.Info("reindex: starting", "provider", embedder.Name(), "model", embedder.Model(), "dims", embedder.Dimensions())

	model := embedder.Model()
	var afterID string
	total, failures := 0, 0
	for {
		// Model-aware: re-embeds rows that are missing a vector OR were produced
		// by a different model, so switching embeddings providers backfills the
		// whole corpus into a single consistent vector space.
		batch, err := st.MemoriesStaleEmbedding(ctx, model, afterID, 200)
		if err != nil {
			return fmt.Errorf("list memories stale embedding: %w", err)
		}
		if len(batch) == 0 {
			break
		}
		for _, m := range batch {
			afterID = m.ID // advance the cursor even on failure to guarantee progress
			vec, err := embeddings.EmbedOne(ctx, embedder, m.Content)
			if err != nil {
				failures++
				logger.Warn("reindex: embed failed", "memory_id", m.ID, "error", err)
				continue
			}
			if err := st.UpdateMemoryEmbedding(ctx, m.ID, vec, embedder.Model()); err != nil {
				failures++
				logger.Warn("reindex: store embedding failed", "memory_id", m.ID, "error", err)
				continue
			}
			total++
		}
		logger.Info("reindex: progress", "embedded", total, "failures", failures)
	}
	logger.Info("reindex: done", "embedded", total, "failures", failures)
	return nil
}

func runBootstrap(s store.Store) error {
	ctx := context.Background()

	org, err := s.CreateOrganization(ctx, "default", "default")
	if err != nil {
		return fmt.Errorf("create organization: %w", err)
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte("changeme"), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	account, err := s.CreateAccount(ctx, "admin@anchored.local", "Admin", string(passwordHash))
	if err != nil {
		return fmt.Errorf("create account: %w", err)
	}

	if err := s.AddOrgMember(ctx, org.ID, account.ID, "admin"); err != nil {
		return fmt.Errorf("add org member: %w", err)
	}

	if err := s.EnsureDefaultTeamMembership(ctx, org.ID, account.ID); err != nil {
		return fmt.Errorf("ensure default team membership: %w", err)
	}

	full, prefix, hash, err := auth.GenerateAPIKey()
	if err != nil {
		return fmt.Errorf("generate api key: %w", err)
	}

	if _, err := s.CreateAPIKey(ctx, org.ID, account.ID, "admin", prefix, hash, "admin", nil); err != nil {
		return fmt.Errorf("create api key: %w", err)
	}

	fmt.Println(full)
	fmt.Fprintf(os.Stderr, "Admin credentials: admin@anchored.local / changeme\n")
	return nil
}
