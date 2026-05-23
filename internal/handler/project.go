package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"

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

type listMemoriesResponse struct {
	Memories []*model.Memory `json:"memories"`
	Total    int             `json:"total"`
	Limit    int             `json:"limit"`
	Offset   int             `json:"offset"`
}

// slugRe matches lowercase kebab-case slugs, 1..64 chars, starting alphanumeric.
var slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

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
	if !slugRe.MatchString(req.Slug) {
		jsonError(w, http.StatusBadRequest, "slug must match ^[a-z0-9][a-z0-9-]{0,63}$")
		return
	}

	project, err := h.store.CreateProject(r.Context(), orgID, req.Name, req.Slug, req.RemoteKey, accountID)
	if err != nil {
		h.logger.Error("create project failed", "error", err, "org_id", orgID)
		jsonError(w, http.StatusInternalServerError, "failed to create project")
		return
	}

	if err := h.store.EnsureCreatorProjectAccess(r.Context(), orgID, accountID, project.ID); err != nil {
		h.logger.Error("grant creator access failed", "error", err, "project_id", project.ID)
		jsonError(w, http.StatusInternalServerError, "failed to grant project access")
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

// Get returns a single active project. Requires admin scope or team-access.
func (h *ProjectHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !uuidRe.MatchString(id) {
		jsonError(w, http.StatusBadRequest, "id must be a UUID")
		return
	}
	if !h.checkAccess(w, r, id) {
		return
	}
	project, err := h.store.GetActiveProjectByID(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		jsonError(w, http.StatusNotFound, "project not found")
		return
	}
	if err != nil {
		h.logger.Error("get project failed", "error", err, "project_id", id)
		jsonError(w, http.StatusInternalServerError, "get project failed")
		return
	}
	jsonResponse(w, http.StatusOK, project)
}

// ListMemories returns a paginated memory list for an active project.
func (h *ProjectHandler) ListMemories(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !uuidRe.MatchString(id) {
		jsonError(w, http.StatusBadRequest, "id must be a UUID")
		return
	}
	if !h.checkAccess(w, r, id) {
		return
	}

	q := r.URL.Query()
	limit := 20
	offset := 0
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			jsonError(w, http.StatusBadRequest, "limit must be an integer")
			return
		}
		limit = n
	}
	if v := q.Get("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			jsonError(w, http.StatusBadRequest, "offset must be an integer")
			return
		}
		offset = n
	}

	memories, total, err := h.store.ListMemoriesPaginated(r.Context(), id, limit, offset)
	if errors.Is(err, store.ErrNotFound) {
		jsonError(w, http.StatusNotFound, "project not found")
		return
	}
	if err != nil {
		h.logger.Error("list memories failed", "error", err, "project_id", id)
		jsonError(w, http.StatusInternalServerError, "list memories failed")
		return
	}
	if memories == nil {
		memories = []*model.Memory{}
	}
	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	jsonResponse(w, http.StatusOK, listMemoriesResponse{
		Memories: memories, Total: total, Limit: limit, Offset: offset,
	})
}

// SoftDelete marks a project as deleted. Admin only.
func (h *ProjectHandler) SoftDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !uuidRe.MatchString(id) {
		jsonError(w, http.StatusBadRequest, "id must be a UUID")
		return
	}
	if err := h.store.SoftDeleteProject(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			jsonError(w, http.StatusNotFound, "project not found")
			return
		}
		h.logger.Error("soft delete project failed", "error", err, "project_id", id)
		jsonError(w, http.StatusInternalServerError, "soft delete project failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// checkAccess enforces team-based access for non-admin callers. Returns true
// when the request should continue. On failure it writes the response and
// returns false.
func (h *ProjectHandler) checkAccess(w http.ResponseWriter, r *http.Request, projectID string) bool {
	scope := middleware.GetScope(r.Context())
	if scope == "admin" {
		return true
	}
	accountID := middleware.GetAccountID(r.Context())
	if accountID == "" {
		jsonError(w, http.StatusUnauthorized, "missing account context")
		return false
	}
	ok, err := h.store.HasProjectAccess(r.Context(), accountID, projectID)
	if err != nil {
		h.logger.Error("project access check failed", "error", err, "project_id", projectID)
		jsonError(w, http.StatusInternalServerError, "access check failed")
		return false
	}
	if !ok {
		jsonError(w, http.StatusForbidden, "no team access to this project")
		return false
	}
	return true
}
