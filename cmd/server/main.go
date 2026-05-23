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

	"github.com/jholhewres/anchored_oss/internal/auth"
	"github.com/jholhewres/anchored_oss/internal/config"
	"github.com/jholhewres/anchored_oss/internal/server"
	"github.com/jholhewres/anchored_oss/internal/store"
	"github.com/jholhewres/anchored_oss/internal/version"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	addr := flag.String("addr", "", "override server address")
	bootstrap := flag.Bool("bootstrap", false, "create default org, admin account, and API key, then exit")
	flag.Parse()

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

	st, err := store.NewPostgresStore(store.PoolConfig{
		DSN:             cfg.Database.DSN,
		MaxOpenConns:    cfg.Database.MaxOpenConns,
		MaxIdleConns:    cfg.Database.MaxIdleConns,
		ConnMaxLifetime: cfg.Database.ConnMaxLifetime,
	})
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

	slog.Info("starting anchored-oss",
		"version", version.Version,
		"address", cfg.Server.Address,
	)

	srv := server.New(cfg, st, logger)

	go func() {
		if err := srv.Start(); err != nil {
			logger.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	<-ctx.Done()
	slog.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown error", "error", err)
	}

	slog.Info("shutdown complete")
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
