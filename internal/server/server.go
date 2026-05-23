package server

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/jholhewres/anchored_oss/internal/config"
	"github.com/jholhewres/anchored_oss/internal/handler"
	"github.com/jholhewres/anchored_oss/internal/middleware"
	"github.com/jholhewres/anchored_oss/internal/policy"
	"github.com/jholhewres/anchored_oss/internal/store"
	syncpkg "github.com/jholhewres/anchored_oss/internal/sync"
	"github.com/jholhewres/anchored_oss/internal/version"
)

const defaultBodyLimit = 1 << 20 // 1MB

type Server struct {
	cfg    *config.Config
	store  store.Store
	logger *slog.Logger
	http   *http.Server
}

func New(cfg *config.Config, st store.Store, logger *slog.Logger) *Server {
	mux := http.NewServeMux()

	healthHandler := handler.NewHealthHandler(version.Version, st)
	mux.HandleFunc("GET /v1/health", healthHandler.ServeHTTP)

	authMW := middleware.Auth(st, logger)
	requireAdmin := middleware.RequireScope("admin")

	apiKeyHandler := handler.NewAPIKeyHandler(st, logger)
	mux.HandleFunc("POST /v1/api-keys", authMW(requireAdmin(http.HandlerFunc(apiKeyHandler.Create))).ServeHTTP)
	mux.HandleFunc("DELETE /v1/api-keys/{id}", authMW(requireAdmin(http.HandlerFunc(apiKeyHandler.Revoke))).ServeHTTP)

	projectHandler := handler.NewProjectHandler(st, logger)
	mux.HandleFunc("POST /v1/projects", authMW(requireAdmin(http.HandlerFunc(projectHandler.Create))).ServeHTTP)
	mux.HandleFunc("GET /v1/projects", authMW(http.HandlerFunc(projectHandler.List)).ServeHTTP)

	syncEngine := syncpkg.NewSyncEngine(st, policy.NewContentFilter(), logger)
	syncHandler := handler.NewSyncHandler(syncEngine, st, logger)
	mux.HandleFunc("POST /v1/sync", authMW(http.HandlerFunc(syncHandler.ServeHTTP)).ServeHTTP)

	var h http.Handler = mux
	h = middleware.BodyLimit(defaultBodyLimit)(h)
	h = middleware.Recovery(h)
	h = middleware.Logging(h)
	h = middleware.CORS(cfg.CORS.AllowedOrigins)(h)

	return &Server{
		cfg:   cfg,
		store: st,
		http: &http.Server{
			Addr:         cfg.Server.Address,
			Handler:      h,
			ReadTimeout:  cfg.Server.ReadTimeout,
			WriteTimeout: cfg.Server.WriteTimeout,
		},
		logger: logger,
	}
}

func (s *Server) Start() error {
	s.logger.Info("server listening", "address", s.cfg.Server.Address)
	if err := s.http.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.http.Shutdown(ctx)
}
