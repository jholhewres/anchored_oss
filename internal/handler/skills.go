package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/jholhewres/anchored_oss/internal/middleware"
	"github.com/jholhewres/anchored_oss/internal/model"
	anchoredskill "github.com/jholhewres/anchored_oss/internal/skill"
	"github.com/jholhewres/anchored_oss/internal/store"
)

const (
	maxSkillContentBytes           = 256 << 10
	maxSkillAttachmentRequestBytes = 16 << 10
	maxSkillQueryBytes             = 500
)

var skillSlugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

type SkillHandler struct {
	store  store.Store
	logger *slog.Logger
}

func NewSkillHandler(st store.Store, logger *slog.Logger) *SkillHandler {
	return &SkillHandler{store: st, logger: logger}
}

type createSkillRequest struct {
	Name        string          `json:"name"`
	Slug        string          `json:"slug"`
	Purpose     string          `json:"purpose"`
	Frontmatter json.RawMessage `json:"frontmatter"`
	Body        *string         `json:"body"`
	// Content is retained for clients which created legacy, raw-SKILL.md rows.
	Content    *string `json:"content"`
	TemplateID string  `json:"template_id"`
	Status     string  `json:"status"`
}

type patchSkillRequest struct {
	Name        *string         `json:"name"`
	Purpose     *string         `json:"purpose"`
	Frontmatter json.RawMessage `json:"frontmatter"`
	Body        *string         `json:"body"`
	Content     *string         `json:"content"`
	Status      *string         `json:"status"`
}

type replaceProjectSkillsRequest struct {
	SkillIDs []string `json:"skill_ids"`
}

type runtimeSkillResponse struct {
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	Slug         string          `json:"slug"`
	Purpose      string          `json:"purpose"`
	Status       string          `json:"status"`
	ProjectCount int             `json:"project_count"`
	Version      int             `json:"version"`
	ContentHash  string          `json:"content_hash"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
	AttachedAt   *time.Time      `json:"attached_at,omitempty"`
	Frontmatter  *map[string]any `json:"frontmatter,omitempty"`
	Body         string          `json:"body,omitempty"`
	Content      string          `json:"content,omitempty"`
}

func validSkillStatus(status string) bool {
	switch status {
	case model.SkillStatusDraft, model.SkillStatusActive, model.SkillStatusDeprecated:
		return true
	}
	return false
}

func validateSkillFields(name, slug, purpose, content, status string) string {
	if name == "" {
		return "name is required"
	}
	if len(name) > 120 {
		return "name must be at most 120 characters"
	}
	if !skillSlugRe.MatchString(slug) {
		return "slug must match ^[a-z0-9][a-z0-9-]{0,63}$"
	}
	if purpose == "" {
		return "purpose is required"
	}
	if len(purpose) > 2000 {
		return "purpose must be at most 2000 characters"
	}
	if strings.TrimSpace(content) == "" {
		return "content is required"
	}
	if len(content) > maxSkillContentBytes {
		return "content exceeds 256 KiB"
	}
	if !validSkillStatus(status) {
		return "status must be one of: draft, active, deprecated"
	}
	return ""
}

func parseSkillFrontmatter(raw json.RawMessage) (*map[string]any, error) {
	if raw == nil {
		return nil, nil
	}
	frontmatter, err := anchoredskill.ParseJSONFrontmatter(raw)
	if err != nil {
		return nil, err
	}
	return &frontmatter, nil
}

func validateSkillBody(body string) string {
	if strings.TrimSpace(body) == "" {
		return "body is required"
	}
	if len(body) > maxSkillContentBytes {
		return "body exceeds 256 KiB"
	}
	return ""
}

func chooseSkillBody(body, content *string) (*string, string) {
	if body != nil && content != nil && *body != *content {
		return nil, "body and content must match when both are provided"
	}
	if body != nil {
		return body, ""
	}
	return content, ""
}

func (h *SkillHandler) ListTemplates(w http.ResponseWriter, r *http.Request) {
	if middleware.GetOrgID(r.Context()) == "" {
		jsonError(w, http.StatusUnauthorized, "missing org context")
		return
	}
	jsonResponse(w, http.StatusOK, anchoredskill.Templates())
}

func (h *SkillHandler) List(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgID(r.Context())
	if orgID == "" {
		jsonError(w, http.StatusUnauthorized, "missing org context")
		return
	}
	skills, err := h.store.ListSkills(r.Context(), orgID)
	if err != nil {
		h.logger.Error("list skills failed", "error", err)
		jsonError(w, http.StatusInternalServerError, "list skills failed")
		return
	}
	response := make([]runtimeSkillResponse, 0, len(skills))
	for _, skill := range skills {
		response = append(response, runtimeSkill(skill, false))
	}
	jsonResponse(w, http.StatusOK, response)
}

func (h *SkillHandler) Create(w http.ResponseWriter, r *http.Request) {
	orgID, actorID := middleware.GetOrgID(r.Context()), middleware.GetAccountID(r.Context())
	if orgID == "" || actorID == "" {
		jsonError(w, http.StatusUnauthorized, "missing org or account context")
		return
	}
	var req createSkillRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxSkillContentBytes+(32<<10)+4096)).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	req.Name, req.Slug, req.Purpose, req.Status = strings.TrimSpace(req.Name), strings.TrimSpace(req.Slug), strings.TrimSpace(req.Purpose), strings.TrimSpace(req.Status)
	if req.Status == "" {
		req.Status = model.SkillStatusDraft
	}
	var template anchoredskill.Template
	if req.TemplateID != "" {
		var ok bool
		template, ok = anchoredskill.FindTemplate(strings.TrimSpace(req.TemplateID), "")
		if !ok {
			jsonError(w, http.StatusBadRequest, "unknown template_id")
			return
		}
		if req.Name == "" {
			req.Name = template.Name
		}
		if req.Slug == "" {
			req.Slug = template.Slug
		}
		if req.Purpose == "" {
			req.Purpose = template.Purpose
		}
		if req.Body == nil && req.Content == nil {
			body := template.Body
			req.Body = &body
		}
		if req.Frontmatter == nil {
			frontmatter, err := json.Marshal(anchoredskill.Header(req.Slug, req.Purpose))
			if err != nil {
				h.logger.Error("marshal built-in skill template failed", "error", err)
				jsonError(w, http.StatusInternalServerError, "create skill failed")
				return
			}
			req.Frontmatter = frontmatter
		}
	}
	body, msg := chooseSkillBody(req.Body, req.Content)
	if msg != "" {
		jsonError(w, http.StatusBadRequest, msg)
		return
	}
	if body == nil {
		jsonError(w, http.StatusBadRequest, "body or content is required")
		return
	}
	if msg := validateSkillFields(req.Name, req.Slug, req.Purpose, *body, req.Status); msg != "" {
		jsonError(w, http.StatusBadRequest, msg)
		return
	}
	frontmatter, err := parseSkillFrontmatter(req.Frontmatter)
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	canonical := req.Body != nil || req.Frontmatter != nil || req.TemplateID != ""
	if canonical && frontmatter == nil {
		header := anchoredskill.Header(req.Slug, req.Purpose)
		frontmatter = &header
	}
	if frontmatter != nil && ((*frontmatter)["name"] != req.Slug || (*frontmatter)["description"] != req.Purpose) {
		jsonError(w, http.StatusBadRequest, "frontmatter name and description must match slug and purpose")
		return
	}
	skill := &model.Skill{OrgID: orgID, Name: req.Name, Slug: req.Slug, Purpose: req.Purpose, Status: req.Status, CreatedBy: actorID}
	if canonical {
		skill.CurrentVersion = &model.SkillVersion{Body: *body, Frontmatter: *frontmatter}
	}
	if err := h.store.CreateSkill(r.Context(), skill, *body); errors.Is(err, store.ErrConflict) {
		jsonError(w, http.StatusConflict, "slug already in use")
		return
	} else if err != nil {
		h.logger.Error("create skill failed", "error", err)
		jsonError(w, http.StatusInternalServerError, "create skill failed")
		return
	}
	h.audit(r, "skill.create", skill.ID, map[string]any{"slug": skill.Slug, "version": skill.CurrentVersion.Version, "status": skill.Status})
	jsonResponse(w, http.StatusCreated, runtimeSkill(skill, true))
}

func (h *SkillHandler) Get(w http.ResponseWriter, r *http.Request) {
	orgID, id := middleware.GetOrgID(r.Context()), r.PathValue("id")
	if orgID == "" {
		jsonError(w, http.StatusUnauthorized, "missing org context")
		return
	}
	if !uuidRe.MatchString(id) {
		jsonError(w, http.StatusBadRequest, "id must be a UUID")
		return
	}
	skill, err := h.store.GetSkill(r.Context(), orgID, id)
	if errors.Is(err, store.ErrNotFound) {
		jsonError(w, http.StatusNotFound, "skill not found")
		return
	}
	if err != nil {
		h.logger.Error("get skill failed", "error", err)
		jsonError(w, http.StatusInternalServerError, "get skill failed")
		return
	}
	jsonResponse(w, http.StatusOK, runtimeSkill(skill, true))
}

func (h *SkillHandler) Update(w http.ResponseWriter, r *http.Request) {
	orgID, actorID, id := middleware.GetOrgID(r.Context()), middleware.GetAccountID(r.Context()), r.PathValue("id")
	if orgID == "" || actorID == "" {
		jsonError(w, http.StatusUnauthorized, "missing org or account context")
		return
	}
	if !uuidRe.MatchString(id) {
		jsonError(w, http.StatusBadRequest, "id must be a UUID")
		return
	}
	var req patchSkillRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxSkillContentBytes+(32<<10)+4096)).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Name == nil && req.Purpose == nil && req.Body == nil && req.Frontmatter == nil && req.Content == nil && req.Status == nil {
		jsonError(w, http.StatusBadRequest, "at least one field is required")
		return
	}
	if req.Name != nil {
		*req.Name = strings.TrimSpace(*req.Name)
		if *req.Name == "" || len(*req.Name) > 120 {
			jsonError(w, http.StatusBadRequest, "name must be 1 to 120 characters")
			return
		}
	}
	if req.Purpose != nil {
		*req.Purpose = strings.TrimSpace(*req.Purpose)
		if *req.Purpose == "" || len(*req.Purpose) > 2000 {
			jsonError(w, http.StatusBadRequest, "purpose must be 1 to 2000 characters")
			return
		}
	}
	body, msg := chooseSkillBody(req.Body, req.Content)
	if msg != "" {
		jsonError(w, http.StatusBadRequest, msg)
		return
	}
	// content is the old opaque-SKILL.md alias. Once a row is canonical its
	// content includes generated frontmatter, so accepting it as a body would
	// nest a second document header. New clients must send body instead.
	if req.Content != nil && req.Body == nil && req.Frontmatter == nil {
		current, getErr := h.store.GetSkill(r.Context(), orgID, id)
		if errors.Is(getErr, store.ErrNotFound) {
			jsonError(w, http.StatusNotFound, "skill not found")
			return
		}
		if getErr != nil {
			h.logger.Error("load skill for legacy content update failed", "error", getErr)
			jsonError(w, http.StatusInternalServerError, "update skill failed")
			return
		}
		if current.CurrentVersion != nil && current.CurrentVersion.Frontmatter != nil {
			jsonError(w, http.StatusBadRequest, "body is required for canonical skills")
			return
		}
	}
	if body != nil {
		if msg := validateSkillBody(*body); msg != "" {
			jsonError(w, http.StatusBadRequest, msg)
			return
		}
	}
	if req.Status != nil {
		*req.Status = strings.TrimSpace(*req.Status)
		if !validSkillStatus(*req.Status) {
			jsonError(w, http.StatusBadRequest, "status must be one of: draft, active, deprecated; use DELETE to revoke")
			return
		}
	}
	frontmatter, err := parseSkillFrontmatter(req.Frontmatter)
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	if frontmatter != nil {
		current, getErr := h.store.GetSkill(r.Context(), orgID, id)
		if errors.Is(getErr, store.ErrNotFound) {
			jsonError(w, http.StatusNotFound, "skill not found")
			return
		}
		if getErr != nil {
			h.logger.Error("load skill for frontmatter validation failed", "error", getErr)
			jsonError(w, http.StatusInternalServerError, "update skill failed")
			return
		}
		purpose := current.Purpose
		if req.Purpose != nil {
			purpose = *req.Purpose
		}
		if (*frontmatter)["name"] != current.Slug || (*frontmatter)["description"] != purpose {
			jsonError(w, http.StatusBadRequest, "frontmatter name and description must match slug and purpose")
			return
		}
	}
	skill, err := h.store.UpdateSkill(r.Context(), orgID, id, actorID, model.SkillUpdate{Name: req.Name, Purpose: req.Purpose, Frontmatter: frontmatter, Body: req.Body, Content: req.Content, Status: req.Status})
	if errors.Is(err, store.ErrNotFound) {
		jsonError(w, http.StatusNotFound, "skill not found")
		return
	}
	if err != nil {
		h.logger.Error("update skill failed", "error", err)
		jsonError(w, http.StatusInternalServerError, "update skill failed")
		return
	}
	if req.Content != nil || req.Body != nil || req.Frontmatter != nil || req.Purpose != nil {
		h.audit(r, "skill.publish", id, map[string]any{"slug": skill.Slug, "version": skill.CurrentVersion.Version, "affected_projects": skill.ProjectCount})
	}
	if req.Status != nil {
		h.audit(r, "skill.status", id, map[string]any{"status": skill.Status})
	}
	jsonResponse(w, http.StatusOK, runtimeSkill(skill, true))
}

func (h *SkillHandler) Delete(w http.ResponseWriter, r *http.Request) {
	orgID, id := middleware.GetOrgID(r.Context()), r.PathValue("id")
	if orgID == "" {
		jsonError(w, http.StatusUnauthorized, "missing org context")
		return
	}
	if !uuidRe.MatchString(id) {
		jsonError(w, http.StatusBadRequest, "id must be a UUID")
		return
	}
	if err := h.store.RevokeSkill(r.Context(), orgID, id); errors.Is(err, store.ErrNotFound) {
		jsonError(w, http.StatusNotFound, "skill not found")
		return
	} else if err != nil {
		h.logger.Error("revoke skill failed", "error", err)
		jsonError(w, http.StatusInternalServerError, "revoke skill failed")
		return
	}
	h.audit(r, "skill.revoke", id, nil)
	w.WriteHeader(http.StatusNoContent)
}

func (h *SkillHandler) ListProject(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if !h.projectReadAccess(w, r, projectID) {
		return
	}
	query := r.URL.Query().Get("q")
	if len(query) > maxSkillQueryBytes {
		jsonError(w, http.StatusBadRequest, "q must be at most 500 characters")
		return
	}
	skills, err := h.store.ListProjectSkills(r.Context(), projectID, query, 20)
	if err != nil {
		h.logger.Error("list project skills failed", "error", err)
		jsonError(w, http.StatusInternalServerError, "list project skills failed")
		return
	}
	if skills == nil {
		skills = []*model.SkillDescriptor{}
	}
	jsonResponse(w, http.StatusOK, skills)
}

func (h *SkillHandler) GetProject(w http.ResponseWriter, r *http.Request) {
	projectID, slug := r.PathValue("id"), strings.TrimSpace(r.PathValue("slug"))
	if !skillSlugRe.MatchString(slug) {
		jsonError(w, http.StatusBadRequest, "slug must match ^[a-z0-9][a-z0-9-]{0,63}$")
		return
	}
	if !h.projectReadAccess(w, r, projectID) {
		return
	}
	skill, err := h.store.GetProjectSkill(r.Context(), projectID, slug)
	if errors.Is(err, store.ErrNotFound) {
		jsonError(w, http.StatusNotFound, "skill not found")
		return
	}
	if err != nil {
		h.logger.Error("get project skill failed", "error", err)
		jsonError(w, http.StatusInternalServerError, "get project skill failed")
		return
	}
	jsonResponse(w, http.StatusOK, runtimeSkill(skill, true))
}

// ListAttachments is the admin management view. Unlike the runtime endpoint,
// it intentionally includes every explicit attachment regardless of lifecycle
// or soft-deletion so an admin can repair an attachment set.
func (h *SkillHandler) ListAttachments(w http.ResponseWriter, r *http.Request) {
	orgID, projectID := middleware.GetOrgID(r.Context()), r.PathValue("id")
	if orgID == "" {
		jsonError(w, http.StatusUnauthorized, "missing org context")
		return
	}
	if !h.projectReadAccess(w, r, projectID) {
		return
	}
	attachments, err := h.store.ListProjectSkillAttachments(r.Context(), orgID, projectID)
	if err != nil {
		h.logger.Error("list project skill attachments failed", "error", err)
		jsonError(w, http.StatusInternalServerError, "list project skill attachments failed")
		return
	}
	if attachments == nil {
		attachments = []*model.SkillDescriptor{}
	}
	jsonResponse(w, http.StatusOK, attachments)
}

func (h *SkillHandler) ReplaceProject(w http.ResponseWriter, r *http.Request) {
	orgID, projectID := middleware.GetOrgID(r.Context()), r.PathValue("id")
	if orgID == "" {
		jsonError(w, http.StatusUnauthorized, "missing org context")
		return
	}
	if !uuidRe.MatchString(projectID) {
		jsonError(w, http.StatusBadRequest, "id must be a UUID")
		return
	}
	if !h.projectReadAccess(w, r, projectID) {
		return
	}
	var req replaceProjectSkillsRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxSkillAttachmentRequestBytes)).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if len(req.SkillIDs) > 100 {
		jsonError(w, http.StatusBadRequest, "at most 100 skills may be attached")
		return
	}
	oldIDs, err := h.store.ReplaceProjectSkills(r.Context(), orgID, projectID, req.SkillIDs)
	if errors.Is(err, store.ErrNotFound) {
		jsonError(w, http.StatusNotFound, "project or skill not found")
		return
	} else if err != nil {
		h.logger.Error("replace project skills failed", "error", err)
		jsonError(w, http.StatusInternalServerError, "replace project skills failed")
		return
	}
	newIDs := sortedUniqueIDs(req.SkillIDs)
	attachedIDs, detachedIDs := attachmentDiff(oldIDs, newIDs)
	if err := h.store.AppendAudit(r.Context(), &model.AuditEntry{
		OrgID:      orgID,
		ProjectID:  projectID,
		ActorID:    middleware.GetAccountID(r.Context()),
		Action:     "skill.attachments.replace",
		TargetType: "project",
		TargetID:   projectID,
		Metadata: map[string]any{
			"attached_ids": attachedIDs,
			"detached_ids": detachedIDs,
			"total_count":  len(newIDs),
		},
	}); err != nil {
		h.logger.Warn("skill attachment audit append failed", "project_id", projectID, "error", err)
	}
	w.WriteHeader(http.StatusNoContent)
}

func sortedUniqueIDs(ids []string) []string {
	set := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		set[id] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func attachmentDiff(oldIDs, newIDs []string) (attached, detached []string) {
	oldSet := make(map[string]struct{}, len(oldIDs))
	for _, id := range oldIDs {
		oldSet[id] = struct{}{}
	}
	newSet := make(map[string]struct{}, len(newIDs))
	for _, id := range newIDs {
		newSet[id] = struct{}{}
	}
	for id := range newSet {
		if _, ok := oldSet[id]; !ok {
			attached = append(attached, id)
		}
	}
	for id := range oldSet {
		if _, ok := newSet[id]; !ok {
			detached = append(detached, id)
		}
	}
	sort.Strings(attached)
	sort.Strings(detached)
	return attached, detached
}

func runtimeSkill(skill *model.Skill, includeContent bool) runtimeSkillResponse {
	v := skill.CurrentVersion
	resp := runtimeSkillResponse{ID: skill.ID, Name: skill.Name, Slug: skill.Slug, Purpose: skill.Purpose, Status: skill.Status, ProjectCount: skill.ProjectCount, CreatedAt: skill.CreatedAt, UpdatedAt: skill.UpdatedAt}
	if v != nil {
		resp.Version, resp.ContentHash = v.Version, v.ContentHash
		if includeContent {
			resp.Body = v.Body
			if v.Frontmatter != nil {
				frontmatter := v.Frontmatter
				resp.Frontmatter = &frontmatter
			}
			resp.Content = v.Content
		}
	}
	return resp
}

func (h *SkillHandler) projectReadAccess(w http.ResponseWriter, r *http.Request, projectID string) bool {
	if !uuidRe.MatchString(projectID) {
		jsonError(w, http.StatusBadRequest, "id must be a UUID")
		return false
	}
	_, ok := requireProjectAccess(w, r, h.store, h.logger, projectID)
	return ok
}

func (h *SkillHandler) audit(r *http.Request, action, targetID string, metadata any) {
	if err := h.store.AppendAudit(r.Context(), &model.AuditEntry{OrgID: middleware.GetOrgID(r.Context()), ActorID: middleware.GetAccountID(r.Context()), Action: action, TargetType: "skill", TargetID: targetID, Metadata: metadata}); err != nil {
		h.logger.Warn("skill audit append failed", "action", action, "error", err)
	}
}
