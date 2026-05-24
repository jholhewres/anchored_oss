package handler

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jholhewres/anchored_oss/internal/config"
	"github.com/jholhewres/anchored_oss/internal/middleware"
	"github.com/jholhewres/anchored_oss/internal/model"
	"github.com/jholhewres/anchored_oss/internal/policy"
	"github.com/jholhewres/anchored_oss/internal/store"
)

var allowedCategories = map[string]bool{
	"fact":     true,
	"decision": true,
	"plan":     true,
	"summary":  true,
	"learning": true,
}

type MemoryHandler struct {
	store  store.Store
	filter *policy.ContentFilter
	cfg    *config.Config
	logger *slog.Logger
}

func NewMemoryHandler(st store.Store, filter *policy.ContentFilter, cfg *config.Config, logger *slog.Logger) *MemoryHandler {
	return &MemoryHandler{store: st, filter: filter, cfg: cfg, logger: logger}
}

type createMemoryRequest struct {
	ProjectID    string              `json:"project_id"`
	ProjectClaim *model.ProjectClaim `json:"project_claim"`
	ID           string              `json:"id,omitempty"`
	Category     string              `json:"category"`
	Content      string              `json:"content"`
	Keywords     []string            `json:"keywords,omitempty"`
	Source       string              `json:"source,omitempty"`
}

func (h *MemoryHandler) Create(w http.ResponseWriter, r *http.Request) {
	accountID := middleware.GetAccountID(r.Context())
	orgID := middleware.GetOrgID(r.Context())

	var req createMemoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "INVALID_REQUEST")
		return
	}

	if req.Content == "" {
		jsonError(w, http.StatusBadRequest, "content is required")
		return
	}

	if req.Category == "" {
		jsonError(w, http.StatusBadRequest, "category is required")
		return
	}

	if req.Category == "event" || req.Category == "preference" {
		jsonError(w, http.StatusBadRequest, "category \""+req.Category+"\" is not allowed")
		return
	}

	if !allowedCategories[req.Category] {
		jsonError(w, http.StatusBadRequest, "invalid category: must be one of fact, decision, plan, summary, learning")
		return
	}

	results := h.filter.Filter([]policy.Filterable{
		{Content: req.Content, Category: req.Category},
	})
	if len(results) > 0 && !results[0].Accepted {
		jsonError(w, http.StatusBadRequest, results[0].Detail)
		return
	}

	projectID, ok := h.resolveProject(w, r, orgID, accountID, &req)
	if !ok {
		return
	}

	if middleware.GetScope(r.Context()) == "readonly" {
		jsonError(w, http.StatusForbidden, "FORBIDDEN")
		return
	}

	if h.cfg.IsCloud() && h.cfg.Quota.MaxStorageBytes > 0 {
		usage, err := h.store.GetOrgStorageBytes(r.Context(), orgID)
		if err != nil {
			h.logger.Error("quota check failed", "error", err)
			jsonError(w, http.StatusInternalServerError, "quota check failed")
			return
		}
		if usage >= h.cfg.Quota.MaxStorageBytes {
			jsonError(w, http.StatusForbidden, "QUOTA_EXCEEDED")
			return
		}
	}

	now := time.Now().UTC()
	id := req.ID
	if id == "" {
		id = newMemoryID()
	}

	authorName := ""
	if accountID != "" {
		if acc, aerr := h.store.GetAccountByID(r.Context(), accountID); aerr == nil && acc != nil {
			authorName = acc.DisplayName
		}
	}

	mem := &model.Memory{
		ID:          id,
		ProjectID:   projectID,
		Category:    req.Category,
		Content:     req.Content,
		ContentHash: "sha256:" + sha256Hex(req.Content),
		Keywords:    req.Keywords,
		Source:      req.Source,
		AuthorID:    accountID,
		AuthorName:  authorName,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := h.store.UpsertMemory(r.Context(), mem); err != nil {
		h.logger.Error("upsert memory failed", "error", err, "project_id", projectID)
		jsonError(w, http.StatusInternalServerError, "failed to create memory")
		return
	}

	jsonResponse(w, http.StatusCreated, mem)
}

func (h *MemoryHandler) Search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	projectID := q.Get("project_id")
	query := strings.TrimSpace(q.Get("q"))

	if projectID == "" {
		jsonError(w, http.StatusBadRequest, "project_id is required")
		return
	}
	if query == "" {
		jsonError(w, http.StatusBadRequest, "q is required")
		return
	}

	limit := 20
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			jsonError(w, http.StatusBadRequest, "limit must be an integer")
			return
		}
		limit = n
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	if !h.checkProjectAccess(w, r, projectID) {
		return
	}

	memories, err := h.store.SearchMemories(r.Context(), projectID, query, limit)
	if err != nil {
		h.logger.Error("search memories failed", "error", err, "project_id", projectID)
		jsonError(w, http.StatusInternalServerError, "search failed")
		return
	}
	if memories == nil {
		memories = []*model.Memory{}
	}

	jsonResponse(w, http.StatusOK, memories)
}

func (h *MemoryHandler) resolveProject(w http.ResponseWriter, r *http.Request, orgID, accountID string, req *createMemoryRequest) (string, bool) {
	if req.ProjectID != "" {
		p, err := h.store.GetActiveProjectByID(r.Context(), req.ProjectID)
		if errors.Is(err, store.ErrNotFound) {
			jsonError(w, http.StatusNotFound, "project not found")
			return "", false
		}
		if err != nil {
			h.logger.Error("project lookup failed", "project_id", req.ProjectID, "error", err)
			jsonError(w, http.StatusInternalServerError, "project lookup failed")
			return "", false
		}
		if p.OrgID != orgID {
			jsonError(w, http.StatusForbidden, "project belongs to a different organization")
			return "", false
		}
		return p.ID, true
	}

	claim := req.ProjectClaim
	if claim == nil {
		jsonError(w, http.StatusBadRequest, "project_id or project_claim is required")
		return "", false
	}

	if claim.RemoteKey == "" || claim.Name == "" {
		jsonError(w, http.StatusBadRequest, "project_claim requires name and remote_key")
		return "", false
	}

	existing, err := h.store.GetProjectByRemoteKey(r.Context(), orgID, claim.RemoteKey)
	if err == nil {
		return existing.ID, true
	}
	if !errors.Is(err, store.ErrNotFound) {
		h.logger.Error("project by remote_key failed", "remote_key", claim.RemoteKey, "error", err)
		jsonError(w, http.StatusInternalServerError, "project lookup failed")
		return "", false
	}

	slug := toSlug(claim.Name)
	if slug == "" {
		jsonError(w, http.StatusBadRequest, "project_claim.name does not produce a valid slug")
		return "", false
	}

	proj, err := h.store.CreateProject(r.Context(), orgID, claim.Name, slug, claim.RemoteKey, accountID)
	if err != nil {
		h.logger.Error("project creation from claim failed", "remote_key", claim.RemoteKey, "error", err)
		jsonError(w, http.StatusInternalServerError, "failed to create project")
		return "", false
	}

	if err := h.store.EnsureCreatorProjectAccess(r.Context(), orgID, accountID, proj.ID); err != nil {
		h.logger.Error("grant creator access failed", "project_id", proj.ID, "error", err)
		jsonError(w, http.StatusInternalServerError, "failed to grant project access")
		return "", false
	}

	return proj.ID, true
}

func (h *MemoryHandler) checkProjectAccess(w http.ResponseWriter, r *http.Request, projectID string) bool {
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

func toSlug(name string) string {
	s := strings.ToLower(name)
	s = strings.ReplaceAll(s, " ", "-")
	var result strings.Builder
	for _, c := range s {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
			result.WriteRune(c)
		}
	}
	return strings.Trim(result.String(), "-")
}

func newMemoryID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
