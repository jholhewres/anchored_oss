package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jholhewres/anchored_oss/internal/config"
	"github.com/jholhewres/anchored_oss/internal/server"
	"github.com/jholhewres/anchored_oss/internal/store"
	"github.com/jholhewres/anchored_oss/internal/version"
)

var versionFlag = "dev"

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	addr := flag.String("addr", "", "override server address")
	bootstrap := flag.Bool("bootstrap", false, "create default org, admin account, and API key, then exit")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		cfg, err = config.LoadFromEnv()
		if err != nil {
			slog.Error("failed to load configuration", "error", err)
			os.Exit(1)
		}
	}

	if *bootstrap {
		if err := runBootstrap(cfg); err != nil {
			slog.Error("bootstrap failed", "error", err)
			os.Exit(1)
		}
		return
	}

	if *addr != "" {
		cfg.Server.Address = *addr
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	slog.Info("starting anchored-oss",
		"version", versionFlag,
		"core_version", version.Version,
		"address", cfg.Server.Address,
	)

	srv := server.New(cfg, nil, logger)

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

func runBootstrap(cfg *config.Config) error {
	ctx := context.Background()

	s, err := store.NewPostgresStore(cfg.Database.DSN)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer s.Close()

	org, err := s.CreateOrganization(ctx, "default", "default")
	if err != nil {
		return fmt.Errorf("create organization: %w", err)
	}

	account, err := s.CreateAccount(ctx, "admin@anchored.local", "Admin")
	if err != nil {
		return fmt.Errorf("create account: %w", err)
	}

	if err := s.AddOrgMember(ctx, org.ID, account.ID, "admin"); err != nil {
		return fmt.Errorf("add org member: %w", err)
	}

	full, prefix, hash, err := generateAPIKey()
	if err != nil {
		return fmt.Errorf("generate api key: %w", err)
	}

	if _, err := s.CreateAPIKey(ctx, org.ID, account.ID, "admin", prefix, hash, "admin", nil); err != nil {
		return fmt.Errorf("create api key: %w", err)
	}

	fmt.Println(full)
	return nil
}

func generateAPIKey() (full, prefix, hash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", "", err
	}
	hexBytes := hex.EncodeToString(b)
	full = "anc_live_" + hexBytes
	prefix = full[:12]

	h256 := sha256.Sum256([]byte(full))
	hash = hex.EncodeToString(h256[:])

	return full, prefix, hash, nil
}
