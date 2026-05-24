package handler

import (
	"log/slog"
	"net/http"

	"github.com/jholhewres/anchored_oss/internal/config"
	"github.com/jholhewres/anchored_oss/internal/middleware"
	"github.com/jholhewres/anchored_oss/internal/store"
)

type QuotaHandler struct {
	store  store.Store
	cfg    *config.Config
	logger *slog.Logger
}

func NewQuotaHandler(st store.Store, cfg *config.Config, logger *slog.Logger) *QuotaHandler {
	return &QuotaHandler{store: st, cfg: cfg, logger: logger}
}

type QuotaResponse struct {
	StorageBytes    int64 `json:"storage_bytes"`
	MaxStorageBytes int64 `json:"max_storage_bytes"`
	MemoryCount     int   `json:"memory_count"`
}

func (h *QuotaHandler) Get(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgID(r.Context())
	if orgID == "" {
		jsonError(w, http.StatusUnauthorized, "UNAUTHORIZED")
		return
	}

	storageBytes, err := h.store.GetOrgStorageBytes(r.Context(), orgID)
	if err != nil {
		h.logger.Error("quota lookup failed", "error", err)
		jsonError(w, http.StatusInternalServerError, "quota lookup failed")
		return
	}

	stats, err := h.store.GetDashboardStats(r.Context(), orgID)
	if err != nil {
		h.logger.Error("stats lookup failed", "error", err)
		jsonError(w, http.StatusInternalServerError, "stats lookup failed")
		return
	}

	jsonResponse(w, http.StatusOK, QuotaResponse{
		StorageBytes:    storageBytes,
		MaxStorageBytes: h.cfg.Quota.MaxStorageBytes,
		MemoryCount:     stats.MemoriesLive,
	})
}
