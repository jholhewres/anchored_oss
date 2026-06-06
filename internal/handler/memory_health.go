package handler

import (
	"log/slog"
	"net/http"

	"github.com/jholhewres/anchored_oss/internal/middleware"
	"github.com/jholhewres/anchored_oss/internal/store"
)

// MemoryHealthHandler serves the anti context-poisoning health view: lifecycle
// counts, noisy sources, rejection pressure and volume anomalies.
type MemoryHealthHandler struct {
	store  store.Store
	logger *slog.Logger
}

func NewMemoryHealthHandler(st store.Store, logger *slog.Logger) *MemoryHealthHandler {
	return &MemoryHealthHandler{store: st, logger: logger}
}

// Project handles GET /v1/projects/{id}/memory-health. Any authenticated key
// with team access to the project (admin bypasses) can read it.
func (h *MemoryHealthHandler) Project(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !uuidRe.MatchString(id) {
		jsonError(w, http.StatusBadRequest, "id must be a UUID")
		return
	}

	scope := middleware.GetScope(r.Context())
	if scope != "admin" {
		accountID := middleware.GetAccountID(r.Context())
		if accountID == "" {
			jsonError(w, http.StatusUnauthorized, "missing account context")
			return
		}
		ok, err := h.store.HasProjectAccess(r.Context(), accountID, id)
		if err != nil {
			h.logger.Error("health access check failed", "error", err, "project_id", id)
			jsonError(w, http.StatusInternalServerError, "access check failed")
			return
		}
		if !ok {
			jsonError(w, http.StatusForbidden, "no team access to this project")
			return
		}
	}

	health, err := h.store.GetProjectMemoryHealth(r.Context(), id)
	if err != nil {
		h.logger.Error("project memory health failed", "error", err, "project_id", id)
		jsonError(w, http.StatusInternalServerError, "failed to compute memory health")
		return
	}
	jsonResponse(w, http.StatusOK, health)
}

// Org handles GET /v1/orgs/memory-health (admin only; org from the token).
func (h *MemoryHealthHandler) Org(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgID(r.Context())
	if orgID == "" {
		jsonError(w, http.StatusUnauthorized, "missing org context")
		return
	}
	health, err := h.store.GetOrgMemoryHealth(r.Context(), orgID)
	if err != nil {
		h.logger.Error("org memory health failed", "error", err, "org_id", orgID)
		jsonError(w, http.StatusInternalServerError, "failed to compute memory health")
		return
	}
	jsonResponse(w, http.StatusOK, health)
}
