package handler

import (
	"log/slog"
	"net/http"

	"github.com/jholhewres/anchored_oss/internal/middleware"
	"github.com/jholhewres/anchored_oss/internal/store"
)

type StatsHandler struct {
	store  store.Store
	logger *slog.Logger
}

func NewStatsHandler(st store.Store, logger *slog.Logger) *StatsHandler {
	return &StatsHandler{store: st, logger: logger}
}

func (h *StatsHandler) Get(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgID(r.Context())
	if orgID == "" {
		jsonError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	stats, err := h.store.GetDashboardStats(r.Context(), orgID)
	if err != nil {
		h.logger.Error("stats lookup failed", "error", err)
		jsonError(w, http.StatusInternalServerError, "stats lookup failed")
		return
	}
	jsonResponse(w, http.StatusOK, stats)
}
