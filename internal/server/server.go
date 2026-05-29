package server

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/jholhewres/anchored_oss/internal/ai/embeddings"
	"github.com/jholhewres/anchored_oss/internal/config"
	"github.com/jholhewres/anchored_oss/internal/handler"
	"github.com/jholhewres/anchored_oss/internal/middleware"
	"github.com/jholhewres/anchored_oss/internal/policy"
	"github.com/jholhewres/anchored_oss/internal/store"
	syncpkg "github.com/jholhewres/anchored_oss/internal/sync"
	"github.com/jholhewres/anchored_oss/internal/version"
	"github.com/jholhewres/anchored_oss/internal/web"
)

// defaultBodyLimit caps a single request body. Sync push payloads can carry
// thousands of memories at a few KB each (initial sync of a full local
// store). 64 MiB is a generous upper bound while still preventing absurd
// allocations; make this configurable when client batching exists.
const defaultBodyLimit = 64 << 20 // 64 MiB

type Server struct {
	cfg    *config.Config
	store  store.Store
	logger *slog.Logger
	http   *http.Server
}

// New builds the HTTP server. embedder is the process-shared embedder (may be
// nil when embeddings are disabled); it is injected into the memory and chat
// handlers so a single instance is reused — the onnx provider keeps a ~470MB
// model resident, so building one per handler would multiply memory use.
func New(ctx context.Context, cfg *config.Config, st store.Store, embedder embeddings.Embedder, logger *slog.Logger) *Server {
	mux := http.NewServeMux()

	// Rate limiters: a global per-client bucket plus a stricter bucket guarding
	// the unauthenticated auth endpoints against brute force. Disabled when
	// cfg.RateLimit.Enabled is false (NewRateLimiter with 0 rpm = pass-through).
	globalRPM, authRPM := 0, 0
	if cfg.RateLimit.Enabled {
		globalRPM = cfg.RateLimit.RequestsPerMinute
		authRPM = cfg.RateLimit.AuthRequestsPerMinute
	}
	globalRL := middleware.NewRateLimiter(ctx, globalRPM, cfg.RateLimit.Burst)
	authRL := middleware.NewRateLimiter(ctx, authRPM, cfg.RateLimit.AuthBurst)

	healthHandler := handler.NewHealthHandler(version.Version, st)
	mux.HandleFunc("GET /v1/health", healthHandler.ServeHTTP)

	bootstrapStatusHandler := handler.NewBootstrapStatusHandler(st, logger)
	mux.HandleFunc("GET /v1/bootstrap-status", bootstrapStatusHandler.Get)

	onboardingHandler := handler.NewOnboardingHandler(st, logger)
	mux.HandleFunc("POST /v1/onboarding/complete", onboardingHandler.Complete)

	mux.Handle("GET /install", web.InstallHandler("anchored.sh"))
	mux.Handle("GET /install-oss", web.InstallHandler("anchored-oss.sh"))

	authHandler := handler.NewAuthHandler(st, logger)
	mux.Handle("POST /v1/auth/login", authRL.Middleware(http.HandlerFunc(authHandler.Login)))

	authMW := middleware.Auth(st, logger)
	requireAdmin := middleware.RequireScope("admin")

	meHandler := handler.NewMeHandler(st, logger)
	mux.HandleFunc("GET /v1/me", authMW(http.HandlerFunc(meHandler.Get)).ServeHTTP)

	statsHandler := handler.NewStatsHandler(st, logger)
	mux.HandleFunc("GET /v1/stats", authMW(requireAdmin(http.HandlerFunc(statsHandler.Get))).ServeHTTP)

	accountHandler := handler.NewAccountHandler(st, logger)
	mux.HandleFunc("GET /v1/accounts", authMW(requireAdmin(http.HandlerFunc(accountHandler.List))).ServeHTTP)
	mux.HandleFunc("POST /v1/accounts", authMW(requireAdmin(http.HandlerFunc(accountHandler.Create))).ServeHTTP)
	mux.HandleFunc("PATCH /v1/accounts/{id}", authMW(requireAdmin(http.HandlerFunc(accountHandler.Update))).ServeHTTP)
	mux.HandleFunc("DELETE /v1/accounts/{id}", authMW(requireAdmin(http.HandlerFunc(accountHandler.Delete))).ServeHTTP)
	mux.HandleFunc("GET /v1/accounts/{id}/projects", authMW(requireAdmin(http.HandlerFunc(accountHandler.ListProjects))).ServeHTTP)
	mux.HandleFunc("PUT /v1/accounts/{id}/projects", authMW(requireAdmin(http.HandlerFunc(accountHandler.SetProjects))).ServeHTTP)

	inviteHandler := handler.NewInviteHandler(st, logger)
	mux.HandleFunc("POST /v1/invites", authMW(requireAdmin(http.HandlerFunc(inviteHandler.Create))).ServeHTTP)
	mux.HandleFunc("GET /v1/invites", authMW(requireAdmin(http.HandlerFunc(inviteHandler.List))).ServeHTTP)
	mux.HandleFunc("DELETE /v1/invites/{id}", authMW(requireAdmin(http.HandlerFunc(inviteHandler.Revoke))).ServeHTTP)
	mux.HandleFunc("GET /v1/invites/accept/{token}", inviteHandler.Get)
	mux.HandleFunc("POST /v1/invites/accept/{token}", inviteHandler.Accept)

	teamHandler := handler.NewTeamHandler(st, logger)
	mux.HandleFunc("GET /v1/teams", authMW(http.HandlerFunc(teamHandler.List)).ServeHTTP)
	mux.HandleFunc("POST /v1/teams", authMW(requireAdmin(http.HandlerFunc(teamHandler.Create))).ServeHTTP)
	mux.HandleFunc("GET /v1/teams/{id}", authMW(http.HandlerFunc(teamHandler.GetDetail)).ServeHTTP)
	mux.HandleFunc("POST /v1/teams/{id}/members", authMW(requireAdmin(http.HandlerFunc(teamHandler.AddMember))).ServeHTTP)
	mux.HandleFunc("DELETE /v1/teams/{id}/members/{account_id}", authMW(requireAdmin(http.HandlerFunc(teamHandler.RemoveMember))).ServeHTTP)

	apiKeyHandler := handler.NewAPIKeyHandler(st, logger)
	mux.HandleFunc("GET /v1/api-keys", authMW(requireAdmin(http.HandlerFunc(apiKeyHandler.List))).ServeHTTP)
	mux.HandleFunc("POST /v1/api-keys", authMW(requireAdmin(http.HandlerFunc(apiKeyHandler.Create))).ServeHTTP)
	mux.HandleFunc("DELETE /v1/api-keys/{id}", authMW(requireAdmin(http.HandlerFunc(apiKeyHandler.Revoke))).ServeHTTP)

	projectHandler := handler.NewProjectHandler(st, logger)
	mux.HandleFunc("POST /v1/projects", authMW(requireAdmin(http.HandlerFunc(projectHandler.Create))).ServeHTTP)
	mux.HandleFunc("GET /v1/projects", authMW(http.HandlerFunc(projectHandler.List)).ServeHTTP)
	mux.HandleFunc("GET /v1/projects/{id}", authMW(http.HandlerFunc(projectHandler.Get)).ServeHTTP)
	mux.HandleFunc("GET /v1/projects/{id}/memories", authMW(http.HandlerFunc(projectHandler.ListMemories)).ServeHTTP)
	mux.HandleFunc("GET /v1/projects/{id}/graph", authMW(http.HandlerFunc(projectHandler.ListGraph)).ServeHTTP)
	mux.HandleFunc("POST /v1/projects/{id}/triples", authMW(http.HandlerFunc(projectHandler.IngestTriples)).ServeHTTP)
	mux.HandleFunc("DELETE /v1/projects/{id}", authMW(requireAdmin(http.HandlerFunc(projectHandler.SoftDelete))).ServeHTTP)

	auditHandler := handler.NewAuditHandler(st, logger)
	mux.HandleFunc("GET /v1/audit", authMW(requireAdmin(http.HandlerFunc(auditHandler.List))).ServeHTTP)

	quotaHandler := handler.NewQuotaHandler(st, cfg, logger)
	mux.HandleFunc("GET /v1/quota", authMW(requireAdmin(http.HandlerFunc(quotaHandler.Get))).ServeHTTP)

	policyHandler := handler.NewPolicyHandler(st, logger)
	mux.HandleFunc("GET /v1/policies", authMW(requireAdmin(http.HandlerFunc(policyHandler.Get))).ServeHTTP)
	mux.HandleFunc("PUT /v1/policies", authMW(requireAdmin(http.HandlerFunc(policyHandler.Update))).ServeHTTP)

	chatHandler := handler.NewChatHandler(st, cfg, embedder, logger)
	mux.HandleFunc("GET /v1/chat/status", authMW(http.HandlerFunc(chatHandler.Status)).ServeHTTP)
	mux.HandleFunc("POST /v1/chat", authMW(http.HandlerFunc(chatHandler.Complete)).ServeHTTP)

	syncEngine := syncpkg.NewSyncEngine(st, policy.NewContentFilter(), logger)
	syncHandler := handler.NewSyncHandler(syncEngine, st, logger)
	mux.HandleFunc("POST /v1/sync", authMW(http.HandlerFunc(syncHandler.ServeHTTP)).ServeHTTP)
	// Compat split-protocol routes for the anchored CLI client.
	mux.HandleFunc("POST /api/v1/sync/push", authMW(http.HandlerFunc(syncHandler.CompatPush)).ServeHTTP)
	mux.HandleFunc("POST /api/v1/sync/pull", authMW(http.HandlerFunc(syncHandler.CompatPull)).ServeHTTP)

	memoryHandler := handler.NewMemoryHandler(st, policy.NewContentFilter(), cfg, embedder, logger)
	mux.HandleFunc("POST /v1/memories", authMW(http.HandlerFunc(memoryHandler.Create)).ServeHTTP)
	mux.HandleFunc("GET /v1/memories/search", authMW(http.HandlerFunc(memoryHandler.Search)).ServeHTTP)

	registerHandler := handler.NewRegisterHandler(st, logger)
	mux.Handle("POST /v1/auth/register", authRL.Middleware(http.HandlerFunc(registerHandler.Register)))

	// SPA fallback. The handler internally returns 404 JSON for /v1/* and
	// /api/* paths so the dashboard never masks an API typo.
	spa, err := web.NewSPAHandler()
	if err != nil {
		// Compile-time embed ensures dist/index.html exists; fall back to
		// a tiny inline placeholder if for any reason the embed read fails.
		logger.Error("spa init failed", "error", err)
	} else {
		mux.Handle("GET /", spa)
	}

	var h http.Handler = mux
	h = middleware.BodyLimit(defaultBodyLimit)(h)
	h = middleware.Recovery(h)
	h = middleware.Logging(h)
	h = globalRL.Middleware(h)
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
