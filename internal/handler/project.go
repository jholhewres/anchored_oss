package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/jholhewres/anchored_oss/internal/middleware"
	"github.com/jholhewres/anchored_oss/internal/model"
	"github.com/jholhewres/anchored_oss/internal/store"
)

type ProjectHandler struct {
	store  store.Store
	logger *slog.Logger
}

func NewProjectHandler(st store.Store, logger *slog.Logger) *ProjectHandler {
	return &ProjectHandler{store: st, logger: logger}
}

type createProjectRequest struct {
	Name      string `json:"name"`
	Slug      string `json:"slug"`
	RemoteKey string `json:"remote_key"`
}

func (h *ProjectHandler) Create(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgID(r.Context())
	accountID := middleware.GetAccountID(r.Context())
	if orgID == "" || accountID == "" {
		jsonError(w, http.StatusUnauthorized, "missing org or account context")
		return
	}

	var req createProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		jsonError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Slug == "" {
		jsonError(w, http.StatusBadRequest, "slug is required")
		return
	}

	project, err := h.store.CreateProject(r.Context(), orgID, req.Name, req.Slug, req.RemoteKey, accountID)
	if err != nil {
		h.logger.Error("create project failed", "error", err, "org_id", orgID)
		jsonError(w, http.StatusInternalServerError, "failed to create project")
		return
	}

	jsonResponse(w, http.StatusCreated, project)
}

func (h *ProjectHandler) List(w http.ResponseWriter, r *http.Request) {
	accountID := middleware.GetAccountID(r.Context())
	if accountID == "" {
		jsonError(w, http.StatusUnauthorized, "missing account context")
		return
	}

	projects, err := h.store.ListProjectsByTeamAccess(r.Context(), accountID)
	if err != nil {
		h.logger.Error("list projects failed", "error", err, "account_id", accountID)
		jsonError(w, http.StatusInternalServerError, "failed to list projects")
		return
	}

	if projects == nil {
		projects = make([]*model.Project, 0)
	}

	jsonResponse(w, http.StatusOK, projects)
}
