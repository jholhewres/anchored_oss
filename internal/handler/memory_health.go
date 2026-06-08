package handler

import (
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/jholhewres/anchored_oss/internal/middleware"
	"github.com/jholhewres/anchored_oss/internal/model"
	"github.com/jholhewres/anchored_oss/internal/store"
)

// healthCacheTTL bounds how long a computed health view is reused. The
// aggregates are advisory and tolerate ~minute-old data; caching shields large
// orgs from recomputing the full scan on every dashboard poll/refresh.
const healthCacheTTL = 60 * time.Second

// healthCacheMaxEntries caps the cache so a long-lived process across many
// projects can't grow it unbounded; on overflow the cache is reset.
const healthCacheMaxEntries = 1000

type healthCacheEntry struct {
	val *model.MemoryHealth
	at  time.Time
}

// MemoryHealthHandler serves the anti context-poisoning health view: lifecycle
// counts, noisy sources, rejection pressure and volume anomalies.
type MemoryHealthHandler struct {
	store  store.Store
	logger *slog.Logger

	mu    sync.Mutex
	cache map[string]healthCacheEntry
}

func NewMemoryHealthHandler(st store.Store, logger *slog.Logger) *MemoryHealthHandler {
	return &MemoryHealthHandler{store: st, logger: logger, cache: make(map[string]healthCacheEntry)}
}

// cached returns a fresh (< TTL) cached health view for key, or computes and
// stores one. compute runs outside the lock so a slow aggregate never blocks
// other keys.
func (h *MemoryHealthHandler) cached(key string, compute func() (*model.MemoryHealth, error)) (*model.MemoryHealth, error) {
	h.mu.Lock()
	if e, ok := h.cache[key]; ok && time.Since(e.at) < healthCacheTTL {
		h.mu.Unlock()
		return e.val, nil
	}
	h.mu.Unlock()

	val, err := compute()
	if err != nil {
		return nil, err
	}

	h.mu.Lock()
	if len(h.cache) >= healthCacheMaxEntries {
		h.cache = make(map[string]healthCacheEntry)
	}
	h.cache[key] = healthCacheEntry{val: val, at: time.Now()}
	h.mu.Unlock()
	return val, nil
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

	health, err := h.cached("p:"+id, func() (*model.MemoryHealth, error) {
		return h.store.GetProjectMemoryHealth(r.Context(), id)
	})
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
	health, err := h.cached("o:"+orgID, func() (*model.MemoryHealth, error) {
		return h.store.GetOrgMemoryHealth(r.Context(), orgID)
	})
	if err != nil {
		h.logger.Error("org memory health failed", "error", err, "org_id", orgID)
		jsonError(w, http.StatusInternalServerError, "failed to compute memory health")
		return
	}
	jsonResponse(w, http.StatusOK, health)
}
