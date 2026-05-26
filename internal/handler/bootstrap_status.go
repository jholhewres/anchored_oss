package handler

import (
	"log/slog"
	"net/http"

	"github.com/jholhewres/anchored_oss/internal/store"
)

type BootstrapStatusHandler struct {
	store  store.Store
	logger *slog.Logger
}

func NewBootstrapStatusHandler(st store.Store, logger *slog.Logger) *BootstrapStatusHandler {
	return &BootstrapStatusHandler{store: st, logger: logger}
}

type bootstrapStatusResponse struct {
	Bootstrapped bool `json:"bootstrapped"`
}

// Get returns whether the instance has at least one organization configured.
// Used by the SPA root router to decide between /onboarding and /login.
// Public — no auth required, since the SPA needs to call this before login.
func (h *BootstrapStatusHandler) Get(w http.ResponseWriter, r *http.Request) {
	count, err := h.store.CountOrganizations(r.Context())
	if err != nil {
		h.logger.Error("bootstrap status: count orgs failed", "error", err)
		jsonError(w, http.StatusInternalServerError, "internal error")
		return
	}
	jsonResponse(w, http.StatusOK, bootstrapStatusResponse{Bootstrapped: count > 0})
}
