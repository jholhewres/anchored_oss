package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/jholhewres/anchored_oss/internal/store"
)

type HealthHandler struct {
	version string
	store   store.Store
}

func NewHealthHandler(version string, st store.Store) *HealthHandler {
	return &HealthHandler{
		version: version,
		store:   st,
	}
}

func (h *HealthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	dbStatus := "unavailable"
	if h.store != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		if err := h.store.Ping(ctx); err == nil {
			dbStatus = "ok"
		}
		cancel()
	}

	jsonResponse(w, http.StatusOK, map[string]string{
		"service":   "anchored-oss",
		"version":   h.version,
		"status":    "ok",
		"db_status": dbStatus,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}
