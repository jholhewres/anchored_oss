package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/jholhewres/anchored_oss/internal/middleware"
	"github.com/jholhewres/anchored_oss/internal/model"
	"github.com/jholhewres/anchored_oss/internal/store"
	syncpkg "github.com/jholhewres/anchored_oss/internal/sync"
)

type SyncHandler struct {
	engine *syncpkg.SyncEngine
	store  store.Store
	logger *slog.Logger
}

func NewSyncHandler(engine *syncpkg.SyncEngine, st store.Store, logger *slog.Logger) *SyncHandler {
	return &SyncHandler{
		engine: engine,
		store:  st,
		logger: logger,
	}
}

func (h *SyncHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req model.SyncRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "INVALID_REQUEST")
		return
	}

	if req.ProjectID == "" && req.ProjectClaim == nil {
		jsonError(w, http.StatusBadRequest, "INVALID_REQUEST")
		return
	}

	scope := middleware.GetScope(r.Context())
	if scope == "readonly" && (len(req.Pushes) > 0 || len(req.Tombstones) > 0) {
		jsonError(w, http.StatusForbidden, "FORBIDDEN")
		return
	}

	accountID := middleware.GetAccountID(r.Context())
	orgID := middleware.GetOrgID(r.Context())

	resp, err := h.engine.Sync(r.Context(), accountID, orgID, &req)
	if err != nil {
		h.writeSyncError(w, err)
		return
	}

	jsonResponse(w, http.StatusOK, resp)
}

func (h *SyncHandler) writeSyncError(w http.ResponseWriter, err error) {
	var se *syncpkg.SyncError
	if errors.As(err, &se) {
		jsonError(w, se.Status, se.Code)
		return
	}
	h.logger.Error("sync failed", "error", err)
	jsonError(w, http.StatusInternalServerError, "INTERNAL_ERROR")
}
